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

// blackbox test of util package
package util_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spdk/spdk-csi/pkg/util"
)

func TestTryLockSequential(t *testing.T) {
	var tryLock util.TryLock

	// acquire lock
	if !tryLock.Lock() {
		t.Fatalf("failed to acquire lock")
	}
	// acquire a locked lock should fail
	if tryLock.Lock() {
		t.Fatalf("acquired a locked lock")
	}
	// acquire a released lock should succeed
	tryLock.Unlock()
	if !tryLock.Lock() {
		t.Fatal("failed to acquire a release lock")
	}
}

func TestTryLockConcurrent(t *testing.T) {
	var tryLock util.TryLock
	var wg sync.WaitGroup
	var lockCount int32
	const taskCount = 50

	// only one task should acquire the lock
	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func() {
			if tryLock.Lock() {
				atomic.AddInt32(&lockCount, 1)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	if lockCount != 1 {
		t.Fatal("concurrency test failed")
	}
}
