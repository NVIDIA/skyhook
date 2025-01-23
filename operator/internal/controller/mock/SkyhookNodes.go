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

package controller

import (
	logr "github.com/go-logr/logr"
	mock "github.com/stretchr/testify/mock"

	v1alpha1 "gitlab-master.nvidia.com/dgx/infra/skyhook-operator/api/v1alpha1"

	wrapper "gitlab-master.nvidia.com/dgx/infra/skyhook-operator/internal/wrapper"
)

// MockSkyhookNodes is an autogenerated mock type for the SkyhookNodes type
type MockSkyhookNodes struct {
	mock.Mock
}

type MockSkyhookNodes_Expecter struct {
	mock *mock.Mock
}

func (_m *MockSkyhookNodes) EXPECT() *MockSkyhookNodes_Expecter {
	return &MockSkyhookNodes_Expecter{mock: &_m.Mock}
}

// CollectNodeStatus provides a mock function with given fields:
func (_m *MockSkyhookNodes) CollectNodeStatus() v1alpha1.Status {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for CollectNodeStatus")
	}

	var r0 v1alpha1.Status
	if rf, ok := ret.Get(0).(func() v1alpha1.Status); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(v1alpha1.Status)
	}

	return r0
}

// MockSkyhookNodes_CollectNodeStatus_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'CollectNodeStatus'
type MockSkyhookNodes_CollectNodeStatus_Call struct {
	*mock.Call
}

// CollectNodeStatus is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) CollectNodeStatus() *MockSkyhookNodes_CollectNodeStatus_Call {
	return &MockSkyhookNodes_CollectNodeStatus_Call{Call: _e.mock.On("CollectNodeStatus")}
}

func (_c *MockSkyhookNodes_CollectNodeStatus_Call) Run(run func()) *MockSkyhookNodes_CollectNodeStatus_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_CollectNodeStatus_Call) Return(_a0 v1alpha1.Status) *MockSkyhookNodes_CollectNodeStatus_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_CollectNodeStatus_Call) RunAndReturn(run func() v1alpha1.Status) *MockSkyhookNodes_CollectNodeStatus_Call {
	_c.Call.Return(run)
	return _c
}

// GetNode provides a mock function with given fields: name
func (_m *MockSkyhookNodes) GetNode(name string) (v1alpha1.Status, wrapper.SkyhookNode) {
	ret := _m.Called(name)

	if len(ret) == 0 {
		panic("no return value specified for GetNode")
	}

	var r0 v1alpha1.Status
	var r1 wrapper.SkyhookNode
	if rf, ok := ret.Get(0).(func(string) (v1alpha1.Status, wrapper.SkyhookNode)); ok {
		return rf(name)
	}
	if rf, ok := ret.Get(0).(func(string) v1alpha1.Status); ok {
		r0 = rf(name)
	} else {
		r0 = ret.Get(0).(v1alpha1.Status)
	}

	if rf, ok := ret.Get(1).(func(string) wrapper.SkyhookNode); ok {
		r1 = rf(name)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(wrapper.SkyhookNode)
		}
	}

	return r0, r1
}

// MockSkyhookNodes_GetNode_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetNode'
type MockSkyhookNodes_GetNode_Call struct {
	*mock.Call
}

// GetNode is a helper method to define mock.On call
//   - name string
func (_e *MockSkyhookNodes_Expecter) GetNode(name interface{}) *MockSkyhookNodes_GetNode_Call {
	return &MockSkyhookNodes_GetNode_Call{Call: _e.mock.On("GetNode", name)}
}

func (_c *MockSkyhookNodes_GetNode_Call) Run(run func(name string)) *MockSkyhookNodes_GetNode_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string))
	})
	return _c
}

func (_c *MockSkyhookNodes_GetNode_Call) Return(_a0 v1alpha1.Status, _a1 wrapper.SkyhookNode) *MockSkyhookNodes_GetNode_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockSkyhookNodes_GetNode_Call) RunAndReturn(run func(string) (v1alpha1.Status, wrapper.SkyhookNode)) *MockSkyhookNodes_GetNode_Call {
	_c.Call.Return(run)
	return _c
}

// GetNodes provides a mock function with given fields:
func (_m *MockSkyhookNodes) GetNodes() []wrapper.SkyhookNode {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetNodes")
	}

	var r0 []wrapper.SkyhookNode
	if rf, ok := ret.Get(0).(func() []wrapper.SkyhookNode); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]wrapper.SkyhookNode)
		}
	}

	return r0
}

// MockSkyhookNodes_GetNodes_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetNodes'
type MockSkyhookNodes_GetNodes_Call struct {
	*mock.Call
}

// GetNodes is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) GetNodes() *MockSkyhookNodes_GetNodes_Call {
	return &MockSkyhookNodes_GetNodes_Call{Call: _e.mock.On("GetNodes")}
}

func (_c *MockSkyhookNodes_GetNodes_Call) Run(run func()) *MockSkyhookNodes_GetNodes_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_GetNodes_Call) Return(_a0 []wrapper.SkyhookNode) *MockSkyhookNodes_GetNodes_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_GetNodes_Call) RunAndReturn(run func() []wrapper.SkyhookNode) *MockSkyhookNodes_GetNodes_Call {
	_c.Call.Return(run)
	return _c
}

// GetSkyhook provides a mock function with given fields:
func (_m *MockSkyhookNodes) GetSkyhook() *wrapper.Skyhook {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for GetSkyhook")
	}

	var r0 *wrapper.Skyhook
	if rf, ok := ret.Get(0).(func() *wrapper.Skyhook); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*wrapper.Skyhook)
		}
	}

	return r0
}

// MockSkyhookNodes_GetSkyhook_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetSkyhook'
type MockSkyhookNodes_GetSkyhook_Call struct {
	*mock.Call
}

// GetSkyhook is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) GetSkyhook() *MockSkyhookNodes_GetSkyhook_Call {
	return &MockSkyhookNodes_GetSkyhook_Call{Call: _e.mock.On("GetSkyhook")}
}

func (_c *MockSkyhookNodes_GetSkyhook_Call) Run(run func()) *MockSkyhookNodes_GetSkyhook_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_GetSkyhook_Call) Return(_a0 *wrapper.Skyhook) *MockSkyhookNodes_GetSkyhook_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_GetSkyhook_Call) RunAndReturn(run func() *wrapper.Skyhook) *MockSkyhookNodes_GetSkyhook_Call {
	_c.Call.Return(run)
	return _c
}

// IsComplete provides a mock function with given fields:
func (_m *MockSkyhookNodes) IsComplete() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for IsComplete")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// MockSkyhookNodes_IsComplete_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'IsComplete'
type MockSkyhookNodes_IsComplete_Call struct {
	*mock.Call
}

// IsComplete is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) IsComplete() *MockSkyhookNodes_IsComplete_Call {
	return &MockSkyhookNodes_IsComplete_Call{Call: _e.mock.On("IsComplete")}
}

func (_c *MockSkyhookNodes_IsComplete_Call) Run(run func()) *MockSkyhookNodes_IsComplete_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_IsComplete_Call) Return(_a0 bool) *MockSkyhookNodes_IsComplete_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_IsComplete_Call) RunAndReturn(run func() bool) *MockSkyhookNodes_IsComplete_Call {
	_c.Call.Return(run)
	return _c
}

// Migrate provides a mock function with given fields: logger
func (_m *MockSkyhookNodes) Migrate(logger logr.Logger) error {
	ret := _m.Called(logger)

	if len(ret) == 0 {
		panic("no return value specified for Migrate")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(logr.Logger) error); ok {
		r0 = rf(logger)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MockSkyhookNodes_Migrate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Migrate'
type MockSkyhookNodes_Migrate_Call struct {
	*mock.Call
}

// Migrate is a helper method to define mock.On call
//   - logger logr.Logger
func (_e *MockSkyhookNodes_Expecter) Migrate(logger interface{}) *MockSkyhookNodes_Migrate_Call {
	return &MockSkyhookNodes_Migrate_Call{Call: _e.mock.On("Migrate", logger)}
}

func (_c *MockSkyhookNodes_Migrate_Call) Run(run func(logger logr.Logger)) *MockSkyhookNodes_Migrate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(logr.Logger))
	})
	return _c
}

func (_c *MockSkyhookNodes_Migrate_Call) Return(_a0 error) *MockSkyhookNodes_Migrate_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_Migrate_Call) RunAndReturn(run func(logr.Logger) error) *MockSkyhookNodes_Migrate_Call {
	_c.Call.Return(run)
	return _c
}

// NodeCount provides a mock function with given fields:
func (_m *MockSkyhookNodes) NodeCount() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for NodeCount")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// MockSkyhookNodes_NodeCount_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NodeCount'
type MockSkyhookNodes_NodeCount_Call struct {
	*mock.Call
}

// NodeCount is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) NodeCount() *MockSkyhookNodes_NodeCount_Call {
	return &MockSkyhookNodes_NodeCount_Call{Call: _e.mock.On("NodeCount")}
}

func (_c *MockSkyhookNodes_NodeCount_Call) Run(run func()) *MockSkyhookNodes_NodeCount_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_NodeCount_Call) Return(_a0 int) *MockSkyhookNodes_NodeCount_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_NodeCount_Call) RunAndReturn(run func() int) *MockSkyhookNodes_NodeCount_Call {
	_c.Call.Return(run)
	return _c
}

// ReportState provides a mock function with given fields:
func (_m *MockSkyhookNodes) ReportState() {
	_m.Called()
}

// MockSkyhookNodes_ReportState_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ReportState'
type MockSkyhookNodes_ReportState_Call struct {
	*mock.Call
}

// ReportState is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) ReportState() *MockSkyhookNodes_ReportState_Call {
	return &MockSkyhookNodes_ReportState_Call{Call: _e.mock.On("ReportState")}
}

func (_c *MockSkyhookNodes_ReportState_Call) Run(run func()) *MockSkyhookNodes_ReportState_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_ReportState_Call) Return() *MockSkyhookNodes_ReportState_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockSkyhookNodes_ReportState_Call) RunAndReturn(run func()) *MockSkyhookNodes_ReportState_Call {
	_c.Call.Return(run)
	return _c
}

// SetStatus provides a mock function with given fields: status
func (_m *MockSkyhookNodes) SetStatus(status v1alpha1.Status) {
	_m.Called(status)
}

// MockSkyhookNodes_SetStatus_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SetStatus'
type MockSkyhookNodes_SetStatus_Call struct {
	*mock.Call
}

// SetStatus is a helper method to define mock.On call
//   - status v1alpha1.Status
func (_e *MockSkyhookNodes_Expecter) SetStatus(status interface{}) *MockSkyhookNodes_SetStatus_Call {
	return &MockSkyhookNodes_SetStatus_Call{Call: _e.mock.On("SetStatus", status)}
}

func (_c *MockSkyhookNodes_SetStatus_Call) Run(run func(status v1alpha1.Status)) *MockSkyhookNodes_SetStatus_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(v1alpha1.Status))
	})
	return _c
}

func (_c *MockSkyhookNodes_SetStatus_Call) Return() *MockSkyhookNodes_SetStatus_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockSkyhookNodes_SetStatus_Call) RunAndReturn(run func(v1alpha1.Status)) *MockSkyhookNodes_SetStatus_Call {
	_c.Call.Return(run)
	return _c
}

// Status provides a mock function with given fields:
func (_m *MockSkyhookNodes) Status() v1alpha1.Status {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Status")
	}

	var r0 v1alpha1.Status
	if rf, ok := ret.Get(0).(func() v1alpha1.Status); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(v1alpha1.Status)
	}

	return r0
}

// MockSkyhookNodes_Status_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Status'
type MockSkyhookNodes_Status_Call struct {
	*mock.Call
}

// Status is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) Status() *MockSkyhookNodes_Status_Call {
	return &MockSkyhookNodes_Status_Call{Call: _e.mock.On("Status")}
}

func (_c *MockSkyhookNodes_Status_Call) Run(run func()) *MockSkyhookNodes_Status_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_Status_Call) Return(_a0 v1alpha1.Status) *MockSkyhookNodes_Status_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_Status_Call) RunAndReturn(run func() v1alpha1.Status) *MockSkyhookNodes_Status_Call {
	_c.Call.Return(run)
	return _c
}

// UpdateCondition provides a mock function with given fields:
func (_m *MockSkyhookNodes) UpdateCondition() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for UpdateCondition")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// MockSkyhookNodes_UpdateCondition_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'UpdateCondition'
type MockSkyhookNodes_UpdateCondition_Call struct {
	*mock.Call
}

// UpdateCondition is a helper method to define mock.On call
func (_e *MockSkyhookNodes_Expecter) UpdateCondition() *MockSkyhookNodes_UpdateCondition_Call {
	return &MockSkyhookNodes_UpdateCondition_Call{Call: _e.mock.On("UpdateCondition")}
}

func (_c *MockSkyhookNodes_UpdateCondition_Call) Run(run func()) *MockSkyhookNodes_UpdateCondition_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockSkyhookNodes_UpdateCondition_Call) Return(_a0 bool) *MockSkyhookNodes_UpdateCondition_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockSkyhookNodes_UpdateCondition_Call) RunAndReturn(run func() bool) *MockSkyhookNodes_UpdateCondition_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockSkyhookNodes creates a new instance of MockSkyhookNodes. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockSkyhookNodes(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockSkyhookNodes {
	mock := &MockSkyhookNodes{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
