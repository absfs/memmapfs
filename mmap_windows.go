//go:build windows

package memmapfs

import (
	"errors"
)

var ErrWindowsNotImplemented = errors.New("memory mapping not yet implemented for Windows (Phase 2)")

// mmap performs the platform-specific memory mapping.
// For Phase 1, Windows support is not implemented.
func (mf *MappedFile) mmap() error {
	return ErrWindowsNotImplemented
}

// munmap unmaps the memory region.
func (mf *MappedFile) munmap() error {
	return nil
}

// msync synchronizes dirty pages to disk.
func (mf *MappedFile) msync() error {
	return nil
}

// preload provides hints to the OS to load pages into memory.
func (mf *MappedFile) preload() error {
	return nil
}

// Data returns a direct slice to the mapped memory.
// For Windows (Phase 1), this returns nil.
func (mf *MappedFile) Data() []byte {
	return nil
}
