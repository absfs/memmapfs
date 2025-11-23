package memmapfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/absfs/absfs"
)

// MappedFile represents a memory-mapped file.
type MappedFile struct {
	// Underlying file from wrapped filesystem
	file absfs.File

	// Memory mapping
	data     []byte // Mapped memory region
	size     int64  // File size
	position int64  // Current read/write position

	// Configuration
	config *Config

	// State
	modified bool         // Track if writes occurred
	mu       sync.RWMutex // Protect concurrent access
}

// newMappedFile creates a new memory-mapped file.
// This function is platform-specific and implemented in mmap_unix.go or mmap_windows.go
func newMappedFile(file absfs.File, config *Config, size int64) (*MappedFile, error) {
	mf := &MappedFile{
		file:     file,
		size:     size,
		position: 0,
		config:   config,
		modified: false,
	}

	// Perform platform-specific mmap
	if err := mf.mmap(); err != nil {
		return nil, err
	}

	// Apply preload hints if requested
	if config.Preload {
		if err := mf.preload(); err != nil {
			// Preload is a hint, don't fail on error
			_ = err
		}
	}

	return mf, nil
}

// Read reads data from the mapped memory.
func (mf *MappedFile) Read(p []byte) (int, error) {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if mf.data == nil {
		return mf.file.Read(p)
	}

	if mf.position >= int64(len(mf.data)) {
		return 0, io.EOF
	}

	// Copy from mapped memory to user buffer
	n := copy(p, mf.data[mf.position:])
	mf.position += int64(n)

	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// ReadAt reads data at a specific offset without changing the file position.
func (mf *MappedFile) ReadAt(p []byte, off int64) (int, error) {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if mf.data == nil {
		return mf.file.ReadAt(p, off)
	}

	if off < 0 || off >= int64(len(mf.data)) {
		return 0, ErrInvalidOffset
	}

	// Copy from mapped memory at offset
	n := copy(p, mf.data[off:])

	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// Write writes data to the mapped memory.
// For Phase 1 (read-only), this returns an error.
func (mf *MappedFile) Write(p []byte) (int, error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	// Phase 1: Read-only support only
	if mf.data != nil && mf.config.Mode == ModeReadOnly {
		return 0, ErrWriteToReadOnlyMap
	}

	// For non-mapped files or future read-write support
	return mf.file.Write(p)
}

// WriteAt writes data at a specific offset.
// For Phase 1 (read-only), this returns an error.
func (mf *MappedFile) WriteAt(p []byte, off int64) (int, error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	// Phase 1: Read-only support only
	if mf.data != nil && mf.config.Mode == ModeReadOnly {
		return 0, ErrWriteToReadOnlyMap
	}

	// For non-mapped files or future read-write support
	return mf.file.WriteAt(p, off)
}

// Seek sets the file position for the next Read or Write.
func (mf *MappedFile) Seek(offset int64, whence int) (int64, error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.data == nil {
		return mf.file.Seek(offset, whence)
	}

	var newPos int64

	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = mf.position + offset
	case io.SeekEnd:
		newPos = int64(len(mf.data)) + offset
	default:
		return 0, ErrInvalidWhence
	}

	if newPos < 0 {
		return 0, ErrInvalidOffset
	}

	mf.position = newPos
	return newPos, nil
}

// Close unmaps the memory and closes the underlying file.
func (mf *MappedFile) Close() error {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	var err error

	// Unmap memory if mapped
	if mf.data != nil {
		if unmapErr := mf.munmap(); unmapErr != nil {
			err = unmapErr
		}
		mf.data = nil
	}

	// Close underlying file
	if closeErr := mf.file.Close(); closeErr != nil {
		if err == nil {
			err = closeErr
		}
	}

	return err
}

// Stat returns file info.
func (mf *MappedFile) Stat() (fs.FileInfo, error) {
	return mf.file.Stat()
}

// Sync synchronizes the file's in-memory state with storage.
// For mapped files, this syncs dirty pages to disk.
func (mf *MappedFile) Sync() error {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.data == nil {
		return mf.file.Sync()
	}

	// For read-only mappings, no sync needed
	if mf.config.Mode == ModeReadOnly {
		return nil
	}

	// Platform-specific sync implementation
	return mf.msync()
}

// Truncate changes the size of the file.
// For mapped files, this is not supported in Phase 1.
func (mf *MappedFile) Truncate(size int64) error {
	// Cannot truncate a mapped file
	if mf.data != nil {
		return errors.New("cannot truncate mapped file")
	}
	return mf.file.Truncate(size)
}

// Name returns the name of the file.
func (mf *MappedFile) Name() string {
	return mf.file.Name()
}

// Readdir reads directory contents.
func (mf *MappedFile) Readdir(n int) ([]os.FileInfo, error) {
	return mf.file.Readdir(n)
}

// Readdirnames reads directory entry names.
func (mf *MappedFile) Readdirnames(n int) ([]string, error) {
	return mf.file.Readdirnames(n)
}

// WriteString writes a string to the file.
// For Phase 1 (read-only), this returns an error for mapped files.
func (mf *MappedFile) WriteString(s string) (int, error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	// Phase 1: Read-only support only
	if mf.data != nil && mf.config.Mode == ModeReadOnly {
		return 0, ErrWriteToReadOnlyMap
	}

	// For non-mapped files or future read-write support
	return mf.file.WriteString(s)
}

// Ensure MappedFile implements absfs.File
var _ absfs.File = (*MappedFile)(nil)
