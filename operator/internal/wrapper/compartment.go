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

package wrapper

import (
	"fmt"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func NewCompartmentWrapper(c *v1alpha1.Compartment) *Compartment {
	return &Compartment{
		Compartment: *c,
	}
}

type Compartment struct {
	v1alpha1.Compartment
	Nodes []SkyhookNode
}

func (c *Compartment) GetName() string {
	return c.Name
}

func (c *Compartment) GetNodes() []SkyhookNode {
	return c.Nodes
}

func (c *Compartment) GetNode(name string) SkyhookNode {
	for _, node := range c.Nodes {
		if node.GetNode().Name == name {
			return node
		}
	}
	return nil
}

func (c *Compartment) AddNode(node SkyhookNode) {
	c.Nodes = append(c.Nodes, node)
}

// AssignNodeToCompartment assigns a single node to the appropriate compartment
func AssignNodeToCompartment(node SkyhookNode, compartments map[string]*Compartment) (string, error) {
	nodeLabels := labels.Set(node.GetNode().Labels)

	// Check all non-default compartments first
	for _, compartment := range compartments {
		// Skip the default compartment - it's a fallback
		if compartment.Name == v1alpha1.DefaultCompartmentName {
			continue
		}

		selector, err := metav1.LabelSelectorAsSelector(&compartment.Selector)
		if err != nil {
			return "", fmt.Errorf("invalid selector for compartment %s: %w", compartment.Name, err)
		}
		if selector.Matches(nodeLabels) {
			return compartment.Name, nil
		}
	}

	// No matches - assign to default
	return v1alpha1.DefaultCompartmentName, nil
}
