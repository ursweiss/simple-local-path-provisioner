package driver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestControllerServer(t *testing.T) *controllerServer {
	t.Helper()
	return &controllerServer{
		cfg: &Config{
			BasePath:     t.TempDir(),
			StaleTimeout: time.Minute,
		},
		store: &MetaStore{},
	}
}

func validCreateVolumeRequest(name string) *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{
		Name: name,
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
		Parameters: map[string]string{
			"csi.storage.k8s.io/pvc/namespace": "default",
			"csi.storage.k8s.io/pvc/name":      "test-pvc",
		},
	}
}

func TestCreateVolume_Validation(t *testing.T) {
	s := newTestControllerServer(t)

	tests := []struct {
		name     string
		req      *csi.CreateVolumeRequest
		wantCode codes.Code
	}{
		{
			name:     "missing name",
			req:      &csi.CreateVolumeRequest{},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "missing capabilities",
			req: &csi.CreateVolumeRequest{
				Name: "vol-1",
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "missing PVC params",
			req: &csi.CreateVolumeRequest{
				Name: "vol-1",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				},
				Parameters: map[string]string{},
			},
			wantCode: codes.InvalidArgument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateVolume(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected error")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", err)
			}
			if st.Code() != tt.wantCode {
				t.Errorf("code = %v, want %v", st.Code(), tt.wantCode)
			}
		})
	}
}

func TestCreateVolume_Success(t *testing.T) {
	s := newTestControllerServer(t)
	req := validCreateVolumeRequest("vol-1")

	resp, err := s.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}

	wantID := "default/test-pvc"
	if resp.Volume.VolumeId != wantID {
		t.Errorf("VolumeId = %q, want %q", resp.Volume.VolumeId, wantID)
	}

	backingPath := filepath.Join(s.cfg.BasePath, "default-test-pvc")
	info, err := os.Stat(backingPath)
	if err != nil {
		t.Fatalf("backing dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("backing path %q is not a directory", backingPath)
	}
}

func TestCreateVolume_Idempotent(t *testing.T) {
	s := newTestControllerServer(t)
	req := validCreateVolumeRequest("vol-1")

	resp1, err := s.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatalf("first CreateVolume failed: %v", err)
	}

	resp2, err := s.CreateVolume(context.Background(), req)
	if err != nil {
		t.Fatalf("second CreateVolume failed: %v", err)
	}

	if resp1.Volume.VolumeId != resp2.Volume.VolumeId {
		t.Errorf("idempotent VolumeId mismatch: %q != %q",
			resp1.Volume.VolumeId, resp2.Volume.VolumeId)
	}
}

func TestDeleteVolume_Validation(t *testing.T) {
	s := newTestControllerServer(t)

	_, err := s.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{})
	if err == nil {
		t.Fatal("expected error for empty volume ID")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want %v", st.Code(), codes.InvalidArgument)
	}
}

func TestDeleteVolume_UnrecognisedHandle(t *testing.T) {
	s := newTestControllerServer(t)

	resp, err := s.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: "malformed-no-slash",
	})
	if err != nil {
		t.Fatalf("expected graceful skip, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestDeleteVolume_Success(t *testing.T) {
	s := newTestControllerServer(t)

	// Create backing dir first.
	backingPath := filepath.Join(s.cfg.BasePath, "default-test-pvc")
	if err := os.MkdirAll(backingPath, 0750); err != nil {
		t.Fatalf("setup: %v", err)
	}

	resp, err := s.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: "default/test-pvc",
	})
	if err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	// Retain policy: directory should still exist.
	if _, err := os.Stat(backingPath); os.IsNotExist(err) {
		t.Error("backing directory was removed, expected Retain")
	}
}

func TestValidateCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		caps    []*csi.VolumeCapability
		wantErr bool
	}{
		{
			name: "SINGLE_NODE_WRITER allowed",
			caps: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "SINGLE_NODE_READER_ONLY allowed",
			caps: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "MULTI_NODE_MULTI_WRITER rejected",
			caps: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "block volume rejected",
			caps: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCapabilities(tt.caps)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCapabilities() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	s := newTestControllerServer(t)

	resp, err := s.ControllerGetCapabilities(context.Background(),
		&csi.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("ControllerGetCapabilities failed: %v", err)
	}

	want := map[csi.ControllerServiceCapability_RPC_Type]bool{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME:     false,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME: false,
	}

	for _, cap := range resp.Capabilities {
		rpc := cap.GetRpc()
		if rpc != nil {
			if _, ok := want[rpc.Type]; ok {
				want[rpc.Type] = true
			}
		}
	}

	for capType, found := range want {
		if !found {
			t.Errorf("capability %v not found in response", capType)
		}
	}
}
