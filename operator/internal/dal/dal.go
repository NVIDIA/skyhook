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

package dal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	skyhookv1alpha1 "github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// podLogStreamTimeout bounds the best-effort deadline log tail. The stream is proxied to the
// kubelet and is read precisely when a node may be slow or unreachable; the reconciler runs
// one pass at a time, so an unbounded read would wedge the whole queue. A truncated or empty
// tail is an acceptable outcome for best-effort evidence.
const podLogStreamTimeout = 10 * time.Second

// logTailLines is the server-side bound on the deadline log tail. Lines, not bytes, because
// TailLines is the only option that reads from the end; it is deliberately generous since
// tailAndSanitize applies the real byte cap.
const logTailLines = 500

// New builds the DAL over the controller-runtime client used for every typed
// get/list, plus a client-go clientset used only by GetPodLogTail; pod logs are
// a subresource stream the controller-runtime client cannot read. clientset may
// be nil in contexts that never read logs (e.g. the event handler's DAL); GetPodLogTail
// returns an error rather than panicking in that case.
func New(c client.Client, clientset kubernetes.Interface) DAL {
	return &dal{client: c, clientset: clientset}
}

// DAL gives a typed interface to the kubernetes interface which is generic ano not typed
// I find this to be more readable and using the generated mock is easier too
// get and list are hard to mock, update is not an issue, but might as well live here too
type DAL interface {
	GetSkyhook(ctx context.Context, name string, opts ...client.ListOption) (*skyhookv1alpha1.NodeWright, error)
	GetSkyhooks(ctx context.Context, opts ...client.ListOption) (*skyhookv1alpha1.NodeWrightList, error)
	GetNode(ctx context.Context, nodeName string) (*corev1.Node, error)
	GetNodes(ctx context.Context, opts ...client.ListOption) (*corev1.NodeList, error)
	GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	GetPods(ctx context.Context, opts ...client.ListOption) (*corev1.PodList, error)
	GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error)
	GetJobs(ctx context.Context, opts ...client.ListOption) (*batchv1.JobList, error)
	GetPodLogTail(ctx context.Context, namespace, pod, container string, maxBytes int64) (string, error)
	GetDeploymentPolicies(ctx context.Context, opts ...client.ListOption) (*skyhookv1alpha1.DeploymentPolicyList, error)
	GetDeploymentPolicy(ctx context.Context, name string) (*skyhookv1alpha1.DeploymentPolicy, error)
}

type dal struct {
	client    client.Client
	clientset kubernetes.Interface
}

func (e *dal) GetSkyhook(ctx context.Context, name string, opts ...client.ListOption) (*skyhookv1alpha1.NodeWright, error) {
	var skyhook skyhookv1alpha1.NodeWright

	// nodes does have namespace so leaving blank
	if err := e.client.Get(ctx, types.NamespacedName{Name: name}, &skyhook); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting nodewright [%s]: %w", name, err)
	}

	return &skyhook, nil
}

func (e *dal) GetSkyhooks(ctx context.Context, opts ...client.ListOption) (*skyhookv1alpha1.NodeWrightList, error) {

	var skyhook skyhookv1alpha1.NodeWrightList
	if err := e.client.List(ctx, &skyhook, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting nodewrights: %w", err)
	}

	if len(skyhook.Items) == 0 {
		return nil, nil
	}

	return &skyhook, nil
}

func (e *dal) GetNode(ctx context.Context, nodeName string) (*corev1.Node, error) {
	var node corev1.Node

	// nodes does have namespace so leaving blank
	if err := e.client.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting node [%s]: %w", nodeName, err)
	}

	return &node, nil
}

func (e *dal) GetNodes(ctx context.Context, opts ...client.ListOption) (*corev1.NodeList, error) {
	var nodes corev1.NodeList
	if err := e.client.List(ctx, &nodes, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		return nil, nil
	}

	return &nodes, nil
}

func (e *dal) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	var pod corev1.Pod

	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting pod [%s|%s]: %w", namespace, name, err)
	}

	return &pod, nil
}

func (e *dal) GetPods(ctx context.Context, opts ...client.ListOption) (*corev1.PodList, error) {
	var pods corev1.PodList
	if err := e.client.List(ctx, &pods, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, nil
	}

	return &pods, nil
}

func (e *dal) GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error) {
	var job batchv1.Job

	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &job); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting job [%s|%s]: %w", namespace, name, err)
	}

	return &job, nil
}

func (e *dal) GetJobs(ctx context.Context, opts ...client.ListOption) (*batchv1.JobList, error) {
	var jobs batchv1.JobList
	if err := e.client.List(ctx, &jobs, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting jobs: %w", err)
	}

	if len(jobs.Items) == 0 {
		return nil, nil
	}

	return &jobs, nil
}

// GetPodLogTail streams a container's logs and returns at most maxBytes from the tail,
// sanitized to valid UTF-8. It is best-effort evidence capture for a stage about to be
// killed by its deadline, so the caller treats any error as "no logs"; the byte cap
// keeps the result inside the object's metadata budget when it lands in an annotation.
func (e *dal) GetPodLogTail(ctx context.Context, namespace, pod, container string, maxBytes int64) (string, error) {
	if e.clientset == nil {
		return "", errors.New("no clientset configured for reading pod logs")
	}

	// Bound the proxied log read so a slow/unreachable kubelet cannot stall the single
	// reconcile pass; the tail is best-effort, so a timeout just yields no logs.
	ctx, cancel := context.WithTimeout(ctx, podLogStreamTimeout)
	defer cancel()

	// TailLines bounds the transfer server-side. Without it the kubelet streams the whole
	// log and tailAndSanitize discards all but the tail locally, so a stage that ran to its
	// deadline producing steady output blows the timeout above and yields nothing: the
	// snapshot fails exactly when its logs are most worth having. TailLines is the only
	// tail-bounded option (LimitBytes reads from the start), so it is a coarse line-based
	// bound with tailAndSanitize still enforcing the exact byte cap.
	tailLines := int64(logTailLines)
	stream, err := e.clientset.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{
		Container: container,
		TailLines: &tailLines,
	}).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("error opening log stream for pod [%s|%s] container [%s]: %w", namespace, pod, container, err)
	}
	// A read handle: the close error carries no information the caller can act on.
	defer func() { _ = stream.Close() }()

	tail, err := tailAndSanitize(stream, maxBytes)
	if err != nil {
		return "", fmt.Errorf("error reading log stream for pod [%s|%s] container [%s]: %w", namespace, pod, container, err)
	}
	return tail, nil
}

// tailAndSanitize reads r to EOF, keeping only the last maxBytes, and returns them
// as valid UTF-8 (invalid byte sequences, including a rune the tail cut in half,
// become U+FFFD). It holds at most maxBytes plus one chunk in memory, so an arbitrarily
// large log stream stays bounded.
func tailAndSanitize(r io.Reader, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		return "", nil
	}

	tail := make([]byte, 0, maxBytes)
	chunk := make([]byte, 32*1024)
	for {
		n, err := r.Read(chunk)
		if n > 0 {
			tail = append(tail, chunk[:n]...)
			if int64(len(tail)) > maxBytes {
				tail = tail[int64(len(tail))-maxBytes:]
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}

	return strings.ToValidUTF8(string(tail), "�"), nil
}

func (e *dal) GetDeploymentPolicies(ctx context.Context, opts ...client.ListOption) (*skyhookv1alpha1.DeploymentPolicyList, error) {
	var policies skyhookv1alpha1.DeploymentPolicyList
	if err := e.client.List(ctx, &policies, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting deployment policies: %w", err)
	}

	return &policies, nil
}

func (e *dal) GetDeploymentPolicy(ctx context.Context, name string) (*skyhookv1alpha1.DeploymentPolicy, error) {
	var policy skyhookv1alpha1.DeploymentPolicy

	// DeploymentPolicy is cluster-scoped, so no namespace is needed
	if err := e.client.Get(ctx, types.NamespacedName{Name: name}, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting deployment policy [%s]: %w", name, err)
	}

	return &policy, nil
}
