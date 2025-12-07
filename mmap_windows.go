//go:build windows

package memmapfs

import (
	"fmt"
	"os"
	"reflect"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mmap performs the platform-specific memory mapping using Windows API.
func (mf *MappedFile) mmap() error {
	// Get file handle
	handle, err := getHandle(mf.file)
	if err != nil {
		return fmt.Errorf("failed to get file handle: %w", err)
	}

	// Store handle for potential remapping
	mf.fd = uintptr(handle)

	// Determine protection based on mode
	protect, access := mf.getProtectionFlags()

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

	// Windows requires page alignment (typically 64KB allocation granularity)
	var si syscall.SystemInfo
	syscall.GetSystemInfo(&si)
	allocGranularity := int64(si.AllocationGranularity)

	// Align offset to allocation granularity
	alignedOffset := (mapOffset / allocGranularity) * allocGranularity
	offsetDiff := mapOffset - alignedOffset

	// Adjust map size to account for alignment
	adjustedMapSize := mapSize + offsetDiff

	// Create file mapping object
	// Convert size to high/low DWORD format
	maxSizeHigh := uint32((alignedOffset + adjustedMapSize) >> 32)
	maxSizeLow := uint32(alignedOffset + adjustedMapSize)

	// Create mapping handle
	mappingHandle, err := windows.CreateFileMapping(
		windows.Handle(handle),
		nil,
		protect,
		maxSizeHigh,
		maxSizeLow,
		nil,
	)
	if err != nil {
		return fmt.Errorf("CreateFileMapping failed: %w", err)
	}

	// Map view of file
	offsetHigh := uint32(alignedOffset >> 32)
	offsetLow := uint32(alignedOffset)

	addr, err := windows.MapViewOfFile(
		mappingHandle,
		access,
		offsetHigh,
		offsetLow,
		uintptr(adjustedMapSize),
	)
	if err != nil {
		windows.CloseHandle(mappingHandle)
		return fmt.Errorf("MapViewOfFile failed: %w", err)
	}

	// Close mapping handle (the view keeps the mapping alive)
	windows.CloseHandle(mappingHandle)

	// Convert to byte slice
	data := unsafe.Slice((*byte)(unsafe.Pointer(addr)), adjustedMapSize)

	// Store the original mapped slice for unmapping
	mf.mmapData = data

	// If we had to align, adjust the data slice to skip the alignment padding
	if offsetDiff > 0 {
		mf.data = data[offsetDiff:]
	} else {
		mf.data = data
	}

	return nil
}

// munmap unmaps the memory region.
func (mf *MappedFile) munmap() error {
	if mf.mmapData == nil {
		return nil
	}

	// Unmap the view
	addr := uintptr(unsafe.Pointer(&mf.mmapData[0]))
	if err := windows.UnmapViewOfFile(addr); err != nil {
		return fmt.Errorf("UnmapViewOfFile failed: %w", err)
	}

	mf.mmapData = nil
	mf.data = nil
	return nil
}

// msync synchronizes dirty pages to disk.
func (mf *MappedFile) msync() error {
	if mf.mmapData == nil {
		return nil
	}

	// For SyncNever mode, skip sync
	if mf.config.SyncMode == SyncNever {
		return nil
	}

	// Flush view to disk
	addr := uintptr(unsafe.Pointer(&mf.mmapData[0]))
	size := uintptr(len(mf.mmapData))

	if err := windows.FlushViewOfFile(addr, size); err != nil {
		return fmt.Errorf("FlushViewOfFile failed: %w", err)
	}

	// For immediate sync, also flush file buffers
	if mf.config.SyncMode == SyncImmediate {
		if err := windows.FlushFileBuffers(windows.Handle(mf.fd)); err != nil {
			return fmt.Errorf("FlushFileBuffers failed: %w", err)
		}
	}

	return nil
}

// preload provides hints to the OS to load pages into memory.
// On Windows, this uses PrefetchVirtualMemory if available (Windows 8+).
func (mf *MappedFile) preload() error {
	if mf.mmapData == nil {
		return nil
	}

	// Windows doesn't have a direct equivalent to madvise(MADV_WILLNEED)
	// PrefetchVirtualMemory is available on Windows 8+ but requires special handling
	// For now, we'll skip preload on Windows or just return success
	// The data will be loaded on first access (demand paging)

	return nil
}

// getProtectionFlags returns the protection and access flags for Windows mapping.
func (mf *MappedFile) getProtectionFlags() (protect uint32, access uint32) {
	switch mf.config.Mode {
	case ModeReadOnly:
		protect = windows.PAGE_READONLY
		access = windows.FILE_MAP_READ
	case ModeReadWrite:
		protect = windows.PAGE_READWRITE
		access = windows.FILE_MAP_WRITE
	case ModeCopyOnWrite:
		protect = windows.PAGE_WRITECOPY
		access = windows.FILE_MAP_COPY
	default:
		protect = windows.PAGE_READONLY
		access = windows.FILE_MAP_READ
	}

	return protect, access
}

// getHandle extracts the Windows file handle from an absfs.File.
// This uses reflection to access the underlying os.File if available.
func getHandle(file interface{}) (syscall.Handle, error) {
	// Try to assert as *os.File directly
	if osFile, ok := file.(*os.File); ok {
		return syscall.Handle(osFile.Fd()), nil
	}

	// Try to call Fd() method directly if it exists
	type fdGetter interface {
		Fd() uintptr
	}
	if fg, ok := file.(fdGetter); ok {
		return syscall.Handle(fg.Fd()), nil
	}

	// Try to find an embedded or wrapped *os.File using reflection
	v := reflect.ValueOf(file)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Look for a field that might contain the os.File
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
				return syscall.Handle(osFile.Fd()), nil
			}
		}

		// Check if field name suggests it's a file (common naming patterns)
		fieldName := fieldType.Name
		if (fieldName == "file" || fieldName == "f" || fieldName == "File") && field.Kind() == reflect.Ptr {
			// Try to extract Fd from this field
			if field.Type() == reflect.TypeOf((*os.File)(nil)) {
				if osFile, ok := field.Interface().(*os.File); ok {
					return syscall.Handle(osFile.Fd()), nil
				}
			}
		}

		// Check if this field implements the Fd() method
		if field.CanInterface() {
			if fdGetter, ok := field.Interface().(interface{ Fd() uintptr }); ok {
				return syscall.Handle(fdGetter.Fd()), nil
			}
		}
	}

	return 0, fmt.Errorf("unable to extract file handle from type %T", file)
}

// Advise provides access pattern hints to the kernel.
// Windows doesn't have a direct equivalent to madvise, so this is mostly a no-op.
func (mf *MappedFile) Advise(advice int) error {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if mf.mmapData == nil {
		return ErrNotMapped
	}

	// Windows doesn't have madvise equivalent
	// Most hints are handled automatically by the OS
	return nil
}

// AdviseSequential hints that the file will be accessed sequentially.
// This is a no-op on Windows.
func (mf *MappedFile) AdviseSequential() error {
	return nil
}

// AdviseRandom hints that the file will be accessed randomly.
// This is a no-op on Windows.
func (mf *MappedFile) AdviseRandom() error {
	return nil
}

// AdviseDontNeed hints that the pages won't be needed soon and can be evicted.
// This is a no-op on Windows.
func (mf *MappedFile) AdviseDontNeed() error {
	return nil
}

// AdviseWillNeed hints that the pages will be needed soon.
// This is a no-op on Windows.
func (mf *MappedFile) AdviseWillNeed() error {
	return nil
}

// AdviseHugePage hints that the kernel should use large pages.
// This is a no-op on Windows (large pages require special privileges).
func (mf *MappedFile) AdviseHugePage() error {
	return nil
}

// AdviseNoHugePage hints that the kernel should not use large pages.
// This is a no-op on Windows.
func (mf *MappedFile) AdviseNoHugePage() error {
	return nil
}

// AdviseFree hints that the pages can be freed.
// This is a no-op on Windows.
func (mf *MappedFile) AdviseFree() error {
	return nil
}

// AdviseRemove hints that pages will not be accessed in the near future.
// This is a no-op on Windows.
func (mf *MappedFile) AdviseRemove() error {
	return nil
}

// Data returns a direct slice to the mapped memory.
// Use with caution - this provides direct access to the mapped region.
func (mf *MappedFile) Data() []byte {
	mf.mu.RLock()
	defer mf.mu.RUnlock()
	return mf.data
}
