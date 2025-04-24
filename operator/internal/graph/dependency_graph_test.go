/*
 * LICENSE START
 *
 *    Copyright (c) NVIDIA CORPORATION.  All rights reserved.
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 *
 * LICENSE END
 */

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
		/* Graph structure:
		  A   E
		  |
		  B
		 / \
		C   D
		*/

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

		completed := make([]string, 0)
		step, err := d.Next(completed...)
		Expect(err).To(BeNil())
		Expect(ok).To(BeTrue())
		Expect(step).To(BeEquivalentTo([]string{"A", "E"}))
		completed = append(completed, step...)

		step, _ = d.Next(completed...)
		Expect(step).To(BeEquivalentTo([]string{"B"}))
		completed = append(completed, step...)

		step, _ = d.Next(completed...)
		Expect(step).To(BeEquivalentTo([]string{"C", "D"}))
		completed = append(completed, step...)

		payloads := d.Get(step...)
		Expect(payloads).To(BeEquivalentTo([]struct{ a string }{payload, payload}))

		By("if we go again, should get nothing, we are at the end")
		step, _ = d.Next(completed...)
		Expect(step).To(BeEmpty())

		By("testing printing does not error")

		err = PrintGraph[struct{ a string }](GinkgoWriter, dag)
		Expect(err).To(BeNil())
	})

	It("test error on dup vertex", func() {
		/* Graph structure:
		   A -> A (duplicate, should fail)
		*/

		d := New[*string]()

		Expect(d.Add("A", nil)).Should(Succeed())
		Expect(d.Add("A", nil)).ShouldNot(Succeed())
	})

	It("walking something even more complicated should work", func() {
		/* Graph structure:
			   A   Z
			   |
			   B
			  / \
			 C   D
			/ \
		       E   G
		       \   /
		         F
		*/

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
			Expect(PrintGraph(GinkgoWriter, d)).To(Succeed())

			uspta := func(as []*string) []string {
				ret := make([]string, 0)
				for _, s := range as {
					ret = append(ret, *s)
				}
				return ret
			}

			completed := make([]string, 0)
			step, err := d.Next(completed...)
			Expect(err).To(BeNil())
			Expect(step).To(BeEquivalentTo([]string{"A", "Z"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"A", "Z"}))
			completed = append(completed, step...)

			step, _ = d.Next(completed...)
			Expect(step).To(BeEquivalentTo([]string{"B"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"B"}))
			completed = append(completed, step...)

			step, _ = d.Next(completed...)
			Expect(step).To(BeEquivalentTo([]string{"C", "D"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"C", "D"}))
			completed = append(completed, step...)

			step, _ = d.Next(completed...)
			Expect(step).To(BeEquivalentTo([]string{"E", "G"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"E", "G"}))
			completed = append(completed, step...)

			step, _ = d.Next(completed...)
			Expect(step).To(BeEquivalentTo([]string{"F"}))
			Expect(uspta(d.Get(step...))).To(BeEquivalentTo([]string{"F"}))
		}
	})

	It("error on walking broken graph", func() {
		/* Graph structure:
		   bar -> foo (foo doesn't exist)
		*/

		d := New[*string]()
		Expect(d.Add("bar", nil, "foo")).Should(Succeed())
		completed := make([]string, 0)
		_, err := d.Next(completed...)
		Expect(err).ToNot(BeNil())
	})

	It("test walking empty graph", func() {
		d := New[*string]()
		completed := make([]string, 0)
		step, err := d.Next(completed...)
		Expect(err).To(BeNil())
		Expect(step).To(BeEmpty())
		completed = append(completed, step...)

		step, err = d.Next(completed...)
		Expect(err).To(BeNil())
		Expect(step).To(BeEmpty())
	})

	It("test walking graph with one vertex", func() {
		/* Graph structure:
		   foo
		*/

		d := New[*string]()
		Expect(d.Add("foo", nil)).Should(Succeed())
		complete := make([]string, 0)
		step, err := d.Next(complete...)
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"foo"}))
	})

	It("should not return duplicates of the same from next", func() {
		/* Graph structure:
		   A   B
		    \ /
		     C
		*/

		d := New[*string]()
		Expect(d.Add("A", nil)).Should(Succeed())
		Expect(d.Add("B", nil)).Should(Succeed())
		Expect(d.Add("C", nil, "A", "B")).Should(Succeed())

		step, err := d.Next()
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"A", "B"}))

		step, err = d.Next("A")
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"B"}))

		step, err = d.Next("A", "B")
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"C"}))
	})

	It("should return the correct next when called with a multiple step from with a partial last step", func() {
		/* Graph structure:
		   A   B
		    \ /
		     C
		    / \
		   D   E
		    \ /
		     F
		*/

		d := New[*string]()
		Expect(d.Add("A", nil)).Should(Succeed())
		Expect(d.Add("B", nil)).Should(Succeed())
		Expect(d.Add("C", nil, "A", "B")).Should(Succeed())
		Expect(d.Add("D", nil, "C")).Should(Succeed())
		Expect(d.Add("E", nil, "C")).Should(Succeed())
		Expect(d.Add("F", nil, "E", "D")).Should(Succeed())

		Expect(PrintGraph(GinkgoWriter, d)).To(Succeed())

		complete := []string{"A", "B", "C", "E"}
		step, err := d.Next(complete...)
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"D"}))
	})

	It("should walk the graph one vertex at a time", func() {
		/* Graph structure:
		   A   B
		    \ /
		     C
		    / \
		   D   E
		    \ /
		     F
		*/

		d := New[*string]()
		spt := func(s string) *string { return &s }

		Expect(d.Add("A", spt("A"))).Should(Succeed())
		Expect(d.Add("B", spt("B"))).Should(Succeed())
		Expect(d.Add("C", spt("C"), "A", "B")).Should(Succeed())
		Expect(d.Add("D", spt("D"), "C")).Should(Succeed())
		Expect(d.Add("E", spt("E"), "C")).Should(Succeed())
		Expect(d.Add("F", spt("F"), "D", "E")).Should(Succeed())

		Expect(PrintGraph(GinkgoWriter, d)).To(Succeed())

		// Walk one at a time, starting with no completed vertices
		step, err := d.Next()
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"A", "B"}))

		// Complete A first
		step, err = d.Next("A")
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"B"}))

		// Now complete B
		step, err = d.Next("A", "B")
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"C"}))

		// Complete C
		step, err = d.Next("A", "B", "C")
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"D", "E"}))

		// Complete D
		step, err = d.Next("A", "B", "C", "D")
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"E"}))

		// Complete E
		step, err = d.Next("A", "B", "C", "D", "E")
		Expect(err).To(BeNil())
		Expect(step).To(BeEquivalentTo([]string{"F"}))

		// Complete F - should get empty list as we're done
		step, err = d.Next("A", "B", "C", "D", "E", "F")
		Expect(err).To(BeNil())
		Expect(step).To(BeEmpty())

		// Verify we get the correct payloads at each step
		Expect(*d.Get("A")[0]).To(Equal("A"))
		Expect(*d.Get("B")[0]).To(Equal("B"))
		Expect(*d.Get("C")[0]).To(Equal("C"))
		Expect(*d.Get("D")[0]).To(Equal("D"))
		Expect(*d.Get("E")[0]).To(Equal("E"))
		Expect(*d.Get("F")[0]).To(Equal("F"))
	})
})
