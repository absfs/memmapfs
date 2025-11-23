//go:build unix || darwin || linux

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

	// Determine protection and flags based on mode
	prot, flags := mf.getProtectionFlags()

	// Perform mmap
	data, err := unix.Mmap(int(fd), 0, int(mf.size), prot, flags)
	if err != nil {
		return fmt.Errorf("mmap failed: %w", err)
	}

	mf.data = data
	return nil
}

// munmap unmaps the memory region.
func (mf *MappedFile) munmap() error {
	if mf.data == nil {
		return nil
	}

	if err := unix.Munmap(mf.data); err != nil {
		return fmt.Errorf("munmap failed: %w", err)
	}

	return nil
}

// msync synchronizes dirty pages to disk.
func (mf *MappedFile) msync() error {
	if mf.data == nil {
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

	if err := unix.Msync(mf.data, flags); err != nil {
		return fmt.Errorf("msync failed: %w", err)
	}

	return nil
}

// preload provides hints to the OS to load pages into memory.
func (mf *MappedFile) preload() error {
	if mf.data == nil {
		return nil
	}

	advice := unix.MADV_WILLNEED
	if mf.config.PreloadAsync {
		// MADV_WILLNEED is already async on most systems
		advice = unix.MADV_WILLNEED
	}

	if err := unix.Madvise(mf.data, advice); err != nil {
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

	if mf.data == nil {
		return ErrNotMapped
	}

	if err := unix.Madvise(mf.data, advice); err != nil {
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
