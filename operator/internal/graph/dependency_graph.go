/*
 * SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package graph

import (
	"fmt"
	"io"
	"slices"
	"sort"
)

// DependencyGraph interface contains all functionally of the graph, adding vertices and walking the graph.
type DependencyGraph[T any] interface {
	Walk[T]
	// Add adds a dependency to the graph along with anything it depends on
	// and an object the user wants .
	Add(name string, object T, dependencies ...string) error
}

// Walk contains methods for just walking of a DependencyGraph.
type Walk[T any] interface {
	// Next is for iterating over the graph. You call it nothing to begin, and each time you call it with
	// what was returned the last time it was called, or from any place you would like to begin
	Next(from ...string) ([]string, error)
	// Returns the object for any set of dependencies. Its meant to be paired with next.
	Get(from ...string) []T
	// Valid will tell you if the graph is valid.
	Valid() error
}

func New[T any]() DependencyGraph[T] {
	return &dag[T]{
		vertices:     map[string]*vertex[T]{},
		placeholders: map[string]*vertex[T]{},
		leafs:        map[string]*vertex[T]{},
	}
}

type dag[T any] struct {
	vertices     map[string]*vertex[T]
	leafs        map[string]*vertex[T]
	placeholders map[string]*vertex[T]
}

type vertex[T any] struct {
	edges   []*vertex[T]
	parents []string
	name    string
	object  T
}

func (v *vertex[T]) IsRoot() bool {
	return len(v.edges) == 0
}

func (d *dag[T]) Add(name string, object T, dependencies ...string) error {

	if _, ok := d.vertices[name]; ok {
		return fmt.Errorf("error [%s] vertex already exists", name)
	}

	var vert *vertex[T]
	vert, ok := d.placeholders[name]
	delete(d.placeholders, name)
	if !ok {
		vert = &vertex[T]{name: name, object: object, parents: dependencies}
	} else {
		// set object on placeholder
		vert.object = object
	}
	d.vertices[name] = vert

	if len(dependencies) == 0 {
		d.leafs[name] = vert
	}

	for _, dependency := range dependencies {
		if dep, ok := d.vertices[dependency]; ok {
			dep.edges = append(dep.edges, vert)
		} else {
			// place holder, ie not yet in graph
			place, ok := d.placeholders[dependency]
			if !ok {
				place = &vertex[T]{name: dependency}
			}
			d.placeholders[dependency] = place
			place.edges = append(place.edges, vert)
		}
	}

	return nil
}

// leaves handle edge cases where there are more then one leaf, and from is a subset of the leaves not in the from
func (d *dag[T]) leaves(from []string) []string {
	leaves := make([]string, 0)
	for _, f := range d.leafs {
		leaves = append(leaves, f.name)
	}
	if len(from) > len(leaves) {
		return nil
	}
	dif := diff(leaves, from)
	return dif
}

// diff returns the elements in a that are not in b
func diff(a, b []string) []string {
	ret := make([]string, 0)
	for _, v := range a {
		if !slices.Contains(b, v) {
			ret = append(ret, v)
		}
	}
	return ret
}

func (d *dag[T]) Next(from ...string) ([]string, error) {
	if err := d.Valid(); err != nil {
		return nil, err
	}

	if len(from) == 0 { // base starting case
		return getNames(flat(d.leafs)), nil
	}
	leaves := d.leaves(from)
	if len(leaves) > 0 {
		return leaves, nil
	}

	// Use a map to deduplicate edges
	seen := make(map[string]*vertex[T])
	for _, f := range from {
		vert := d.vertices[f]
		for _, edge := range vert.edges {
			// Skip if already processed
			if slices.Contains(from, edge.name) {
				continue
			}

			// Check if all parents are in the completed set
			allParentsSatisfied := true
			for _, parent := range edge.parents {
				if !slices.Contains(from, parent) {
					allParentsSatisfied = false
					break
				}
			}
			if allParentsSatisfied {
				seen[edge.name] = edge
			}
		}
	}

	// Convert map to slice
	root := make([]*vertex[T], 0, len(seen))
	for _, v := range seen {
		root = append(root, v)
	}

	sortEdges(root)
	return getNames(root), nil
}

func (d *dag[T]) Get(from ...string) []T {
	ret := make([]T, 0)
	for _, f := range from {
		ret = append(ret, d.vertices[f].object)
	}
	return ret
}

func (d *dag[T]) Valid() error {
	if len(d.placeholders) > 0 {
		miss := make([]string, 0)
		for k := range d.placeholders {
			miss = append(miss, k)
		}
		return fmt.Errorf("error graph is not valid, missing: %v", miss)
	}
	return nil
}

func flat[T any](m map[string]*vertex[T]) []*vertex[T] {
	root := make([]*vertex[T], 0)
	for _, val := range m {
		root = append(root, val)
	}

	sortEdges(root)
	return root
}

func sortEdges[T any](e []*vertex[T]) {
	sort.Slice(e, func(i, j int) bool {
		return e[i].name < e[j].name
	})
}

func getNames[T any](vs []*vertex[T]) []string {
	ret := make([]string, 0)
	for _, v := range vs {
		ret = append(ret, v.name)
	}
	return ret
}

// PrintGraph is helper function for print out a DependencyGraph graph,
// or any thing that implements the Walk interface
func PrintGraph[T any](out io.Writer, d Walk[T]) error {
	ret := make([][]string, 0)
	completed := make([]string, 0)
	for {
		step, _ := d.Next(completed...)
		if len(step) == 0 {
			break // end of graph
		}
		ret = append(ret, step)
		completed = append(completed, step...)
	}

	for i := range ret {
		if _, err := fmt.Fprintf(out, "%v", ret[i]); err != nil {
			return err
		}
		if i != len(ret)-1 {
			if _, err := fmt.Fprint(out, " -> "); err != nil {
				return err
			}
		}
	}
	return nil
}
