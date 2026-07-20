/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("legacyCleanupShouldPrune", func() {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	rfc := func(t time.Time) string { return t.Format(time.RFC3339) }

	DescribeTable("prune decision",
		func(stamp string, delay time.Duration, want bool) {
			Expect(legacyCleanupShouldPrune(stamp, delay, now)).To(Equal(want))
		},
		Entry("zero delay prunes immediately regardless of stamp", "", time.Duration(0), true),
		Entry("negative delay prunes immediately", rfc(now), -time.Hour, true),
		Entry("positive delay with no stamp converges first", "", 24*time.Hour, false),
		Entry("window not yet elapsed holds", rfc(now.Add(-time.Hour)), 24*time.Hour, false),
		Entry("window elapsed prunes", rfc(now.Add(-25*time.Hour)), 24*time.Hour, true),
		Entry("exactly at the deadline prunes", rfc(now.Add(-24*time.Hour)), 24*time.Hour, true),
		Entry("unparseable stamp fails toward prune", "not-a-timestamp", 24*time.Hour, true),
	)
})
