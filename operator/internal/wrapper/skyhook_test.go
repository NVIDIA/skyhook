/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package wrapper

import (
	"github.com/NVIDIA/skyhook/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Skyhook wrapper tests", func() {
	It("Should get config updates", func() {
		skyhook := &Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Status: v1alpha1.SkyhookStatus{
					ConfigUpdates: map[string][]string{
						"foo": {
							"changed",
						},
						"bar": {
							"changed",
						},
					},
				},
			},
			Updated: false,
		}

		Expect(skyhook.GetConfigUpdates()).To(BeEquivalentTo(map[string][]string{
			"foo": {
				"changed",
			},
			"bar": {
				"changed",
			},
		}))

		skyhook = &Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Status: v1alpha1.SkyhookStatus{},
			},
			Updated: false,
		}

		Expect(skyhook.GetConfigUpdates()).To(BeNil())
	})

	It("Should add config updates", func() {
		skyhook := &Skyhook{
			Skyhook: &v1alpha1.Skyhook{},
			Updated: false,
		}

		skyhook.AddConfigUpdates("foo", "changed")
		Expect(skyhook.Status.ConfigUpdates).To(BeEquivalentTo(map[string][]string{
			"foo": {
				"changed",
			},
		}))

		skyhook.AddConfigUpdates("foo", "added")
		skyhook.AddConfigUpdates("bar", "changed")

		Expect(skyhook.Status.ConfigUpdates).To(BeEquivalentTo(map[string][]string{
			"foo": {
				"changed",
				"added",
			},
			"bar": {
				"changed",
			},
		}))

		skyhook.AddConfigUpdates("foo", "added", "changed")

		Expect(skyhook.Status.ConfigUpdates).To(BeEquivalentTo(map[string][]string{
			"foo": {
				"changed",
				"added",
			},
			"bar": {
				"changed",
			},
		}))
	})

	It("Should remove config updates on a per-package basis", func() {
		skyhook := &Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Status: v1alpha1.SkyhookStatus{},
			},
			Updated: false,
		}

		skyhook.RemoveConfigUpdates("foo")
		skyhook.RemoveConfigUpdates("")
		Expect(skyhook.Status.ConfigUpdates).To(BeNil())

		skyhook = &Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Status: v1alpha1.SkyhookStatus{
					ConfigUpdates: map[string][]string{
						"foo": {
							"changed",
							"again",
						},
						"bar": {
							"changed",
							"again",
						},
					},
				},
			},
			Updated: false,
		}

		skyhook.RemoveConfigUpdates("foo")
		Expect(skyhook.Status.ConfigUpdates).To(BeEquivalentTo(map[string][]string{
			"bar": {
				"changed",
				"again",
			},
		}))
	})

	It("Should get config interrupts", func() {
		skyhook := &Skyhook{
			Skyhook: &v1alpha1.Skyhook{},
			Updated: false,
		}

		Expect(skyhook.GetConfigInterrupts()).To(BeEquivalentTo(map[string][]*v1alpha1.Interrupt{}))

		skyhook = &Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: v1alpha1.Packages{
						"foo": v1alpha1.Package{
							PackageRef: v1alpha1.PackageRef{
								Name:    "foo",
								Version: "1.1.2",
							},
						},
						"bar": v1alpha1.Package{
							PackageRef: v1alpha1.PackageRef{
								Name:    "bar",
								Version: "3",
							},
							ConfigInterrupts: map[string]v1alpha1.Interrupt{
								"run.sh": {
									Type: v1alpha1.REBOOT,
								},
								"check.sh": {
									Type:     v1alpha1.SERVICE,
									Services: []string{"cron", "foo"},
								},
							},
						},
					},
				},
				Status: v1alpha1.SkyhookStatus{
					ConfigUpdates: map[string][]string{
						"bar": {
							"run.sh",
						},
						"foo": {
							"check.sh",
						},
					},
				},
			},
			Updated: false,
		}

		Expect(skyhook.GetConfigInterrupts()).To(BeEquivalentTo(map[string][]*v1alpha1.Interrupt{
			"bar": {
				{
					Type: v1alpha1.REBOOT,
				},
			},
		}))

		skyhook = &Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: v1alpha1.Packages{
						"foo": v1alpha1.Package{
							PackageRef: v1alpha1.PackageRef{
								Name:    "foo",
								Version: "1.1.2",
							},
						},
						"bar": v1alpha1.Package{
							PackageRef: v1alpha1.PackageRef{
								Name:    "bar",
								Version: "3",
							},
							ConfigInterrupts: map[string]v1alpha1.Interrupt{
								"run.sh": {
									Type: v1alpha1.REBOOT,
								},
								"check.sh": {
									Type:     v1alpha1.SERVICE,
									Services: []string{"cron", "foo"},
								},
								"foobar.sh": {
									Type:     v1alpha1.SERVICE,
									Services: []string{"testing"},
								},
							},
						},
					},
				},
				Status: v1alpha1.SkyhookStatus{
					ConfigUpdates: map[string][]string{
						"bar": {
							"run.sh",
							"check.sh",
							"foobar.sh",
						},
						"foo": {
							"check.sh",
						},
					},
				},
			},
			Updated: false,
		}

		Expect(skyhook.GetConfigInterrupts()).To(BeEquivalentTo(map[string][]*v1alpha1.Interrupt{
			"bar": {
				{
					Type: v1alpha1.REBOOT,
				},
				{
					Type:     v1alpha1.SERVICE,
					Services: []string{"cron", "foo"},
				},
				{
					Type:     v1alpha1.SERVICE,
					Services: []string{"testing"},
				},
			},
		}))
	})
})
