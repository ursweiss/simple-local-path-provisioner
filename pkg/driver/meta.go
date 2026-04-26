package driver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const metaFileName = ".csi-meta.json"

// VolumeMetadata holds persisted state for a single volume.
type VolumeMetadata struct {
	Identity      string    `json:"identity"`
	BackingPath   string    `json:"backingPath"`
	PublishedNode string    `json:"publishedNode"`
	PublishedAt   time.Time `json:"publishedAt"`
	CreatedAt     time.Time `json:"createdAt"`
}

// MetaStore manages per-volume metadata with safe concurrent access.
// A per-volume mutex is stored in a sync.Map keyed by volume handle.
type MetaStore struct {
	mu sync.Map
}

func (ms *MetaStore) lockFor(handle string) *sync.Mutex {
	v, _ := ms.mu.LoadOrStore(handle, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// Update reads, applies fn, and atomically writes metadata for the given
// volume. If no meta file exists yet, a new VolumeMetadata is initialised.
// Errors returned by fn are propagated as-is (gRPC status errors are preserved).
func (ms *MetaStore) Update(handle, backingPath string, fn func(*VolumeMetadata) error) error {
	mu := ms.lockFor(handle)
	mu.Lock()
	defer mu.Unlock()

	meta, err := readMetaFile(backingPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read volume metadata: %w", err)
		}
		meta = &VolumeMetadata{
			Identity:    handle,
			BackingPath: backingPath,
			CreatedAt:   time.Now(),
		}
	}

	if err := fn(meta); err != nil {
		return err
	}

	if err := writeMetaFile(backingPath, meta); err != nil {
		return fmt.Errorf("write volume metadata: %w", err)
	}
	return nil
}

// Read returns the current metadata for the given volume.
func (ms *MetaStore) Read(handle, backingPath string) (*VolumeMetadata, error) {
	mu := ms.lockFor(handle)
	mu.Lock()
	defer mu.Unlock()
	return readMetaFile(backingPath)
}

func readMetaFile(backingPath string) (*VolumeMetadata, error) {
	data, err := os.ReadFile(filepath.Join(backingPath, metaFileName)) //nolint:gosec // path is validated by validateUnderBase; filename is a constant
	if err != nil {
		return nil, err
	}
	var m VolumeMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &m, nil
}

func writeMetaFile(backingPath string, m *VolumeMetadata) error {
	p := filepath.Join(backingPath, metaFileName)
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write temp metadata file: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename metadata file: %w", err)
	}
	return nil
}
