/*
 * Copyright (C) 2025 Intel Corporation
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
	"path"
	"testing"

	"github.com/vishvananda/netlink"
)

const (
	sysfsDevicePath = "bus/pci/devices"
	netDevicePath   = "net"
)

func TestSelectMask30L3Address(t *testing.T) {
	expectedpeer := net.IPv4(10, 210, 8, 122)
	expectedaddr := net.IPv4(10, 210, 8, 121)

	nwconfig := networkConfiguration{
		portDescription: "no-alert " + expectedpeer.String() + "/30",
	}

	peeraddr, localaddr, err := selectMask30L3Address(&nwconfig)
	if !peeraddr.Equal(expectedpeer) {
		t.Errorf("Peer addresses do not match, expected %s got %s: %v", expectedpeer.String(), peeraddr.String(), err)
	}
	if !localaddr.Equal(expectedaddr) {
		t.Errorf("Local addresses do not match, expected %s got %s: %v", expectedaddr.String(), localaddr.String(), err)
	}

	addrmask := "/16"
	addrtext := "10.210.8.122"
	nwconfig = networkConfiguration{
		portDescription: "no-alert " + addrtext + addrmask,
	}
	peeraddr, localaddr, err = selectMask30L3Address(&nwconfig)
	if err == nil || peeraddr.String() != addrtext || localaddr.String() != "<nil>" {
		t.Errorf("netmask %s unexpectedly returned values '%s', '%s' or no error '%v'",
			addrmask, peeraddr.String(), localaddr.String(), err)
	}
}

func TestSysFsRoot(t *testing.T) {
	testSysfsRoot, err := os.MkdirTemp("", "networkoperator.")
	if err != nil {
		t.Errorf("cannot create tmp dir: %v", err)
	}
	defer os.RemoveAll(testSysfsRoot)

	os.Setenv("SYSFS_ROOT", testSysfsRoot)

	detectedSysfsRoot := getSysfsRoot()
	if detectedSysfsRoot != testSysfsRoot {
		t.Errorf("Sysfs root directory is '%s', expected '%s'", detectedSysfsRoot, testSysfsRoot)
	}

	expectedpath := path.Join(testSysfsRoot, "bus/pci/drivers/habanalabs")
	if detectedsysfsdriverpath := sysfsDriverPath(); detectedsysfsdriverpath != expectedpath {
		t.Errorf("got sysfs driver path '%s', expected '%s'", detectedsysfsdriverpath, expectedpath)
	}

}

func writeFakeSysfsEntries(testSysfsRoot string, devices map[string]fakeNetworkTestData, t *testing.T) {
	driverdir := path.Join(testSysfsRoot, driverPath)
	if err := os.MkdirAll(driverdir, 0755); err != nil {
		t.Errorf("cannot create fake driver dir '%s': %v", driverdir, err)
	}

	pcidevicedir := path.Join(testSysfsRoot, sysfsDevicePath)

	for netdev, fakenwconfig := range devices {
		pcidev := fakenwconfig.pcidevice
		netdevice := path.Join(pcidevicedir, pcidev, netDevicePath, netdev)
		if err := os.MkdirAll(netdevice, 0755); err != nil {
			t.Errorf("cannot create fake PCI device dir '%s': %v", netdevice, err)
		}

		// ...bus/pci/drivers/habanalabs/xxxx:xx:xx.x -> ...bus/pci/devices/xxxx:xx:xx.x
		driverdirsymlink := path.Join(driverdir, pcidev)
		pcidirdevice := path.Join(pcidevicedir, pcidev)
		if err := os.Symlink(pcidirdevice, driverdirsymlink); err != nil {
			t.Errorf("cannot create symlink '%s' to '%s': %v", driverdirsymlink, pcidirdevice, err)
		}
	}
}

type fakeLink struct {
	fakeAttrs netlink.LinkAttrs
}

func (l *fakeLink) Attrs() *netlink.LinkAttrs {
	return &l.fakeAttrs
}

func (l *fakeLink) Type() string {
	return ""
}

type fakeNetworkTestData struct {
	pcidevice     string
	linkaddrs     []net.IPNet
	nwconfig      networkConfiguration
	configureaddr bool
}

func getFakeNetworkData() map[string]fakeNetworkTestData {
	return map[string]fakeNetworkTestData{
		// Address and proper LLDP Port Description field
		"eth_a": {
			pcidevice: "0000:aa:00.0",
			linkaddrs: []net.IPNet{
				{
					IP:   net.IPv4(192, 192, 192, 1),
					Mask: net.IPv4Mask(255, 255, 255, 0),
				},
			},
			nwconfig: networkConfiguration{
				link: &fakeLink{
					fakeAttrs: netlink.LinkAttrs{
						Name: "eth_a",
					},
				},
				portDescription: "no-alert 10.210.8.122/30",
				peerHWAddr:      &net.HardwareAddr{0x01, 0x01, 0x02, 0x02, 0x03, 0x03},
			},
			configureaddr: true,
		},
		// No address, Port Description field with other string
		"eth_b": {
			pcidevice: "0000:bb:00.0",
			linkaddrs: []net.IPNet{},
			nwconfig: networkConfiguration{
				link: &fakeLink{
					fakeAttrs: netlink.LinkAttrs{
						Name: "eth_b",
					},
				},
				portDescription: "unexpected port description",
				peerHWAddr:      &net.HardwareAddr{0x02, 0x02, 0x03, 0x03, 0x4, 0x4},
			},
			configureaddr: false,
		},
		// Already configured with LLDP address 10.210.8.125/30
		"eth_c": {
			pcidevice: "0000:cc:00.0",
			linkaddrs: []net.IPNet{
				{
					IP:   net.IPv4(10, 210, 8, 125),
					Mask: net.IPv4Mask(255, 255, 255, 252),
				},
			},
			nwconfig: networkConfiguration{
				link: &fakeLink{
					fakeAttrs: netlink.LinkAttrs{
						Name: "eth_c",
					},
				},
				portDescription: "no-alert 10.210.8.126/30",
				peerHWAddr:      &net.HardwareAddr{0x03, 0x03, 0x4, 0x4, 0x5, 0x5},
			},
			configureaddr: false,
		},
	}
}

func getFakeNetworkDataConfigs() map[string]*networkConfiguration {
	nwconfigs := make(map[string]*networkConfiguration)
	for idx, fakenwconfigdata := range getFakeNetworkData() {
		nwconfigs[idx] = &fakenwconfigdata.nwconfig
	}
	return nwconfigs
}

func fakeLinkByName(name string) (netlink.Link, error) {
	fakenwconfigs := getFakeNetworkDataConfigs()
	if l := fakenwconfigs[name]; l != nil {
		return &fakeLink{
			fakeAttrs: netlink.LinkAttrs{
				Name:         l.link.Attrs().Name,
				HardwareAddr: l.link.Attrs().HardwareAddr,
			},
		}, nil
	}

	return nil, fmt.Errorf("no fake link for '%s' defined in test cases", name)
}

func TestFakeSysfs(t *testing.T) {
	testSysfsRoot, err := os.MkdirTemp("", "networkoperator.")
	if err != nil {
		t.Errorf("cannot create tmp dir: %v", err)
	}
	defer os.RemoveAll(testSysfsRoot)

	os.Setenv("SYSFS_ROOT", testSysfsRoot)

	// no devices in the fake sysfs directory
	for _, d := range getNetworks() {
		t.Errorf("no devices should have been found: %s", d)
	}

	devices := getFakeNetworkData()
	writeFakeSysfsEntries(testSysfsRoot, devices, t)

	for _, d := range getNetworks() {
		if _, exists := devices[d]; !exists {
			t.Errorf("found unexpected device '%s'", d)
		}
		delete(devices, d)
	}
	if len(devices) > 0 {
		t.Errorf("not all devices were detected: %v", devices)
	}
}

func TestLldpResults(t *testing.T) {
	nwconfigs := getFakeNetworkDataConfigs()
	foundpeers := lldpResults(nwconfigs)

	if !foundpeers {
		t.Errorf("expected to find at least one peer addresses, none found")
	}

	delete(nwconfigs, "eth_c")
	foundpeers = lldpResults(nwconfigs)
	if !foundpeers {
		t.Errorf("expected to find at least one peer addresses, none found")
	}

	delete(nwconfigs, "eth_a")
	foundpeers = lldpResults(nwconfigs)
	if foundpeers {
		t.Errorf("expected not to find any peer addresses, at least none found")
	}
}

func TestGetNetworkConfigs(t *testing.T) {
	networkLink.LinkByName = fakeLinkByName
	networks := []string{"eth_a", "eth_b", "eth_c"}
	networkconfigs := getNetworkConfigs(networks)

	if len(networkconfigs) != len(networks) {
		t.Errorf("number of networkconfig and networks don't match")
	}
	for _, iface := range networks {
		if _, exists := networkconfigs[iface]; !exists {
			t.Errorf("name '%s' was not found in networkconfigs", iface)
		}
		delete(networkconfigs, iface)
	}
	if len(networkconfigs) > 0 {
		t.Errorf("not all networkconfigs were created")
	}

	networks = []string{"eth_c", "eth_b", "foo"}
	networkconfigs = getNetworkConfigs(networks)

	if len(networkconfigs) != 2 {
		t.Errorf("wrong number (%d) of networkconfigs detected", len(networkconfigs))
	}
	if _, exists := networkconfigs["foo"]; exists {
		t.Errorf("name 'foo' exists when it should not")
	}
	for _, iface := range networks {
		delete(networkconfigs, iface)
	}
	if len(networkconfigs) > 0 {
		t.Errorf("networkconfig has left over items")
	}
}

func fakeLinkAddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	netlinkaddrs := []netlink.Addr{}
	name := link.Attrs().Name
	fakenwdata, exists := getFakeNetworkData()[name]
	if !exists {
		return nil, fmt.Errorf("fake link '%s' does not exist in test data for fakeLinkAddrList", name)
	}

	for _, linkaddr := range fakenwdata.linkaddrs {
		netlinkaddrs = append(netlinkaddrs, netlink.Addr{
			IPNet: &net.IPNet{
				IP:   linkaddr.IP,
				Mask: linkaddr.Mask,
			},
			Peer:      &net.IPNet{},
			Broadcast: net.IP{},
		})
	}

	return netlinkaddrs, nil
}

var fakelinkAddAddrs map[string]*netlink.Addr

func fakeLinkAddrAdd(link netlink.Link, addr *netlink.Addr) error {
	name := link.Attrs().Name

	if addr == nil {
		return fmt.Errorf("no address for fakeLinkAddrAdd interface '%s'", name)
	}

	if _, exists := getFakeNetworkData()[name]; !exists {
		return fmt.Errorf("fake link '%s' does not exist in test data for fakeLinkAddrAdd", name)
	}

	fakelinkAddAddrs[name] = addr

	return nil
}

func TestConfigureInterfaces(t *testing.T) {
	networkLink.AddrList = fakeLinkAddrList
	networkLink.AddrAdd = fakeLinkAddrAdd

	fakenwconfigs := getFakeNetworkDataConfigs()

	fakelinkAddAddrs = make(map[string]*netlink.Addr)
	_ = lldpResults(fakenwconfigs)

	configured, total := configureInterfaces(fakenwconfigs)

	if len(fakelinkAddAddrs) == 0 {
		t.Errorf("no addresses added")
	}

	configureaddr := 0
	for ifname, nwdata := range getFakeNetworkData() {
		var str string

		if nwdata.configureaddr {
			configureaddr++
		}
		resultaddr, exists := fakelinkAddAddrs[ifname]
		if exists {
			str = "configured"
			delete(fakelinkAddAddrs, ifname)
		} else {
			str = "not configured"
		}

		if nwdata.configureaddr != exists {
			t.Errorf("Interface '%s' %s", ifname, str)
		}

		if !exists {
			continue
		}

		localAddr := fakenwconfigs[ifname].localAddr
		if localAddr == nil || !resultaddr.IPNet.IP.Equal(*localAddr) {
			t.Errorf("Addresses for interface '%s' do not match: %v", ifname, nwdata.nwconfig.localAddr)
		}
	}

	if configured != configureaddr || total != len(fakenwconfigs) {
		t.Errorf("configured %d/%d %v", configured, total, fakelinkAddAddrs)
	}
}
