package driver

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMetaStoreUpdateAndRead(t *testing.T) {
	dir := t.TempDir()
	handle := "default/test-pvc"

	store := &MetaStore{}

	err := store.Update(handle, dir, func(meta *VolumeMetadata) error {
		meta.PublishedNode = "node-1"
		meta.PublishedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		return nil
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	meta, err := store.Read(handle, dir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if meta.Identity != handle {
		t.Errorf("Identity = %q, want %q", meta.Identity, handle)
	}
	if meta.BackingPath != dir {
		t.Errorf("BackingPath = %q, want %q", meta.BackingPath, dir)
	}
	if meta.PublishedNode != "node-1" {
		t.Errorf("PublishedNode = %q, want %q", meta.PublishedNode, "node-1")
	}
}

func TestMetaStoreUpdateCreatesFile(t *testing.T) {
	dir := t.TempDir()
	handle := "default/test-pvc"

	store := &MetaStore{}
	err := store.Update(handle, dir, func(_ *VolumeMetadata) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	metaPath := filepath.Join(dir, metaFileName)
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Fatalf("meta file %s was not created", metaPath)
	}
}

func TestMetaStoreUpdatePreservesCreatedAt(t *testing.T) {
	dir := t.TempDir()
	handle := "default/test-pvc"
	store := &MetaStore{}

	err := store.Update(handle, dir, func(_ *VolumeMetadata) error {
		return nil
	})
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}

	meta1, err := store.Read(handle, dir)
	if err != nil {
		t.Fatalf("first Read failed: %v", err)
	}
	createdAt := meta1.CreatedAt

	err = store.Update(handle, dir, func(meta *VolumeMetadata) error {
		meta.PublishedNode = "node-2"
		return nil
	})
	if err != nil {
		t.Fatalf("second Update failed: %v", err)
	}

	meta2, err := store.Read(handle, dir)
	if err != nil {
		t.Fatalf("second Read failed: %v", err)
	}

	if !meta2.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt changed: got %v, want %v", meta2.CreatedAt, createdAt)
	}
	if meta2.PublishedNode != "node-2" {
		t.Errorf("PublishedNode = %q, want %q", meta2.PublishedNode, "node-2")
	}
}

func TestMetaStoreConcurrentUpdates(t *testing.T) {
	dir := t.TempDir()
	handle := "default/concurrent-pvc"
	store := &MetaStore{}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make(chan error, goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			err := store.Update(handle, dir, func(meta *VolumeMetadata) error {
				meta.PublishedNode = "node"
				return nil
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Update error: %v", err)
	}

	meta, err := store.Read(handle, dir)
	if err != nil {
		t.Fatalf("Read after concurrent updates failed: %v", err)
	}
	if meta.Identity != handle {
		t.Errorf("Identity = %q, want %q", meta.Identity, handle)
	}
}
