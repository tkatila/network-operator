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
	"errors"
	"os"
	"testing"
)

type MockNetworkManager struct {
	mockVersionQuery  func() (string, error)
	mockGetAllDevices func() ([]DeviceWrapperIf, error)
}

func (m *MockNetworkManager) GetPropertyVersion() (string, error) {
	return m.mockVersionQuery()
}
func (m *MockNetworkManager) GetAllDevices() ([]DeviceWrapperIf, error) {
	return m.mockGetAllDevices()
}

type MockDevice struct {
	mockIface      func() (string, error)
	mockSetManaged func(bool) error
}

func (d *MockDevice) GetPropertyInterface() (string, error) {
	return d.mockIface()
}
func (d *MockDevice) SetPropertyManaged(manage bool) error {
	return d.mockSetManaged(manage)
}

// Not sure if this works in CI, but it takes the coverage from ~50% to 90%.
func TestDisableNetworkManagerForInterfacesOnHost(t *testing.T) {
	interfaces := []string{"ethXYZ", "ethZYX"}

	nm, err := NewNetworkManager()
	if nm == nil || err != nil {
		t.Fatalf("NewNetworkManager failed")
	}

	err = DisableNetworkManagerForInterfaces(nm, interfaces)
	if err != nil {
		t.Errorf("DisableNetworkManagerForInterfaces failed: %v", err)
	}
}

func TestDisableNetworkManagerForInterfaces(t *testing.T) {
	interfaces := []string{"ethXYZ", "ethZYX"}

	nm := &MockNetworkManager{
		mockVersionQuery: func() (string, error) {
			return "1.0.0", nil
		},
		mockGetAllDevices: func() ([]DeviceWrapperIf, error) {
			return []DeviceWrapperIf{
				&MockDevice{
					mockIface: func() (string, error) {
						return "ethXYZ", nil
					},
					mockSetManaged: func(manage bool) error {
						return nil
					},
				},
				&MockDevice{
					mockIface: func() (string, error) {
						return "ethZYX", nil
					},
					mockSetManaged: func(manage bool) error {
						return nil
					},
				},
			}, nil
		},
	}

	err := DisableNetworkManagerForInterfaces(nm, interfaces)
	if err != nil {
		t.Errorf("DisableNetworkManagerForInterfaces failed: %v", err)
	}
}

type TestCase struct {
	name          string
	ifaces        []string
	versionErr    error
	getDevicesErr error
	ifaceErr      error
	setManagedErr error
	expectedErr   error
}

var testCases = []TestCase{
	{
		name:        "TestDisableNetworkManagerVersionQueryFails",
		versionErr:  os.ErrInvalid,
		expectedErr: nil,
	},
	{
		name:          "TestDisableNetworkManagerGetDevicesFails",
		getDevicesErr: os.ErrDeadlineExceeded,
		expectedErr:   os.ErrDeadlineExceeded,
	},
	{
		name:        "TestDisableNetworkManagerGetDeviceInterfaceFails",
		ifaces:      []string{"ethXYZ", "ethZYX"},
		ifaceErr:    os.ErrPermission,
		expectedErr: os.ErrPermission,
	},
	{
		name:          "TestDisableNetworkManagerSetDeviceManagedFails",
		ifaces:        []string{"ethXYZ", "ethZYX"},
		setManagedErr: os.ErrProcessDone,
		expectedErr:   os.ErrProcessDone,
	},
}

func TestDisableNetworkManagerForInterfacesError(t *testing.T) {
	interfaces := []string{"ethXYZ", "ethZYX"}

	for _, tc := range testCases {

		nm := &MockNetworkManager{
			mockVersionQuery: func() (string, error) {
				return "1.0.0", tc.versionErr
			},
			mockGetAllDevices: func() ([]DeviceWrapperIf, error) {
				if tc.getDevicesErr != nil {
					return nil, tc.getDevicesErr
				}

				ret := []DeviceWrapperIf{}
				for _, iface := range tc.ifaces {
					ret = append(ret, &MockDevice{
						mockIface: func() (string, error) {
							if tc.ifaceErr != nil {
								return "", tc.ifaceErr
							}
							return iface, nil
						},
						mockSetManaged: func(manage bool) error {
							if tc.setManagedErr != nil {
								return tc.setManagedErr
							}
							return nil
						},
					})
				}

				return ret, nil
			},
		}

		err := DisableNetworkManagerForInterfaces(nm, interfaces)
		if !errors.Is(err, tc.expectedErr) {
			t.Errorf("%s should have failed: %v", tc.name, err)
		}
	}
}
