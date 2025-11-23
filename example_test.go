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

// ExampleNew_windowedMapping demonstrates using windowed mapping for large files.
func ExampleNew_windowedMapping() {
	// Create a large test file
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "large.dat")
	// Create a 10MB file
	largeData := make([]byte, 10*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	os.WriteFile(tmpFile, largeData, 0644)

	// Configure windowed mapping with 2MB window
	config := &memmapfs.Config{
		Mode:        memmapfs.ModeReadOnly,
		SyncMode:    memmapfs.SyncNever,
		MapFullFile: false,          // Use windowed mapping
		WindowSize:  2 * 1024 * 1024, // 2MB window
	}

	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, config)

	// Open large file - only 2MB will be mapped at a time
	file, _ := mfs.Open(tmpFile)
	defer file.Close()

	// Sequential reading automatically slides the window
	buf := make([]byte, 4096)
	totalRead := 0
	for {
		n, err := file.Read(buf)
		totalRead += n
		if err == io.EOF {
			break
		}
	}

	fmt.Printf("Read %d bytes from large file using windowed mapping\n", totalRead)
	// Output: Read 10485760 bytes from large file using windowed mapping
}

// ExampleMappedFile_windowedSeek demonstrates seeking across windows.
func ExampleMappedFile_windowedSeek() {
	// Create a large test file
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "large.dat")
	largeData := make([]byte, 10*1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	os.WriteFile(tmpFile, largeData, 0644)

	// Configure windowed mapping
	config := &memmapfs.Config{
		Mode:        memmapfs.ModeReadOnly,
		SyncMode:    memmapfs.SyncNever,
		MapFullFile: false,
		WindowSize:  2 * 1024 * 1024,
	}

	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, config)

	file, _ := mfs.Open(tmpFile)
	defer file.Close()

	// Seek to different positions - window will slide automatically
	offsets := []int64{0, 5 * 1024 * 1024, 9 * 1024 * 1024}
	for _, offset := range offsets {
		file.Seek(offset, io.SeekStart)
		buf := make([]byte, 10)
		n, _ := file.Read(buf)
		fmt.Printf("At offset %d: read %d bytes\n", offset, n)
	}
	// Output:
	// At offset 0: read 10 bytes
	// At offset 5242880: read 10 bytes
	// At offset 9437184: read 10 bytes
}

// ExampleNew_windowedWrite demonstrates writing with windowed mapping.
func ExampleNew_windowedWrite() {
	// Create a test file
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "output.dat")
	// Create initial file
	data := make([]byte, 5*1024*1024) // 5MB
	os.WriteFile(tmpFile, data, 0644)

	// Configure windowed mapping for writing
	config := &memmapfs.Config{
		Mode:        memmapfs.ModeReadWrite,
		SyncMode:    memmapfs.SyncImmediate,
		MapFullFile: false,
		WindowSize:  1 * 1024 * 1024, // 1MB window
	}

	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, config)

	file, _ := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	defer file.Close()

	// Write to different positions - window will slide
	pattern := []byte("WINDOW")
	offsets := []int64{0, 2 * 1024 * 1024, 4 * 1024 * 1024}

	for _, offset := range offsets {
		file.WriteAt(pattern, offset)
	}

	fmt.Printf("Wrote to %d positions using windowed mapping\n", len(offsets))
	// Output: Wrote to 3 positions using windowed mapping
}

// ExampleConfig_performanceTuning demonstrates performance tuning options.
func ExampleConfig_performanceTuning() {
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "data.bin")
	data := make([]byte, 5*1024*1024) // 5MB
	os.WriteFile(tmpFile, data, 0644)

	osFS, _ := osfs.NewFS()

	// Configuration for maximum performance (database-like workload)
	highPerfConfig := &memmapfs.Config{
		Mode:          memmapfs.ModeReadWrite,
		SyncMode:      memmapfs.SyncLazy,       // Sync only on close
		PopulatePages: true,                    // Eagerly load all pages
		MapFullFile:   true,                    // Map entire file
		Preload:       false,                   // PopulatePages is more aggressive
	}

	mfs := memmapfs.New(osFS, highPerfConfig)
	file, _ := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	defer file.Close()

	// Access pattern hints for further optimization
	if mf, ok := file.(*memmapfs.MappedFile); ok {
		// Hint: random access pattern
		mf.AdviseRandom()
	}

	fmt.Printf("File opened with high-performance configuration\n")
	// Output: File opened with high-performance configuration
}

// ExampleConfig_memoryOptimized demonstrates memory-optimized configuration.
func ExampleConfig_memoryOptimized() {
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "large.dat")
	data := make([]byte, 100*1024*1024) // 100MB
	os.WriteFile(tmpFile, data, 0644)

	osFS, _ := osfs.NewFS()

	// Configuration optimized for low memory usage
	memOptConfig := &memmapfs.Config{
		Mode:          memmapfs.ModeReadOnly,
		SyncMode:      memmapfs.SyncNever,
		PopulatePages: false,                   // Don't preload
		MapFullFile:   false,                   // Use windowing
		WindowSize:    10 * 1024 * 1024,        // 10MB window
		Preload:       false,
	}

	mfs := memmapfs.New(osFS, memOptConfig)
	file, _ := mfs.Open(tmpFile)
	defer file.Close()

	// Hint: sequential access to help OS optimize
	if mf, ok := file.(*memmapfs.MappedFile); ok {
		mf.AdviseSequential()
	}

	fmt.Printf("File opened with memory-optimized configuration\n")
	// Output: File opened with memory-optimized configuration
}

// ExampleMappedFile_hugePages demonstrates using huge pages for large files.
func ExampleMappedFile_hugePages() {
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "huge.dat")
	// Create file >= 2MB (typical huge page size)
	data := make([]byte, 4*1024*1024) // 4MB
	os.WriteFile(tmpFile, data, 0644)

	osFS, _ := osfs.NewFS()

	// Try to use huge pages for better TLB performance
	hugePagesConfig := &memmapfs.Config{
		Mode:          memmapfs.ModeReadOnly,
		UseHugePages:  true, // Request huge pages (Linux-specific)
		PopulatePages: true, // Preload for immediate access
	}

	mfs := memmapfs.New(osFS, hugePagesConfig)
	file, _ := mfs.Open(tmpFile)
	if file != nil {
		defer file.Close()

		// Further hint to use transparent huge pages
		if mf, ok := file.(*memmapfs.MappedFile); ok {
			mf.AdviseHugePage()
		}

		fmt.Printf("File opened with huge pages support\n")
	} else {
		fmt.Printf("Huge pages not available (requires system configuration)\n")
	}
	// Output: File opened with huge pages support
}

// ExampleMappedFile_accessPatterns demonstrates access pattern optimization.
func ExampleMappedFile_accessPatterns() {
	tmpDir, _ := os.MkdirTemp("", "memmapfs-example")
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "data.bin")
	data := make([]byte, 10*1024*1024) // 10MB
	os.WriteFile(tmpFile, data, 0644)

	osFS, _ := osfs.NewFS()
	mfs := memmapfs.New(osFS, memmapfs.DefaultConfig())

	file, _ := mfs.Open(tmpFile)
	defer file.Close()

	mf, _ := file.(*memmapfs.MappedFile)

	// Example 1: Sequential scan
	mf.AdviseSequential() // OS can readahead aggressively
	buf := make([]byte, 4096)
	for i := 0; i < 100; i++ {
		file.Read(buf)
	}

	// Example 2: Random access
	file.Seek(0, io.SeekStart)
	mf.AdviseRandom() // OS disables readahead
	offsets := []int64{1000, 50000, 10000, 80000}
	for _, offset := range offsets {
		file.ReadAt(buf, offset)
	}

	// Example 3: Done with data
	mf.AdviseDontNeed() // Allow OS to reclaim pages

	fmt.Printf("Applied access pattern hints\n")
	// Output: Applied access pattern hints
}
