// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
)

func MatchFinalizer(finalizer string) gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Finalizers": MatchAllElements(stringIdentifier, Elements{
				finalizer: Equal(finalizer),
			}),
		}),
	})
}

func stringIdentifier(element interface{}) string {
	return element.(string)
}

func ParseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
}

// SwitchDirectory sets the working directory and returns a function to revert to the previous one.
func SwitchDirectory(path string) func() {
	oldPath, err := os.Getwd()
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	if err := os.Chdir(path); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	return func() {
		if err := os.Chdir(oldPath); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}
