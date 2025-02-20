/*
 * Copyright (C) 2024 Intel Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	goflag "flag"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/intel/intel-network-operator-for-kubernetes/pkg/lldp"
)

type layerMode int

const (
	L2 layerMode = iota
	L3

	nfdFeatureDir         = "/etc/kubernetes/node-feature-discovery/features.d/"
	nfdLabelFile          = nfdFeatureDir + "scale-out-readiness.txt"
	nfdScaleOutReadyLabel = "intel.feature.node.kubernetes.io/gaudi-scale-out=true"
)

type cmdConfig struct {
	ctx          context.Context
	timeout      time.Duration
	configure    bool
	gaudinetfile string
	ifaces       []string
	mode         layerMode
	keepRunning  bool
}

func getConfig(cmd *cobra.Command) (*cmdConfig, error) {
	config := &cmdConfig{ctx: context.Background()}

	wait, err := cmd.Flags().GetString("wait")
	if err != nil {
		return nil, fmt.Errorf("Cannot parse time expression: %v", err)
	}
	config.timeout, err = time.ParseDuration(wait)
	if err != nil {
		var secs_err error

		// Let's give it a shot with seconds added, if not return
		// previous duration parsing error
		config.timeout, secs_err = time.ParseDuration(wait + "s")
		if secs_err != nil {
			return nil, fmt.Errorf("Cannot parse duration: %v", err)
		}
	}

	config.configure, _ = cmd.Flags().GetBool("configure")

	config.gaudinetfile, err = cmd.Flags().GetString("gaudinet")
	if err != nil {
		return nil, fmt.Errorf("Cannot parse gaudinet argument")
	}

	config.ifaces = getNetworks()
	extrainterfaces, err := cmd.Flags().GetString("interfaces")
	if err == nil && len(extrainterfaces) > 0 {
		config.ifaces = append(config.ifaces, strings.Split(extrainterfaces, ",")...)
	}

	if len(config.ifaces) == 0 {
		return nil, fmt.Errorf("No devices found")
	}

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return nil, fmt.Errorf("Cannot parse mode expression: %v", err)
	}
	switch strings.ToLower(mode) {
	case "l3":
		fallthrough
	case "l3switch":
		config.mode = L3

	case "l2":
		config.mode = L2

	default:
		return nil, fmt.Errorf("Cannot parse mode '%s'", mode)
	}

	keepRunning, err := cmd.Flags().GetBool("keep-running")
	if err != nil {
		return nil, fmt.Errorf("Cannot parse keep-running expression: %v", err)
	}
	config.keepRunning = keepRunning

	return config, nil
}

func detectLLDP(config *cmdConfig, networkConfigs map[string]*networkConfiguration) {
	var wg sync.WaitGroup
	lldpResultChan := make(chan lldp.DiscoveryResult, len(networkConfigs))
	timeoutctx, cancelctx := context.WithTimeout(config.ctx, config.timeout)

	defer cancelctx()

	for _, networkconfig := range networkConfigs {
		if networkconfig.link.Attrs().Flags&net.FlagUp == 0 {
			klog.Infof("Link '%s' %s, cannot start LLDP\n",
				networkconfig.link.Attrs().Name, networkconfig.link.Attrs().OperState.String())
			continue
		}

		wg.Add(1)
		go func() {
			lldpClient := lldp.NewClient(timeoutctx, networkconfig.link.Attrs().Name, *networkconfig.localHwAddr)
			if err := lldpClient.Start(lldpResultChan); err != nil {
				klog.Infof("Cannot start LLDP client: %v\n", err)
			}
			wg.Done()
		}()

		klog.Infof("Started LLDP discovery for '%s'...\n", networkconfig.link.Attrs().Name)
	}

	wg.Wait()

	for len(lldpResultChan) > 0 {
		result := <-lldpResultChan

		if nwconfig, exists := networkConfigs[result.InterfaceName]; exists {
			nwconfig.portDescription = result.PortDescription

			var hwaddr net.HardwareAddr = result.PeerMAC
			nwconfig.peerHWAddr = &hwaddr
		}
	}
}

func cmdRun(cmd *cobra.Command, args []string) error {
	config, err := getConfig(cmd)
	if err != nil {
		return err
	}

	cmd.SilenceUsage = true

	networkConfigs := getNetworkConfigs(config.ifaces)

	if err := interfacesUp(networkConfigs); err != nil {
		return err
	}

	numConfigured := 0
	numTotal := len(networkConfigs)

	if config.mode == L3 {
		detectLLDP(config, networkConfigs)

		foundpeers := lldpResults(networkConfigs)

		if config.gaudinetfile != "" {
			err := WriteGaudiNet(config.gaudinetfile, networkConfigs)
			if err != nil {
				klog.Errorf("Error: %v\n", err)
			}
		}

		if config.configure && foundpeers {
			numConfigured, numTotal = configureInterfaces(networkConfigs)
			klog.Infof("Configured %d of %d interfaces\n", numConfigured, numTotal)
		}
	}

	logResults(config, networkConfigs)

	if !config.configure {
		if err := interfacesRestoreDown(networkConfigs); err != nil {
			return err
		}
	} else if config.configure && config.mode == L3 {
		if numConfigured < numTotal {
			return fmt.Errorf("Not all interfaces were configured (%d/%d).", numConfigured, numTotal)
		}
	}

	if config.configure && config.keepRunning {
		if s, err := os.Stat(nfdFeatureDir); err == nil && s.IsDir() {
			content := nfdScaleOutReadyLabel + "\n"

			if err := os.WriteFile(nfdLabelFile, []byte(content), 0644); err != nil {
				return fmt.Errorf("Failed to write NFD label to indicate scale-out readiness: %+v\n", err)
			}
		}

		klog.Infof("Configurations done. Idling...")

		for {
			time.Sleep(time.Second)
		}
	}

	return nil
}

// error is always nil, but keep the logic incase we want to return it later on.
// nolint: unparam
func setupCmd() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover and optionally configure network devices",
		RunE:  cmdRun,
	}
	fs := goflag.FlagSet{}
	klog.InitFlags(&fs)

	cmd.Flags().AddGoFlagSet(&fs)
	cmd.Flags().SortFlags = false

	cmd.Flags().StringP("mode", "", "L3", "'L2' for network layer 2 or 'L3' for network layer 3 (L3) using LLDP")
	cmd.Flags().BoolP("configure", "", false, "Configure L3 network with LLDP or set interfaces up with L2 networks")
	cmd.Flags().StringP("interfaces", "", "", "Comma separated list of additional network interfaces")
	cmd.Flags().StringP("wait", "", "30s", "Time to wait for LLDP packets")
	cmd.Flags().StringP("gaudinet", "", "", "gaudinet file path")
	cmd.Flags().BoolP("keep-running", "", false, "Keep running after any configurations are done")

	return cmd, nil
}

func main() {
	defer klog.Flush()
	cmd, err := setupCmd()

	if err != nil {
		klog.Errorf("Could not start: %v\n", err)
		return
	}

	_ = cmd.Execute()
}
