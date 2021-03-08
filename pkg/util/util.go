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
	"encoding/json"
	"io/ioutil"
	"os"
	"sync/atomic"
)

func ParseJSONFile(fileName string, result interface{}) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(bytes, result)
}

// round up bytes to megabytes
func ToMiB(bytes int64) int64 {
	const mi = 1024 * 1024
	return (bytes + mi - 1) / mi
}

// ${env:-def}
func FromEnv(env, def string) string {
	s := os.Getenv(env)
	if s != "" {
		return s
	}
	return def
}

// a trivial trylock implementation
type TryLock struct {
	locked int32
}

// acquire lock w/o waiting, return true if acquired, false otherwise
func (lock *TryLock) Lock() bool {
	// golang CAS forces sequential consistent memory order
	return atomic.CompareAndSwapInt32(&lock.locked, 0, 1)
}

// release lock
func (lock *TryLock) Unlock() {
	// golang atomic store forces release memory order
	atomic.StoreInt32(&lock.locked, 0)
}
