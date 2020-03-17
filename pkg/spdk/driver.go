/*
Copyright 2020 The SPDK-CSI Authors.

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

package spdk

import (
	"time"

	"k8s.io/klog"

	"github.com/spdk/spdk-csi/pkg/util"
)

func Run(conf *util.Config) {
	klog.Infof("dummy driver started: %s", conf.DriverName)
	for {
		time.Sleep(time.Hour)
	}
}
