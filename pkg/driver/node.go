package driver

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type nodeServer struct {
	csi.UnimplementedNodeServer
	cfg *Config
}

func (s *nodeServer) NodePublishVolume(
	_ context.Context,
	req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability is required")
	}

	backingPath := req.PublishContext["backingPath"]
	if backingPath == "" {
		// Fall back to deriving from volume handle (e.g. when attachRequired=false).
		namespace, pvcName, err := IdentityFromVolumeHandle(req.VolumeId)
		if err != nil {
			return nil, err
		}
		backingPath, err = DeriveBackingPath(s.cfg.BasePath, namespace, pvcName)
		if err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(backingPath); os.IsNotExist(err) {
		klog.Warningf("NodePublishVolume: vol=%s backing directory %s not found, creating it",
			req.VolumeId, backingPath)
		if err := os.MkdirAll(backingPath, 0750); err != nil {
			return nil, status.Errorf(codes.Internal,
				"create backing directory %q: %v", backingPath, err)
		}
	}

	target := filepath.Clean(req.TargetPath)

	mounted, err := isMountPoint(target)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal,
			"check mount at %q: %v", target, err)
	}
	if mounted {
		klog.V(2).Infof("NodePublishVolume: vol=%s already mounted at %s (idempotent)",
			req.VolumeId, target)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	if err := os.MkdirAll(target, 0750); err != nil {
		return nil, status.Errorf(codes.Internal,
			"create target path %q: %v", target, err)
	}

	flags := uintptr(unix.MS_BIND)
	klog.V(2).Infof("NodePublishVolume: vol=%s bind-mount %s -> %s (readonly=%v)",
		req.VolumeId, backingPath, target, req.Readonly)

	if err := unix.Mount(backingPath, target, "", flags, ""); err != nil {
		return nil, status.Errorf(codes.Internal,
			"bind mount %q -> %q: %v", backingPath, target, err)
	}

	if req.Readonly {
		roFlags := uintptr(unix.MS_BIND | unix.MS_REMOUNT | unix.MS_RDONLY)
		if err := unix.Mount("", target, "", roFlags, ""); err != nil {
			_ = unix.Unmount(target, unix.MNT_DETACH)
			return nil, status.Errorf(codes.Internal,
				"remount read-only at %q: %v", target, err)
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (s *nodeServer) NodeUnpublishVolume(
	_ context.Context,
	req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}

	target := filepath.Clean(req.TargetPath)

	mounted, err := isMountPoint(target)
	if err != nil {
		if os.IsNotExist(err) {
			klog.V(2).Infof("NodeUnpublishVolume: vol=%s target %s not found (idempotent)",
				req.VolumeId, target)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal,
			"check mount at %q: %v", target, err)
	}
	if !mounted {
		klog.V(2).Infof("NodeUnpublishVolume: vol=%s target %s not mounted (idempotent)",
			req.VolumeId, target)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	klog.V(2).Infof("NodeUnpublishVolume: vol=%s unmounting %s", req.VolumeId, target)
	if err := unix.Unmount(target, unix.MNT_DETACH); err != nil {
		return nil, status.Errorf(codes.Internal,
			"unmount %q: %v", target, err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (s *nodeServer) NodeGetCapabilities(
	_ context.Context,
	_ *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{},
	}, nil
}

func (s *nodeServer) NodeGetInfo(
	_ context.Context,
	_ *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{NodeId: s.cfg.NodeID}, nil
}

// isMountPoint reports whether path is currently a mount point by consulting
// /proc/self/mounts.
func isMountPoint(path string) (bool, error) {
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return false, fmt.Errorf("open /proc/self/mounts: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && filepath.Clean(fields[1]) == path {
			return true, nil
		}
	}
	return false, scanner.Err()
}
