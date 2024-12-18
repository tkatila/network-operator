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
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vishvananda/netlink"

	"github.com/intel/intel-network-operator-for-kubernetes/pkg/lldp"
)

const (
	driverPath       = "bus/pci/drivers/habanalabs/"
	pciDevicePattern = "????:??:??.?"
	netDevicePattern = "net/*"

	noAddress = "none"
)

type discoveryResult struct {
	ifname string
	err    error
	lldp   lldp.DiscoveryResult
}

type networkConfiguration struct {
	link            netlink.Link
	origState       netlink.LinkOperState
	portDescription string
	lldpPeer        *net.IP
	localAddr       *net.IP
	peerHWAddr      *net.HardwareAddr
}

var sysfsRoot string = ""

func getSysfsRoot() string {
	if sysfsRoot != "" {
		return sysfsRoot
	}
	sysfsRoot = os.Getenv("SYSFS_ROOT")
	if sysfsRoot == "" {
		sysfsRoot = "/sys/"
	}
	return sysfsRoot
}

func sysfsDriverPath() string {
	return filepath.Join(getSysfsRoot(), driverPath)
}

func getNetworks() []string {
	habanaNetDevices := []string{}

	pattern := filepath.Join(sysfsDriverPath(), pciDevicePattern)
	paths, err := filepath.Glob(pattern)

	if err != nil {
		fmt.Printf("no PCI devices found\n")
		return habanaNetDevices
	}

	for _, p := range paths {
		devicesymlinktarget, err := filepath.EvalSymlinks(p)
		if err != nil {
			fmt.Printf("Expected '%s' to be a symlink: %v\n", p, err)
			continue
		}

		netdevicepattern := filepath.Join(devicesymlinktarget, netDevicePattern)
		netdevices, err := filepath.Glob(netdevicepattern)
		if err != nil {
			fmt.Printf("Could not find network device files: %v", err)
		}
		for _, n := range netdevices {
			name := filepath.Base(n)
			habanaNetDevices = append(habanaNetDevices, name)
		}

	}

	return habanaNetDevices
}

func getNetworkConfigs(ifacenames []string) map[string]*networkConfiguration {
	links := make(map[string]*networkConfiguration)

	for _, name := range ifacenames {
		link, err := netlink.LinkByName(name)
		if err != nil {
			fmt.Printf("Link '%s' not found: %v\n", name, err)
			continue
		}

		links[name] = &networkConfiguration{
			link: link,
		}
	}

	return links
}

func startLLDP(ctx context.Context, result discoveryResult, lldpResultChan chan discoveryResult) {
	lldpChan := make(chan lldp.DiscoveryResult)

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})
	log := slog.New(jsonHandler)

	iface, err := net.InterfaceByName(result.ifname)
	if err != nil {
		fmt.Printf("failed to find interface '%s': %v\n", result.ifname, err)
		result.err = err
		lldpResultChan <- result
		return
	}

	lldpClient := lldp.NewClient(ctx, *iface)
	go func() {
		if err := lldpClient.Start(log, lldpChan); err != nil {
			lldpChan <- lldp.DiscoveryResult{}
		}
	}()

	result.lldp = <-lldpChan
	lldpResultChan <- result

}

func waitResults(ctx context.Context, timeout time.Duration, networkConfigs map[string]*networkConfiguration,
	lldpResultChan chan discoveryResult) {

	timeoutctx, cancelctx := context.WithTimeout(ctx, timeout)
	defer cancelctx()

	for i := 0; i < len(networkConfigs); i++ {
		select {
		case <-timeoutctx.Done():
			fmt.Printf("Timed out\n")
			return

		case result := <-lldpResultChan:
			if result.err != nil {
				fmt.Printf("%s replied with error: %v\n", result.ifname, result.err)
				continue
			} else {
				if nwconfig, exists := networkConfigs[result.ifname]; exists {
					nwconfig.portDescription = result.lldp.PortDescription

					var hwaddr net.HardwareAddr = result.lldp.PeerMAC
					nwconfig.peerHWAddr = &hwaddr
				}
			}
		}
	}
}

func selectMask30L3Address(nwconfig *networkConfiguration) (*net.IP, *net.IP, error) {
	var (
		peerNetwork *net.IPNet
		peeraddr    net.IP
		localaddr   net.IP
		err         error
	)

	substrings := strings.Split(nwconfig.portDescription, " ")
	if len(substrings) < 2 {
		return nil, nil, fmt.Errorf("interface '%s' could not split string '%s'",
			nwconfig.link.Attrs().Name, nwconfig.portDescription)
	}

	peeraddr, peerNetwork, err = net.ParseCIDR(substrings[1])
	if err != nil {
		return nil, nil, fmt.Errorf("interface '%s' could not parse '%s': %v",
			nwconfig.link.Attrs().Name, nwconfig.portDescription, err)
	}

	mask, _ := peerNetwork.Mask.Size()
	if mask == 30 {
		// toggle the lowest two bits of the switch IPv4 address to get
		// the local address
		peer := peeraddr.To4()
		localaddr = net.IPv4(peer[0], peer[1], peer[2], peer[3]^0x3)
	} else {
		err = fmt.Errorf("Mask is %d, not the expected 30", mask)
	}

	return &peeraddr, &localaddr, err
}

func printResult(nwconfig *networkConfiguration) {
	fmt.Printf("Interface '%s' %s:\n", nwconfig.link.Attrs().Name, nwconfig.link.Attrs().OperState.String())

	fmt.Printf("\tConfigured addresses: ")
	addrs, err := netlink.AddrList(nwconfig.link, netlink.FAMILY_ALL)
	if len(addrs) == 0 || err != nil {
		fmt.Printf("no addresses")
	} else {
		for _, addr := range addrs {
			fmt.Printf("%s", addr.IPNet.String())
			if nwconfig.localAddr != nil && addr.IPNet.IP.Equal(*nwconfig.localAddr) {
				fmt.Printf("(matches lldp)")
			}
			fmt.Printf(" ")
		}
	}
	fmt.Printf("\n")

	addr := noAddress
	if nwconfig.peerHWAddr != nil {
		addr = nwconfig.peerHWAddr.String()
	}
	fmt.Printf("\tPeer MAC address: %s\n", addr)

	addr = noAddress
	if nwconfig.lldpPeer != nil {
		addr = nwconfig.lldpPeer.String()
	}
	fmt.Printf("\tLLDP address: %s\n", addr)
	addr = noAddress
	if nwconfig.localAddr != nil {
		addr = nwconfig.localAddr.String()
	}
	fmt.Printf("\tLLDP /30 address to add: %s\n", addr)
}

func examineResults(networkConfigs map[string]*networkConfiguration) bool {
	foundpeers := false

	for _, nwconfig := range networkConfigs {
		lldpPeer, localAddr, err := selectMask30L3Address(nwconfig)
		if err == nil {
			nwconfig.lldpPeer = lldpPeer
			nwconfig.localAddr = localAddr
			foundpeers = true
		}

		printResult(nwconfig)
	}

	return foundpeers
}

func configureInterfaces(networkConfigs map[string]*networkConfiguration) (int, int) {
	configured := 0

	fmt.Printf("Configuring interfaces...\n")

	for _, nwconfig := range networkConfigs {
		if nwconfig.localAddr == nil {
			continue
		}

		addrs, err := netlink.AddrList(nwconfig.link, netlink.FAMILY_V4)
		ifname := nwconfig.link.Attrs().Name
		if err != nil {
			fmt.Printf("Could not get addresses for link '%s': %v\n", ifname, err)
			continue
		}

		for _, addr := range addrs {
			if nwconfig.localAddr.Equal(addr.IPNet.IP) {
				fmt.Printf("Interface '%s' already configured '%s'\n", ifname, addr.IPNet.String())
				continue
			}
			newlinkaddr := &netlink.Addr{
				IPNet: &net.IPNet{
					IP:   *nwconfig.localAddr,
					Mask: net.CIDRMask(30, 32),
				},
			}
			if err := netlink.AddrAdd(nwconfig.link, newlinkaddr); err != nil {
				fmt.Printf("Could not configure address '%s' to interface '%s': %v\n", nwconfig.localAddr.String(), ifname, err)
				continue
			}
			configured++

			fmt.Printf("Configured address '%s' to interface '%s'\n", nwconfig.localAddr.String(), ifname)
		}
	}

	return configured, len(networkConfigs)
}

func cmdRun(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	lldpResultChan := make(chan discoveryResult)

	wait, err := cmd.Flags().GetString("wait")
	if err != nil {
		return fmt.Errorf("Cannot parse time expression: %v", err)
	}
	timeout, err := time.ParseDuration(wait)
	if err != nil {
		var secs_err error

		// Let's give it a shot with seconds added, if not return
		// previous duration parsing error
		timeout, secs_err = time.ParseDuration(wait + "s")
		if secs_err != nil {
			return fmt.Errorf("Cannot parse duration: %v", err)
		}
	}

	configure, _ := cmd.Flags().GetBool("configure")

	gaudinetfile, err := cmd.Flags().GetString("gaudinet")
	if err != nil {
		return fmt.Errorf("Cannot parse gaudinet argument")
	}

	ifacenames := getNetworks()
	extrainterfaces, err := cmd.Flags().GetString("interfaces")
	if err == nil && len(extrainterfaces) > 0 {
		ifacenames = append(ifacenames, strings.Split(extrainterfaces, ",")...)
	}

	if len(ifacenames) == 0 {
		fmt.Printf("No devices found\n")
		return nil
	}

	networkConfigs := getNetworkConfigs(ifacenames)

	for _, networkconfig := range networkConfigs {
		networkconfig.origState = networkconfig.link.Attrs().OperState

		if networkconfig.link.Attrs().OperState != netlink.OperUp {
			if err := netlink.LinkSetUp(networkconfig.link); err != nil {
				fmt.Printf("Cannot set link '%s' up: %v\n", networkconfig.link.Attrs().Name, err)
				continue
			}
		}

		go func() {
			result := discoveryResult{
				ifname: networkconfig.link.Attrs().Name,
			}
			startLLDP(ctx, result, lldpResultChan)
		}()

		fmt.Printf("Started LLDP discovery for '%s'...\n", networkconfig.link.Attrs().Name)
	}

	waitResults(ctx, timeout, networkConfigs, lldpResultChan)

	foundpeers := examineResults(networkConfigs)

	if gaudinetfile != "" {
		err := WriteGaudiNet(gaudinetfile, networkConfigs)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	if configure && foundpeers {
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
