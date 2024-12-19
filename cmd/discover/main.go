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
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/vishvananda/netlink"

	"github.com/intel/intel-network-operator-for-kubernetes/pkg/lldp"
)

type cmdConfig struct {
	ctx          context.Context
	timeout      time.Duration
	configure    bool
	gaudinetfile string
	ifaces       []string
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

	return config, nil
}

func detectLLDP(config *cmdConfig, networkConfigs map[string]*networkConfiguration) {
	var wg sync.WaitGroup
	lldpResultChan := make(chan lldp.DiscoveryResult, len(networkConfigs))
	timeoutctx, cancelctx := context.WithTimeout(config.ctx, config.timeout)

	defer cancelctx()

	for _, networkconfig := range networkConfigs {
		networkconfig.origState = networkconfig.link.Attrs().OperState

		if networkconfig.link.Attrs().OperState != netlink.OperUp {
			if err := netlink.LinkSetUp(networkconfig.link); err != nil {
				fmt.Printf("Cannot set link '%s' up: %v\n", networkconfig.link.Attrs().Name, err)
				continue
			}
		}

		wg.Add(1)
		go func() {
			lldpClient := lldp.NewClient(timeoutctx, networkconfig.link.Attrs().Name)
			if err := lldpClient.Start(lldpResultChan); err != nil {
				fmt.Printf("Cannot start LLDP client: %v\n", err)
			}
			wg.Done()
		}()

		fmt.Printf("Started LLDP discovery for '%s'...\n", networkconfig.link.Attrs().Name)
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

	networkConfigs := getNetworkConfigs(config.ifaces)

	detectLLDP(config, networkConfigs)

	foundpeers := examineResults(networkConfigs)

	if config.gaudinetfile != "" {
		err := WriteGaudiNet(config.gaudinetfile, networkConfigs)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	if config.configure && foundpeers {
		num, total := configureInterfaces(networkConfigs)
		fmt.Printf("Configured %d of %d interfaces\n", num, total)
	} else {
		for _, networkconfig := range networkConfigs {
			if networkconfig.origState != netlink.OperUp {

				if err := netlink.LinkSetDown(networkconfig.link); err == nil {
					fmt.Printf("Setting link '%s' back down\n", networkconfig.link.Attrs().Name)
				} else {
					fmt.Printf("Cannot set link '%s' back down: %v\n", networkconfig.link.Attrs().Name, err)
				}
			}
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

	cmd.Flags().BoolP("configure", "", false, "Configure network discovered with LLDP")
	cmd.Flags().StringP("interfaces", "", "", "Comma separated list of additional network interfaces")
	cmd.Flags().StringP("wait", "", "30s", "Time to wait for LLDP packets")
	cmd.Flags().StringP("gaudinet", "", "", "gaudinet file path")

	return cmd, nil
}

func main() {
	cmd, err := setupCmd()

	if err != nil {
		fmt.Printf("Could not start: %v\n", err)
		return
	}

	_ = cmd.Execute()
}
