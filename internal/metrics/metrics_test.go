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

package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics", func() {
	Describe("Testing helper functions for metrics initialization", func() {
		Context("Testing wrapInSlice with input", func() {
			It("should return expected output", func() {
				input := []string{"a", "b", "c"}
				expectedOutput := [][]string{{"a"}, {"b"}, {"c"}}
				output := wrapInSlice(input)
				Expect(output).Should(Equal(expectedOutput))
			})
		})
		Context("Testing cartesianProduct with inputs", func() {
			It("should return expected output", func() {
				input1 := [][]string{{"p", "q"}, {"r", "s"}}
				input2 := [][]string{{"1", "2"}, {"3", "4"}}
				expectedOutput := [][]string{
					{"p", "q", "1", "2"},
					{"p", "q", "3", "4"},
					{"r", "s", "1", "2"},
					{"r", "s", "3", "4"},
				}
				output := cartesianProduct(input1, input2)
				Expect(output).Should(Equal(expectedOutput))
			})
		})
		Context("Testing generateLabelCombinations with input of one label", func() {
			It("should return expected output", func() {
				input := map[string][]string{
					"a": {
						"1",
						"2",
						"3",
					},
				}
				expectedOutput := []map[string]string{
					{"a": "1"},
					{"a": "2"},
					{"a": "3"},
				}
				output := GenerateLabelCombinations(input)
				Expect(output).Should(Equal(expectedOutput))
			})
		})
		Context("Testing generateLabelCombinations with input of two labels", func() {
			It("should return expected output", func() {
				input := map[string][]string{
					"a": {
						"1",
						"2",
						"3",
					},
					"b": {
						"4",
						"5",
					},
				}
				expectedOutput := []map[string]string{
					{"a": "1", "b": "4"},
					{"a": "1", "b": "5"},
					{"a": "2", "b": "4"},
					{"a": "2", "b": "5"},
					{"a": "3", "b": "4"},
					{"a": "3", "b": "5"},
				}
				output := GenerateLabelCombinations(input)
				Expect(output).Should(Equal(expectedOutput))
			})
		})
	})
})
