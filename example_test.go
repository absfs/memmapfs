package memmapfs_test

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/absfs/memmapfs"
	"github.com/absfs/osfs"
)

// Example demonstrates basic usage of memmapfs for reading a file.
func Example() {
	// Create a temporary test file
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello, memory-mapped filesystem!"
	os.WriteFile(tmpFile, []byte(content), 0644)

	// Create the underlying OS filesystem
	osFS, err := osfs.NewFS()
	if err != nil {
		log.Fatal(err)
	}

	// Wrap it with memmapfs
	mfs := memmapfs.New(osFS, memmapfs.DefaultConfig())

	// Open a file - it will be memory-mapped automatically
	file, err := mfs.Open(tmpFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Read from the memory-mapped file
	data := make([]byte, len(content))
	n, err := file.Read(data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Read %d bytes: %s\n", n, string(data))
	// Output: Read 32 bytes: Hello, memory-mapped filesystem!
}

// ExampleMappedFile_ReadAt demonstrates random access reading.
func ExampleMappedFile_ReadAt() {
	// Create a test file
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "data.bin")
	content := "0123456789ABCDEF"
	os.WriteFile(tmpFile, []byte(content), 0644)

	// Setup memmapfs
	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, memmapfs.DefaultConfig())

	file, _ := mfs.Open(tmpFile)
	defer file.Close()

	// Read from specific offset without seeking
	buf := make([]byte, 6)
	n, _ := file.ReadAt(buf, 10) // Read "ABCDEF" starting at offset 10

	fmt.Printf("Read %d bytes from offset 10: %s\n", n, string(buf))
	// Output: Read 6 bytes from offset 10: ABCDEF
}

// ExampleMappedFile_Seek demonstrates seeking within a memory-mapped file.
func ExampleMappedFile_Seek() {
	// Create a test file
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "data.txt")
	content := "Hello, World!"
	os.WriteFile(tmpFile, []byte(content), 0644)

	// Setup memmapfs
	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, memmapfs.DefaultConfig())

	file, _ := mfs.Open(tmpFile)
	defer file.Close()

	// Seek to offset 7
	file.Seek(7, io.SeekStart)

	// Read from new position
	buf := make([]byte, 6)
	n, _ := file.Read(buf)

	fmt.Printf("After seeking to 7, read %d bytes: %s\n", n, string(buf))
	// Output: After seeking to 7, read 6 bytes: World!
}

// ExampleNew_customConfig demonstrates using a custom configuration.
func ExampleNew_customConfig() {
	// Create custom configuration
	config := &memmapfs.Config{
		Mode:     memmapfs.ModeReadOnly,
		SyncMode: memmapfs.SyncNever,
		Preload:  false, // Don't preload pages
	}

	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, config)

	// Use mfs for file operations...
	fmt.Printf("Created memmapfs with custom config\n")
	// Output: Created memmapfs with custom config
	_ = mfs
}

// ExampleMappedFile_Data demonstrates direct access to mapped memory.
func ExampleMappedFile_Data() {
	// Create a test file
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "data.txt")
	content := "Direct memory access!"
	os.WriteFile(tmpFile, []byte(content), 0644)

	// Setup memmapfs
	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, memmapfs.DefaultConfig())

	file, _ := mfs.Open(tmpFile)
	defer file.Close()

	// Get direct access to mapped memory (zero-copy)
	mf := file.(*memmapfs.MappedFile)
	data := mf.Data()

	fmt.Printf("Direct access: %s\n", string(data))
	// Output: Direct access: Direct memory access!
}
