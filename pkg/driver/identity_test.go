package driver

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func TestGetPluginInfo(t *testing.T) {
	s := &identityServer{cfg: &Config{DriverName: "test-driver"}}

	resp, err := s.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo failed: %v", err)
	}

	if resp.Name != "test-driver" {
		t.Errorf("Name = %q, want %q", resp.Name, "test-driver")
	}
	if resp.VendorVersion != driverVersion {
		t.Errorf("VendorVersion = %q, want %q", resp.VendorVersion, driverVersion)
	}
}

func TestGetPluginCapabilities(t *testing.T) {
	s := &identityServer{cfg: &Config{DriverName: "test-driver"}}

	resp, err := s.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("GetPluginCapabilities failed: %v", err)
	}

	if len(resp.Capabilities) == 0 {
		t.Fatal("expected at least one capability")
	}

	found := false
	for _, cap := range resp.Capabilities {
		svc := cap.GetService()
		if svc != nil && svc.Type == csi.PluginCapability_Service_CONTROLLER_SERVICE {
			found = true
			break
		}
	}
	if !found {
		t.Error("CONTROLLER_SERVICE capability not found")
	}
}

func TestProbe(t *testing.T) {
	s := &identityServer{cfg: &Config{DriverName: "test-driver"}}

	resp, err := s.Probe(context.Background(), &csi.ProbeRequest{})
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}
