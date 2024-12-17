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
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/vishvananda/netlink"
)

type FakeLink struct {
	fakeAttrs netlink.LinkAttrs
}

func (l *FakeLink) Attrs() *netlink.LinkAttrs {
	return &l.fakeAttrs
}

func (l *FakeLink) Type() string {
	return ""
}

func fakenetworkconfigs() (map[string]*networkConfiguration, string) {
	lldpPeer := net.IPv4(10, 120, 0, 2)
	localAddr := net.IPv4(10, 120, 0, 1)
	networkconfig := networkConfiguration{
		link: &FakeLink{
			fakeAttrs: netlink.LinkAttrs{
				HardwareAddr: net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
			},
		},
		lldpPeer:   &lldpPeer,
		localAddr:  &localAddr,
		peerHWAddr: &net.HardwareAddr{0x06, 0x05, 0x04, 0x03, 0x02, 0x01},
	}
	expectedoutput := "{\"NIC_NET_CONFIG\":[{\"NIC_MAC\":\"01:02:03:04:05:06\"," +
		"\"NIC_IP\":\"10.120.0.1\",\"SUBNET_MASK\":\"255.255.255.252\"," +
		"\"GATEWAY_MAC\":\"06:05:04:03:02:01\"}]}"

	nwconfigs := make(map[string]*networkConfiguration)
	nwconfigs["eth1234"] = &networkconfig

	return nwconfigs, expectedoutput
}

func TestGenerateGaudiNet(t *testing.T) {
	nwconfigs, expectedoutput := fakenetworkconfigs()

	json, err := GenerateGaudiNet(nwconfigs)
	if string(json) != expectedoutput {
		t.Errorf("Expected result '%s', returned '%s': %v", expectedoutput, json, err)
	}
}

func TestWriteGaudiNet(t *testing.T) {
	dir, err := os.MkdirTemp("", "gaudinet.")
	if err != nil {
		t.Errorf("cannot create tmp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "gaudinet.txt")
	nwconfigs, expectedoutput := fakenetworkconfigs()

	if err := WriteGaudiNet(file, nwconfigs); err != nil {
		t.Errorf("cannot write to '%s': %v", file, err)
	}

	json, err := os.ReadFile(file)
	if err != nil {
		t.Errorf("could not read tmp gaudinet file: %v", err)
	}

	if string(json) != expectedoutput {
		t.Errorf("Expected tmp file contents '%s', returned '%s': %v", expectedoutput, json, err)
	}
}
