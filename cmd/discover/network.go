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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/vishvananda/netlink"
)

const (
	driverPath       = "bus/pci/drivers/habanalabs/"
	pciDevicePattern = "????:??:??.?"
	netDevicePattern = "net/*"

	noAddress = "none"
)

type networkLinkFn struct {
	LinkByName func(name string) (netlink.Link, error)
	AddrList   func(link netlink.Link, family int) ([]netlink.Addr, error)
	AddrAdd    func(link netlink.Link, addr *netlink.Addr) error
}

var networkLink = networkLinkFn{
	LinkByName: netlink.LinkByName,
	AddrList:   netlink.AddrList,
	AddrAdd:    netlink.AddrAdd,
}

type networkConfiguration struct {
	link            netlink.Link
	origState       netlink.LinkOperState
	portDescription string
	lldpPeer        *net.IP
	localAddr       *net.IP
	peerHWAddr      *net.HardwareAddr
}

func getSysfsRoot() string {
	sysfsRoot := os.Getenv("SYSFS_ROOT")
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
		link, err := networkLink.LinkByName(name)
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
	addrs, err := networkLink.AddrList(nwconfig.link, netlink.FAMILY_ALL)
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

		addrs, err := networkLink.AddrList(nwconfig.link, netlink.FAMILY_V4)
		ifname := nwconfig.link.Attrs().Name
		if err != nil {
			fmt.Printf("Could not get addresses for link '%s': %v\n", ifname, err)
			continue
		}

		for _, addr := range addrs {
			if nwconfig.localAddr.Equal(addr.IPNet.IP) {
				fmt.Printf("Interface '%s' already configured '%s'\n",
					ifname, addr.IPNet.String())
				continue
			}
			newlinkaddr := &netlink.Addr{
				IPNet: &net.IPNet{
					IP:   *nwconfig.localAddr,
					Mask: net.CIDRMask(30, 32),
				},
			}
			if err := networkLink.AddrAdd(nwconfig.link, newlinkaddr); err != nil {
				fmt.Printf("Could not configure address '%s' to interface '%s': %v\n",
					nwconfig.localAddr.String(), ifname, err)
				continue
			}
			configured++

			fmt.Printf("Configured address '%s' to interface '%s'\n",
				nwconfig.localAddr.String(), ifname)
		}
	}

	return configured, len(networkConfigs)
}
