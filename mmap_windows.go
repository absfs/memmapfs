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

// Advise provides access pattern hints to the kernel.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) Advise(advice int) error {
	return nil
}

// AdviseSequential hints that the file will be accessed sequentially.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseSequential() error {
	return nil
}

// AdviseRandom hints that the file will be accessed randomly.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseRandom() error {
	return nil
}

// AdviseDontNeed hints that the pages won't be needed soon and can be evicted.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseDontNeed() error {
	return nil
}

// AdviseWillNeed hints that the pages will be needed soon.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseWillNeed() error {
	return nil
}

// AdviseHugePage hints that the kernel should use transparent huge pages.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseHugePage() error {
	return nil
}

// AdviseNoHugePage hints that the kernel should not use transparent huge pages.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseNoHugePage() error {
	return nil
}

// AdviseFree hints that the pages can be freed.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseFree() error {
	return nil
}

// AdviseRemove hints that pages will not be accessed in the near future.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) AdviseRemove() error {
	return nil
}
