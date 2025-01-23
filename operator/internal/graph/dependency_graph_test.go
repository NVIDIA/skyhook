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

package graph

import (
	"math/rand"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDependencyGraph(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "DAG Suite")
}

var _ = Describe("DAG tests", func() {

	It("test building when created in broken order", func() {

		d := New[struct{ a string }]()

		payload := struct{ a string }{a: "bar"}

		Expect(d.Add("A", payload)).Should(Succeed())
		Expect(d.Add("C", payload, "B")).Should(Succeed())
		Expect(d.Add("B", payload, "A")).Should(Succeed())
		Expect(d.Add("D", payload, "A", "B")).Should(Succeed())
		Expect(d.Add("E", payload)).Should(Succeed())

		dag, ok := d.(*dag[struct{ a string }])
		Expect(ok).To(BeTrue())

		// make sure the relationships are correct
		Expect(dag.vertices["A"].edges[0] == dag.vertices["B"]).To(BeTrue())
		Expect(dag.vertices["B"].edges[0] == dag.vertices["C"]).To(BeTrue())

		By("walk the graph")

		step, err := d.Next()
		Expect(err).To(BeNil())
		Expect(ok).To(BeTrue())
		Expect(step).To(BeEquivalentTo([]string{"A", "E"}))

		step, _ = d.Next(step...)
		Expect(step).To(BeEquivalentTo([]string{"B"}))

		step, _ = d.Next(step...)
		Expect(step).To(BeEquivalentTo([]string{"C", "D"}))

		payloads := d.Get(step...)
		Expect(payloads).To(BeEquivalentTo([]struct{ a string }{payload, payload}))

		By("if we go again, should get nothing, we are at the end")
		step, _ = d.Next(step...)
		Expect(step).To(BeEmpty())

		By("testing printing does not error")

		err = PrintGraph[struct{ a string }](GinkgoWriter, dag)
		Expect(err).To(BeNil())

	})

	It("test error on dup vertex", func() {
		d := New[*string]()

		Expect(d.Add("A", nil)).Should(Succeed())
		Expect(d.Add("A", nil)).ShouldNot(Succeed())
	})

	It("walking something even more complicated should work", func() {
		var d DependencyGraph[*string]

		// setting this test to make sure creating and walking works
		// we are creating the graph in random orders to make sure it works in all orders

		spt := func(s string) *string {
			return &s
		}

		adds := make(map[string]func(), 0)

		adds["F"] = func() {
			//root 1
			Expect(d.Add("F", spt("F"), "E", "B")).Should(Succeed())
		}

		adds["G"] = func() {
			// root 2
			Expect(d.Add("G", spt("G"), "C")).Should(Succeed())
		}

		adds["Z"] = func() {
			// root 3 and leaf
			Expect(d.Add("Z", spt("Z"))).Should(Succeed())
		}

		adds["D"] = func() {
			// root 4
			Expect(d.Add("D", spt("D"), "B", "A")).Should(Succeed())
		}

		adds["E"] = func() {
			// level 2
			Expect(d.Add("E", spt("E"), "C")).Should(Succeed())
		}

		adds["C"] = func() {
			// level 3
			Expect(d.Add("C", spt("C"), "B")).Should(Succeed())
		}

		adds["B"] = func() {
			// level 4
			Expect(d.Add("B", spt("B"), "A")).Should(Succeed())
		}

		adds["A"] = func() {
			// leaf 5
			Expect(d.Add("A", spt("A"))).Should(Succeed())
		}

		order := make([]string, 0)
		for f := range adds {
			order = append(order, f)
		}

		// do in many random orders
		for i := 1; i < len(order); i++ {
			d = New[*string]()

			rand.Shuffle(len(order), func(i, j int) {
				order[i], order[j] = order[j], order[i]
			})

			GinkgoLogr.Info("order", "vertex order", order)
			for _, f := range order {
				adds[f]()
			}

			By("walk the graph we should get the following")

			uspta := func(as []*string) []string {
				ret := make([]string, 0)
				for _, s := range as {
					ret = append(ret, *s)
				}
				return ret
			}

			step, err := d.Next()
			Expect(err).To(BeNil())
			Expect(step).To(BeEquivalentTo([]string{"A", "Z"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"A", "Z"}))

			step, _ = d.Next(step...)
			Expect(step).To(BeEquivalentTo([]string{"B"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"B"}))

			step, _ = d.Next(step...)
			Expect(step).To(BeEquivalentTo([]string{"C", "D"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"C", "D"}))

			step, _ = d.Next(step...)
			Expect(step).To(BeEquivalentTo([]string{"E", "G"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"E", "G"}))

			step, _ = d.Next(step...)
			Expect(step).To(BeEquivalentTo([]string{"F"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"F"}))
		}

	})

	It("error on walking broken graph", func() {
		d := New[*string]()
		Expect(d.Add("bar", nil, "foo")).Should(Succeed())
		_, err := d.Next()
		Expect(err).ToNot(BeNil())

	})
})
