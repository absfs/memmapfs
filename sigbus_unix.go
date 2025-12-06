//go:build !windows

package memmapfs

import (
	"fmt"
	"os"
	"os/signal"
	"sync"

	"golang.org/x/sys/unix"
)

// SIGBUSHandler manages SIGBUS signal handling for memory-mapped files.
// SIGBUS can occur when:
// - File is truncated while mapped
// - I/O error occurs reading from disk
// - Accessing beyond mapped region
type SIGBUSHandler struct {
	mu       sync.RWMutex
	files    map[*MappedFile]bool
	sigChan  chan os.Signal
	enabled  bool
	handlers []func(*MappedFile, error)
}

var (
	globalSIGBUSHandler     *SIGBUSHandler
	globalSIGBUSHandlerOnce sync.Once
)

// GetSIGBUSHandler returns the global SIGBUS handler instance.
func GetSIGBUSHandler() *SIGBUSHandler {
	globalSIGBUSHandlerOnce.Do(func() {
		globalSIGBUSHandler = &SIGBUSHandler{
			files:    make(map[*MappedFile]bool),
			sigChan:  make(chan os.Signal, 1),
			handlers: make([]func(*MappedFile, error), 0),
		}
	})
	return globalSIGBUSHandler
}

// Enable starts monitoring for SIGBUS signals.
func (h *SIGBUSHandler) Enable() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.enabled {
		return
	}

	signal.Notify(h.sigChan, unix.SIGBUS)
	h.enabled = true

	go h.handleSignals()
}

// Disable stops monitoring for SIGBUS signals.
func (h *SIGBUSHandler) Disable() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.enabled {
		return
	}

	signal.Stop(h.sigChan)
	h.enabled = false
}

// Register adds a mapped file to the monitored set.
func (h *SIGBUSHandler) Register(mf *MappedFile) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.files[mf] = true

	// Auto-enable if first file
	if len(h.files) == 1 && !h.enabled {
		h.mu.Unlock()
		h.Enable()
		h.mu.Lock()
	}
}

// Unregister removes a mapped file from the monitored set.
func (h *SIGBUSHandler) Unregister(mf *MappedFile) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.files, mf)

	// Auto-disable if no more files
	if len(h.files) == 0 && h.enabled {
		h.mu.Unlock()
		h.Disable()
		h.mu.Lock()
	}
}

// OnSIGBUS registers a handler function called when SIGBUS occurs.
func (h *SIGBUSHandler) OnSIGBUS(handler func(*MappedFile, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.handlers = append(h.handlers, handler)
}

// handleSignals processes SIGBUS signals.
func (h *SIGBUSHandler) handleSignals() {
	for range h.sigChan {
		h.handleSIGBUS()
	}
}

// handleSIGBUS is called when a SIGBUS signal is received.
func (h *SIGBUSHandler) handleSIGBUS() {
	h.mu.RLock()
	files := make([]*MappedFile, 0, len(h.files))
	for mf := range h.files {
		files = append(files, mf)
	}
	handlers := make([]func(*MappedFile, error), len(h.handlers))
	copy(handlers, h.handlers)
	h.mu.RUnlock()

	err := ErrSIGBUS

	// Check each mapped file for potential issues
	for _, mf := range files {
		// Try to detect if this file was truncated
		if isTruncated, truncErr := mf.checkTruncation(); isTruncated {
			err = fmt.Errorf("file truncated while mapped: %w", truncErr)
		}

		// Call registered handlers
		for _, handler := range handlers {
			handler(mf, err)
		}
	}
}

// checkTruncation checks if the file has been truncated.
func (mf *MappedFile) checkTruncation() (bool, error) {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if mf.file == nil {
		return false, nil
	}

	fi, err := mf.file.Stat()
	if err != nil {
		return false, fmt.Errorf("stat failed: %w", err)
	}

	currentSize := fi.Size()
	if currentSize < mf.size {
		return true, fmt.Errorf("file size decreased from %d to %d bytes", mf.size, currentSize)
	}

	return false, nil
}

// EnableSIGBUSProtection enables SIGBUS monitoring for a mapped file.
// This should be called after opening a file if you want protection.
func (mf *MappedFile) EnableSIGBUSProtection() {
	handler := GetSIGBUSHandler()
	handler.Register(mf)
}

// DisableSIGBUSProtection disables SIGBUS monitoring for a mapped file.
func (mf *MappedFile) DisableSIGBUSProtection() {
	handler := GetSIGBUSHandler()
	handler.Unregister(mf)
}

// RemapAfterTruncation attempts to remap the file after detecting truncation.
// This can be called from a SIGBUS handler to recover from truncation.
func (mf *MappedFile) RemapAfterTruncation() error {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	// Get current file size
	fi, err := mf.file.Stat()
	if err != nil {
		return fmt.Errorf("stat failed: %w", err)
	}

	newSize := fi.Size()
	if newSize >= mf.size {
		return nil // File wasn't actually truncated
	}

	// Unmap current mapping
	if mf.mmapData != nil {
		if err := unix.Munmap(mf.mmapData); err != nil {
			return fmt.Errorf("munmap failed: %w", err)
		}
		mf.mmapData = nil
		mf.data = nil
	}

	// Remap with new size
	if newSize == 0 {
		// File is now empty, no mapping needed
		mf.size = 0
		return nil
	}

	// Perform new mmap with updated size
	mf.size = newSize
	if err := mf.mmap(); err != nil {
		return fmt.Errorf("remap failed: %w", err)
	}

	return nil
}
