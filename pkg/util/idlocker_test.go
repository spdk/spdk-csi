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
	"testing"
	"time"
)

func TestIDLocker(t *testing.T) {
	t.Parallel()
	fakeID := "fake-id"
	locks := NewVolumeLocks()
	// acquire lock for fake-id
	start := time.Now()
	unlock := locks.Lock(fakeID)
	go func() {
		time.Sleep(1 * time.Second)
		unlock()
	}()
	unlock2 := locks.Lock(fakeID)
	unlock2()
	duration := time.Since(start)
	// time.Sleep() might not be very accurate,
	// using 900 milliseconds to check the test would be more stable.
	if duration.Milliseconds() < 900 {
		t.Errorf("lock was required before it was released")
	}
}
