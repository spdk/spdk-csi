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

import "encoding/json"

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

// CSIControllerConfig config for csi driver controller server, see deploy/kubernetes/config-map.yaml
//
//nolint:tagliatelle // not using json:snake case
type CSIControllerConfig struct {
	Nodes []SpdkNodeConfig `json:"Nodes"`
}

// SpdkNodeConfig config for spdk storage cluster
//
//nolint:tagliatelle // not using json:snake case
type SpdkNodeConfig struct {
	Name       string `json:"name"`
	URL        string `json:"rpcURL"`
	TargetType string `json:"targetType"`
	TargetAddr string `json:"targetAddr"`
}

func NewCSIControllerConfig(env, def string) (*CSIControllerConfig, error) {
	var config CSIControllerConfig
	configFile := FromEnv(env, def)
	err := ParseJSONFile(configFile, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SpdkSecrets spdk storage cluster connection secrets, see deploy/kubernetes/secrets.yaml
//
//nolint:tagliatelle // not using json:snake case
type SpdkSecrets struct {
	Tokens []struct {
		Name     string `json:"name"`
		UserName string `json:"username"`
		Password string `json:"password"`
	} `json:"rpcTokens"`
}

func NewSpdkSecrets(jsonSecrets string) (*SpdkSecrets, error) {
	var secs SpdkSecrets
	err := json.Unmarshal([]byte(jsonSecrets), &secs)
	if err != nil {
		return nil, err
	}
	return &secs, nil
}
