package memmapfs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/absfs/absfs"
	"github.com/absfs/osfs"
)

// SharedMemory provides utilities for inter-process communication
// via memory-mapped files.
type SharedMemory struct {
	path string
	size int64
	mfs  *MemMapFS
	file absfs.File
	data []byte
}

// SharedMemoryConfig configures shared memory creation.
type SharedMemoryConfig struct {
	// Path to the shared file (required)
	Path string

	// Size of the shared memory region in bytes (required)
	Size int64

	// Mode for the memory mapping
	Mode MappingMode

	// SyncMode controls when changes are written to disk
	SyncMode SyncMode

	// SyncInterval for periodic sync
	SyncInterval int

	// Permissions for the shared file (default: 0644)
	Permissions os.FileMode

	// PopulatePages eagerly loads pages
	PopulatePages bool
}

// CreateSharedMemory creates a new shared memory region.
// The file will be created if it doesn't exist.
func CreateSharedMemory(config *SharedMemoryConfig) (*SharedMemory, error) {
	if config.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	if config.Size <= 0 {
		return nil, fmt.Errorf("size must be positive")
	}

	// Set default permissions
	if config.Permissions == 0 {
		config.Permissions = 0644
	}

	// Ensure directory exists
	dir := filepath.Dir(config.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create or open the file
	f, err := os.OpenFile(config.Path, os.O_RDWR|os.O_CREATE, config.Permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	// Set file size
	if err := f.Truncate(config.Size); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to set file size: %w", err)
	}
	f.Close()

	// Create memmapfs wrapper
	osFS, err := osfs.NewFS()
	if err != nil {
		return nil, fmt.Errorf("failed to create osfs: %w", err)
	}

	mmapConfig := &Config{
		Mode:          config.Mode,
		SyncMode:      config.SyncMode,
		PopulatePages: config.PopulatePages,
		MapFullFile:   true,
	}

	if mmapConfig.Mode == 0 {
		mmapConfig.Mode = ModeReadWrite
	}

	mfs := New(osFS, mmapConfig)

	// Open with mmap
	file, err := mfs.OpenFile(config.Path, os.O_RDWR, config.Permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to mmap file: %w", err)
	}

	// Get direct access to mapped memory
	mf, ok := file.(*MappedFile)
	if !ok {
		file.Close()
		return nil, fmt.Errorf("file is not a MappedFile")
	}

	return &SharedMemory{
		path: config.Path,
		size: config.Size,
		mfs:  mfs,
		file: file,
		data: mf.Data(),
	}, nil
}

// OpenSharedMemory opens an existing shared memory region.
func OpenSharedMemory(path string, writable bool) (*SharedMemory, error) {
	// Get file size
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		return nil, fmt.Errorf("failed to create osfs: %w", err)
	}

	mode := ModeReadOnly
	if writable {
		mode = ModeReadWrite
	}

	mmapConfig := &Config{
		Mode:        mode,
		SyncMode:    SyncLazy,
		MapFullFile: true,
	}

	mfs := New(osFS, mmapConfig)

	// Open file
	var file absfs.File
	if writable {
		file, err = mfs.OpenFile(path, os.O_RDWR, 0)
	} else {
		file, err = mfs.Open(path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to mmap file: %w", err)
	}

	// Get direct access to mapped memory
	mf, ok := file.(*MappedFile)
	if !ok {
		file.Close()
		return nil, fmt.Errorf("file is not a MappedFile")
	}

	return &SharedMemory{
		path: path,
		size: fi.Size(),
		mfs:  mfs,
		file: file,
		data: mf.Data(),
	}, nil
}

// Data returns a direct slice to the shared memory region.
// CAUTION: Concurrent access from multiple processes requires synchronization.
func (sm *SharedMemory) Data() []byte {
	return sm.data
}

// Size returns the size of the shared memory region.
func (sm *SharedMemory) Size() int64 {
	return sm.size
}

// Path returns the filesystem path to the shared memory file.
func (sm *SharedMemory) Path() string {
	return sm.path
}

// Sync synchronizes the shared memory to disk.
func (sm *SharedMemory) Sync() error {
	if mf, ok := sm.file.(*MappedFile); ok {
		return mf.Sync()
	}
	return nil
}

// Close closes the shared memory region.
// The underlying file remains on disk and can be reopened.
func (sm *SharedMemory) Close() error {
	if sm.file != nil {
		return sm.file.Close()
	}
	return nil
}

// Remove closes and deletes the shared memory file.
func (sm *SharedMemory) Remove() error {
	if sm.file != nil {
		sm.file.Close()
		sm.file = nil
	}

	return os.Remove(sm.path)
}

// MappedFile returns the underlying MappedFile for advanced operations.
func (sm *SharedMemory) MappedFile() *MappedFile {
	if mf, ok := sm.file.(*MappedFile); ok {
		return mf
	}
	return nil
}
