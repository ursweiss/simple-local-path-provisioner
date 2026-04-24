package driver

import (
	"context"
	"os"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type controllerServer struct {
	csi.UnimplementedControllerServer
	cfg   *Config
	store *MetaStore
}

func (s *controllerServer) CreateVolume(
	_ context.Context,
	req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name is required")
	}
	if len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}
	if err := validateCapabilities(req.VolumeCapabilities); err != nil {
		return nil, err
	}

	namespace := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	pvcName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	if namespace == "" || pvcName == "" {
		return nil, status.Error(codes.InvalidArgument,
			"PVC namespace and name are required; ensure external-provisioner "+
				"is running with --extra-create-metadata=true")
	}

	handle := VolumeHandleFromIdentity(namespace, pvcName)
	backingPath, err := DeriveBackingPath(s.cfg.BasePath, namespace, pvcName)
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("CreateVolume: identity=%s backingPath=%s", handle, backingPath)

	if err := os.MkdirAll(backingPath, 0755); err != nil {
		return nil, status.Errorf(codes.Internal,
			"create backing directory %q: %v", backingPath, err)
	}

	err = s.store.Update(handle, backingPath, func(meta *VolumeMetadata) error {
		if meta.CreatedAt.IsZero() {
			klog.V(2).Infof("CreateVolume: new directory created: %s", backingPath)
			meta.CreatedAt = time.Now()
		} else {
			klog.V(2).Infof("CreateVolume: reusing existing directory: %s (created %s)",
				backingPath, meta.CreatedAt.Format(time.RFC3339))
		}
		return nil
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update volume metadata: %v", err)
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      handle,
			CapacityBytes: 0,
		},
	}, nil
}

func (s *controllerServer) DeleteVolume(
	_ context.Context,
	req *csi.DeleteVolumeRequest,
) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}

	namespace, pvcName, err := IdentityFromVolumeHandle(req.VolumeId)
	if err != nil {
		// Unrecognised handle format — likely from another provisioner; skip safely.
		klog.Warningf("DeleteVolume: unrecognised volume handle %q, skipping: %v",
			req.VolumeId, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	backingPath, err := DeriveBackingPath(s.cfg.BasePath, namespace, pvcName)
	if err != nil {
		return nil, err
	}

	klog.V(2).Infof("DeleteVolume: retaining backing directory %s (reclaim=Retain)",
		backingPath)
	return &csi.DeleteVolumeResponse{}, nil
}

func (s *controllerServer) ControllerPublishVolume(
	_ context.Context,
	req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node ID is required")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability is required")
	}

	namespace, pvcName, err := IdentityFromVolumeHandle(req.VolumeId)
	if err != nil {
		return nil, err
	}

	backingPath, err := DeriveBackingPath(s.cfg.BasePath, namespace, pvcName)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(backingPath); os.IsNotExist(err) {
		return nil, status.Errorf(codes.NotFound,
			"backing directory %q does not exist for volume %q", backingPath, req.VolumeId)
	}

	if err := s.store.Update(req.VolumeId, backingPath, func(meta *VolumeMetadata) error {
		if meta.PublishedNode == "" {
			klog.V(2).Infof("ControllerPublishVolume: vol=%s node=%s (new owner)",
				req.VolumeId, req.NodeId)
			meta.PublishedNode = req.NodeId
			meta.PublishedAt = time.Now()
			return nil
		}

		if meta.PublishedNode == req.NodeId {
			klog.V(2).Infof("ControllerPublishVolume: vol=%s node=%s (idempotent)",
				req.VolumeId, req.NodeId)
			return nil
		}

		age := time.Since(meta.PublishedAt)
		if s.cfg.StaleTimeout > 0 && age > s.cfg.StaleTimeout {
			klog.Warningf("ControllerPublishVolume: vol=%s stale owner %s (age=%v, timeout=%v), "+
				"takeover by node=%s",
				req.VolumeId, meta.PublishedNode, age.Round(time.Second),
				s.cfg.StaleTimeout, req.NodeId)
			meta.PublishedNode = req.NodeId
			meta.PublishedAt = time.Now()
			return nil
		}

		if s.cfg.AllowForceTakeover {
			klog.Warningf("ControllerPublishVolume: vol=%s force takeover from %s to %s",
				req.VolumeId, meta.PublishedNode, req.NodeId)
			meta.PublishedNode = req.NodeId
			meta.PublishedAt = time.Now()
			return nil
		}

		return status.Errorf(codes.FailedPrecondition,
			"volume %s is already published on node %s (published %v ago); "+
				"stale timeout is %v, force takeover is disabled",
			req.VolumeId, meta.PublishedNode, age.Round(time.Second),
			s.cfg.StaleTimeout)
	}); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, status.Errorf(codes.Internal, "update publish metadata: %v", err)
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			"backingPath": backingPath,
		},
	}, nil
}

func (s *controllerServer) ControllerUnpublishVolume(
	_ context.Context,
	req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}

	namespace, pvcName, err := IdentityFromVolumeHandle(req.VolumeId)
	if err != nil {
		klog.Warningf("ControllerUnpublishVolume: unrecognised volume handle %q, skipping: %v",
			req.VolumeId, err)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	backingPath, err := DeriveBackingPath(s.cfg.BasePath, namespace, pvcName)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(backingPath); os.IsNotExist(err) {
		klog.V(2).Infof("ControllerUnpublishVolume: backing dir %s not found, nothing to do",
			backingPath)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	if err := s.store.Update(req.VolumeId, backingPath, func(meta *VolumeMetadata) error {
		if meta.PublishedNode == "" {
			klog.V(2).Infof("ControllerUnpublishVolume: vol=%s not published (idempotent)",
				req.VolumeId)
			return nil
		}
		if req.NodeId != "" && meta.PublishedNode != req.NodeId {
			klog.Warningf("ControllerUnpublishVolume: vol=%s owned by %s, clearing on behalf of %s",
				req.VolumeId, meta.PublishedNode, req.NodeId)
		} else {
			klog.V(2).Infof("ControllerUnpublishVolume: vol=%s clearing owner %s",
				req.VolumeId, meta.PublishedNode)
		}
		meta.PublishedNode = ""
		meta.PublishedAt = time.Time{}
		return nil
	}); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		return nil, status.Errorf(codes.Internal, "update unpublish metadata: %v", err)
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (s *controllerServer) ValidateVolumeCapabilities(
	_ context.Context,
	req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}

	namespace, pvcName, err := IdentityFromVolumeHandle(req.VolumeId)
	if err != nil {
		return nil, err
	}

	backingPath, err := DeriveBackingPath(s.cfg.BasePath, namespace, pvcName)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(backingPath); os.IsNotExist(err) {
		return nil, status.Errorf(codes.NotFound, "volume %q not found", req.VolumeId)
	}

	if err := validateCapabilities(req.VolumeCapabilities); err != nil {
		return &csi.ValidateVolumeCapabilitiesResponse{}, nil
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.VolumeCapabilities,
		},
	}, nil
}

func (s *controllerServer) ControllerGetCapabilities(
	_ context.Context,
	_ *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {
	caps := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	}
	out := make([]*csi.ControllerServiceCapability, len(caps))
	for i, c := range caps {
		out[i] = &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{Type: c},
			},
		}
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: out}, nil
}

// validateCapabilities returns an error if any capability is unsupported.
func validateCapabilities(caps []*csi.VolumeCapability) error {
	for _, cap := range caps {
		if cap.GetBlock() != nil {
			return status.Error(codes.InvalidArgument, "block volume mode is not supported")
		}
		am := cap.GetAccessMode()
		if am == nil {
			continue
		}
		switch am.Mode {
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY:
			// supported
		default:
			return status.Errorf(codes.InvalidArgument,
				"access mode %v is not supported; only SINGLE_NODE_WRITER and "+
					"SINGLE_NODE_READER_ONLY are supported", am.Mode)
		}
	}
	return nil
}

