//go:build freebsd || netbsd || openbsd || dragonfly

package memmapfs

import (
	"fmt"
	"os"
	"reflect"
	"unsafe"

	"golang.org/x/sys/unix"
)

// mmap performs the platform-specific memory mapping.
func (mf *MappedFile) mmap() error {
	// Get file descriptor
	fd, err := getFD(mf.file)
	if err != nil {
		return fmt.Errorf("failed to get file descriptor: %w", err)
	}

	// Store fd for potential remapping
	mf.fd = fd

	// Determine protection and flags based on mode
	prot, flags := mf.getProtectionFlags()

	// Note: PopulatePages and UseHugePages are handled differently on BSD.
	// FreeBSD has MAP_PREFAULT_READ, other BSDs use madvise.
	// UseHugePages has no direct equivalent on BSDs.

	// Calculate map size based on windowing
	mapSize := mf.size
	mapOffset := int64(0)

	if mf.windowSize > 0 {
		// Using windowed mapping
		mapOffset = mf.windowOffset
		mapSize = mf.windowSize

		// Don't map beyond end of file
		if mapOffset+mapSize > mf.size {
			mapSize = mf.size - mapOffset
		}
	}

	// Ensure offset is page-aligned
	pageSize := int64(unix.Getpagesize())
	alignedOffset := (mapOffset / pageSize) * pageSize
	offsetDiff := mapOffset - alignedOffset

	// Adjust map size to account for alignment
	adjustedMapSize := mapSize + offsetDiff

	// Perform mmap
	data, err := unix.Mmap(int(fd), alignedOffset, int(adjustedMapSize), prot, flags)
	if err != nil {
		return fmt.Errorf("mmap failed: %w", err)
	}

	// Store the original mmap'd slice for munmap
	mf.mmapData = data

	// If we had to align, adjust the data slice to skip the alignment padding
	if offsetDiff > 0 {
		mf.data = data[offsetDiff:]
	} else {
		mf.data = data
	}

	// On BSD, if PopulatePages was requested, use madvise(MADV_WILLNEED)
	// as an alternative to Linux's MAP_POPULATE
	if mf.config.PopulatePages {
		// MADV_WILLNEED hints to the kernel to prefetch the pages
		_ = unix.Madvise(mf.mmapData, unix.MADV_WILLNEED)
	}

	return nil
}

// munmap unmaps the memory region.
func (mf *MappedFile) munmap() error {
	if mf.mmapData == nil {
		return nil
	}

	// Unmap the original mmap'd slice, not the adjusted one
	if err := unix.Munmap(mf.mmapData); err != nil {
		return fmt.Errorf("munmap failed: %w", err)
	}

	mf.mmapData = nil
	return nil
}

// msync synchronizes dirty pages to disk.
func (mf *MappedFile) msync() error {
	if mf.mmapData == nil {
		return nil
	}

	var flags int
	switch mf.config.SyncMode {
	case SyncImmediate:
		flags = unix.MS_SYNC
	case SyncLazy, SyncPeriodic:
		flags = unix.MS_ASYNC
	case SyncNever:
		return nil
	}

	// Use the original mmap'd slice for msync
	if err := unix.Msync(mf.mmapData, flags); err != nil {
		return fmt.Errorf("msync failed: %w", err)
	}

	return nil
}

// preload provides hints to the OS to load pages into memory.
func (mf *MappedFile) preload() error {
	if mf.mmapData == nil {
		return nil
	}

	// Use the original mmap'd slice for madvise
	if err := unix.Madvise(mf.mmapData, unix.MADV_WILLNEED); err != nil {
		return fmt.Errorf("madvise failed: %w", err)
	}

	return nil
}

// getProtectionFlags returns the protection and mapping flags based on the mode.
func (mf *MappedFile) getProtectionFlags() (prot int, flags int) {
	switch mf.config.Mode {
	case ModeReadOnly:
		prot = unix.PROT_READ
		flags = unix.MAP_SHARED
	case ModeReadWrite:
		prot = unix.PROT_READ | unix.PROT_WRITE
		flags = unix.MAP_SHARED
	case ModeCopyOnWrite:
		prot = unix.PROT_READ | unix.PROT_WRITE
		flags = unix.MAP_PRIVATE
	default:
		prot = unix.PROT_READ
		flags = unix.MAP_SHARED
	}

	return prot, flags
}

// getFD extracts the file descriptor from an absfs.File.
// This uses reflection to access the underlying os.File if available.
func getFD(file interface{}) (uintptr, error) {
	// Try to assert as *os.File directly
	if osFile, ok := file.(*os.File); ok {
		return osFile.Fd(), nil
	}

	// Try to call Fd() method directly if it exists
	type fdGetter interface {
		Fd() uintptr
	}
	if fg, ok := file.(fdGetter); ok {
		return fg.Fd(), nil
	}

	// Try to find an embedded or wrapped *os.File using reflection
	v := reflect.ValueOf(file)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Look for a field that might contain the os.File
	// This includes both exported and unexported fields
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// For unexported fields, we need to use unsafe to access them
		if !field.CanInterface() {
			// Create a new value that can be interfaced
			field = reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
		}

		// Check if this field is an *os.File
		if field.Type() == reflect.TypeOf((*os.File)(nil)) {
			if osFile, ok := field.Interface().(*os.File); ok {
				return osFile.Fd(), nil
			}
		}

		// Check if field name suggests it's a file (common naming patterns)
		fieldName := fieldType.Name
		if (fieldName == "file" || fieldName == "f" || fieldName == "File") && field.Kind() == reflect.Ptr {
			// Try to extract Fd from this field
			if field.Type() == reflect.TypeOf((*os.File)(nil)) {
				if osFile, ok := field.Interface().(*os.File); ok {
					return osFile.Fd(), nil
				}
			}
		}

		// Check if this field implements the Fd() method
		if field.CanInterface() {
			if fdGetter, ok := field.Interface().(interface{ Fd() uintptr }); ok {
				return fdGetter.Fd(), nil
			}
		}
	}

	return 0, fmt.Errorf("unable to extract file descriptor from type %T", file)
}

// Advise provides access pattern hints to the kernel.
// This is a utility function for advanced use cases.
func (mf *MappedFile) Advise(advice int) error {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if mf.mmapData == nil {
		return ErrNotMapped
	}

	// Use the original mmap'd slice for madvise
	if err := unix.Madvise(mf.mmapData, advice); err != nil {
		return fmt.Errorf("madvise failed: %w", err)
	}

	return nil
}

// AdviseSequential hints that the file will be accessed sequentially.
func (mf *MappedFile) AdviseSequential() error {
	return mf.Advise(unix.MADV_SEQUENTIAL)
}

// AdviseRandom hints that the file will be accessed randomly.
func (mf *MappedFile) AdviseRandom() error {
	return mf.Advise(unix.MADV_RANDOM)
}

// AdviseDontNeed hints that the pages won't be needed soon and can be evicted.
func (mf *MappedFile) AdviseDontNeed() error {
	return mf.Advise(unix.MADV_DONTNEED)
}

// AdviseWillNeed hints that the pages will be needed soon.
func (mf *MappedFile) AdviseWillNeed() error {
	return mf.Advise(unix.MADV_WILLNEED)
}

// AdviseHugePage is a no-op on BSD.
// BSD systems do not have explicit huge page APIs.
func (mf *MappedFile) AdviseHugePage() error {
	// No-op on BSD
	return nil
}

// AdviseNoHugePage is a no-op on BSD.
// BSD systems do not have explicit huge page APIs.
func (mf *MappedFile) AdviseNoHugePage() error {
	// No-op on BSD
	return nil
}

// AdviseFree hints that the pages can be freed.
// On BSD, this uses MADV_FREE which is available on most BSD variants.
func (mf *MappedFile) AdviseFree() error {
	return mf.Advise(unix.MADV_FREE)
}

// AdviseRemove is a no-op on BSD as this advice is Linux-specific.
func (mf *MappedFile) AdviseRemove() error {
	// No equivalent on BSD, use MADV_DONTNEED as closest alternative
	return mf.Advise(unix.MADV_DONTNEED)
}

// Data returns a direct slice to the mapped memory.
// Use with caution - this provides direct access to the mapped region.
// For read-only mappings, modifications will cause a panic.
func (mf *MappedFile) Data() []byte {
	mf.mu.RLock()
	defer mf.mu.RUnlock()
	return mf.data
}

// unsafeString creates a string from a byte slice without copying.
// This is useful for zero-copy string operations on mapped memory.
func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// unsafeBytes creates a byte slice from a string without copying.
// This is useful for zero-copy operations.
func unsafeBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}
