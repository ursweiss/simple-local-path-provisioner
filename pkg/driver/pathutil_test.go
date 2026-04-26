package driver

import (
	"testing"
)

func TestSanitizeComponent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase passthrough", "hello", "hello"},
		{"uppercase to lower", "Hello-World", "hello-world"},
		{"special chars to hyphen", "my_ns!@#foo", "my-ns-foo"},
		{"dots preserved", "my.namespace", "my.namespace"},
		{"leading trailing hyphens trimmed", "---abc---", "abc"},
		{"unicode replaced", "nämespace", "n-mespace"},
		{"already clean", "clean-name", "clean-name"},
		{"empty string", "", ""},
		{"all special chars", "!!!@@@", ""},
		{"spaces replaced", "a b c", "a-b-c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeComponent(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeComponent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDeriveBackingPath(t *testing.T) {
	tests := []struct {
		name      string
		basePath  string
		namespace string
		pvcName   string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "normal inputs",
			basePath:  "/data",
			namespace: "default",
			pvcName:   "my-pvc",
			wantPath:  "/data/default-my-pvc",
		},
		{
			name:      "uppercase sanitized",
			basePath:  "/data",
			namespace: "MyNamespace",
			pvcName:   "MyPVC",
			wantPath:  "/data/mynamespace-mypvc",
		},
		{
			name:      "empty namespace",
			basePath:  "/data",
			namespace: "",
			pvcName:   "my-pvc",
			wantErr:   true,
		},
		{
			name:      "empty pvcName",
			basePath:  "/data",
			namespace: "default",
			pvcName:   "",
			wantErr:   true,
		},
		{
			name:      "traversal attempt in namespace",
			basePath:  "/data",
			namespace: "../../etc",
			pvcName:   "passwd",
			wantPath:  "/data/..-..-etc-passwd",
		},
		{
			name:      "dot-dot namespace sanitized",
			basePath:  "/data",
			namespace: "..",
			pvcName:   "test",
			wantPath:  "/data/..-test",
		},
		{
			name:      "special chars in both",
			basePath:  "/data",
			namespace: "ns_with_underscores",
			pvcName:   "pvc@special!",
			wantPath:  "/data/ns-with-underscores-pvc-special",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveBackingPath(tt.basePath, tt.namespace, tt.pvcName)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got path %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantPath {
				t.Errorf("DeriveBackingPath(%q, %q, %q) = %q, want %q",
					tt.basePath, tt.namespace, tt.pvcName, got, tt.wantPath)
			}
		})
	}
}

func TestValidateUnderBase(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		resolved string
		wantErr  bool
	}{
		{"valid sub-path", "/data", "/data/vol-1", false},
		{"nested sub-path", "/data", "/data/a/b/c", false},
		{"escape attempt", "/data", "/etc/passwd", true},
		{"parent traversal", "/data", "/data/../etc", true},
		{"identical paths", "/data", "/data", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUnderBase(tt.basePath, tt.resolved)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUnderBase(%q, %q) error = %v, wantErr %v",
					tt.basePath, tt.resolved, err, tt.wantErr)
			}
		})
	}
}

func TestVolumeHandleFromIdentity(t *testing.T) {
	got := VolumeHandleFromIdentity("default", "my-pvc")
	want := "default/my-pvc"
	if got != want {
		t.Errorf("VolumeHandleFromIdentity = %q, want %q", got, want)
	}
}

func TestIdentityFromVolumeHandle(t *testing.T) {
	tests := []struct {
		name    string
		handle  string
		wantNS  string
		wantPVC string
		wantErr bool
	}{
		{"valid handle", "default/my-pvc", "default", "my-pvc", false},
		{"handle with extra slashes", "ns/pvc/extra", "ns", "pvc/extra", false},
		{"missing slash", "nopvc", "", "", true},
		{"empty namespace", "/my-pvc", "", "", true},
		{"empty pvcName", "default/", "", "", true},
		{"empty string", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, pvc, err := IdentityFromVolumeHandle(tt.handle)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got ns=%q pvc=%q", ns, pvc)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ns != tt.wantNS || pvc != tt.wantPVC {
				t.Errorf("IdentityFromVolumeHandle(%q) = (%q, %q), want (%q, %q)",
					tt.handle, ns, pvc, tt.wantNS, tt.wantPVC)
			}
		})
	}
}

func TestVolumeHandleRoundTrip(t *testing.T) {
	ns, pvc := "my-namespace", "my-pvc"
	handle := VolumeHandleFromIdentity(ns, pvc)
	gotNS, gotPVC, err := IdentityFromVolumeHandle(handle)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if gotNS != ns || gotPVC != pvc {
		t.Errorf("round-trip: got (%q, %q), want (%q, %q)", gotNS, gotPVC, ns, pvc)
	}
}
