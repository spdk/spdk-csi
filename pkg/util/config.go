/*
Copyright (c) Arm Limited and Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

const (
	// TODO: move hardcoded settings to config map
	cfgRPCTimeoutSeconds = 20
	cfgLvolClearMethod   = "unmap" // none, unmap, write_zeroes
	cfgLvolThinProvision = true
	cfgNVMfSvcPort       = "4420"
	cfgISCSISvcPort      = "3260"
	cfgAllowAnyHost      = true
	cfgAddrFamily        = "IPv4" // IPv4, IPv6, IB, FC
)

// Config stores parsed command line parameters
type Config struct {
	DriverName    string
	DriverVersion string
	Endpoint      string
	NodeID        string

	IsControllerServer bool
	IsNodeServer       bool
}
