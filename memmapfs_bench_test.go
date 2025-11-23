package memmapfs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/absfs/osfs"
)

// setupBenchmarkFile creates a test file of the given size for benchmarking.
func setupBenchmarkFile(b *testing.B, size int) (string, func()) {
	b.Helper()

	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "benchmark.dat")

	// Create file with random-ish data
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		b.Fatalf("Failed to create benchmark file: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpFile, cleanup
}

// BenchmarkSequentialRead compares sequential read performance.
func BenchmarkSequentialRead(b *testing.B) {
	sizes := []int{
		4 * 1024,       // 4 KB
		64 * 1024,      // 64 KB
		1024 * 1024,    // 1 MB
		16 * 1024 * 1024, // 16 MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			b.Run("Standard", func(b *testing.B) {
				benchmarkStandardSequentialRead(b, size)
			})
			b.Run("MemMap", func(b *testing.B) {
				benchmarkMemMapSequentialRead(b, size)
			})
		})
	}
}

func benchmarkStandardSequentialRead(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	buf := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		file, err := os.Open(tmpFile)
		if err != nil {
			b.Fatal(err)
		}

		for {
			_, err := file.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}

		file.Close()
	}
}

func benchmarkMemMapSequentialRead(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}
	mfs := New(osFS, DefaultConfig())

	buf := make([]byte, 4096)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		file, err := mfs.Open(tmpFile)
		if err != nil {
			b.Fatal(err)
		}

		for {
			_, err := file.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}

		file.Close()
	}
}

// BenchmarkRandomRead compares random access read performance.
func BenchmarkRandomRead(b *testing.B) {
	sizes := []int{
		1024 * 1024,    // 1 MB
		16 * 1024 * 1024, // 16 MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			b.Run("Standard", func(b *testing.B) {
				benchmarkStandardRandomRead(b, size)
			})
			b.Run("MemMap", func(b *testing.B) {
				benchmarkMemMapRandomRead(b, size)
			})
		})
	}
}

func benchmarkStandardRandomRead(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	file, err := os.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	buf := make([]byte, 4096)
	offsets := []int64{0, 1024, 4096, 8192, 16384, 32768, 65536}

	b.ResetTimer()
	b.SetBytes(int64(len(offsets) * len(buf)))

	for i := 0; i < b.N; i++ {
		for _, offset := range offsets {
			if offset+int64(len(buf)) > int64(size) {
				continue
			}
			_, err := file.ReadAt(buf, offset)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func benchmarkMemMapRandomRead(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	buf := make([]byte, 4096)
	offsets := []int64{0, 1024, 4096, 8192, 16384, 32768, 65536}

	b.ResetTimer()
	b.SetBytes(int64(len(offsets) * len(buf)))

	for i := 0; i < b.N; i++ {
		for _, offset := range offsets {
			if offset+int64(len(buf)) > int64(size) {
				continue
			}
			_, err := file.ReadAt(buf, offset)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkSmallReads compares performance with small read sizes.
func BenchmarkSmallReads(b *testing.B) {
	size := 1024 * 1024 // 1 MB file
	readSizes := []int{16, 64, 256, 1024}

	for _, readSize := range readSizes {
		b.Run(formatSize(readSize), func(b *testing.B) {
			b.Run("Standard", func(b *testing.B) {
				benchmarkStandardSmallReads(b, size, readSize)
			})
			b.Run("MemMap", func(b *testing.B) {
				benchmarkMemMapSmallReads(b, size, readSize)
			})
		})
	}
}

func benchmarkStandardSmallReads(b *testing.B, fileSize, readSize int) {
	tmpFile, cleanup := setupBenchmarkFile(b, fileSize)
	defer cleanup()

	file, err := os.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	buf := make([]byte, readSize)

	b.ResetTimer()
	b.SetBytes(int64(readSize))

	for i := 0; i < b.N; i++ {
		file.Seek(0, io.SeekStart)
		_, err := file.Read(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkMemMapSmallReads(b *testing.B, fileSize, readSize int) {
	tmpFile, cleanup := setupBenchmarkFile(b, fileSize)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	buf := make([]byte, readSize)

	b.ResetTimer()
	b.SetBytes(int64(readSize))

	for i := 0; i < b.N; i++ {
		file.Seek(0, io.SeekStart)
		_, err := file.Read(buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSeek compares seek performance.
func BenchmarkSeek(b *testing.B) {
	size := 16 * 1024 * 1024 // 16 MB

	b.Run("Standard", func(b *testing.B) {
		benchmarkStandardSeek(b, size)
	})
	b.Run("MemMap", func(b *testing.B) {
		benchmarkMemMapSeek(b, size)
	})
}

func benchmarkStandardSeek(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	file, err := os.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	offsets := []int64{0, 1024, 4096, 1024 * 1024, 8 * 1024 * 1024}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, offset := range offsets {
			_, err := file.Seek(offset, io.SeekStart)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func benchmarkMemMapSeek(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	offsets := []int64{0, 1024, 4096, 1024 * 1024, 8 * 1024 * 1024}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, offset := range offsets {
			_, err := file.Seek(offset, io.SeekStart)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkReadAtParallel tests concurrent random access performance.
func BenchmarkReadAtParallel(b *testing.B) {
	size := 16 * 1024 * 1024 // 16 MB

	b.Run("Standard", func(b *testing.B) {
		benchmarkStandardReadAtParallel(b, size)
	})
	b.Run("MemMap", func(b *testing.B) {
		benchmarkMemMapReadAtParallel(b, size)
	})
}

func benchmarkStandardReadAtParallel(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	file, err := os.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	b.ResetTimer()
	b.SetBytes(4096)

	b.RunParallel(func(pb *testing.PB) {
		buf := make([]byte, 4096)
		offset := int64(0)

		for pb.Next() {
			_, err := file.ReadAt(buf, offset)
			if err != nil {
				b.Fatal(err)
			}
			offset = (offset + 4096) % int64(size-4096)
		}
	})
}

func benchmarkMemMapReadAtParallel(b *testing.B, size int) {
	tmpFile, cleanup := setupBenchmarkFile(b, size)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}
	mfs := New(osFS, DefaultConfig())

	file, err := mfs.Open(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	b.ResetTimer()
	b.SetBytes(4096)

	b.RunParallel(func(pb *testing.PB) {
		buf := make([]byte, 4096)
		offset := int64(0)

		for pb.Next() {
			_, err := file.ReadAt(buf, offset)
			if err != nil {
				b.Fatal(err)
			}
			offset = (offset + 4096) % int64(size-4096)
		}
	})
}

// formatSize formats a byte size for display in benchmark names.
func formatSize(size int) string {
	switch {
	case size >= 1024*1024:
		mb := size / (1024 * 1024)
		return fmt.Sprintf("%dMB", mb)
	case size >= 1024:
		kb := size / 1024
		return fmt.Sprintf("%dKB", kb)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

// BenchmarkWrite compares write performance.
func BenchmarkWrite(b *testing.B) {
	sizes := []int{
		4 * 1024,       // 4 KB
		64 * 1024,      // 64 KB
		1024 * 1024,    // 1 MB
		16 * 1024 * 1024, // 16 MB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			b.Run("Standard", func(b *testing.B) {
				benchmarkStandardWrite(b, size)
			})
			b.Run("MemMap", func(b *testing.B) {
				benchmarkMemMapWrite(b, size)
			})
		})
	}
}

func benchmarkStandardWrite(b *testing.B, size int) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "benchmark.dat")

	// Create initial file
	initialData := make([]byte, size)
	if err := os.WriteFile(tmpFile, initialData, 0644); err != nil {
		b.Fatal(err)
	}

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.SetBytes(4096)

	for i := 0; i < b.N; i++ {
		file, err := os.OpenFile(tmpFile, os.O_RDWR, 0644)
		if err != nil {
			b.Fatal(err)
		}

		_, err = file.Write(data)
		if err != nil {
			b.Fatal(err)
		}

		file.Close()
	}
}

func benchmarkMemMapWrite(b *testing.B, size int) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "benchmark.dat")

	// Create initial file
	initialData := make([]byte, size)
	if err := os.WriteFile(tmpFile, initialData, 0644); err != nil {
		b.Fatal(err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}

	config := &Config{
		Mode:     ModeReadWrite,
		SyncMode: SyncNever, // For fair comparison
	}
	mfs := New(osFS, config)

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.SetBytes(4096)

	for i := 0; i < b.N; i++ {
		file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
		if err != nil {
			b.Fatal(err)
		}

		_, err = file.Write(data)
		if err != nil {
			b.Fatal(err)
		}

		file.Close()
	}
}

// BenchmarkWriteAt compares random write performance.
func BenchmarkWriteAt(b *testing.B) {
	size := 16 * 1024 * 1024 // 16 MB

	b.Run("Standard", func(b *testing.B) {
		benchmarkStandardWriteAt(b, size)
	})
	b.Run("MemMap", func(b *testing.B) {
		benchmarkMemMapWriteAt(b, size)
	})
}

func benchmarkStandardWriteAt(b *testing.B, size int) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "benchmark.dat")

	// Create initial file
	initialData := make([]byte, size)
	if err := os.WriteFile(tmpFile, initialData, 0644); err != nil {
		b.Fatal(err)
	}

	file, err := os.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	offsets := []int64{0, 1024, 4096, 8192, 16384, 32768, 65536}

	b.ResetTimer()
	b.SetBytes(int64(len(offsets) * len(data)))

	for i := 0; i < b.N; i++ {
		for _, offset := range offsets {
			if offset+int64(len(data)) > int64(size) {
				continue
			}
			_, err := file.WriteAt(data, offset)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func benchmarkMemMapWriteAt(b *testing.B, size int) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "benchmark.dat")

	// Create initial file
	initialData := make([]byte, size)
	if err := os.WriteFile(tmpFile, initialData, 0644); err != nil {
		b.Fatal(err)
	}

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}

	config := &Config{
		Mode:     ModeReadWrite,
		SyncMode: SyncNever,
	}
	mfs := New(osFS, config)

	file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	offsets := []int64{0, 1024, 4096, 8192, 16384, 32768, 65536}

	b.ResetTimer()
	b.SetBytes(int64(len(offsets) * len(data)))

	for i := 0; i < b.N; i++ {
		for _, offset := range offsets {
			if offset+int64(len(data)) > int64(size) {
				continue
			}
			_, err := file.WriteAt(data, offset)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkSyncModes compares different sync strategies.
func BenchmarkSyncModes(b *testing.B) {
	modes := []struct {
		name string
		mode SyncMode
	}{
		{"Immediate", SyncImmediate},
		{"Lazy", SyncLazy},
		{"Never", SyncNever},
	}

	for _, m := range modes {
		b.Run(m.name, func(b *testing.B) {
			tmpDir := b.TempDir()
			tmpFile := filepath.Join(tmpDir, "benchmark.dat")

			size := 1024 * 1024 // 1 MB
			initialData := make([]byte, size)
			if err := os.WriteFile(tmpFile, initialData, 0644); err != nil {
				b.Fatal(err)
			}

			osFS, err := osfs.NewFS()
			if err != nil {
				b.Fatal(err)
			}

			config := &Config{
				Mode:     ModeReadWrite,
				SyncMode: m.mode,
			}
			mfs := New(osFS, config)

			data := make([]byte, 4096)
			for i := range data {
				data[i] = byte(i % 256)
			}

			b.ResetTimer()
			b.SetBytes(4096)

			for i := 0; i < b.N; i++ {
				file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
				if err != nil {
					b.Fatal(err)
				}

				_, err = file.Write(data)
				if err != nil {
					b.Fatal(err)
				}

				file.Close()
			}
		})
	}
}
// BenchmarkWindowedSequentialRead benchmarks sequential reading with windowed mapping.
func BenchmarkWindowedSequentialRead(b *testing.B) {
	// Use a file larger than the window size
	fileSize := 16 * 1024 * 1024 // 16 MB
	windowSize := int64(4 * 1024 * 1024) // 4 MB window

	tmpFile, cleanup := setupBenchmarkFile(b, fileSize)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}

	b.Run("FullFile", func(b *testing.B) {
		config := &Config{
			Mode:        ModeReadOnly,
			SyncMode:    SyncNever,
			MapFullFile: true,
		}
		mfs := New(osFS, config)

		file, err := mfs.Open(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
		defer file.Close()

		buf := make([]byte, 4096)
		b.ResetTimer()
		b.SetBytes(int64(fileSize))

		for i := 0; i < b.N; i++ {
			file.Seek(0, io.SeekStart)
			bytesRead := 0
			for bytesRead < fileSize {
				n, err := file.Read(buf)
				if err != nil && err != io.EOF {
					b.Fatal(err)
				}
				bytesRead += n
				if err == io.EOF {
					break
				}
			}
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		config := &Config{
			Mode:        ModeReadOnly,
			SyncMode:    SyncNever,
			MapFullFile: false,
			WindowSize:  windowSize,
		}
		mfs := New(osFS, config)

		file, err := mfs.Open(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
		defer file.Close()

		buf := make([]byte, 4096)
		b.ResetTimer()
		b.SetBytes(int64(fileSize))

		for i := 0; i < b.N; i++ {
			file.Seek(0, io.SeekStart)
			bytesRead := 0
			for bytesRead < fileSize {
				n, err := file.Read(buf)
				if err != nil && err != io.EOF {
					b.Fatal(err)
				}
				bytesRead += n
				if err == io.EOF {
					break
				}
			}
		}
	})
}

// BenchmarkWindowedRandomRead benchmarks random access with windowed mapping.
func BenchmarkWindowedRandomRead(b *testing.B) {
	fileSize := 16 * 1024 * 1024 // 16 MB
	windowSize := int64(4 * 1024 * 1024) // 4 MB window

	tmpFile, cleanup := setupBenchmarkFile(b, fileSize)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}

	// Pre-generate random offsets
	numReads := 1000
	offsets := make([]int64, numReads)
	for i := range offsets {
		offsets[i] = int64((i * 7919) % (fileSize - 4096))
	}

	b.Run("FullFile", func(b *testing.B) {
		config := &Config{
			Mode:        ModeReadOnly,
			SyncMode:    SyncNever,
			MapFullFile: true,
		}
		mfs := New(osFS, config)

		file, err := mfs.Open(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
		defer file.Close()

		buf := make([]byte, 4096)
		b.ResetTimer()
		b.SetBytes(4096 * int64(numReads))

		for i := 0; i < b.N; i++ {
			for _, offset := range offsets {
				_, err := file.ReadAt(buf, offset)
				if err != nil && err != io.EOF {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		config := &Config{
			Mode:        ModeReadOnly,
			SyncMode:    SyncNever,
			MapFullFile: false,
			WindowSize:  windowSize,
		}
		mfs := New(osFS, config)

		file, err := mfs.Open(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
		defer file.Close()

		buf := make([]byte, 4096)
		b.ResetTimer()
		b.SetBytes(4096 * int64(numReads))

		for i := 0; i < b.N; i++ {
			for _, offset := range offsets {
				_, err := file.ReadAt(buf, offset)
				if err != nil && err != io.EOF {
					b.Fatal(err)
				}
			}
		}
	})
}

// BenchmarkWindowedWrite benchmarks writing with windowed mapping.
func BenchmarkWindowedWrite(b *testing.B) {
	fileSize := 16 * 1024 * 1024 // 16 MB
	windowSize := int64(4 * 1024 * 1024) // 4 MB window

	tmpFile, cleanup := setupBenchmarkFile(b, fileSize)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}

	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.Run("FullFile", func(b *testing.B) {
		config := &Config{
			Mode:        ModeReadWrite,
			SyncMode:    SyncLazy,
			MapFullFile: true,
		}
		mfs := New(osFS, config)

		b.ResetTimer()
		b.SetBytes(4096)

		for i := 0; i < b.N; i++ {
			file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
			if err != nil {
				b.Fatal(err)
			}

			offset := int64((i * 7919) % (fileSize - 4096))
			_, err = file.WriteAt(data, offset)
			if err != nil {
				b.Fatal(err)
			}

			file.Close()
		}
	})

	b.Run("Windowed", func(b *testing.B) {
		config := &Config{
			Mode:        ModeReadWrite,
			SyncMode:    SyncLazy,
			MapFullFile: false,
			WindowSize:  windowSize,
		}
		mfs := New(osFS, config)

		b.ResetTimer()
		b.SetBytes(4096)

		for i := 0; i < b.N; i++ {
			file, err := mfs.OpenFile(tmpFile, os.O_RDWR, 0644)
			if err != nil {
				b.Fatal(err)
			}

			offset := int64((i * 7919) % (fileSize - 4096))
			_, err = file.WriteAt(data, offset)
			if err != nil {
				b.Fatal(err)
			}

			file.Close()
		}
	})
}

// BenchmarkWindowSize benchmarks different window sizes for sequential access.
func BenchmarkWindowSize(b *testing.B) {
	fileSize := 16 * 1024 * 1024 // 16 MB

	tmpFile, cleanup := setupBenchmarkFile(b, fileSize)
	defer cleanup()

	osFS, err := osfs.NewFS()
	if err != nil {
		b.Fatal(err)
	}

	windowSizes := []int64{
		1 * 1024 * 1024,  // 1 MB
		4 * 1024 * 1024,  // 4 MB
		8 * 1024 * 1024,  // 8 MB
		16 * 1024 * 1024, // 16 MB (full file)
	}

	for _, windowSize := range windowSizes {
		b.Run(formatSize(int(windowSize)), func(b *testing.B) {
			config := &Config{
				Mode:        ModeReadOnly,
				SyncMode:    SyncNever,
				MapFullFile: false,
				WindowSize:  windowSize,
			}
			mfs := New(osFS, config)

			file, err := mfs.Open(tmpFile)
			if err != nil {
				b.Fatal(err)
			}
			defer file.Close()

			buf := make([]byte, 4096)
			b.ResetTimer()
			b.SetBytes(int64(fileSize))

			for i := 0; i < b.N; i++ {
				file.Seek(0, io.SeekStart)
				bytesRead := 0
				for bytesRead < fileSize {
					n, err := file.Read(buf)
					if err != nil && err != io.EOF {
						b.Fatal(err)
					}
					bytesRead += n
					if err == io.EOF {
						break
					}
				}
			}
		})
	}
}
