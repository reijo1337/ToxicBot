// Code generated by MockGen. DO NOT EDIT.
// Source: contract.go
//
// Generated by this command:
//
//	mockgen -source contract.go -destination mocks_test.go -package on_voice
//

// Package on_voice is a generated GoMock package.
package on_voice

import (
	context "context"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockvoicesRepository is a mock of voicesRepository interface.
type MockvoicesRepository struct {
	ctrl     *gomock.Controller
	recorder *MockvoicesRepositoryMockRecorder
}

// MockvoicesRepositoryMockRecorder is the mock recorder for MockvoicesRepository.
type MockvoicesRepositoryMockRecorder struct {
	mock *MockvoicesRepository
}

// NewMockvoicesRepository creates a new mock instance.
func NewMockvoicesRepository(ctrl *gomock.Controller) *MockvoicesRepository {
	mock := &MockvoicesRepository{ctrl: ctrl}
	mock.recorder = &MockvoicesRepositoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockvoicesRepository) EXPECT() *MockvoicesRepositoryMockRecorder {
	return m.recorder
}

// GetEnabledVoices mocks base method.
func (m *MockvoicesRepository) GetEnabledVoices() ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetEnabledVoices")
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetEnabledVoices indicates an expected call of GetEnabledVoices.
func (mr *MockvoicesRepositoryMockRecorder) GetEnabledVoices() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetEnabledVoices", reflect.TypeOf((*MockvoicesRepository)(nil).GetEnabledVoices))
}

// Mocklogger is a mock of logger interface.
type Mocklogger struct {
	ctrl     *gomock.Controller
	recorder *MockloggerMockRecorder
}

// MockloggerMockRecorder is the mock recorder for Mocklogger.
type MockloggerMockRecorder struct {
	mock *Mocklogger
}

// NewMocklogger creates a new mock instance.
func NewMocklogger(ctrl *gomock.Controller) *Mocklogger {
	mock := &Mocklogger{ctrl: ctrl}
	mock.recorder = &MockloggerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *Mocklogger) EXPECT() *MockloggerMockRecorder {
	return m.recorder
}

// Error mocks base method.
func (m *Mocklogger) Error(arg0 context.Context, arg1 string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Error", arg0, arg1)
}

// Error indicates an expected call of Error.
func (mr *MockloggerMockRecorder) Error(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Error", reflect.TypeOf((*Mocklogger)(nil).Error), arg0, arg1)
}

// Warn mocks base method.
func (m *Mocklogger) Warn(arg0 context.Context, arg1 string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Warn", arg0, arg1)
}

// Warn indicates an expected call of Warn.
func (mr *MockloggerMockRecorder) Warn(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Warn", reflect.TypeOf((*Mocklogger)(nil).Warn), arg0, arg1)
}

// WithError mocks base method.
func (m *Mocklogger) WithError(arg0 context.Context, arg1 error) context.Context {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WithError", arg0, arg1)
	ret0, _ := ret[0].(context.Context)
	return ret0
}

// WithError indicates an expected call of WithError.
func (mr *MockloggerMockRecorder) WithError(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WithError", reflect.TypeOf((*Mocklogger)(nil).WithError), arg0, arg1)
}

// WithField mocks base method.
func (m *Mocklogger) WithField(arg0 context.Context, arg1 string, arg2 any) context.Context {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WithField", arg0, arg1, arg2)
	ret0, _ := ret[0].(context.Context)
	return ret0
}

// WithField indicates an expected call of WithField.
func (mr *MockloggerMockRecorder) WithField(arg0, arg1, arg2 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WithField", reflect.TypeOf((*Mocklogger)(nil).WithField), arg0, arg1, arg2)
}

// Mockrandomizer is a mock of randomizer interface.
type Mockrandomizer struct {
	ctrl     *gomock.Controller
	recorder *MockrandomizerMockRecorder
}

// MockrandomizerMockRecorder is the mock recorder for Mockrandomizer.
type MockrandomizerMockRecorder struct {
	mock *Mockrandomizer
}

// NewMockrandomizer creates a new mock instance.
func NewMockrandomizer(ctrl *gomock.Controller) *Mockrandomizer {
	mock := &Mockrandomizer{ctrl: ctrl}
	mock.recorder = &MockrandomizerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *Mockrandomizer) EXPECT() *MockrandomizerMockRecorder {
	return m.recorder
}

// Float32 mocks base method.
func (m *Mockrandomizer) Float32() float32 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Float32")
	ret0, _ := ret[0].(float32)
	return ret0
}

// Float32 indicates an expected call of Float32.
func (mr *MockrandomizerMockRecorder) Float32() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Float32", reflect.TypeOf((*Mockrandomizer)(nil).Float32))
}

// Intn mocks base method.
func (m *Mockrandomizer) Intn(n int) int {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Intn", n)
	ret0, _ := ret[0].(int)
	return ret0
}

// Intn indicates an expected call of Intn.
func (mr *MockrandomizerMockRecorder) Intn(n any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Intn", reflect.TypeOf((*Mockrandomizer)(nil).Intn), n)
}
