// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSchema(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Schema Suite")
}

var _ = Describe("SchemaVersion", func() {
	It("compares concrete versions numerically", func() {
		Expect(Compare("v1", "v2")).To(Equal(-1))
		Expect(Compare("v3", "v2")).To(Equal(1))
		Expect(Compare("v3", "v3")).To(Equal(0))
	})

	It("treats latest as greatest", func() {
		Expect(Compare(Latest, V1)).To(Equal(1))
		Expect(Compare(V1, Latest)).To(Equal(-1))
		Expect(Compare(Latest, Latest)).To(Equal(0))
	})

	It("fails parsing invalid versions", func() {
		_, err := ParseSchemaVersion("invalid")
		Expect(err).To(HaveOccurred())
	})

	It("finds latest from a schema list", func() {
		latest, err := LatestFrom([]SchemaVersion{"v1", "v2", "v3", Latest})
		Expect(err).NotTo(HaveOccurred())
		Expect(latest).To(Equal(SchemaVersion("v3")))
	})

	It("returns v1 as current highest concrete schema", func() {
		Expect(Highest()).To(Equal(V1))
	})

	It("stringifies to its underlying value", func() {
		Expect(V1.String()).To(Equal("v1"))
		Expect(Latest.String()).To(Equal("latest"))
	})
})
