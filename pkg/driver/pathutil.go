package driver

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	reUnsafe  = regexp.MustCompile(`[^a-z0-9.-]+`)
	reEdgeDash = regexp.MustCompile(`(^-+|-+$)`)
)

// sanitizeComponent lowercases s and replaces any character that is not
// alphanumeric, dot, or hyphen with a hyphen, then trims leading/trailing hyphens.
func sanitizeComponent(s string) string {
	s = strings.ToLower(s)
	s = reUnsafe.ReplaceAllString(s, "-")
	s = reEdgeDash.ReplaceAllString(s, "")
	return s
}

// DeriveBackingPath returns the deterministic backing directory path for a volume.
// The result is {basePath}/{sanitize(namespace)}-{sanitize(pvcName)}.
func DeriveBackingPath(basePath, namespace, pvcName string) (string, error) {
	if namespace == "" {
		return "", status.Error(codes.InvalidArgument, "namespace must not be empty")
	}
	if pvcName == "" {
		return "", status.Error(codes.InvalidArgument, "pvcName must not be empty")
	}
	ns := sanitizeComponent(namespace)
	name := sanitizeComponent(pvcName)
	resolved := filepath.Join(basePath, fmt.Sprintf("%s-%s", ns, name))
	if err := validateUnderBase(basePath, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// validateUnderBase ensures resolved is strictly under basePath to prevent
// path traversal attacks.
func validateUnderBase(basePath, resolved string) error {
	base := filepath.Clean(basePath) + string(filepath.Separator)
	candidate := filepath.Clean(resolved) + string(filepath.Separator)
	if !strings.HasPrefix(candidate, base) {
		return status.Errorf(codes.InvalidArgument,
			"resolved path %q escapes base path %q", resolved, basePath)
	}
	return nil
}

// VolumeHandleFromIdentity builds the stable volume handle from namespace and
// PVC name. The format is "namespace/pvcName".
func VolumeHandleFromIdentity(namespace, pvcName string) string {
	return namespace + "/" + pvcName
}

// IdentityFromVolumeHandle parses namespace and pvcName from a volume handle
// produced by VolumeHandleFromIdentity.
func IdentityFromVolumeHandle(handle string) (namespace, pvcName string, err error) {
	parts := strings.SplitN(handle, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", status.Errorf(codes.InvalidArgument,
			"invalid volume handle %q: expected namespace/pvcName", handle)
	}
	return parts[0], parts[1], nil
}
