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

type NetworkManagerIf interface {
	GetPropertyVersion() (string, error)
	GetAllDevices() ([]DeviceWrapperIf, error)
}

type DeviceWrapperIf interface {
	GetPropertyInterface() (string, error)
	SetPropertyManaged(managed bool) error
}

type DeviceWrapper struct {
	device gonetworkmanager.Device
}

type NetworkManager struct {
	nm gonetworkmanager.NetworkManager
}

func NewNetworkManager() (NetworkManagerIf, error) {
	nm, err := gonetworkmanager.NewNetworkManager()
	if err != nil {
		// Typically means there's no DBus connection
		return nil, err
	}
	return &NetworkManager{nm: nm}, nil
}

func (r *NetworkManager) GetPropertyVersion() (string, error) {
	return r.nm.GetPropertyVersion()
}

func (r *NetworkManager) GetAllDevices() ([]DeviceWrapperIf, error) {
	devices, err := r.nm.GetAllDevices()
	if err != nil {
		return nil, err
	}

	wrappedDevices := make([]DeviceWrapperIf, 0, len(devices))
	for _, device := range devices {
		wrappedDevices = append(wrappedDevices, &DeviceWrapper{device: device})
	}

	return wrappedDevices, nil
}

func (d *DeviceWrapper) GetPropertyInterface() (string, error) {
	return d.device.GetPropertyInterface()
}

func (d *DeviceWrapper) SetPropertyManaged(managed bool) error {
	return d.device.SetPropertyManaged(managed)
}

func DisableNetworkManagerForInterfaces(nm NetworkManagerIf, interfaces []string) error {
	// Check if NetworkManager is accessible
	_, err := nm.GetPropertyVersion()
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
