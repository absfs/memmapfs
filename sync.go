package memmapfs

import (
	"sync"
	"time"
)

// syncManager manages periodic synchronization of mapped files.
type syncManager struct {
	files    map[*MappedFile]struct{}
	mu       sync.RWMutex
	ticker   *time.Ticker
	stopChan chan struct{}
	stopped  bool
}

// newSyncManager creates a new sync manager with the given interval.
func newSyncManager(interval time.Duration) *syncManager {
	sm := &syncManager{
		files:    make(map[*MappedFile]struct{}),
		ticker:   time.NewTicker(interval),
		stopChan: make(chan struct{}),
	}

	go sm.run()
	return sm
}

// run is the main loop that periodically syncs all registered files.
func (sm *syncManager) run() {
	for {
		select {
		case <-sm.ticker.C:
			sm.syncAll()
		case <-sm.stopChan:
			return
		}
	}
}

// syncAll syncs all registered files.
func (sm *syncManager) syncAll() {
	sm.mu.RLock()
	files := make([]*MappedFile, 0, len(sm.files))
	for f := range sm.files {
		files = append(files, f)
	}
	sm.mu.RUnlock()

	// Sync each file (without holding the manager lock)
	for _, f := range files {
		_ = f.Sync() // Ignore errors during periodic sync
	}
}

// register adds a file to the sync manager.
func (sm *syncManager) register(mf *MappedFile) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.stopped {
		return
	}

	sm.files[mf] = struct{}{}
}

// unregister removes a file from the sync manager.
func (sm *syncManager) unregister(mf *MappedFile) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.files, mf)
}

// stop stops the sync manager and cleans up resources.
func (sm *syncManager) stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.stopped {
		return
	}

	sm.stopped = true
	close(sm.stopChan)
	sm.ticker.Stop()

	// Clear all files
	sm.files = make(map[*MappedFile]struct{})
}
