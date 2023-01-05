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

import (
	"sync"
)

// VolumeLocks simple locks that can be acquired by volumeID
type VolumeLocks struct {
	mutexes sync.Map
}

// NewVolumeLocks returns new VolumeLocks.
func NewVolumeLocks() *VolumeLocks {
	return &VolumeLocks{}
}

// Lock obtain the lock corresponding to the volumeID
func (vl *VolumeLocks) Lock(volumeID string) func() {
	value, _ := vl.mutexes.LoadOrStore(volumeID, &sync.Mutex{})
	mtx, _ := value.(*sync.Mutex) //nolint:errcheck // will not fail to convert
	mtx.Lock()
	return func() { mtx.Unlock() }
}
