package memmapfs

import (
	"io"
	"os"
	"path/filepath"
	"testing"

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
