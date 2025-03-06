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






// Code generated by mockery v2.42.3. DO NOT EDIT.

package workqueue

import (
	time "time"

	mock "github.com/stretchr/testify/mock"
)

// MockRateLimitingInterface is an autogenerated mock type for the RateLimitingInterface type
type MockRateLimitingInterface struct {
	mock.Mock
}

type MockRateLimitingInterface_Expecter struct {
	mock *mock.Mock
}

func (_m *MockRateLimitingInterface) EXPECT() *MockRateLimitingInterface_Expecter {
	return &MockRateLimitingInterface_Expecter{mock: &_m.Mock}
}

// Add provides a mock function with given fields: item
func (_m *MockRateLimitingInterface) Add(item interface{}) {
	_m.Called(item)
}

// MockRateLimitingInterface_Add_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Add'
type MockRateLimitingInterface_Add_Call struct {
	*mock.Call
}

// Add is a helper method to define mock.On call
//   - item interface{}
func (_e *MockRateLimitingInterface_Expecter) Add(item interface{}) *MockRateLimitingInterface_Add_Call {
	return &MockRateLimitingInterface_Add_Call{Call: _e.mock.On("Add", item)}
}

func (_c *MockRateLimitingInterface_Add_Call) Run(run func(item interface{})) *MockRateLimitingInterface_Add_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(interface{}))
	})
	return _c
}

func (_c *MockRateLimitingInterface_Add_Call) Return() *MockRateLimitingInterface_Add_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockRateLimitingInterface_Add_Call) RunAndReturn(run func(interface{})) *MockRateLimitingInterface_Add_Call {
	_c.Call.Return(run)
	return _c
}

// AddAfter provides a mock function with given fields: item, duration
func (_m *MockRateLimitingInterface) AddAfter(item interface{}, duration time.Duration) {
	_m.Called(item, duration)
}

// MockRateLimitingInterface_AddAfter_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AddAfter'
type MockRateLimitingInterface_AddAfter_Call struct {
	*mock.Call
}

// AddAfter is a helper method to define mock.On call
//   - item interface{}
//   - duration time.Duration
func (_e *MockRateLimitingInterface_Expecter) AddAfter(item interface{}, duration interface{}) *MockRateLimitingInterface_AddAfter_Call {
	return &MockRateLimitingInterface_AddAfter_Call{Call: _e.mock.On("AddAfter", item, duration)}
}

func (_c *MockRateLimitingInterface_AddAfter_Call) Run(run func(item interface{}, duration time.Duration)) *MockRateLimitingInterface_AddAfter_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(interface{}), args[1].(time.Duration))
	})
	return _c
}

func (_c *MockRateLimitingInterface_AddAfter_Call) Return() *MockRateLimitingInterface_AddAfter_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockRateLimitingInterface_AddAfter_Call) RunAndReturn(run func(interface{}, time.Duration)) *MockRateLimitingInterface_AddAfter_Call {
	_c.Call.Return(run)
	return _c
}

// AddRateLimited provides a mock function with given fields: item
func (_m *MockRateLimitingInterface) AddRateLimited(item interface{}) {
	_m.Called(item)
}

// MockRateLimitingInterface_AddRateLimited_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'AddRateLimited'
type MockRateLimitingInterface_AddRateLimited_Call struct {
	*mock.Call
}

// AddRateLimited is a helper method to define mock.On call
//   - item interface{}
func (_e *MockRateLimitingInterface_Expecter) AddRateLimited(item interface{}) *MockRateLimitingInterface_AddRateLimited_Call {
	return &MockRateLimitingInterface_AddRateLimited_Call{Call: _e.mock.On("AddRateLimited", item)}
}

func (_c *MockRateLimitingInterface_AddRateLimited_Call) Run(run func(item interface{})) *MockRateLimitingInterface_AddRateLimited_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(interface{}))
	})
	return _c
}

func (_c *MockRateLimitingInterface_AddRateLimited_Call) Return() *MockRateLimitingInterface_AddRateLimited_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockRateLimitingInterface_AddRateLimited_Call) RunAndReturn(run func(interface{})) *MockRateLimitingInterface_AddRateLimited_Call {
	_c.Call.Return(run)
	return _c
}

// Done provides a mock function with given fields: item
func (_m *MockRateLimitingInterface) Done(item interface{}) {
	_m.Called(item)
}

// MockRateLimitingInterface_Done_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Done'
type MockRateLimitingInterface_Done_Call struct {
	*mock.Call
}

// Done is a helper method to define mock.On call
//   - item interface{}
func (_e *MockRateLimitingInterface_Expecter) Done(item interface{}) *MockRateLimitingInterface_Done_Call {
	return &MockRateLimitingInterface_Done_Call{Call: _e.mock.On("Done", item)}
}

func (_c *MockRateLimitingInterface_Done_Call) Run(run func(item interface{})) *MockRateLimitingInterface_Done_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(interface{}))
	})
	return _c
}

func (_c *MockRateLimitingInterface_Done_Call) Return() *MockRateLimitingInterface_Done_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockRateLimitingInterface_Done_Call) RunAndReturn(run func(interface{})) *MockRateLimitingInterface_Done_Call {
	_c.Call.Return(run)
	return _c
}

// Forget provides a mock function with given fields: item
func (_m *MockRateLimitingInterface) Forget(item interface{}) {
	_m.Called(item)
}

// MockRateLimitingInterface_Forget_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Forget'
type MockRateLimitingInterface_Forget_Call struct {
	*mock.Call
}

// Forget is a helper method to define mock.On call
//   - item interface{}
func (_e *MockRateLimitingInterface_Expecter) Forget(item interface{}) *MockRateLimitingInterface_Forget_Call {
	return &MockRateLimitingInterface_Forget_Call{Call: _e.mock.On("Forget", item)}
}

func (_c *MockRateLimitingInterface_Forget_Call) Run(run func(item interface{})) *MockRateLimitingInterface_Forget_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(interface{}))
	})
	return _c
}

func (_c *MockRateLimitingInterface_Forget_Call) Return() *MockRateLimitingInterface_Forget_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockRateLimitingInterface_Forget_Call) RunAndReturn(run func(interface{})) *MockRateLimitingInterface_Forget_Call {
	_c.Call.Return(run)
	return _c
}

// Get provides a mock function with given fields:
func (_m *MockRateLimitingInterface) Get() (interface{}, bool) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 interface{}
	var r1 bool
	if rf, ok := ret.Get(0).(func() (interface{}, bool)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() interface{}); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(interface{})
		}
	}

	if rf, ok := ret.Get(1).(func() bool); ok {
		r1 = rf()
	} else {
		r1 = ret.Get(1).(bool)
	}

	return r0, r1
}

// MockRateLimitingInterface_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type MockRateLimitingInterface_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
func (_e *MockRateLimitingInterface_Expecter) Get() *MockRateLimitingInterface_Get_Call {
	return &MockRateLimitingInterface_Get_Call{Call: _e.mock.On("Get")}
}

func (_c *MockRateLimitingInterface_Get_Call) Run(run func()) *MockRateLimitingInterface_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockRateLimitingInterface_Get_Call) Return(item interface{}, shutdown bool) *MockRateLimitingInterface_Get_Call {
	_c.Call.Return(item, shutdown)
	return _c
}

func (_c *MockRateLimitingInterface_Get_Call) RunAndReturn(run func() (interface{}, bool)) *MockRateLimitingInterface_Get_Call {
	_c.Call.Return(run)
	return _c
}

// Len provides a mock function with given fields:
func (_m *MockRateLimitingInterface) Len() int {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Len")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// MockRateLimitingInterface_Len_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Len'
type MockRateLimitingInterface_Len_Call struct {
	*mock.Call
}

// Len is a helper method to define mock.On call
func (_e *MockRateLimitingInterface_Expecter) Len() *MockRateLimitingInterface_Len_Call {
	return &MockRateLimitingInterface_Len_Call{Call: _e.mock.On("Len")}
}

func (_c *MockRateLimitingInterface_Len_Call) Run(run func()) *MockRateLimitingInterface_Len_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockRateLimitingInterface_Len_Call) Return(_a0 int) *MockRateLimitingInterface_Len_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockRateLimitingInterface_Len_Call) RunAndReturn(run func() int) *MockRateLimitingInterface_Len_Call {
	_c.Call.Return(run)
	return _c
}

// NumRequeues provides a mock function with given fields: item
func (_m *MockRateLimitingInterface) NumRequeues(item interface{}) int {
	ret := _m.Called(item)

	if len(ret) == 0 {
		panic("no return value specified for NumRequeues")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func(interface{}) int); ok {
		r0 = rf(item)
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// MockRateLimitingInterface_NumRequeues_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'NumRequeues'
type MockRateLimitingInterface_NumRequeues_Call struct {
	*mock.Call
}

// NumRequeues is a helper method to define mock.On call
//   - item interface{}
func (_e *MockRateLimitingInterface_Expecter) NumRequeues(item interface{}) *MockRateLimitingInterface_NumRequeues_Call {
	return &MockRateLimitingInterface_NumRequeues_Call{Call: _e.mock.On("NumRequeues", item)}
}

func (_c *MockRateLimitingInterface_NumRequeues_Call) Run(run func(item interface{})) *MockRateLimitingInterface_NumRequeues_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(interface{}))
	})
	return _c
}

func (_c *MockRateLimitingInterface_NumRequeues_Call) Return(_a0 int) *MockRateLimitingInterface_NumRequeues_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockRateLimitingInterface_NumRequeues_Call) RunAndReturn(run func(interface{}) int) *MockRateLimitingInterface_NumRequeues_Call {
	_c.Call.Return(run)
	return _c
}

// ShutDown provides a mock function with given fields:
func (_m *MockRateLimitingInterface) ShutDown() {
	_m.Called()
}

// MockRateLimitingInterface_ShutDown_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ShutDown'
type MockRateLimitingInterface_ShutDown_Call struct {
	*mock.Call
}

// ShutDown is a helper method to define mock.On call
func (_e *MockRateLimitingInterface_Expecter) ShutDown() *MockRateLimitingInterface_ShutDown_Call {
	return &MockRateLimitingInterface_ShutDown_Call{Call: _e.mock.On("ShutDown")}
}

func (_c *MockRateLimitingInterface_ShutDown_Call) Run(run func()) *MockRateLimitingInterface_ShutDown_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockRateLimitingInterface_ShutDown_Call) Return() *MockRateLimitingInterface_ShutDown_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockRateLimitingInterface_ShutDown_Call) RunAndReturn(run func()) *MockRateLimitingInterface_ShutDown_Call {
	_c.Call.Return(run)
	return _c
}

// ShutDownWithDrain provides a mock function with given fields:
func (_m *MockRateLimitingInterface) ShutDownWithDrain() {
	_m.Called()
}

// MockRateLimitingInterface_ShutDownWithDrain_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ShutDownWithDrain'
type MockRateLimitingInterface_ShutDownWithDrain_Call struct {
	*mock.Call
}

// ShutDownWithDrain is a helper method to define mock.On call
func (_e *MockRateLimitingInterface_Expecter) ShutDownWithDrain() *MockRateLimitingInterface_ShutDownWithDrain_Call {
	return &MockRateLimitingInterface_ShutDownWithDrain_Call{Call: _e.mock.On("ShutDownWithDrain")}
}

func (_c *MockRateLimitingInterface_ShutDownWithDrain_Call) Run(run func()) *MockRateLimitingInterface_ShutDownWithDrain_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockRateLimitingInterface_ShutDownWithDrain_Call) Return() *MockRateLimitingInterface_ShutDownWithDrain_Call {
	_c.Call.Return()
	return _c
}

func (_c *MockRateLimitingInterface_ShutDownWithDrain_Call) RunAndReturn(run func()) *MockRateLimitingInterface_ShutDownWithDrain_Call {
	_c.Call.Return(run)
	return _c
}

// ShuttingDown provides a mock function with given fields:
func (_m *MockRateLimitingInterface) ShuttingDown() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ShuttingDown")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// MockRateLimitingInterface_ShuttingDown_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ShuttingDown'
type MockRateLimitingInterface_ShuttingDown_Call struct {
	*mock.Call
}

// ShuttingDown is a helper method to define mock.On call
func (_e *MockRateLimitingInterface_Expecter) ShuttingDown() *MockRateLimitingInterface_ShuttingDown_Call {
	return &MockRateLimitingInterface_ShuttingDown_Call{Call: _e.mock.On("ShuttingDown")}
}

func (_c *MockRateLimitingInterface_ShuttingDown_Call) Run(run func()) *MockRateLimitingInterface_ShuttingDown_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockRateLimitingInterface_ShuttingDown_Call) Return(_a0 bool) *MockRateLimitingInterface_ShuttingDown_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockRateLimitingInterface_ShuttingDown_Call) RunAndReturn(run func() bool) *MockRateLimitingInterface_ShuttingDown_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockRateLimitingInterface creates a new instance of MockRateLimitingInterface. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockRateLimitingInterface(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockRateLimitingInterface {
	mock := &MockRateLimitingInterface{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
