package driver

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const driverVersion = "0.1.0"

// Version returns the driver version string.
func Version() string { return driverVersion }

type identityServer struct {
	csi.UnimplementedIdentityServer
	cfg *Config
}

func (s *identityServer) GetPluginInfo(
	_ context.Context,
	_ *csi.GetPluginInfoRequest,
) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          s.cfg.DriverName,
		VendorVersion: driverVersion,
	}, nil
}

func (s *identityServer) GetPluginCapabilities(
	_ context.Context,
	_ *csi.GetPluginCapabilitiesRequest,
) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

func (s *identityServer) Probe(
	_ context.Context,
	_ *csi.ProbeRequest,
) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}
