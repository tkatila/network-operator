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

package networkmanager

import (
	"slices"

	"github.com/Wifx/gonetworkmanager/v3"
	"k8s.io/klog/v2"
)

func DisableNetworkManagerForInterfaces(interfaces []string) error {
	nm, err := gonetworkmanager.NewNetworkManager()
	if err != nil {
		// This means that the DBus connection failed.
		return err
	}

	// Check if NetworkManager is accessible
	_, err = nm.GetPropertyVersion()
	if err != nil {
		klog.Info("Couldn't read NetworkManager version. It's probably not running.")

		return nil
	}

	devices, err := nm.GetAllDevices()
	if err != nil {
		return err
	}

	for _, device := range devices {
		netif, err := device.GetPropertyInterface()
		if err != nil {
			return err
		}

		if slices.Contains(interfaces, netif) {
			err = device.SetPropertyManaged(false)
			if err != nil {
				return err
			}

			klog.Infof("Disabled NetworkManager for interface %s", netif)
		}
	}

	return nil
}
