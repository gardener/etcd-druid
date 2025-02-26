// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package imagevector_test

import (
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	. "github.com/gardener/etcd-druid/internal/utils/imagevector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("validation", func() {
	var (
		imageVector func(string, *string, *string, *string, string, string) ImageVector
	)

	BeforeEach(func() {
		imageVector = func(name string, ref, repository, tag *string, runtimeVersion, targetVersion string) ImageVector {
			return ImageVector{
				{
					Name:           name,
					Ref:            ref,
					Repository:     repository,
					Tag:            tag,
					RuntimeVersion: ptr.To(runtimeVersion),
					TargetVersion:  ptr.To(targetVersion),
				},
			}
		}
	})

	Describe("#ValidateImageVector", func() {
		It("should allow valid image vectors", func() {
			errorList := ValidateImageVector(imageVector("test-image1", nil, ptr.To("test-repo"), ptr.To("test-tag"), ">= 1.6, < 1.8", ">= 1.8"), field.NewPath("images"))

			Expect(errorList).To(BeEmpty())
		})

		It("should forbid invalid image vectors", func() {
			errorList := ValidateImageVector(imageVector("", nil, nil, ptr.To(""), "", "!@#"), field.NewPath("images"))

			Expect(errorList).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("images[0].name"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("images[0].ref/repository"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("images[0].tag"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("images[0].runtimeVersion"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("images[0].targetVersion"),
				})),
			))
		})

		It("should forbid empty ref", func() {
			Expect(ValidateImageVector(imageVector("foo", ptr.To(""), nil, nil, ">= 1.6", "< 1.8"), field.NewPath("images"))).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(field.ErrorTypeInvalid),
					"Field":  Equal("images[0].ref"),
					"Detail": Equal("ref must not be empty if specified"),
				})),
			))
		})

		It("should forbid empty repository", func() {
			Expect(ValidateImageVector(imageVector("foo", nil, ptr.To(""), nil, ">= 1.6", "< 1.8"), field.NewPath("images"))).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(field.ErrorTypeInvalid),
					"Field":  Equal("images[0].repository"),
					"Detail": Equal("repository must not be empty if specified"),
				})),
			))
		})

		It("should forbid specifying repository/tag when ref is set", func() {
			Expect(ValidateImageVector(imageVector("foo", ptr.To("ref"), ptr.To("repo"), ptr.To("tag"), ">= 1.6", "< 1.8"), field.NewPath("images"))).To(ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(field.ErrorTypeForbidden),
					"Field":  Equal("images[0].repository"),
					"Detail": Equal("cannot specify repository when ref is set"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":   Equal(field.ErrorTypeForbidden),
					"Field":  Equal("images[0].tag"),
					"Detail": Equal("cannot specify tag when ref is set"),
				})),
			))
		})
	})
})
