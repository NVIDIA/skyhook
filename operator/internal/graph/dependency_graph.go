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
	"fmt"
	"io"
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
	edges  []*vertex[T]
	name   string
	object T
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
		vert = &vertex[T]{name: name, object: object}
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

func (d *dag[T]) Next(from ...string) ([]string, error) {

	if err := d.Valid(); err != nil {
		return nil, err
	}

	if len(from) == 0 { // base staring case
		return getNames(flat(d.leafs)), nil
	}
	root := make([]*vertex[T], 0)

	for _, f := range from {
		vert := d.vertices[f]
		// if _, ok := d.vertices[f]; !ok {
		// 	// verts := make([]string, 0)
		// 	// for k := range d.vertices {
		// 	// 	verts = append(verts, k)
		// 	// }
		// 	// return nil, fmt.Errorf("error [%s] does not exist in the graph. Possible vertices %v", f, verts)
		// 	continue // not a thing, well ignore it
		// }
		for _, edge := range d.vertices[f].edges {
			// search the path before adding it, if the path is longer than 1 then we don't want to add it
			// this aids in walking the graph later because erroneous paths have been pre pruned
			if len(d.find_longest_path(vert.name, edge.name)) <= 1 {
				root = append(root, edge)
			}
		}
	}

	// remove matching from input
	in := make(map[string]struct{})
	for _, f := range from {
		in[f] = struct{}{}
	}

	temp := root[:0]
	for _, out := range root {
		if _, ok := in[out.name]; !ok {
			temp = append(temp, out)
		}
	}
	root = temp

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

// search dag for longest common path
func (d *dag[T]) find_longest_path(start, end string) []string {

	from := d.vertices[start]
	if from.IsRoot() {
		return nil
	}

	paths := make(map[string][]string)
	for _, vert := range from.edges {
		if vert.name == end {
			paths[vert.name] = []string{end}
		}
		path := d.find_longest_path(vert.name, end)
		if path != nil {
			paths[vert.name] = append([]string{vert.name}, path...)
		}
	}
	var ret []string
	for _, v := range paths {
		if len(v) > len(ret) {
			ret = v
		}
	}
	return ret
}

// PrintGraph is helper function for print out a DependencyGraph graph,
// or any thing that implements the Walk interface
func PrintGraph[T any](out io.Writer, d Walk[T]) error {
	step := make([]string, 0)
	ret := make([][]string, 0)
	for {
		step, _ = d.Next(step...)
		if len(step) == 0 {
			break // end
		}
		ret = append(ret, step)
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
