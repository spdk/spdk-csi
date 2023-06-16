/*
Copyright (c) Intel Corporation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//nolint:lll,gocritic,forcetypeassert
package util

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	opiapiStorage "github.com/opiproject/opi-api/storage/v1alpha1/gen/go"
)

// MockNvmeRemoteControllerServiceClient is a mock implementation of the NvmeRemoteControllerServiceClient interface
type MockNvmeRemoteControllerServiceClient struct {
	mock.Mock
}

var _ opiapiStorage.NvmeRemoteControllerServiceClient = &MockNvmeRemoteControllerServiceClient{}

func (m *MockNvmeRemoteControllerServiceClient) CreateNvmeRemoteController(ctx context.Context, in *opiapiStorage.CreateNvmeRemoteControllerRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeRemoteController, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.NvmeRemoteController), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) DeleteNvmeRemoteController(ctx context.Context, in *opiapiStorage.DeleteNvmeRemoteControllerRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	args := m.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*emptypb.Empty), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) UpdateNvmeRemoteController(ctx context.Context, in *opiapiStorage.UpdateNvmeRemoteControllerRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeRemoteController, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.NvmeRemoteController), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) ListNvmeRemoteControllers(ctx context.Context, in *opiapiStorage.ListNvmeRemoteControllersRequest, _ ...grpc.CallOption) (*opiapiStorage.ListNvmeRemoteControllersResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.ListNvmeRemoteControllersResponse), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) GetNvmeRemoteController(ctx context.Context, in *opiapiStorage.GetNvmeRemoteControllerRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeRemoteController, error) {
	args := m.Called(ctx, in)
	return nil, args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) CreateNvmePath(ctx context.Context, in *opiapiStorage.CreateNvmePathRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmePath, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.NvmePath), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) DeleteNvmePath(ctx context.Context, in *opiapiStorage.DeleteNvmePathRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	args := m.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*emptypb.Empty), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) UpdateNvmePath(ctx context.Context, in *opiapiStorage.UpdateNvmePathRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmePath, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.NvmePath), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) ListNvmePaths(ctx context.Context, in *opiapiStorage.ListNvmePathsRequest, _ ...grpc.CallOption) (*opiapiStorage.ListNvmePathsResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.ListNvmePathsResponse), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) GetNvmePath(ctx context.Context, in *opiapiStorage.GetNvmePathRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmePath, error) {
	args := m.Called(ctx, in)
	return nil, args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) NvmePathStats(ctx context.Context, in *opiapiStorage.NvmePathStatsRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmePathStatsResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.NvmePathStatsResponse), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) ListNvmeRemoteNamespaces(ctx context.Context, in *opiapiStorage.ListNvmeRemoteNamespacesRequest, _ ...grpc.CallOption) (*opiapiStorage.ListNvmeRemoteNamespacesResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.ListNvmeRemoteNamespacesResponse), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) NvmeRemoteControllerReset(ctx context.Context, in *opiapiStorage.NvmeRemoteControllerResetRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*emptypb.Empty), args.Error(1)
}

func (m *MockNvmeRemoteControllerServiceClient) NvmeRemoteControllerStats(ctx context.Context, in *opiapiStorage.NvmeRemoteControllerStatsRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeRemoteControllerStatsResponse, error) {
	args := m.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.NvmeRemoteControllerStatsResponse), args.Error(1)
}

type MockFrontendVirtioBlkServiceClient struct {
	mock.Mock
}

func (i *MockFrontendVirtioBlkServiceClient) CreateVirtioBlk(ctx context.Context, in *opiapiStorage.CreateVirtioBlkRequest, _ ...grpc.CallOption) (*opiapiStorage.VirtioBlk, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.VirtioBlk), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendVirtioBlkServiceClient) DeleteVirtioBlk(ctx context.Context, in *opiapiStorage.DeleteVirtioBlkRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*emptypb.Empty), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendVirtioBlkServiceClient) UpdateVirtioBlk(ctx context.Context, in *opiapiStorage.UpdateVirtioBlkRequest, _ ...grpc.CallOption) (*opiapiStorage.VirtioBlk, error) {
	args := i.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.VirtioBlk), args.Error(1)
}

func (i *MockFrontendVirtioBlkServiceClient) ListVirtioBlks(ctx context.Context, in *opiapiStorage.ListVirtioBlksRequest, _ ...grpc.CallOption) (*opiapiStorage.ListVirtioBlksResponse, error) {
	args := i.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.ListVirtioBlksResponse), args.Error(1)
}

func (i *MockFrontendVirtioBlkServiceClient) GetVirtioBlk(ctx context.Context, in *opiapiStorage.GetVirtioBlkRequest, _ ...grpc.CallOption) (*opiapiStorage.VirtioBlk, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.VirtioBlk), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendVirtioBlkServiceClient) VirtioBlkStats(ctx context.Context, in *opiapiStorage.VirtioBlkStatsRequest, _ ...grpc.CallOption) (*opiapiStorage.VirtioBlkStatsResponse, error) {
	args := i.Called(ctx, in)
	return args.Get(0).(*opiapiStorage.VirtioBlkStatsResponse), args.Error(1)
}

type MockFrontendNvmeServiceClient struct {
	mock.Mock
}

func (i *MockFrontendNvmeServiceClient) CreateNvmeSubsystem(ctx context.Context, in *opiapiStorage.CreateNvmeSubsystemRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeSubsystem, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.NvmeSubsystem), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) DeleteNvmeSubsystem(ctx context.Context, in *opiapiStorage.DeleteNvmeSubsystemRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*emptypb.Empty), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) UpdateNvmeSubsystem(_ context.Context, _ *opiapiStorage.UpdateNvmeSubsystemRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeSubsystem, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) ListNvmeSubsystems(_ context.Context, _ *opiapiStorage.ListNvmeSubsystemsRequest, _ ...grpc.CallOption) (*opiapiStorage.ListNvmeSubsystemsResponse, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) GetNvmeSubsystem(ctx context.Context, in *opiapiStorage.GetNvmeSubsystemRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeSubsystem, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.NvmeSubsystem), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) NvmeSubsystemStats(_ context.Context, _ *opiapiStorage.NvmeSubsystemStatsRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeSubsystemStatsResponse, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) CreateNvmeController(ctx context.Context, in *opiapiStorage.CreateNvmeControllerRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeController, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.NvmeController), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) DeleteNvmeController(ctx context.Context, in *opiapiStorage.DeleteNvmeControllerRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*emptypb.Empty), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) UpdateNvmeController(_ context.Context, _ *opiapiStorage.UpdateNvmeControllerRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeController, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) ListNvmeControllers(_ context.Context, _ *opiapiStorage.ListNvmeControllersRequest, _ ...grpc.CallOption) (*opiapiStorage.ListNvmeControllersResponse, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) GetNvmeController(ctx context.Context, in *opiapiStorage.GetNvmeControllerRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeController, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.NvmeController), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) NvmeControllerStats(_ context.Context, _ *opiapiStorage.NvmeControllerStatsRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeControllerStatsResponse, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) CreateNvmeNamespace(ctx context.Context, in *opiapiStorage.CreateNvmeNamespaceRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeNamespace, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.NvmeNamespace), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) DeleteNvmeNamespace(ctx context.Context, in *opiapiStorage.DeleteNvmeNamespaceRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*emptypb.Empty), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) UpdateNvmeNamespace(_ context.Context, _ *opiapiStorage.UpdateNvmeNamespaceRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeNamespace, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) ListNvmeNamespaces(_ context.Context, _ *opiapiStorage.ListNvmeNamespacesRequest, _ ...grpc.CallOption) (*opiapiStorage.ListNvmeNamespacesResponse, error) {
	return nil, nil
}

func (i *MockFrontendNvmeServiceClient) GetNvmeNamespace(ctx context.Context, in *opiapiStorage.GetNvmeNamespaceRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeNamespace, error) {
	args := i.Called(ctx, in)
	if args.Get(0) != nil {
		return args.Get(0).(*opiapiStorage.NvmeNamespace), nil
	}
	return nil, args.Error(1)
}

func (i *MockFrontendNvmeServiceClient) NvmeNamespaceStats(_ context.Context, _ *opiapiStorage.NvmeNamespaceStatsRequest, _ ...grpc.CallOption) (*opiapiStorage.NvmeNamespaceStatsResponse, error) {
	return nil, nil
}

func TestCreateNvmeRemoteController(t *testing.T) {
	// Create a mock NvmeRemoteControllerServiceClient
	mockClient := new(MockNvmeRemoteControllerServiceClient)

	// Create an instance of opiCommon
	volumeContext := map[string]string{
		"targetPort": "1234",
		"targetAddr": "192.168.0.1",
		"nqn":        "nqn-value",
		"model":      "model-value",
	}
	opi := &opiCommon{
		volumeContext:              volumeContext,
		opiClient:                  nil,
		nvmfRemoteControllerClient: mockClient,
	}

	controllerID := opiObjectPrefix + volumeContext["model"]
	// Mock the CreateNvmeRemoteController function to return a response with the specified ID
	mockClient.On("CreateNvmeRemoteController", mock.Anything, mock.Anything).Return(&opiapiStorage.NvmeRemoteController{
		Name: controllerID,
	}, nil)

	// Mock the GetNvmeRemoteController function to return a nil response and an error
	mockClient.On("GetNvmeRemoteController", mock.Anything, mock.Anything).Return(nil, errors.New("Controller does not exist"))

	// Set the necessary volume context values

	// Call the function under test
	err := opi.createNvmeRemoteController(context.Background())
	// Assert that the error returned is nil
	assert.NoError(t, err, "create remote Nvme controller")

	// Assert that the GetNvmeRemoteController and CreateNvmeRemoteController functions were called with the expected arguments
	mockClient.AssertCalled(t, "CreateNvmeRemoteController", mock.Anything, &opiapiStorage.CreateNvmeRemoteControllerRequest{
		NvmeRemoteController: &opiapiStorage.NvmeRemoteController{
			Multipath: opiapiStorage.NvmeMultipath_NVME_MULTIPATH_MULTIPATH,
		},
		NvmeRemoteControllerId: controllerID,
	})
}

func TestDeleteNvmeRemoteController_Success(t *testing.T) {
	// Create a mock client
	mockClient := new(MockNvmeRemoteControllerServiceClient)

	// Create the OPI instance with the mock client
	opi := &opiCommon{
		nvmfRemoteControllerClient: mockClient,
		volumeContext: map[string]string{
			"model":      "model-value",
			"targetPort": "9009",
		},
	}

	// Set up expectations
	controllerID := opiObjectPrefix + opi.volumeContext["mode"]
	mockClient.On("CreateNvmeRemoteController", mock.Anything, mock.Anything).Return(&opiapiStorage.NvmeRemoteController{
		Name: controllerID,
	}, nil)
	deleteReq := &opiapiStorage.DeleteNvmeRemoteControllerRequest{
		Name:         controllerID,
		AllowMissing: true,
	}
	mockClient.On("DeleteNvmeRemoteController", mock.Anything, deleteReq).Return(&emptypb.Empty{}, nil)

	// Call the method under test
	err := opi.createNvmeRemoteController(context.TODO())
	assert.NoError(t, err)
	err = opi.deleteNvmeRemoteController(context.TODO())
	assert.NoError(t, err)

	// Assert that the expected methods were called
	mockClient.AssertExpectations(t)
}

func TestDeleteNvmeRemoteController_Error(t *testing.T) {
	// Create a mock client
	mockClient := new(MockNvmeRemoteControllerServiceClient)

	NvmeRemoteControllerName := "non-existing-controller"
	// Create the OPI instance with the mock client
	opi := &opiCommon{
		nvmfRemoteControllerClient: mockClient,
		volumeContext: map[string]string{
			"model": "model-value",
		},
		nvmfRemoteControllerName: "non-existing-controller",
	}

	// Set up expectations

	deleteReq := &opiapiStorage.DeleteNvmeRemoteControllerRequest{
		Name:         NvmeRemoteControllerName,
		AllowMissing: true,
	}
	expectedErr := errors.New("delete error")
	mockClient.On("DeleteNvmeRemoteController", mock.Anything, deleteReq).Return(nil, expectedErr)

	// Call the method under test
	err := opi.deleteNvmeRemoteController(context.TODO())

	// Assert that the expected methods were called
	mockClient.AssertExpectations(t)

	// Assert that the correct error was returned
	assert.ErrorContains(t, err, expectedErr.Error())
}

func TestCreateNvmePath(t *testing.T) {
	// Create a mock NvmeRemoteControllerServiceClient
	mockClient := new(MockNvmeRemoteControllerServiceClient)

	// Create an instance of opiCommon
	volumeContext := map[string]string{
		"targetPort": "1234",
		"targetAddr": "192.168.0.1",
		"nqn":        "nqn-value",
		"model":      "model-value",
	}
	opi := &opiCommon{
		volumeContext:              volumeContext,
		opiClient:                  nil,
		nvmfRemoteControllerClient: mockClient,
	}

	controllerID := opiObjectPrefix + volumeContext["model"]
	NvmePathID := opiObjectPrefix + volumeContext["model"]
	// Mock the CreateNvmeRemoteController function to return a response with the specified ID
	mockClient.On("CreateNvmeRemoteController", mock.Anything, mock.Anything).Return(&opiapiStorage.NvmeRemoteController{
		Name: controllerID,
	}, nil)

	// Mock the CreateNvmePath function to return a nil response and an error
	mockClient.On("CreateNvmePath", mock.Anything, mock.Anything).Return(&opiapiStorage.NvmePath{
		Name: NvmePathID,
	}, nil)

	// Call the function under test
	err := opi.createNvmeRemoteController(context.Background())
	// Assert that the error returned is nil
	assert.NoError(t, err, "create remote Nvme controller")

	err = opi.createNvmfPath(context.Background())
	// Assert that the error returned is nil
	assert.NoError(t, err, "create Nvme path")

	// Assert that the CreateNvmeRemoteController function was called with the expected arguments
	mockClient.AssertCalled(t, "CreateNvmeRemoteController", mock.Anything, &opiapiStorage.CreateNvmeRemoteControllerRequest{
		NvmeRemoteController: &opiapiStorage.NvmeRemoteController{
			Multipath: opiapiStorage.NvmeMultipath_NVME_MULTIPATH_MULTIPATH,
		},
		NvmeRemoteControllerId: controllerID,
	})

	mockClient.AssertCalled(t, "CreateNvmePath", mock.Anything, &opiapiStorage.CreateNvmePathRequest{
		NvmePath: &opiapiStorage.NvmePath{
			Trtype:            opiapiStorage.NvmeTransportType_NVME_TRANSPORT_TCP,
			Adrfam:            opiapiStorage.NvmeAddressFamily_NVME_ADRFAM_IPV4,
			Traddr:            "192.168.0.1",
			Trsvcid:           1234,
			Subnqn:            "nqn-value",
			Hostnqn:           "nqn.2023-04.io.spdk.csi:remote.controller:uuid:" + opi.volumeContext["model"],
			ControllerNameRef: opi.nvmfRemoteControllerName,
		},
		NvmePathId: NvmePathID,
	})
}

func TestCreateVirtioBlk_Failure(t *testing.T) {
	// Create a mock client
	mockClient := new(MockFrontendVirtioBlkServiceClient)

	// Create a mock NvmeRemoteControllerServiceClient
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	// Create the OPI instance with the mock client
	opi := &opiInitiatorVirtioBlk{
		frontendVirtioBlkClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{},
			nvmfRemoteControllerClient: mockClient2,
		},
	}

	// Mock the CreateNvmeRemoteController function to return a response with the specified ID
	mockClient.On("GetVirtioBlk", mock.Anything, mock.Anything).Return(nil, errors.New("Could not find Controller"))

	// Mock the GetNvmeRemoteController function to return a nil response and an error
	mockClient.On("CreateVirtioBlk", mock.Anything, mock.Anything).Return(nil, errors.New("Controller does not exist"))

	err := opi.createVirtioBlk(context.TODO(), 1)
	assert.NotEqual(t, err, nil)
}

func TestCreateVirtioBlk_Success(t *testing.T) {
	// Create a mock client
	mockClient := new(MockFrontendVirtioBlkServiceClient)

	// Create a mock NvmeRemoteControllerServiceClient
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	// Create the OPI instance with the mock client
	opi := &opiInitiatorVirtioBlk{
		frontendVirtioBlkClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{"model": "xxx"},
			opiClient:                  nil,
			nvmfRemoteControllerClient: mockClient2,
		},
	}

	// Mock the CreateNvmeRemoteController function to return a response with the specified ID
	mockClient.On("GetVirtioBlk", mock.Anything, mock.Anything).Return(nil, errors.New("Could not find Controller"))

	// Mock the GetNvmeRemoteController function to return a nil response and an error
	mockClient.On("CreateVirtioBlk", mock.Anything, mock.Anything).Return(
		&opiapiStorage.VirtioBlk{
			Name: opiObjectPrefix + opi.volumeContext["mode"],
		}, nil)

	err := opi.createVirtioBlk(context.TODO(), 1)
	assert.Equal(t, err, nil)
}

func TestDeleteVirtioBlk_Success(t *testing.T) {
	// Create a mock client
	mockClient := new(MockFrontendVirtioBlkServiceClient)

	// Create a mock NvmeRemoteControllerServiceClient
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	// Create the OPI instance with the mock client
	opi := &opiInitiatorVirtioBlk{
		frontendVirtioBlkClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{"model": "xxx"},
			nvmfRemoteControllerClient: mockClient2,
		},
	}

	// Mock the CreateNvmeRemoteController function to return a response with the specified ID
	mockClient.On("DeleteVirtioBlk", mock.Anything, mock.Anything).Return(nil, errors.New("Could not find Controller"))

	err := opi.deleteVirtioBlk(context.TODO())
	assert.Equal(t, err, nil)
}

func TestDeleteVirtioBlk_Failure(t *testing.T) {
	// Create a mock client
	mockClient := new(MockFrontendVirtioBlkServiceClient)

	// Create a mock NvmeRemoteControllerServiceClient
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	// Create the OPI instance with the mock client
	opi := &opiInitiatorVirtioBlk{
		frontendVirtioBlkClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{"model": "xxx"},
			nvmfRemoteControllerClient: mockClient2,
		},
	}

	// Mock the CreateNvmeRemoteController function to return a response with the specified ID
	mockClient.On("DeleteVirtioBlk", mock.Anything, mock.Anything).Return(nil, errors.New("failed to delete device"))

	err := opi.deleteVirtioBlk(context.TODO())
	assert.Equal(t, err, nil)
}

func TestCreateNvmeSubsystem(t *testing.T) {
	mockClient := new(MockFrontendNvmeServiceClient)
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	opi := &opiInitiatorNvme{
		frontendNvmeClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{"model": "xxx"},
			nvmfRemoteControllerClient: mockClient2,
		},
	}
	mockClient.On("GetNvmeSubsystem", mock.Anything, mock.Anything).Return(nil,
		status.Error(codes.NotFound, "subsystem not found"))
	mockClient.On("CreateNvmeSubsystem", mock.Anything, mock.Anything).Return(
		&opiapiStorage.NvmeSubsystem{
			Name: opiObjectPrefix + opi.volumeContext["model"],
		}, nil)

	err := opi.createNvmeSubsystem(context.TODO())
	assert.Equal(t, err, nil)
}

func TestDeleteNvmeSubsystem(t *testing.T) {
	mockClient := new(MockFrontendNvmeServiceClient)
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	opi := &opiInitiatorNvme{
		frontendNvmeClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{},
			nvmfRemoteControllerClient: mockClient2,
		},
		subsystemName: "some-name",
	}
	mockClient.On("DeleteNvmeSubsystem", mock.Anything, mock.Anything).Return(nil, errors.New("unable to find key"))
	err := opi.deleteNvmeSubsystem(context.TODO())
	assert.NotEqual(t, err, nil)
}

func TestCreateNvmeController(t *testing.T) {
	mockClient := new(MockFrontendNvmeServiceClient)
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	opi := &opiInitiatorNvme{
		frontendNvmeClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{"model": "xxx"},
			nvmfRemoteControllerClient: mockClient2,
		},
	}
	mockClient.On("GetNvmeController", mock.Anything, mock.Anything).Return(nil, errors.New("unable to find key"))
	mockClient.On("CreateNvmeController", mock.Anything, mock.Anything).Return(
		&opiapiStorage.NvmeController{
			Name: opiObjectPrefix + opi.volumeContext["model"],
		}, nil)
	err := opi.createNvmeController(context.TODO(), 1)
	assert.Equal(t, err, nil)
}

func TestDeleteNvmeController(t *testing.T) {
	mockClient := new(MockFrontendNvmeServiceClient)
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	opi := &opiInitiatorNvme{
		frontendNvmeClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{},
			nvmfRemoteControllerClient: mockClient2,
		},
		nvmeControllerName: "controller-foo",
	}
	mockClient.On("DeleteNvmeController", mock.Anything, mock.Anything).Return(nil, errors.New("unable to find key"))
	err := opi.deleteNvmeController(context.TODO())
	assert.NotEqual(t, err, nil)
}

func TestCreateNvmeNamespace(t *testing.T) {
	mockClient := new(MockFrontendNvmeServiceClient)
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	opi := &opiInitiatorNvme{
		frontendNvmeClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{},
			nvmfRemoteControllerClient: mockClient2,
		},
	}
	mockClient.On("CreateNvmeNamespace", mock.Anything, mock.Anything).Return(nil, errors.New("unable to find key"))
	err := opi.createNvmeNamespace(context.TODO())
	assert.NotEqual(t, err, nil)
}

func TestDeleteNvmeNamespace(t *testing.T) {
	mockClient := new(MockFrontendNvmeServiceClient)
	mockClient2 := new(MockNvmeRemoteControllerServiceClient)
	opi := &opiInitiatorNvme{
		frontendNvmeClient: mockClient,
		// Create an instance of opiCommon
		opiCommon: &opiCommon{
			volumeContext:              map[string]string{},
			nvmfRemoteControllerClient: mockClient2,
		},
		namespaceName: "namespace-foo",
	}
	mockClient.On("DeleteNvmeNamespace", mock.Anything, mock.Anything).Return(nil, errors.New("unable to find key"))
	err := opi.deleteNvmeNamespace(context.TODO())
	assert.NotEqual(t, err, nil)
}
