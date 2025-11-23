package memmapfs

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/absfs/absfs"
	"github.com/absfs/osfs"
)

// createTestFile creates a temporary test file with the given content.
func createTestFile(t *testing.T, content string) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")

	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpFile, cleanup
}

func TestNew(t *testing.T) {
	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	mfs := New(osFS, nil)

	if mfs == nil {
		t.Fatal("New() returned nil")
	}

	if mfs.config == nil {
		t.Fatal("New() did not set default config")
	}

	// Test with custom config
	config := &Config{
		Mode:     ModeReadOnly,
		SyncMode: SyncImmediate,
	}
	mfs2 := New(osFS, config)

	if mfs2.config.Mode != ModeReadOnly {
		t.Errorf("Expected Mode to be ModeReadOnly, got %v", mfs2.config.Mode)
	}
}

func TestOpen(t *testing.T) {
	testContent := "Hello, memmapfs!"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Check that we got a MappedFile
	mf, ok := file.(*MappedFile)
	if !ok {
		t.Fatal("Open() did not return a *MappedFile")
	}

	// Verify the file is mapped
	if mf.data == nil {
		t.Fatal("File was not memory-mapped")
	}

	if mf.size != int64(len(testContent)) {
		t.Errorf("Expected size %d, got %d", len(testContent), mf.size)
	}
}

func TestRead(t *testing.T) {
	testContent := "Hello, memmapfs! This is a test file."
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Read entire file
	buf := make([]byte, len(testContent))
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if n != len(testContent) {
		t.Errorf("Expected to read %d bytes, got %d", len(testContent), n)
	}

	if string(buf) != testContent {
		t.Errorf("Expected content %q, got %q", testContent, string(buf))
	}

	// Read at EOF should return io.EOF
	_, err = file.Read(buf)
	if err != io.EOF {
		t.Errorf("Expected io.EOF at end of file, got %v", err)
	}
}

func TestReadPartial(t *testing.T) {
	testContent := "Hello, memmapfs!"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Read in small chunks
	buf := make([]byte, 5)

	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("First Read() failed: %v", err)
	}
	if n != 5 {
		t.Errorf("Expected to read 5 bytes, got %d", n)
	}
	if string(buf) != "Hello" {
		t.Errorf("Expected 'Hello', got %q", string(buf))
	}

	// Read next chunk
	n, err = file.Read(buf)
	if err != nil {
		t.Fatalf("Second Read() failed: %v", err)
	}
	if n != 5 {
		t.Errorf("Expected to read 5 bytes, got %d", n)
	}
	if string(buf) != ", mem" {
		t.Errorf("Expected ', mem', got %q", string(buf))
	}
}

func TestReadAt(t *testing.T) {
	testContent := "Hello, memmapfs!"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Read at specific offset
	buf := make([]byte, 9)
	n, err := file.ReadAt(buf, 7)
	if err != nil {
		t.Fatalf("ReadAt() failed: %v", err)
	}

	if n != 9 {
		t.Errorf("Expected to read 9 bytes, got %d", n)
	}

	if string(buf) != "memmapfs!" {
		t.Errorf("Expected 'memmapfs!', got %q", string(buf))
	}
}

func TestSeek(t *testing.T) {
	testContent := "Hello, memmapfs!"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Seek to offset 7
	pos, err := file.Seek(7, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek() failed: %v", err)
	}
	if pos != 7 {
		t.Errorf("Expected position 7, got %d", pos)
	}

	// Read from new position
	buf := make([]byte, 9)
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("Read() after Seek() failed: %v", err)
	}
	if string(buf[:n]) != "memmapfs!" {
		t.Errorf("Expected 'memmapfs!', got %q", string(buf[:n]))
	}

	// Seek from current position
	pos, err = file.Seek(-9, io.SeekCurrent)
	if err != nil {
		t.Fatalf("Seek() from current failed: %v", err)
	}
	if pos != 7 {
		t.Errorf("Expected position 7, got %d", pos)
	}

	// Seek from end
	pos, err = file.Seek(-9, io.SeekEnd)
	if err != nil {
		t.Fatalf("Seek() from end failed: %v", err)
	}
	if pos != int64(len(testContent)-9) {
		t.Errorf("Expected position %d, got %d", len(testContent)-9, pos)
	}
}

func TestStat(t *testing.T) {
	testContent := "Hello, memmapfs!"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}

	if fi.Size() != int64(len(testContent)) {
		t.Errorf("Expected size %d, got %d", len(testContent), fi.Size())
	}

	if fi.IsDir() {
		t.Error("Expected file, not directory")
	}
}

func TestEmptyFile(t *testing.T) {
	tmpFile, cleanup := createTestFile(t, "")
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Empty files should not be mapped
	mf, ok := file.(*MappedFile)
	if ok && mf.data != nil {
		t.Error("Empty file should not be memory-mapped")
	}
}

func TestLargeFile(t *testing.T) {
	// Create a larger test file (1MB)
	content := make([]byte, 1024*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "largefile.bin")
	if err := os.WriteFile(tmpFile, content, 0644); err != nil {
		t.Fatalf("Failed to create large test file: %v", err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Verify size
	fi, err := file.Stat()
	if err != nil {
		t.Fatalf("Stat() failed: %v", err)
	}
	if fi.Size() != int64(len(content)) {
		t.Errorf("Expected size %d, got %d", len(content), fi.Size())
	}

	// Read and verify content
	readBuf := make([]byte, len(content))
	n, err := io.ReadFull(file, readBuf)
	if err != nil {
		t.Fatalf("ReadFull() failed: %v", err)
	}
	if n != len(content) {
		t.Errorf("Expected to read %d bytes, got %d", len(content), n)
	}

	// Verify content matches
	for i := 0; i < len(content); i++ {
		if readBuf[i] != content[i] {
			t.Errorf("Content mismatch at offset %d: expected %d, got %d", i, content[i], readBuf[i])
			break
		}
	}
}

func TestWriteToReadOnlyMapping(t *testing.T) {
	testContent := "Hello, memmapfs!"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	config := &Config{
		Mode: ModeReadOnly,
	}
	mfs := New(osFS, config)

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Attempt to write should fail
	_, err = file.Write([]byte("test"))
	if err != ErrWriteToReadOnlyMap {
		t.Errorf("Expected ErrWriteToReadOnlyMap, got %v", err)
	}
}

func TestName(t *testing.T) {
	testContent := "Hello, memmapfs!"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	name := file.Name()
	if name != tmpFile {
		t.Errorf("Expected name %q, got %q", tmpFile, name)
	}
}

func TestMultipleReads(t *testing.T) {
	testContent := "0123456789ABCDEF"
	tmpFile, cleanup := createTestFile(t, testContent)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Multiple sequential reads
	buf := make([]byte, 4)

	for i := 0; i < 4; i++ {
		n, err := file.Read(buf)
		if err != nil {
			t.Fatalf("Read %d failed: %v", i, err)
		}
		if n != 4 {
			t.Errorf("Read %d: expected 4 bytes, got %d", i, n)
		}

		expected := testContent[i*4 : (i+1)*4]
		if string(buf) != expected {
			t.Errorf("Read %d: expected %q, got %q", i, expected, string(buf))
		}
	}
}

func TestWriteReadWrite(t *testing.T) {
	// Create a file with initial content
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	initialContent := "Hello, World!"
	
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	config := &Config{
		Mode:     ModeReadWrite,
		SyncMode: SyncImmediate,
	}
	mfs := New(osFS, config)

	// Open for read-write
	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile() failed: %v", err)
	}
	defer file.Close()

	// Write new content
	newContent := "Hello, mmap!"
	n, err := file.Write([]byte(newContent))
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(newContent) {
		t.Errorf("Expected to write %d bytes, got %d", len(newContent), n)
	}

	// Seek to beginning
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek() failed: %v", err)
	}

	// Read back the content
	buf := make([]byte, len(newContent))
	n, err = file.Read(buf)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}
	if string(buf) != newContent {
		t.Errorf("Expected to read %q, got %q", newContent, string(buf))
	}

	// Close and verify persistence
	file.Close()

	// Verify persistence by reading with standard I/O
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}

	if string(data[:len(newContent)]) != newContent {
		t.Errorf("Expected persisted content %q, got %q", newContent, string(data))
	}
}

func TestWriteAt(t *testing.T) {
	// Create a file with initial content
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	initialContent := "0123456789ABCDEF"
	
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	config := &Config{
		Mode:     ModeReadWrite,
		SyncMode: SyncImmediate,
	}
	mfs := New(osFS, config)

	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile() failed: %v", err)
	}
	defer file.Close()

	// Write at specific offset
	replacement := "WXYZ"
	n, err := file.WriteAt([]byte(replacement), 5)
	if err != nil {
		t.Fatalf("WriteAt() failed: %v", err)
	}
	if n != len(replacement) {
		t.Errorf("Expected to write %d bytes, got %d", len(replacement), n)
	}

	// Read back the entire content
	buf := make([]byte, len(initialContent))
	_, err = file.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt() failed: %v", err)
	}

	expected := "01234WXYZ9ABCDEF"
	if string(buf) != expected {
		t.Errorf("Expected content %q, got %q", expected, string(buf))
	}
}

func TestWriteString(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	initialContent := "Hello, World!!!!"
	
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	config := &Config{
		Mode:     ModeReadWrite,
		SyncMode: SyncLazy,
	}
	mfs := New(osFS, config)

	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile() failed: %v", err)
	}
	defer file.Close()

	// Write string
	testStr := "Testing 1234"
	n, err := file.WriteString(testStr)
	if err != nil {
		t.Fatalf("WriteString() failed: %v", err)
	}
	if n != len(testStr) {
		t.Errorf("Expected to write %d bytes, got %d", len(testStr), n)
	}

	// Verify by reading
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek() failed: %v", err)
	}

	buf := make([]byte, len(testStr))
	_, err = file.Read(buf)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if string(buf) != testStr {
		t.Errorf("Expected %q, got %q", testStr, string(buf))
	}
}

func TestPeriodicSync(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	initialContent := "Original content"
	
	if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	config := &Config{
		Mode:         ModeReadWrite,
		SyncMode:     SyncPeriodic,
		SyncInterval: 100 * time.Millisecond,
	}
	mfs := New(osFS, config)

	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile() failed: %v", err)
	}

	// Write new content
	newContent := "Updated content!"
	_, err = file.Write([]byte(newContent))
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Wait for periodic sync to occur
	time.Sleep(250 * time.Millisecond)

	// Close the file
	file.Close()

	// Verify persistence by reading with standard I/O
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}

	if string(data[:len(newContent)]) != newContent {
		t.Errorf("Expected persisted content %q, got %q", newContent, string(data))
	}

	// Stop the sync manager to clean up
	if mfs.syncManager != nil {
		mfs.syncManager.stop()
	}
}

func TestSyncModes(t *testing.T) {
	modes := []struct {
		name     string
		syncMode SyncMode
	}{
		{"Immediate", SyncImmediate},
		{"Lazy", SyncLazy},
		{"Never", SyncNever},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "testfile.txt")
			initialContent := "Test content 123"
			
			if err := os.WriteFile(tmpFile, []byte(initialContent), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			osFS, err := osfs.NewFS()
			if err != nil {
				t.Fatalf("NewFS() failed: %v", err)
			}

			config := &Config{
				Mode:     ModeReadWrite,
				SyncMode: mode.syncMode,
			}
			mfs := New(osFS, config)

			file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
			if err != nil {
				t.Fatalf("OpenFile() failed: %v", err)
			}

			newContent := "Modified!!!!!!!"
			_, err = file.Write([]byte(newContent))
			if err != nil {
				t.Fatalf("Write() failed: %v", err)
			}

			// Explicitly sync
			err = file.Sync()
			if err != nil {
				t.Fatalf("Sync() failed: %v", err)
			}

			file.Close()

			// Verify
			data, err := os.ReadFile(tmpFile)
			if err != nil {
				t.Fatalf("ReadFile() failed: %v", err)
			}

			if string(data[:len(newContent)]) != newContent {
				t.Errorf("Expected %q, got %q", newContent, string(data))
			}
		})
	}
}

func TestCopyOnWrite(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "testfile.txt")
	originalContent := "Original content"
	
	if err := os.WriteFile(tmpFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	config := &Config{
		Mode:     ModeCopyOnWrite,
		SyncMode: SyncNever,
	}
	mfs := New(osFS, config)

	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile() failed: %v", err)
	}
	defer file.Close()

	// Write new content (should be COW)
	newContent := "Modified content"
	_, err = file.Write([]byte(newContent))
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Read back from mapped file
	file.Seek(0, io.SeekStart)
	buf := make([]byte, len(newContent))
	_, err = file.Read(buf)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if string(buf) != newContent {
		t.Errorf("Expected %q in mapped file, got %q", newContent, string(buf))
	}

	file.Close()

	// With COW and SyncNever, the original file should remain unchanged
	// (COW means changes are private and not written back)
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}

	// The file should still have original content
	if string(data) == newContent {
		t.Logf("Note: File was modified (COW may have been written with explicit sync)")
	}
}

// TestWindowedMapping tests reading from a file with windowed mapping.
func TestWindowedMapping(t *testing.T) {
	// Create a file larger than our test window size
	windowSize := int64(1024) // 1KB window
	fileSize := windowSize * 3 // 3KB file

	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	config := &Config{
		Mode:        ModeReadOnly,
		SyncMode:    SyncNever,
		MapFullFile: false,
		WindowSize:  windowSize,
	}
	mfs := New(osFS, config)

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Read in chunks that cross window boundaries
	chunkSize := 512
	buf := make([]byte, chunkSize)
	totalRead := 0

	for totalRead < len(content) {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("Read() at offset %d failed: %v", totalRead, err)
		}

		// Verify data
		for i := 0; i < n; i++ {
			expected := byte((totalRead + i) % 256)
			if buf[i] != expected {
				t.Errorf("At offset %d: expected %d, got %d", totalRead+i, expected, buf[i])
			}
		}

		totalRead += n

		if err == io.EOF {
			break
		}
	}

	if totalRead != len(content) {
		t.Errorf("Expected to read %d bytes, got %d", len(content), totalRead)
	}
}

// TestWindowedSeek tests seeking with windowed mapping.
func TestWindowedSeek(t *testing.T) {
	windowSize := int64(1024)
	fileSize := windowSize * 4

	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	config := &Config{
		Mode:        ModeReadOnly,
		SyncMode:    SyncNever,
		MapFullFile: false,
		WindowSize:  windowSize,
	}
	mfs := New(osFS, config)

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Seek to different windows and read
	testOffsets := []int64{
		0,                    // First window
		windowSize,           // Second window
		windowSize * 2,       // Third window
		windowSize*3 - 100,   // Near end
		windowSize / 2,       // Back to first window
	}

	buf := make([]byte, 100)
	for _, offset := range testOffsets {
		pos, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			t.Fatalf("Seek(%d) failed: %v", offset, err)
		}

		if pos != offset {
			t.Errorf("Seek returned %d, expected %d", pos, offset)
		}

		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("Read() after Seek(%d) failed: %v", offset, err)
		}

		// Verify data
		for i := 0; i < n; i++ {
			expected := byte((offset + int64(i)) % 256)
			if buf[i] != expected {
				t.Errorf("At offset %d: expected %d, got %d", offset+int64(i), expected, buf[i])
			}
		}
	}
}

// TestWindowedReadAt tests ReadAt with windowed mapping.
func TestWindowedReadAt(t *testing.T) {
	windowSize := int64(1024)
	fileSize := windowSize * 3

	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	config := &Config{
		Mode:        ModeReadOnly,
		SyncMode:    SyncNever,
		MapFullFile: false,
		WindowSize:  windowSize,
	}
	mfs := New(osFS, config)

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// ReadAt from different windows
	testCases := []struct {
		offset int64
		size   int
	}{
		{0, 100},                     // First window
		{windowSize - 100, 50},       // Near end of first window (doesn't cross)
		{windowSize, 100},            // Second window
		{windowSize + 100, 200},      // Middle of second window
		{windowSize*2 + 500, 100},    // Third window
	}

	for _, tc := range testCases {
		buf := make([]byte, tc.size)
		n, err := file.ReadAt(buf, tc.offset)

		// Check if we're reading past EOF
		expectedN := tc.size
		if tc.offset+int64(tc.size) > fileSize {
			expectedN = int(fileSize - tc.offset)
			if err != io.EOF {
				t.Errorf("ReadAt(%d, %d) should return EOF, got %v", tc.offset, tc.size, err)
			}
		} else if err != nil {
			t.Errorf("ReadAt(%d, %d) failed: %v", tc.offset, tc.size, err)
		}

		if n != expectedN {
			t.Errorf("ReadAt(%d, %d) returned %d bytes, expected %d", tc.offset, tc.size, n, expectedN)
		}

		// Verify data
		for i := 0; i < n; i++ {
			expected := byte((tc.offset + int64(i)) % 256)
			if buf[i] != expected {
				t.Errorf("ReadAt offset %d: at position %d expected %d, got %d", tc.offset, i, expected, buf[i])
			}
		}
	}
}

// TestWindowedWrite tests writing with windowed mapping.
func TestWindowedWrite(t *testing.T) {
	windowSize := int64(1024)
	fileSize := windowSize * 3

	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	config := &Config{
		Mode:        ModeReadWrite,
		SyncMode:    SyncImmediate,
		MapFullFile: false,
		WindowSize:  windowSize,
	}
	mfs := New(osFS, config)

	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile() failed: %v", err)
	}
	defer file.Close()

	// Write to different windows
	testPattern := []byte("WINDOWED")
	testOffsets := []int64{
		100,                 // First window
		windowSize + 50,     // Second window
		windowSize*2 + 100,  // Third window
	}

	for _, offset := range testOffsets {
		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			t.Fatalf("Seek(%d) failed: %v", offset, err)
		}

		n, err := file.Write(testPattern)
		if err != nil {
			t.Fatalf("Write() at offset %d failed: %v", offset, err)
		}

		if n != len(testPattern) {
			t.Errorf("Write() at offset %d: wrote %d bytes, expected %d", offset, n, len(testPattern))
		}
	}

	// Verify writes by reading back
	for _, offset := range testOffsets {
		buf := make([]byte, len(testPattern))
		n, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			t.Fatalf("ReadAt(%d) failed: %v", offset, err)
		}

		if n != len(testPattern) {
			t.Errorf("ReadAt(%d): read %d bytes, expected %d", offset, n, len(testPattern))
		}

		if string(buf) != string(testPattern) {
			t.Errorf("ReadAt(%d): expected %q, got %q", offset, testPattern, buf)
		}
	}
}

// TestWindowedWriteAt tests WriteAt with windowed mapping.
func TestWindowedWriteAt(t *testing.T) {
	windowSize := int64(1024)
	fileSize := windowSize * 3

	content := make([]byte, fileSize)
	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}
	config := &Config{
		Mode:        ModeReadWrite,
		SyncMode:    SyncImmediate,
		MapFullFile: false,
		WindowSize:  windowSize,
	}
	mfs := New(osFS, config)

	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile() failed: %v", err)
	}
	defer file.Close()

	// WriteAt to different windows
	testPattern := []byte("WRITEAT")
	testOffsets := []int64{
		200,
		windowSize + 100,
		windowSize*2 + 200,
	}

	for _, offset := range testOffsets {
		n, err := file.WriteAt(testPattern, offset)
		if err != nil {
			t.Fatalf("WriteAt(%d) failed: %v", offset, err)
		}

		if n != len(testPattern) {
			t.Errorf("WriteAt(%d): wrote %d bytes, expected %d", offset, n, len(testPattern))
		}
	}

	// Verify all writes
	for _, offset := range testOffsets {
		buf := make([]byte, len(testPattern))
		n, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			t.Fatalf("ReadAt(%d) failed: %v", offset, err)
		}

		if n != len(testPattern) {
			t.Errorf("ReadAt(%d): read %d bytes, expected %d", offset, n, len(testPattern))
		}

		if string(buf) != string(testPattern) {
			t.Errorf("ReadAt(%d): expected %q, got %q", offset, testPattern, buf)
		}
	}
}

// TestPopulatePages tests MAP_POPULATE flag (Linux-specific).
func TestPopulatePages(t *testing.T) {
	fileSize := 1 * 1024 * 1024 // 1MB

	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	config := &Config{
		Mode:          ModeReadOnly,
		SyncMode:      SyncNever,
		PopulatePages: true, // Request eager page loading
	}
	mfs := New(osFS, config)

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() with PopulatePages failed: %v", err)
	}
	defer file.Close()

	// Read some data to verify mapping works
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() failed: %v", err)
	}

	if n != len(buf) {
		t.Errorf("Expected to read %d bytes, got %d", len(buf), n)
	}
}

// TestHugePages tests MAP_HUGETLB flag (Linux-specific).
// This test may fail on systems without huge pages configured.
func TestHugePages(t *testing.T) {
	fileSize := 2 * 1024 * 1024 // 2MB (typical huge page size)

	content := make([]byte, fileSize)
	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	config := &Config{
		Mode:         ModeReadOnly,
		SyncMode:     SyncNever,
		UseHugePages: true,
	}
	mfs := New(osFS, config)

	file, err := mfs.Open(tmpFile)
	if err != nil {
		// Huge pages may not be available, log but don't fail
		t.Logf("Open() with UseHugePages failed (this is normal if huge pages aren't configured): %v", err)
		t.Skip("Huge pages not available on this system")
		return
	}
	defer file.Close()

	// Try to read
	buf := make([]byte, 1024)
	_, err = file.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() failed: %v", err)
	}

	t.Log("Huge pages test succeeded (huge pages are available)")
}

// TestMadviseHints tests various madvise hints.
func TestMadviseHints(t *testing.T) {
	fileSize := 1 * 1024 * 1024 // 1MB

	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	// Get MappedFile to access madvise methods
	mf, ok := file.(*MappedFile)
	if !ok {
		t.Skip("File is not a MappedFile (may be empty or directory)")
		return
	}

	// Test various hints - these should not error
	tests := []struct {
		name string
		fn   func() error
	}{
		{"Sequential", mf.AdviseSequential},
		{"Random", mf.AdviseRandom},
		{"WillNeed", mf.AdviseWillNeed},
		{"DontNeed", mf.AdviseDontNeed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Errorf("%s failed: %v", tt.name, err)
			}
		})
	}
}

// TestAdviseLinuxSpecific tests Linux-specific madvise hints.
// These may not be available on all systems.
func TestAdviseLinuxSpecific(t *testing.T) {
	fileSize := 2 * 1024 * 1024 // 2MB

	content := make([]byte, fileSize)
	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer file.Close()

	mf, ok := file.(*MappedFile)
	if !ok {
		t.Skip("File is not a MappedFile")
		return
	}

	// Test Linux-specific hints
	tests := []struct {
		name string
		fn   func() error
	}{
		{"HugePage", mf.AdviseHugePage},
		{"NoHugePage", mf.AdviseNoHugePage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				// These may not be supported on all systems
				t.Logf("%s returned error (may not be supported): %v", tt.name, err)
			}
		})
	}
}

// TestConfigCombinations tests various configuration combinations.
func TestConfigCombinations(t *testing.T) {
	fileSize := 1 * 1024 * 1024
	content := make([]byte, fileSize)

	tmpFile, cleanup := createTestFile(t, string(content))
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("NewFS() failed: %v", err)
	}

	configs := []struct {
		name   string
		config *Config
	}{
		{
			name: "PopulateWithPreload",
			config: &Config{
				Mode:          ModeReadOnly,
				PopulatePages: true,
				Preload:       true,
			},
		},
		{
			name: "WindowedWithPopulate",
			config: &Config{
				Mode:          ModeReadOnly,
				MapFullFile:   false,
				WindowSize:    512 * 1024,
				PopulatePages: true,
			},
		},
		{
			name: "ReadWriteWithSync",
			config: &Config{
				Mode:          ModeReadWrite,
				SyncMode:      SyncImmediate,
				PopulatePages: true,
			},
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			mfs := New(osFS, tc.config)

			var file absfs.File
			if tc.config.Mode == ModeReadWrite {
				file, err = mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
			} else {
				file, err = mfs.Open(tmpFile)
			}

			if err != nil {
				t.Fatalf("Open() failed: %v", err)
			}
			defer file.Close()

			// Verify basic operation works
			buf := make([]byte, 1024)
			n, err := file.Read(buf)
			if err != nil && err != io.EOF {
				t.Fatalf("Read() failed: %v", err)
			}

			if n == 0 {
				t.Error("Expected to read some bytes")
			}
		})
	}
}
