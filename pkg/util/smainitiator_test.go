/*
Copyright (c) Intel Corporation.

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
	"strings"
	"testing"
)

func TestVolumeUUID(t *testing.T) {
	tests := map[string]struct {
		model  string
		errMsg string
	}{
		"empty-uuid-string": {
			model:  "",
			errMsg: "no volume available",
		},
		"invalid-uuid-string": {
			model:  "invalid-uuid-string",
			errMsg: "uuid.Parse(invalid-uuid-string) failed:",
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			_, err := volumeUUID(test.model)
			if err != nil {
				if !strings.Contains(err.Error(), test.errMsg) {
					t.Errorf("Error message mismatch. Expected: %s, Got: %s", test.errMsg, err.Error())
				}
			}
		})
	}
}
