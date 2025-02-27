// Code generated by MockGen. DO NOT EDIT.
// Source: maker.go

// Package job is a generated GoMock package.
package job

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	api "github.com/kubernetes-sigs/kernel-module-management/internal/api"
	v1 "k8s.io/api/batch/v1"
	v10 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockMaker is a mock of Maker interface.
type MockMaker struct {
	ctrl     *gomock.Controller
	recorder *MockMakerMockRecorder
}

// MockMakerMockRecorder is the mock recorder for MockMaker.
type MockMakerMockRecorder struct {
	mock *MockMaker
}

// NewMockMaker creates a new mock instance.
func NewMockMaker(ctrl *gomock.Controller) *MockMaker {
	mock := &MockMaker{ctrl: ctrl}
	mock.recorder = &MockMakerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMaker) EXPECT() *MockMakerMockRecorder {
	return m.recorder
}

// MakeJobTemplate mocks base method.
func (m *MockMaker) MakeJobTemplate(ctx context.Context, mld *api.ModuleLoaderData, owner v10.Object, pushImage bool) (*v1.Job, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MakeJobTemplate", ctx, mld, owner, pushImage)
	ret0, _ := ret[0].(*v1.Job)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// MakeJobTemplate indicates an expected call of MakeJobTemplate.
func (mr *MockMakerMockRecorder) MakeJobTemplate(ctx, mld, owner, pushImage interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MakeJobTemplate", reflect.TypeOf((*MockMaker)(nil).MakeJobTemplate), ctx, mld, owner, pushImage)
}
