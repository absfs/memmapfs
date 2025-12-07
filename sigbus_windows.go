//go:build windows

package memmapfs

// SIGBUSHandler is a no-op on Windows (SIGBUS doesn't exist).
type SIGBUSHandler struct{}

// GetSIGBUSHandler returns a no-op handler on Windows.
func GetSIGBUSHandler() *SIGBUSHandler {
	return &SIGBUSHandler{}
}

// Enable is a no-op on Windows.
func (h *SIGBUSHandler) Enable() {}

// Disable is a no-op on Windows.
func (h *SIGBUSHandler) Disable() {}

// Register is a no-op on Windows.
func (h *SIGBUSHandler) Register(mf *MappedFile) {}

// Unregister is a no-op on Windows.
func (h *SIGBUSHandler) Unregister(mf *MappedFile) {}

// OnSIGBUS is a no-op on Windows.
func (h *SIGBUSHandler) OnSIGBUS(handler func(*MappedFile, error)) {}

// EnableSIGBUSProtection is a no-op on Windows.
func (mf *MappedFile) EnableSIGBUSProtection() {}

// DisableSIGBUSProtection is a no-op on Windows.
func (mf *MappedFile) DisableSIGBUSProtection() {}

// RemapAfterTruncation is a no-op on Windows.
func (mf *MappedFile) RemapAfterTruncation() error {
	return nil
}

// checkTruncation checks if the file has been truncated.
// This is a no-op on Windows (Phase 1).
func (mf *MappedFile) checkTruncation() (bool, error) {
	return false, nil
}
