package driver

import (
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// Driver wires together all CSI server components.
type Driver struct {
	cfg   *Config
	store *MetaStore
}

// New creates a Driver from the provided config.
func New(cfg *Config) *Driver {
	return &Driver{
		cfg:   cfg,
		store: &MetaStore{},
	}
}

// Run starts the gRPC listener and blocks until stopCh receives a signal or
// the server exits with an error.
func (d *Driver) Run(stopCh <-chan os.Signal) error {
	u, err := url.Parse(d.cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint %q: %w", d.cfg.Endpoint, err)
	}

	if u.Scheme == "unix" {
		if err := os.Remove(u.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing socket %q: %w", u.Path, err)
		}
	}

	addr := u.Host
	if u.Scheme == "unix" {
		addr = u.Path
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", d.cfg.Endpoint, err)
	}

	srv := grpc.NewServer()
	csi.RegisterIdentityServer(srv, &identityServer{cfg: d.cfg})

	switch d.cfg.Mode {
	case "controller":
		klog.Infof("Starting in controller mode (basePath=%s)", d.cfg.BasePath)
		csi.RegisterControllerServer(srv, &controllerServer{
			cfg:   d.cfg,
			store: d.store,
		})
	case "node":
		klog.Infof("Starting in node mode (nodeID=%s)", d.cfg.NodeID)
		csi.RegisterNodeServer(srv, &nodeServer{cfg: d.cfg})
	default:
		return fmt.Errorf("unknown mode %q: must be controller or node", d.cfg.Mode)
	}

	klog.Infof("gRPC server listening on %s", d.cfg.Endpoint)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(listener)
	}()

	select {
	case sig := <-stopCh:
		klog.Infof("Received signal %v, shutting down gracefully", sig)
		srv.GracefulStop()
		return nil
	case err := <-errCh:
		return fmt.Errorf("gRPC server: %w", err)
	}
}
