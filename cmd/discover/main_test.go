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
	"testing"
)

func TestSelectMask30L3Address(t *testing.T) {
	peer := net.IPv4(10, 210, 8, 122)
	expected := net.IPv4(10, 210, 8, 121)

	nwconfig := networkConfiguration{
		portDescription: "no-alert 10.210.8.122/30",
	}

	peeraddr, localaddr, err := selectMask30L3Address(&nwconfig)
	if !peeraddr.Equal(peer) {
		t.Errorf("Peer addresses do not match, expected %s got %s: %v", peer.String(), peeraddr.String(), err)
	}
	if !localaddr.Equal(expected) {
		t.Errorf("Local addresses do not match, expected %s got %s: %v", expected.String(), localaddr.String(), err)
	}
}
