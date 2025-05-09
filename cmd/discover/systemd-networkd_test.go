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
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestNetworkdFilename(t *testing.T) {
	expectedfile := SystemdNetworkdPath + "/foo.network"
	createdfile := networkdFilename(SystemdNetworkdPath, "foo")
	if expectedfile != createdfile {
		t.Errorf("wrong systemd networkd path, expected '%s', got '%s'", expectedfile, createdfile)
	}
}

func TestCheckNetworkConfig(t *testing.T) {
	addr := net.IPv4(10, 210, 8, 121)
	nwconfig := networkConfiguration{
		link: &fakeLink{
			fakeAttrs: netlink.LinkAttrs{
				HardwareAddr: net.HardwareAddr{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f},
			},
		},
		localAddr: &addr,
	}
	if err := checkNetworkConfig("eth_a", &nwconfig); err != nil {
		t.Errorf("failed checking interface: %v", err)
	}

	nwconfig.localAddr = nil
	if err := checkNetworkConfig("eth_a", &nwconfig); err == nil {
		t.Errorf("failed to spot missing localAddr")
	}

	nwconfig.localAddr = &addr
	nwconfig.link.Attrs().HardwareAddr = nil
	if err := checkNetworkConfig("eth_a", &nwconfig); err == nil {
		t.Errorf("failed to spot missing Hardwareaddr")
	}

}

func fakesystemdnetworkdconfigs() (map[string]*networkConfiguration, map[string]string) {
	expectedoutput := make(map[string]string, 0)

	// reuse earlier test data and create local addresses
	nwconfigs := getFakeNetworkDataConfigs()
	_ = lldpResults(nwconfigs)

	for iface, nwconfig := range nwconfigs {
		if nwconfig.localAddr == nil {
			expectedoutput[iface] = ""
		} else {
			networkAddr := nwconfig.localAddr.Mask(net.CIDRMask(int(RouteMaskRoutedNetwork), 32))

			expectedoutput[iface] = "[Match]\nMACAddress=" +
				nwconfig.link.Attrs().HardwareAddr.String() +
				"\n\n" +
				"[Network]\nDescription=Networkd configuration for " +
				iface +
				" created by network-operator\n" +
				"Address=" +
				nwconfig.localAddr.String() + "/30" +
				"\n\n" +
				"[Route]\nDestination=" +
				networkAddr.String() + "/16\n"
		}
	}

	return nwconfigs, expectedoutput
}

func TestSystemdNetworkdConfig(t *testing.T) {
	testDir, err := os.MkdirTemp("", "networkoperator.")
	if err != nil {
		t.Errorf("cannot create tmp dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	confDir := filepath.Join(testDir, SystemdNetworkdPath)
	if err := os.MkdirAll(confDir, 0755); err != nil {
		t.Errorf("cannot create systemd-networkd config dir: %v", err)
	}

	nwconfigs, expectedoutput := fakesystemdnetworkdconfigs()

	for iface, nwconfig := range nwconfigs {
		configfile := filepath.Join(testDir, SystemdNetworkdPath, iface+".network")
		expectedstr := expectedoutput[iface]

		ifacelist, err := WriteSystemdNetworkd(confDir, map[string]*networkConfiguration{iface: nwconfig})
		if err != nil {
			if expectedstr == "" {
				continue
			}
			t.Errorf("could not create config file %s: %v", configfile, err)
		}

		if len(ifacelist) != 1 {
			t.Errorf("received wrong number of configured interfaces (%d)", len(ifacelist))
		}

		if ifacelist[0] != iface {
			t.Errorf("expected interface '%s', got '%s'", iface, ifacelist[0])
		}

		configuredstr, err := os.ReadFile(configfile)
		if string(configuredstr) != expectedstr {
			t.Errorf("read config file '%s', expected\n'%s', got \n'%s': %v",
				configfile, expectedstr, string(configuredstr), err)
		}
	}
}

func TestSystemdNetworkdConfigNoDir(t *testing.T) {
	testDir, err := os.MkdirTemp("", "networkoperator.")
	if err != nil {
		t.Errorf("cannot create tmp dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	confDir := filepath.Join(testDir, SystemdNetworkdPath)
	nwconfigs, expectedoutput := fakesystemdnetworkdconfigs()

	attemptedToConfigureNetworkd := false

	for iface, nwconfig := range nwconfigs {
		configfile := filepath.Join(testDir, SystemdNetworkdPath, iface+".network")
		expectedstr := expectedoutput[iface]

		if expectedstr == "" {
			continue
		}

		attemptedToConfigureNetworkd = true

		_, err := WriteSystemdNetworkd(confDir, map[string]*networkConfiguration{iface: nwconfig})
		if err == nil {
			t.Errorf("wrote config file %s when directory is missing: %v", configfile, err)
		}
	}

	if !attemptedToConfigureNetworkd {
		t.Errorf("there should have been some networks to configure, fix test cases!")
	}
}

func TestDeleteSystemdNetworkd(t *testing.T) {
	testDir, err := os.MkdirTemp("", "networkoperator.")
	if err != nil {
		t.Errorf("cannot create tmp dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	fakeNetworkdPath := filepath.Join(testDir, SystemdNetworkdPath)
	if err := os.MkdirAll(fakeNetworkdPath, 0755); err != nil {
		t.Errorf("cannot create systemd-networkd config dir: %v", err)
	}

	networkdfiles := []string{"eth_a", "eth_b"}
	preexisting := "eth_c"
	created := append(networkdfiles, preexisting)

	for _, ifname := range created {
		filename := networkdFilename(fakeNetworkdPath, ifname)
		_ = os.WriteFile(filename, []byte("nothing to see here\n"), 0644)
	}

	DeleteSystemdNetworkd(fakeNetworkdPath, networkdfiles)

	pattern := filepath.Join(fakeNetworkdPath, "*")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Errorf("pattern match failed: %v", err)
	}

	for _, p := range paths {
		if p != filepath.Join(fakeNetworkdPath, preexisting+".network") {
			t.Errorf("expected '%s', got '%s'", preexisting, p)
		}
	}
}
