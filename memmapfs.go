// Package memmapfs provides a memory-mapped file wrapper for absfs with zero-copy I/O.
package memmapfs

import (
	"errors"
	"os"
	"time"

	"github.com/absfs/absfs"
)

// MappingMode defines how files should be memory-mapped.
type MappingMode int

const (
	// ModeReadOnly maps files as read-only (PROT_READ, MAP_PRIVATE/MAP_SHARED)
	ModeReadOnly MappingMode = iota
	// ModeReadWrite maps files as read-write (PROT_READ|PROT_WRITE, MAP_SHARED)
	ModeReadWrite
	// ModeCopyOnWrite maps files as copy-on-write (PROT_READ|PROT_WRITE, MAP_PRIVATE)
	ModeCopyOnWrite
)

// SyncMode defines how modified pages are synchronized to disk.
type SyncMode int

const (
	// SyncImmediate syncs after every write operation
	SyncImmediate SyncMode = iota
	// SyncPeriodic syncs at regular intervals
	SyncPeriodic
	// SyncLazy syncs only on file close
	SyncLazy
	// SyncNever lets OS handle sync automatically
	SyncNever
)

// Config holds configuration for the memory-mapped filesystem.
type Config struct {
	// Mode specifies the mapping mode (read-only, read-write, copy-on-write)
	Mode MappingMode

	// SyncMode specifies when to sync dirty pages to disk
	SyncMode SyncMode

	// SyncInterval is the interval for periodic sync (only used with SyncPeriodic)
	SyncInterval time.Duration

	// MapFullFile determines whether to map the entire file at once
	// If false, WindowSize is used for windowed mapping
	MapFullFile bool

	// WindowSize specifies the size of the mapping window for large files
	// Only used when MapFullFile is false. If 0, defaults to 1GB.
	WindowSize int64

	// Preload hints that pages should be loaded immediately
	Preload bool

	// PreloadAsync performs preload asynchronously
	PreloadAsync bool
}

// DefaultConfig returns a configuration suitable for most use cases.
func DefaultConfig() *Config {
	return &Config{
		Mode:         ModeReadOnly,
		SyncMode:     SyncNever,
		MapFullFile:  true,
		Preload:      false,
		PreloadAsync: false,
	}
}

// MemMapFS wraps an existing filesystem and provides memory-mapped file access.
type MemMapFS struct {
	underlying  absfs.FileSystem
	config      *Config
	syncManager *syncManager
}

// New creates a new memory-mapped filesystem wrapper.
// The underlying filesystem is typically osfs.NewFS() or another absfs.FileSystem implementation.
func New(underlying absfs.FileSystem, config *Config) *MemMapFS {
	if config == nil {
		config = DefaultConfig()
	}

	mfs := &MemMapFS{
		underlying: underlying,
		config:     config,
	}

	// Initialize periodic sync manager if needed
	if config.SyncMode == SyncPeriodic && config.SyncInterval > 0 {
		mfs.syncManager = newSyncManager(config.SyncInterval)
	}

	return mfs
}

// Open opens a file for reading and maps it into memory.
// For Phase 1, only read operations are supported.
func (mfs *MemMapFS) Open(name string) (absfs.File, error) {
	return mfs.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile opens a file with specified flags and permissions.
// For Phase 1, only read-only mode is fully supported.
func (mfs *MemMapFS) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error) {
	// Open the underlying file
	file, err := mfs.underlying.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	// Get file info to determine size
	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	// Don't mmap empty files or directories
	if fi.IsDir() {
		return file, nil
	}

	size := fi.Size()
	if size == 0 {
		return file, nil
	}

	// Create mapped file
	mf, err := newMappedFile(file, mfs.config, size, mfs.syncManager)
	if err != nil {
		file.Close()
		return nil, err
	}

	return mf, nil
}

// Create creates a new file.
// For Phase 1, this delegates to the underlying filesystem.
func (mfs *MemMapFS) Create(name string) (absfs.File, error) {
	return mfs.underlying.Create(name)
}

// Mkdir creates a directory.
func (mfs *MemMapFS) Mkdir(name string, perm os.FileMode) error {
	return mfs.underlying.Mkdir(name, perm)
}

// MkdirAll creates a directory and all parent directories.
func (mfs *MemMapFS) MkdirAll(name string, perm os.FileMode) error {
	return mfs.underlying.MkdirAll(name, perm)
}

// Remove removes a file or directory.
func (mfs *MemMapFS) Remove(name string) error {
	return mfs.underlying.Remove(name)
}

// RemoveAll removes a path and all children.
func (mfs *MemMapFS) RemoveAll(name string) error {
	return mfs.underlying.RemoveAll(name)
}

// Rename renames a file or directory.
func (mfs *MemMapFS) Rename(oldname, newname string) error {
	return mfs.underlying.Rename(oldname, newname)
}

// Stat returns file info.
func (mfs *MemMapFS) Stat(name string) (os.FileInfo, error) {
	return mfs.underlying.Stat(name)
}

// Chmod changes file permissions.
func (mfs *MemMapFS) Chmod(name string, mode os.FileMode) error {
	return mfs.underlying.Chmod(name, mode)
}

// Chown changes file ownership.
func (mfs *MemMapFS) Chown(name string, uid, gid int) error {
	return mfs.underlying.Chown(name, uid, gid)
}

// Chtimes changes file access and modification times.
func (mfs *MemMapFS) Chtimes(name string, atime, mtime time.Time) error {
	return mfs.underlying.Chtimes(name, atime, mtime)
}

// Truncate changes the size of a file.
func (mfs *MemMapFS) Truncate(name string, size int64) error {
	return mfs.underlying.Truncate(name, size)
}

// Separator returns the path separator for this filesystem.
func (mfs *MemMapFS) Separator() uint8 {
	return mfs.underlying.Separator()
}

// ListSeparator returns the list separator for this filesystem.
func (mfs *MemMapFS) ListSeparator() uint8 {
	return mfs.underlying.ListSeparator()
}

// Chdir changes the current working directory.
func (mfs *MemMapFS) Chdir(dir string) error {
	return mfs.underlying.Chdir(dir)
}

// Getwd returns the current working directory.
func (mfs *MemMapFS) Getwd() (string, error) {
	return mfs.underlying.Getwd()
}

// TempDir returns the temporary directory.
func (mfs *MemMapFS) TempDir() string {
	return mfs.underlying.TempDir()
}

// Ensure MemMapFS implements absfs.FileSystem
var _ absfs.FileSystem = (*MemMapFS)(nil)

// Common errors
var (
	ErrNotMapped     = errors.New("file is not memory-mapped")
	ErrInvalidOffset = errors.New("invalid offset")
	ErrInvalidWhence = errors.New("invalid whence")
	ErrWriteToReadOnlyMap = errors.New("cannot write to read-only mapping")
)
