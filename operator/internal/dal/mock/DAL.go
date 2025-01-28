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
// Code generated by mockery v2.42.3. DO NOT EDIT.

package dal

import (
	context "context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	mock "github.com/stretchr/testify/mock"

	v1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/NVIDIA/skyhook/api/v1alpha1"
)

// MockDAL is an autogenerated mock type for the DAL type
type MockDAL struct {
	mock.Mock
}

type MockDAL_Expecter struct {
	mock *mock.Mock
}

func (_m *MockDAL) EXPECT() *MockDAL_Expecter {
	return &MockDAL_Expecter{mock: &_m.Mock}
}

// GetNode provides a mock function with given fields: ctx, nodeName
func (_m *MockDAL) GetNode(ctx context.Context, nodeName string) (*v1.Node, error) {
	ret := _m.Called(ctx, nodeName)

	if len(ret) == 0 {
		panic("no return value specified for GetNode")
	}

	var r0 *v1.Node
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*v1.Node, error)); ok {
		return rf(ctx, nodeName)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *v1.Node); ok {
		r0 = rf(ctx, nodeName)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Node)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, nodeName)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDAL_GetNode_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetNode'
type MockDAL_GetNode_Call struct {
	*mock.Call
}

// GetNode is a helper method to define mock.On call
//   - ctx context.Context
//   - nodeName string
func (_e *MockDAL_Expecter) GetNode(ctx interface{}, nodeName interface{}) *MockDAL_GetNode_Call {
	return &MockDAL_GetNode_Call{Call: _e.mock.On("GetNode", ctx, nodeName)}
}

func (_c *MockDAL_GetNode_Call) Run(run func(ctx context.Context, nodeName string)) *MockDAL_GetNode_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *MockDAL_GetNode_Call) Return(_a0 *v1.Node, _a1 error) *MockDAL_GetNode_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDAL_GetNode_Call) RunAndReturn(run func(context.Context, string) (*v1.Node, error)) *MockDAL_GetNode_Call {
	_c.Call.Return(run)
	return _c
}

// GetNodes provides a mock function with given fields: ctx, opts
func (_m *MockDAL) GetNodes(ctx context.Context, opts ...client.ListOption) (*v1.NodeList, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetNodes")
	}

	var r0 *v1.NodeList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, ...client.ListOption) (*v1.NodeList, error)); ok {
		return rf(ctx, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, ...client.ListOption) *v1.NodeList); ok {
		r0 = rf(ctx, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.NodeList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, ...client.ListOption) error); ok {
		r1 = rf(ctx, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDAL_GetNodes_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetNodes'
type MockDAL_GetNodes_Call struct {
	*mock.Call
}

// GetNodes is a helper method to define mock.On call
//   - ctx context.Context
//   - opts ...client.ListOption
func (_e *MockDAL_Expecter) GetNodes(ctx interface{}, opts ...interface{}) *MockDAL_GetNodes_Call {
	return &MockDAL_GetNodes_Call{Call: _e.mock.On("GetNodes",
		append([]interface{}{ctx}, opts...)...)}
}

func (_c *MockDAL_GetNodes_Call) Run(run func(ctx context.Context, opts ...client.ListOption)) *MockDAL_GetNodes_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]client.ListOption, len(args)-1)
		for i, a := range args[1:] {
			if a != nil {
				variadicArgs[i] = a.(client.ListOption)
			}
		}
		run(args[0].(context.Context), variadicArgs...)
	})
	return _c
}

func (_c *MockDAL_GetNodes_Call) Return(_a0 *v1.NodeList, _a1 error) *MockDAL_GetNodes_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDAL_GetNodes_Call) RunAndReturn(run func(context.Context, ...client.ListOption) (*v1.NodeList, error)) *MockDAL_GetNodes_Call {
	_c.Call.Return(run)
	return _c
}

// GetPod provides a mock function with given fields: ctx, namespace, name
func (_m *MockDAL) GetPod(ctx context.Context, namespace string, name string) (*v1.Pod, error) {
	ret := _m.Called(ctx, namespace, name)

	if len(ret) == 0 {
		panic("no return value specified for GetPod")
	}

	var r0 *v1.Pod
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) (*v1.Pod, error)); ok {
		return rf(ctx, namespace, name)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string) *v1.Pod); ok {
		r0 = rf(ctx, namespace, name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.Pod)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string) error); ok {
		r1 = rf(ctx, namespace, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDAL_GetPod_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPod'
type MockDAL_GetPod_Call struct {
	*mock.Call
}

// GetPod is a helper method to define mock.On call
//   - ctx context.Context
//   - namespace string
//   - name string
func (_e *MockDAL_Expecter) GetPod(ctx interface{}, namespace interface{}, name interface{}) *MockDAL_GetPod_Call {
	return &MockDAL_GetPod_Call{Call: _e.mock.On("GetPod", ctx, namespace, name)}
}

func (_c *MockDAL_GetPod_Call) Run(run func(ctx context.Context, namespace string, name string)) *MockDAL_GetPod_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(string))
	})
	return _c
}

func (_c *MockDAL_GetPod_Call) Return(_a0 *v1.Pod, _a1 error) *MockDAL_GetPod_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDAL_GetPod_Call) RunAndReturn(run func(context.Context, string, string) (*v1.Pod, error)) *MockDAL_GetPod_Call {
	_c.Call.Return(run)
	return _c
}

// GetPods provides a mock function with given fields: ctx, opts
func (_m *MockDAL) GetPods(ctx context.Context, opts ...client.ListOption) (*v1.PodList, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetPods")
	}

	var r0 *v1.PodList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, ...client.ListOption) (*v1.PodList, error)); ok {
		return rf(ctx, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, ...client.ListOption) *v1.PodList); ok {
		r0 = rf(ctx, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.PodList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, ...client.ListOption) error); ok {
		r1 = rf(ctx, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDAL_GetPods_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetPods'
type MockDAL_GetPods_Call struct {
	*mock.Call
}

// GetPods is a helper method to define mock.On call
//   - ctx context.Context
//   - opts ...client.ListOption
func (_e *MockDAL_Expecter) GetPods(ctx interface{}, opts ...interface{}) *MockDAL_GetPods_Call {
	return &MockDAL_GetPods_Call{Call: _e.mock.On("GetPods",
		append([]interface{}{ctx}, opts...)...)}
}

func (_c *MockDAL_GetPods_Call) Run(run func(ctx context.Context, opts ...client.ListOption)) *MockDAL_GetPods_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]client.ListOption, len(args)-1)
		for i, a := range args[1:] {
			if a != nil {
				variadicArgs[i] = a.(client.ListOption)
			}
		}
		run(args[0].(context.Context), variadicArgs...)
	})
	return _c
}

func (_c *MockDAL_GetPods_Call) Return(_a0 *v1.PodList, _a1 error) *MockDAL_GetPods_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDAL_GetPods_Call) RunAndReturn(run func(context.Context, ...client.ListOption) (*v1.PodList, error)) *MockDAL_GetPods_Call {
	_c.Call.Return(run)
	return _c
}

// GetSkyhook provides a mock function with given fields: ctx, name, opts
func (_m *MockDAL) GetSkyhook(ctx context.Context, name string, opts ...client.ListOption) (*v1alpha1.Skyhook, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, name)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetSkyhook")
	}

	var r0 *v1alpha1.Skyhook
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, ...client.ListOption) (*v1alpha1.Skyhook, error)); ok {
		return rf(ctx, name, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, ...client.ListOption) *v1alpha1.Skyhook); ok {
		r0 = rf(ctx, name, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.Skyhook)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, ...client.ListOption) error); ok {
		r1 = rf(ctx, name, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDAL_GetSkyhook_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetSkyhook'
type MockDAL_GetSkyhook_Call struct {
	*mock.Call
}

// GetSkyhook is a helper method to define mock.On call
//   - ctx context.Context
//   - name string
//   - opts ...client.ListOption
func (_e *MockDAL_Expecter) GetSkyhook(ctx interface{}, name interface{}, opts ...interface{}) *MockDAL_GetSkyhook_Call {
	return &MockDAL_GetSkyhook_Call{Call: _e.mock.On("GetSkyhook",
		append([]interface{}{ctx, name}, opts...)...)}
}

func (_c *MockDAL_GetSkyhook_Call) Run(run func(ctx context.Context, name string, opts ...client.ListOption)) *MockDAL_GetSkyhook_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]client.ListOption, len(args)-2)
		for i, a := range args[2:] {
			if a != nil {
				variadicArgs[i] = a.(client.ListOption)
			}
		}
		run(args[0].(context.Context), args[1].(string), variadicArgs...)
	})
	return _c
}

func (_c *MockDAL_GetSkyhook_Call) Return(_a0 *v1alpha1.Skyhook, _a1 error) *MockDAL_GetSkyhook_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDAL_GetSkyhook_Call) RunAndReturn(run func(context.Context, string, ...client.ListOption) (*v1alpha1.Skyhook, error)) *MockDAL_GetSkyhook_Call {
	_c.Call.Return(run)
	return _c
}

// GetSkyhooks provides a mock function with given fields: ctx, opts
func (_m *MockDAL) GetSkyhooks(ctx context.Context, opts ...client.ListOption) (*v1alpha1.SkyhookList, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	if len(ret) == 0 {
		panic("no return value specified for GetSkyhooks")
	}

	var r0 *v1alpha1.SkyhookList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, ...client.ListOption) (*v1alpha1.SkyhookList, error)); ok {
		return rf(ctx, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, ...client.ListOption) *v1alpha1.SkyhookList); ok {
		r0 = rf(ctx, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.SkyhookList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, ...client.ListOption) error); ok {
		r1 = rf(ctx, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDAL_GetSkyhooks_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetSkyhooks'
type MockDAL_GetSkyhooks_Call struct {
	*mock.Call
}

// GetSkyhooks is a helper method to define mock.On call
//   - ctx context.Context
//   - opts ...client.ListOption
func (_e *MockDAL_Expecter) GetSkyhooks(ctx interface{}, opts ...interface{}) *MockDAL_GetSkyhooks_Call {
	return &MockDAL_GetSkyhooks_Call{Call: _e.mock.On("GetSkyhooks",
		append([]interface{}{ctx}, opts...)...)}
}

func (_c *MockDAL_GetSkyhooks_Call) Run(run func(ctx context.Context, opts ...client.ListOption)) *MockDAL_GetSkyhooks_Call {
	_c.Call.Run(func(args mock.Arguments) {
		variadicArgs := make([]client.ListOption, len(args)-1)
		for i, a := range args[1:] {
			if a != nil {
				variadicArgs[i] = a.(client.ListOption)
			}
		}
		run(args[0].(context.Context), variadicArgs...)
	})
	return _c
}

func (_c *MockDAL_GetSkyhooks_Call) Return(_a0 *v1alpha1.SkyhookList, _a1 error) *MockDAL_GetSkyhooks_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDAL_GetSkyhooks_Call) RunAndReturn(run func(context.Context, ...client.ListOption) (*v1alpha1.SkyhookList, error)) *MockDAL_GetSkyhooks_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockDAL creates a new instance of MockDAL. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockDAL(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockDAL {
	mock := &MockDAL{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
