# memmapfs

Memory-mapped file wrapper for absfs with zero-copy I/O and efficient random access.

## Overview and Purpose

memmapfs provides a high-performance filesystem wrapper for absfs that uses memory-mapped file access. By mapping files directly into the process address space, it eliminates buffer copies and provides fast random access to file contents. This implementation wraps existing filesystems (typically osfs) and transparently uses memory mapping for improved performance on supported operations.

## What is Memory Mapping?

Memory mapping is an operating system feature that maps file contents directly into a process's virtual address space. Instead of reading file data into buffers with traditional I/O operations, the OS maps file pages into memory. When the application accesses these memory addresses:

1. The OS loads the relevant file pages from disk (if not already cached)
2. The application accesses data directly through memory addresses
3. Modifications to mapped memory can be synced back to the file
4. The OS page cache manages physical memory allocation

This approach provides several key advantages:
- **Zero-copy I/O**: No intermediate buffers required
- **Fast random access**: Direct memory addressing without seek operations
- **Efficient sparse access**: Only accessed pages are loaded into memory
- **OS-managed caching**: Integrates with the kernel's page cache
- **Shared memory**: Multiple processes can map the same file

## Use Cases

### Large File Access with Minimal Memory Footprint
- Process multi-gigabyte files without loading entire file into RAM
- Access only the portions needed, letting the OS manage paging
- Ideal for log file analysis, large datasets, and binary processing

### Random Access Patterns
- Database files (B-trees, hash tables, indexes)
- Binary search in sorted files
- Sparse data access patterns
- No seek overhead for jumping between file positions

### Shared Memory Between Processes
- Multiple processes can map the same file with MAP_SHARED
- Changes visible across all processes
- Useful for inter-process communication
- Lock-free data structures in shared memory

### Zero-Copy I/O Operations
- Direct memory access without buffer allocation
- Reduced CPU usage for data movement
- Lower memory overhead
- Faster data processing pipelines

## Architecture Design

### Core Components

#### Wrapper Architecture
```
Application
    ↓
memmapfs (memory-mapped operations)
    ↓
Underlying FS (typically osfs)
    ↓
Operating System (mmap/MapViewOfFile)
    ↓
Disk Storage
```

The memmapfs wrapper:
- Implements the absfs.FS interface
- Delegates non-mmap operations to underlying filesystem
- Manages memory mapping lifecycle for opened files
- Provides configuration for mapping behavior

#### Memory Mapping Strategy

**Unix/Linux (mmap syscall)**:
```go
addr := mmap(
    nil,                    // Let kernel choose address
    length,                 // Size to map
    PROT_READ|PROT_WRITE,  // Protection flags
    MAP_SHARED,             // Sharing mode
    fd,                     // File descriptor
    offset                  // File offset (page-aligned)
)
```

**Windows (MapViewOfFile)**:
```go
handle := CreateFileMapping(
    fileHandle,             // File handle
    nil,                    // Security attributes
    PAGE_READWRITE,         // Protection
    sizeHigh,               // Size high 32 bits
    sizeLow,                // Size low 32 bits
    nil                     // Name
)
view := MapViewOfFile(
    handle,                 // Mapping handle
    FILE_MAP_ALL_ACCESS,    // Access mode
    offsetHigh,             // Offset high 32 bits
    offsetLow,              // Offset low 32 bits
    length                  // Bytes to map
)
```

#### Lazy Loading
- Files are mapped when opened with appropriate flags
- Actual pages are loaded on first access (page fault)
- OS handles demand paging automatically
- Memory pressure causes automatic page eviction

#### Automatic Sync/Flush Strategies

**Immediate Sync (MAP_SYNC + msync)**:
- Every write immediately synced to disk
- Highest durability, lowest performance
- Use for critical data integrity requirements

**Periodic Sync**:
- Background goroutine calls msync at intervals
- Balances durability and performance
- Configurable interval (e.g., every 5 seconds)

**Lazy Sync (on-close)**:
- Sync only when file is closed
- Highest performance, relies on OS caching
- Risk of data loss on crash before close

**Write-through via OS**:
- Let OS handle sync with normal page cache behavior
- No explicit msync calls
- OS schedules writeback based on system policy

## Platform Support

### Linux (mmap)
- Full support for all mmap features
- Advanced flags: MAP_POPULATE, MAP_HUGETLB
- Performance hints: madvise, fadvise
- Non-blocking I/O with MAP_POPULATE

### macOS (mmap)
- POSIX mmap implementation
- Supports MAP_PRIVATE, MAP_SHARED, MAP_ANON
- madvise for access pattern hints
- F_NOCACHE fcntl for bypassing cache

### Windows (MapViewOfFile)
- Equivalent functionality via Windows API
- CreateFileMapping + MapViewOfFile
- FlushViewOfFile for sync
- Page protection via PAGE_* constants

## Implementation Details

### File Handle Management

```go
type MappedFile struct {
    // Underlying file from wrapped filesystem
    file      absfs.File
    fd        uintptr        // OS file descriptor

    // Memory mapping
    data      []byte         // Mapped memory region
    mapping   uintptr        // Platform-specific mapping handle
    size      int64          // Mapped size
    offset    int64          // File offset of mapping

    // Configuration
    mode      MappingMode    // Read-only, read-write, COW
    syncMode  SyncMode       // Immediate, periodic, lazy

    // State
    position  int64          // Current read/write position
    modified  bool           // Track if writes occurred
    mu        sync.RWMutex   // Protect concurrent access
}
```

### Memory Mapping Lifecycle

#### 1. Map (Open)
```go
func (fs *MemMapFS) Open(name string) (absfs.File, error) {
    // Open underlying file
    file, err := fs.underlying.Open(name)

    // Get file descriptor
    fd := getFD(file)

    // Get file size
    size, err := file.Stat()

    // Perform mmap
    data, err := unix.Mmap(
        int(fd),
        0,              // offset
        int(size),      // length
        unix.PROT_READ, // protection
        unix.MAP_SHARED // flags
    )

    return &MappedFile{
        file: file,
        fd:   fd,
        data: data,
        size: size,
    }, nil
}
```

#### 2. Access (Read/Write)
```go
func (mf *MappedFile) Read(p []byte) (int, error) {
    mf.mu.RLock()
    defer mf.mu.RUnlock()

    // Direct memory copy from mapped region
    n := copy(p, mf.data[mf.position:])
    mf.position += int64(n)

    if n < len(p) {
        return n, io.EOF
    }
    return n, nil
}

func (mf *MappedFile) Write(p []byte) (int, error) {
    mf.mu.Lock()
    defer mf.mu.Unlock()

    // Direct memory copy to mapped region
    n := copy(mf.data[mf.position:], p)
    mf.position += int64(n)
    mf.modified = true

    // Sync if immediate mode
    if mf.syncMode == SyncImmediate {
        mf.syncLocked()
    }

    return n, nil
}
```

#### 3. Unmap (Close)
```go
func (mf *MappedFile) Close() error {
    mf.mu.Lock()
    defer mf.mu.Unlock()

    // Sync if modified
    if mf.modified {
        if err := mf.syncLocked(); err != nil {
            return err
        }
    }

    // Unmap memory
    if err := unix.Munmap(mf.data); err != nil {
        return err
    }

    // Close underlying file
    return mf.file.Close()
}
```

### Read/Write Operations Via Mapped Memory

All I/O operations work directly on the mapped memory region:
- **Read**: `copy()` from mapped memory to user buffer
- **Write**: `copy()` from user buffer to mapped memory
- **Seek**: Update position pointer (no syscall)
- **ReadAt/WriteAt**: Direct offset-based access

### Sync Strategies

#### Immediate (msync after every write)
```go
func (mf *MappedFile) syncLocked() error {
    return unix.Msync(mf.data, unix.MS_SYNC)
}
```

#### Periodic (background sync goroutine)
```go
func (fs *MemMapFS) startPeriodicSync() {
    ticker := time.NewTicker(fs.syncInterval)
    go func() {
        for range ticker.C {
            fs.syncAllFiles()
        }
    }()
}
```

#### Lazy (sync on close only)
```go
// No explicit msync during writes
// Only sync in Close() if modified
```

### Page Fault Handling

When accessing unmapped or evicted pages:
1. CPU generates page fault
2. OS kernel handles fault transparently
3. Kernel loads page from disk into memory
4. Kernel updates page table
5. Instruction is retried and succeeds

Application code doesn't need explicit page fault handling - the OS manages this automatically.

### Memory Pressure Handling

Under memory pressure:
1. OS evicts clean pages (can be reloaded from disk)
2. Dirty pages are written back before eviction
3. Access to evicted pages causes automatic reload
4. Applications can provide hints via madvise

```go
// Hint that pages will be needed soon
unix.Madvise(data, unix.MADV_WILLNEED)

// Hint that pages won't be needed soon
unix.Madvise(data, unix.MADV_DONTNEED)

// Hint for sequential access
unix.Madvise(data, unix.MADV_SEQUENTIAL)

// Hint for random access
unix.Madvise(data, unix.MADV_RANDOM)
```

## Configuration Options

### Map Size

**Full File Mapping**:
```go
type Config struct {
    MapFullFile bool  // Map entire file
}
```

**Windowed Mapping** (for files larger than available address space):
```go
type Config struct {
    WindowSize  int64  // Size of mapping window (e.g., 1GB)
    WindowSlide bool   // Automatically slide window on access
}
```

### Access Mode

```go
type MappingMode int

const (
    ModeReadOnly  MappingMode = iota  // PROT_READ, MAP_PRIVATE
    ModeReadWrite                      // PROT_READ|PROT_WRITE, MAP_SHARED
    ModeCopyOnWrite                    // PROT_READ|PROT_WRITE, MAP_PRIVATE
)
```

**Read-Only**:
- `PROT_READ` protection
- Safe for concurrent reads
- Attempts to write cause SIGSEGV

**Read-Write**:
- `PROT_READ | PROT_WRITE` protection
- Changes visible to all mappings (MAP_SHARED)
- Requires write permission on file

**Copy-On-Write**:
- `PROT_READ | PROT_WRITE` with MAP_PRIVATE
- Writes create private copies (not visible to other processes)
- Useful for modifying data without affecting original file

### Sync Mode

```go
type SyncMode int

const (
    SyncImmediate SyncMode = iota  // msync after every write
    SyncPeriodic                    // msync at regular intervals
    SyncLazy                        // msync only on close
    SyncNever                       // Let OS handle sync
)
```

### Preload Hint

```go
type Config struct {
    Preload       bool   // MAP_POPULATE or madvise(MADV_WILLNEED)
    PreloadAsync  bool   // Non-blocking preload
}
```

**MAP_POPULATE** (Linux):
- Preload all pages at map time
- Blocks until pages are loaded
- Avoids page faults during access

**madvise(MADV_WILLNEED)**:
- Hint to preload pages
- Non-blocking
- OS may ignore hint under memory pressure

## Implementation Phases

### Phase 1: Basic Read-Only mmap Support
**Goals**:
- Implement core memmapfs wrapper around osfs
- Unix/Linux mmap support for read-only files
- Basic file operations (Open, Read, Seek, Close)
- Full file mapping only

**Deliverables**:
- `MemMapFS` type implementing absfs.FS
- `MappedFile` type implementing absfs.File
- Read-only mmap on Unix platforms
- Unit tests for basic operations
- Benchmarks vs standard file I/O

### Phase 2: Read-Write with Sync
**Goals**:
- Add write support with MAP_SHARED
- Implement sync strategies (immediate, periodic, lazy)
- Handle dirty page tracking
- Add Windows support

**Deliverables**:
- Write operations on mapped memory
- msync/FlushViewOfFile integration
- Configurable sync modes
- Windows implementation (CreateFileMapping, MapViewOfFile)
- Tests for data durability

### Phase 3: Windowed Mapping for Large Files
**Goals**:
- Support files larger than address space
- Sliding window mechanism
- Automatic window management on access
- Performance optimization for large files

**Deliverables**:
- Windowed mapping implementation
- Automatic window sliding on sequential access
- Configurable window size
- Tests with multi-gigabyte files
- Large file benchmarks

### Phase 4: Platform-Specific Optimizations
**Goals**:
- Advanced Linux features (MAP_POPULATE, MAP_HUGETLB)
- macOS optimizations (F_NOCACHE, madvise)
- Windows optimizations (large pages, FILE_FLAG_NO_BUFFERING)
- Platform-specific tuning

**Deliverables**:
- MAP_POPULATE support for preloading
- madvise hints for access patterns
- Huge page support (when available)
- Platform-specific configuration options
- Performance tuning guide

### Phase 5: Advanced Features
**Goals**:
- Memory advice API (MADV_SEQUENTIAL, MADV_RANDOM, etc.)
- Copy-on-write mode (MAP_PRIVATE)
- Shared memory IPC utilities
- Advanced error handling (SIGBUS recovery)

**Deliverables**:
- Full madvise API exposure
- MAP_PRIVATE support for COW semantics
- SIGBUS signal handling for truncated files
- SharedMemory helper utilities
- Complete feature documentation

## Performance Characteristics

### Zero-Copy Reads
- No intermediate buffer allocation
- Direct memory access via `copy()` or direct slice access
- Eliminates memory allocation overhead
- Reduced garbage collector pressure

**Traditional I/O**:
```
File → Kernel Buffer → User Buffer → Application
```

**Memory-Mapped I/O**:
```
File → Shared Page Cache → Application (direct access)
```

### Fast Random Access
- No `lseek()` syscall overhead
- Position tracking in memory only
- Immediate access to any file offset
- Ideal for database-style workloads

**Benchmark Expectations**:
- Random reads: 10-100x faster than traditional I/O
- Sequential reads: Similar or slightly faster
- Seeks: Near-zero cost (memory operation only)

### OS Page Cache Integration
- Mapped pages use the kernel's unified page cache
- Multiple processes share physical pages
- Efficient memory usage across system
- OS manages eviction under memory pressure

### Memory Efficiency for Sparse Access
- Only accessed pages consume physical memory
- Virtual address space is cheap
- Perfect for large files with sparse access
- Example: 1TB file mapped, only 100MB accessed → 100MB RAM used

### Performance Comparison

| Operation | Traditional I/O | Memory-Mapped I/O | Improvement |
|-----------|----------------|-------------------|-------------|
| Sequential Read | Good | Similar | ~1x |
| Random Read | Poor (seek overhead) | Excellent | 10-100x |
| Small Reads | Moderate (syscall overhead) | Excellent | 5-50x |
| Large Sequential | Excellent | Excellent | ~1x |
| Seek | Moderate (syscall) | Instant | 1000x |
| Memory Usage (sparse) | Fixed buffer size | Only accessed pages | Varies |

## Go Libraries to Use

### golang.org/x/sys/unix
Primary library for Unix/Linux mmap operations:

```go
import "golang.org/x/sys/unix"

// Mmap
data, err := unix.Mmap(
    fd,                          // File descriptor
    offset,                      // File offset (page-aligned)
    length,                      // Number of bytes to map
    unix.PROT_READ|unix.PROT_WRITE, // Protection
    unix.MAP_SHARED,             // Flags
)

// Msync
err = unix.Msync(data, unix.MS_SYNC)

// Madvise
err = unix.Madvise(data, unix.MADV_SEQUENTIAL)

// Munmap
err = unix.Munmap(data)
```

### golang.org/x/sys/windows
Windows equivalent for memory mapping:

```go
import "golang.org/x/sys/windows"

// CreateFileMapping
handle, err := windows.CreateFileMapping(
    fileHandle,
    nil,                           // Security attributes
    windows.PAGE_READWRITE,        // Protection
    uint32(size >> 32),            // High 32 bits of size
    uint32(size),                  // Low 32 bits of size
    nil,                           // Name
)

// MapViewOfFile
addr, err := windows.MapViewOfFile(
    handle,
    windows.FILE_MAP_READ|windows.FILE_MAP_WRITE,
    uint32(offset >> 32),          // High 32 bits of offset
    uint32(offset),                // Low 32 bits of offset
    uintptr(length),
)

// FlushViewOfFile
err = windows.FlushViewOfFile(addr, uintptr(length))

// UnmapViewOfFile
err = windows.UnmapViewOfFile(addr)

// CloseHandle
err = windows.CloseHandle(handle)
```

### Optional: github.com/edsrzf/mmap-go
Cross-platform abstraction layer (evaluate for use):

```go
import "github.com/edsrzf/mmap-go"

// Map file
mmap, err := mmap.Map(file, mmap.RDWR, 0)

// Access data
data := mmap[offset:offset+length]

// Flush
err = mmap.Flush()

// Unmap
err = mmap.Unmap()
```

**Consideration**: Evaluate whether to use mmap-go for abstraction or implement platform-specific code directly for better control and optimization opportunities.

## Usage Examples

### Database File Access

```go
// Open database file with memory mapping
fs := memmapfs.New(osfs.New(), &memmapfs.Config{
    Mode:     memmapfs.ModeReadWrite,
    SyncMode: memmapfs.SyncPeriodic,
    SyncInterval: 5 * time.Second,
})

db, err := fs.Open("data.db")
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// Fast random access to index
index := make([]byte, 8)
_, err = db.ReadAt(index, indexOffset)

// Direct access to data pages
data := make([]byte, pageSize)
_, err = db.ReadAt(data, pageOffset)
```

### Log File Analysis

```go
// Map large log file for analysis
fs := memmapfs.New(osfs.New(), &memmapfs.Config{
    Mode:     memmapfs.ModeReadOnly,
    Preload:  false, // Don't preload entire file
})

log, err := fs.Open("app.log")
if err != nil {
    log.Fatal(err)
}
defer log.Close()

// Scan through file efficiently
scanner := bufio.NewScanner(log)
for scanner.Scan() {
    // Process lines - only accessed pages loaded
    line := scanner.Text()
    if strings.Contains(line, "ERROR") {
        fmt.Println(line)
    }
}
```

### Large Binary File Processing

```go
// Process large binary file with minimal memory
fs := memmapfs.New(osfs.New(), &memmapfs.Config{
    Mode:       memmapfs.ModeReadOnly,
    WindowSize: 1 << 30, // 1GB window
})

file, err := fs.Open("large-dataset.bin")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// Process in chunks
chunk := make([]byte, 1<<20) // 1MB chunks
for {
    n, err := file.Read(chunk)
    if err == io.EOF {
        break
    }
    processChunk(chunk[:n])
}
```

### Shared Memory IPC

```go
// Process A: Create and write
fs := memmapfs.New(osfs.New(), &memmapfs.Config{
    Mode:     memmapfs.ModeReadWrite,
    SyncMode: memmapfs.SyncImmediate,
})

shared, err := fs.OpenFile("shared.dat", os.O_RDWR|os.O_CREATE, 0644)
if err != nil {
    log.Fatal(err)
}
shared.Write(data)
shared.Close()

// Process B: Read shared data
fs2 := memmapfs.New(osfs.New(), &memmapfs.Config{
    Mode: memmapfs.ModeReadOnly,
})

shared2, err := fs2.Open("shared.dat")
if err != nil {
    log.Fatal(err)
}
data := make([]byte, 1024)
shared2.Read(data)
shared2.Close()
```

## Comparison with Standard File I/O

| Aspect | Standard I/O | Memory-Mapped I/O |
|--------|-------------|-------------------|
| **Buffer Management** | Manual buffer allocation | OS-managed page cache |
| **Random Access** | Requires lseek() syscalls | Direct memory addressing |
| **Memory Footprint** | Fixed buffer sizes | Demand-paged, only accessed data |
| **Copy Operations** | Read: file→kernel→user<br>Write: user→kernel→file | Direct memory access, zero-copy |
| **Concurrency** | Requires locking around syscalls | Lock-free reads possible |
| **Latency** | Syscall overhead per operation | Page fault on first access only |
| **Large Files** | Works with any size | May need windowing for huge files |
| **Simplicity** | Well-understood model | Requires understanding of virtual memory |
| **Error Handling** | Traditional errno | SIGBUS on truncation/errors |
| **Portability** | Universal | Platform-specific APIs |

**When to Use Standard I/O**:
- Small files read once sequentially
- Streaming data (network, pipes)
- Simple, portable code requirements
- File size unknown or highly variable

**When to Use Memory-Mapped I/O**:
- Large files with random access patterns
- Database files, indexes, caches
- High-performance requirements
- Shared memory between processes
- Memory-efficient sparse access

## Testing Strategy

### Unit Tests
- Test all absfs.FS interface methods
- Test all absfs.File interface methods
- Verify read-only, read-write, and COW modes
- Test sync strategies (immediate, periodic, lazy)
- Test error conditions (permission denied, truncated files)

### Integration Tests
- Test with various underlying filesystems (osfs, memfs)
- Test concurrent access from multiple goroutines
- Test file size edge cases (empty, tiny, huge)
- Test platform-specific code paths (Unix, Windows)

### Performance Tests
- Benchmark vs standard file I/O
- Measure random access performance
- Measure sequential access performance
- Test memory usage with large files
- Profile page fault behavior

### Stress Tests
- Test with files larger than RAM
- Test under memory pressure
- Test with many concurrent mapped files
- Test long-running processes with periodic sync

### Platform Tests
- Linux: Test MAP_POPULATE, MAP_HUGETLB, madvise
- macOS: Test macOS-specific flags and behavior
- Windows: Test MapViewOfFile, large pages, flush behavior
- Cross-platform: Verify consistent behavior across platforms

### SIGBUS Testing
- Test access to truncated files
- Test recovery mechanisms
- Test error reporting to application

## Security Considerations

### MAP_SHARED vs MAP_PRIVATE

**MAP_SHARED (Shared Mapping)**:
- Modifications visible to all processes
- Changes written to underlying file
- Use for IPC and persistent data
- **Security**: Other processes can see changes immediately
- **Risk**: Accidental modification affects all consumers

**MAP_PRIVATE (Copy-On-Write)**:
- Modifications private to process
- Changes not written to file
- Use for read-modify-process workflows
- **Security**: Isolates modifications from other processes
- **Risk**: Unexpected memory usage on writes (COW copies)

### File Permission Considerations

- **Read-only mapping**: Requires file read permission
- **Read-write mapping**: Requires file write permission
- **Mapping mode must match file open mode**
- **Check permissions before mapping**

```go
// Safe permission checking
fi, err := os.Stat(filename)
if err != nil {
    return err
}

mode := fi.Mode()
if mode.Perm() & 0200 == 0 {
    return errors.New("file not writable, cannot use read-write mapping")
}
```

### SIGBUS and Data Integrity

**SIGBUS Causes**:
- File truncated while mapped
- I/O error reading from disk
- Access beyond mapped region

**Mitigation**:
```go
// Install SIGBUS handler (Unix)
sigc := make(chan os.Signal, 1)
signal.Notify(sigc, unix.SIGBUS)

go func() {
    <-sigc
    log.Error("SIGBUS received - file may be truncated or I/O error occurred")
    // Perform cleanup
}()
```

### Memory Disclosure Risks

- **Risk**: Sensitive data in mapped memory could be swapped to disk
- **Mitigation**: Use mlock() to prevent swapping (requires privileges)
- **Alternative**: Use MAP_PRIVATE and avoid MAP_SHARED for sensitive data

```go
// Prevent swapping of sensitive data (requires CAP_IPC_LOCK)
err := unix.Mlock(data)
if err != nil {
    return fmt.Errorf("failed to lock memory: %w", err)
}
defer unix.Munlock(data)
```

### Concurrent Access Safety

- **Multiple readers**: Safe with MAP_PRIVATE or proper synchronization
- **Concurrent writers**: Requires application-level locking
- **Cross-process**: Use file locks (flock/fcntl) for coordination

```go
// Example: File locking for cross-process safety
err := unix.Flock(int(fd), unix.LOCK_EX) // Exclusive lock
if err != nil {
    return err
}
defer unix.Flock(int(fd), unix.LOCK_UN) // Unlock
```

## Technical Specifications

### mmap Syscall Parameters and Flags

#### Unix/Linux Flags

**Protection Flags** (prot):
- `PROT_NONE`: Pages cannot be accessed
- `PROT_READ`: Pages can be read
- `PROT_WRITE`: Pages can be written
- `PROT_EXEC`: Pages can be executed

**Mapping Flags** (flags):
- `MAP_SHARED`: Share mapping with other processes, changes written to file
- `MAP_PRIVATE`: Private copy-on-write mapping, changes not written to file
- `MAP_FIXED`: Map at exact address (dangerous, avoid)
- `MAP_ANONYMOUS`: Map not backed by file (private memory)
- `MAP_POPULATE`: Populate page tables, preload pages (Linux)
- `MAP_NONBLOCK`: Don't block on page faults during mapping (Linux)
- `MAP_LOCKED`: Lock pages in memory (Linux, requires privileges)
- `MAP_HUGETLB`: Use huge pages (Linux, requires configuration)
- `MAP_SYNC`: Synchronous page faults (Linux 4.15+, for DAX)

**Example**:
```go
data, err := unix.Mmap(
    fd,
    0,
    fileSize,
    unix.PROT_READ|unix.PROT_WRITE,
    unix.MAP_SHARED|unix.MAP_POPULATE,
)
```

#### Windows Flags

**Page Protection**:
- `PAGE_NOACCESS`: No access
- `PAGE_READONLY`: Read-only access
- `PAGE_READWRITE`: Read and write access
- `PAGE_WRITECOPY`: Copy-on-write access

**Access Flags**:
- `FILE_MAP_READ`: Read access
- `FILE_MAP_WRITE`: Write access
- `FILE_MAP_ALL_ACCESS`: All access
- `FILE_MAP_COPY`: Copy-on-write

**Example**:
```go
mapping, err := windows.CreateFileMapping(
    handle,
    nil,
    windows.PAGE_READWRITE,
    0,
    0,
    nil,
)

view, err := windows.MapViewOfFile(
    mapping,
    windows.FILE_MAP_READ|windows.FILE_MAP_WRITE,
    0,
    0,
    0,
)
```

### Page Size Alignment Requirements

#### Why Alignment Matters
- MMU (Memory Management Unit) works with fixed-size pages
- Mappings must start on page boundaries
- Offset and address must be page-aligned

#### Platform Page Sizes
- **x86/x86_64**: 4KB (4096 bytes) standard, 2MB/1GB huge pages
- **ARM**: 4KB, 16KB, or 64KB (varies by platform)
- **PowerPC**: 4KB or 64KB

#### Getting Page Size
```go
// Unix
pageSize := unix.Getpagesize() // Returns 4096 on most systems

// Portable
pageSize := os.Getpagesize()
```

#### Alignment Functions
```go
// Align offset down to page boundary
func alignDown(offset int64, pageSize int) int64 {
    return offset & ^(int64(pageSize) - 1)
}

// Align size up to page boundary
func alignUp(size int64, pageSize int) int64 {
    return (size + int64(pageSize) - 1) & ^(int64(pageSize) - 1)
}

// Example usage
offset := int64(5000)
size := int64(8192)
pageSize := 4096

alignedOffset := alignDown(offset, pageSize)  // 4096
alignedSize := alignUp(size, pageSize)        // 8192
```

### File Descriptor Lifecycle Management

#### Obtaining File Descriptors

**Unix**:
```go
// From os.File
type fdGetter interface {
    Fd() uintptr
}

file, _ := os.Open("file.dat")
fd := file.Fd() // Returns uintptr

// Direct open
fd, err := unix.Open("file.dat", unix.O_RDWR, 0644)
```

**Windows**:
```go
// From os.File
file, _ := os.Open("file.dat")
handle := windows.Handle(file.Fd())

// Direct open
handle, err := windows.CreateFile(
    filename,
    windows.GENERIC_READ|windows.GENERIC_WRITE,
    windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
    nil,
    windows.OPEN_EXISTING,
    windows.FILE_ATTRIBUTE_NORMAL,
    0,
)
```

#### Lifecycle Rules

1. **File must remain open while mapped**
   - Don't close file descriptor while memory is mapped
   - Mapping holds reference to file

2. **Unmap before closing**
   ```go
   // Correct order
   unix.Munmap(data)
   file.Close()

   // Incorrect - undefined behavior
   file.Close()
   unix.Munmap(data)
   ```

3. **File descriptor is duplicated on mmap**
   - Closing original fd doesn't invalidate mapping
   - But best practice: keep original open until unmap

4. **Multiple mappings of same file**
   ```go
   // Same file, multiple mappings allowed
   data1, _ := unix.Mmap(fd, 0, size, prot, flags)
   data2, _ := unix.Mmap(fd, 0, size, prot, flags)
   // Both mappings valid, may share pages if MAP_SHARED
   ```

### Error Handling

#### SIGBUS on Truncated Files

**When SIGBUS Occurs**:
- File truncated while mapped (mapped size > actual file size)
- I/O error reading from disk
- Access to mapped region beyond file

**Handling Strategy**:
```go
// 1. Install signal handler
func init() {
    sigc := make(chan os.Signal, 1)
    signal.Notify(sigc, unix.SIGBUS)

    go func() {
        for sig := range sigc {
            handleSIGBUS(sig)
        }
    }()
}

// 2. Track mapped files
type MappedFile struct {
    // ... fields
    sigbusHandler func()
}

// 3. Handle SIGBUS
func handleSIGBUS(sig os.Signal) {
    // Log error
    log.Error("SIGBUS received - file I/O error or truncation")

    // Attempt recovery
    // - Remap file with current size
    // - Return error to application
    // - Graceful degradation
}

// 4. Prevent SIGBUS by checking size
func (mf *MappedFile) checkSize() error {
    fi, err := mf.file.Stat()
    if err != nil {
        return err
    }

    if fi.Size() < mf.size {
        return errors.New("file truncated while mapped")
    }

    return nil
}
```

#### mmap Error Codes

**Unix errno Values**:
- `EACCES`: File not open for required access
- `EAGAIN`: File locked or too many mappings
- `EBADF`: Invalid file descriptor
- `EINVAL`: Invalid argument (misaligned offset, invalid flags)
- `ENFILE`: System-wide open file limit
- `ENODEV`: File system doesn't support mmap
- `ENOMEM`: Not enough memory to map
- `EOVERFLOW`: Mapping size too large
- `EPERM`: Operation not permitted (e.g., PROT_EXEC on noexec mount)

**Windows Error Codes**:
- `ERROR_ACCESS_DENIED`: Insufficient permissions
- `ERROR_INVALID_HANDLE`: Invalid file handle
- `ERROR_NOT_ENOUGH_MEMORY`: Insufficient memory
- `ERROR_DISK_FULL`: Not enough disk space

### Memory Unmapping Cleanup

#### Unmap Process
```go
func (mf *MappedFile) Close() error {
    var errs []error

    // 1. Sync dirty pages if modified
    if mf.modified {
        if err := unix.Msync(mf.data, unix.MS_SYNC); err != nil {
            errs = append(errs, fmt.Errorf("msync: %w", err))
        }
    }

    // 2. Unmap memory
    if err := unix.Munmap(mf.data); err != nil {
        errs = append(errs, fmt.Errorf("munmap: %w", err))
    }

    // 3. Close file descriptor
    if err := mf.file.Close(); err != nil {
        errs = append(errs, fmt.Errorf("close: %w", err))
    }

    if len(errs) > 0 {
        return fmt.Errorf("close errors: %v", errs)
    }

    return nil
}
```

#### Cleanup on Error
```go
func (fs *MemMapFS) Open(name string) (absfs.File, error) {
    file, err := fs.underlying.Open(name)
    if err != nil {
        return nil, err
    }

    // Get size
    fi, err := file.Stat()
    if err != nil {
        file.Close() // Cleanup
        return nil, err
    }

    // Mmap
    fd := getFD(file)
    data, err := unix.Mmap(int(fd), 0, int(fi.Size()), prot, flags)
    if err != nil {
        file.Close() // Cleanup
        return nil, fmt.Errorf("mmap: %w", err)
    }

    // Success
    return &MappedFile{
        file: file,
        data: data,
        size: fi.Size(),
    }, nil
}
```

#### Partial Unmap (Windowing)
```go
// Unmap old window, map new window
func (mf *MappedFile) slideWindow(newOffset int64) error {
    // Unmap current window
    if err := unix.Munmap(mf.data); err != nil {
        return err
    }

    // Map new window
    data, err := unix.Mmap(
        int(mf.fd),
        newOffset,
        int(mf.windowSize),
        mf.prot,
        mf.flags,
    )
    if err != nil {
        return err
    }

    mf.data = data
    mf.offset = newOffset

    return nil
}
```

### Cross-Platform Compatibility Layer

#### Abstraction Interface
```go
// Platform-independent mapping interface
type Mapping interface {
    Map(fd uintptr, offset, length int64, prot, flags int) ([]byte, error)
    Sync(data []byte, flags int) error
    Unmap(data []byte) error
    Advise(data []byte, advice int) error
}

// Unix implementation
type UnixMapping struct{}

func (m *UnixMapping) Map(fd uintptr, offset, length int64, prot, flags int) ([]byte, error) {
    return unix.Mmap(int(fd), offset, int(length), prot, flags)
}

func (m *UnixMapping) Sync(data []byte, flags int) error {
    return unix.Msync(data, flags)
}

// Windows implementation
type WindowsMapping struct {
    handles map[uintptr]windows.Handle
}

func (m *WindowsMapping) Map(fd uintptr, offset, length int64, prot, flags int) ([]byte, error) {
    // Convert Unix flags to Windows flags
    winProt := toWindowsProtection(prot)
    winAccess := toWindowsAccess(flags)

    // CreateFileMapping
    handle, err := windows.CreateFileMapping(...)

    // MapViewOfFile
    view, err := windows.MapViewOfFile(...)

    // Convert to []byte
    data := unsafe.Slice((*byte)(unsafe.Pointer(view)), length)
    return data, nil
}
```

#### Platform Detection
```go
// +build unix

package memmapfs

type platformMapping = UnixMapping

// +build windows

package memmapfs

type platformMapping = WindowsMapping
```

### Read vs Write Mapping Modes

#### Mode Matrix

| Mode | Protection | Flags | Use Case |
|------|------------|-------|----------|
| Read-Only | PROT_READ | MAP_PRIVATE or MAP_SHARED | Safe concurrent reads |
| Read-Write Shared | PROT_READ\|PROT_WRITE | MAP_SHARED | Database files, IPC |
| Read-Write Private | PROT_READ\|PROT_WRITE | MAP_PRIVATE | Process local modifications |
| Execute | PROT_READ\|PROT_EXEC | MAP_PRIVATE | Loading shared libraries |

#### Mode Implementation
```go
type MappingMode int

const (
    ModeReadOnly MappingMode = iota
    ModeReadWrite
    ModeCopyOnWrite
    ModeExecute
)

func (mode MappingMode) toProtFlags() int {
    switch mode {
    case ModeReadOnly:
        return unix.PROT_READ
    case ModeReadWrite:
        return unix.PROT_READ | unix.PROT_WRITE
    case ModeCopyOnWrite:
        return unix.PROT_READ | unix.PROT_WRITE
    case ModeExecute:
        return unix.PROT_READ | unix.PROT_EXEC
    default:
        return unix.PROT_READ
    }
}

func (mode MappingMode) toMapFlags() int {
    switch mode {
    case ModeReadOnly, ModeReadWrite:
        return unix.MAP_SHARED
    case ModeCopyOnWrite, ModeExecute:
        return unix.MAP_PRIVATE
    default:
        return unix.MAP_SHARED
    }
}
```

### Sync Strategies

#### msync Syscall
```go
// Unix msync flags
const (
    MS_ASYNC      = unix.MS_ASYNC      // Asynchronous sync
    MS_SYNC       = unix.MS_SYNC       // Synchronous sync
    MS_INVALIDATE = unix.MS_INVALIDATE // Invalidate cached data
)

// Sync all pages
func (mf *MappedFile) Sync() error {
    return unix.Msync(mf.data, unix.MS_SYNC)
}

// Async sync (initiate writeback, don't wait)
func (mf *MappedFile) SyncAsync() error {
    return unix.Msync(mf.data, unix.MS_ASYNC)
}

// Sync specific range
func (mf *MappedFile) SyncRange(offset, length int64) error {
    if offset+length > int64(len(mf.data)) {
        return errors.New("range out of bounds")
    }
    return unix.Msync(mf.data[offset:offset+length], unix.MS_SYNC)
}
```

#### MAP_SYNC (DAX)
```go
// Linux 4.15+ with DAX (Direct Access)
// Provides synchronous page faults for persistent memory

data, err := unix.Mmap(
    fd,
    0,
    size,
    unix.PROT_READ|unix.PROT_WRITE,
    unix.MAP_SHARED_VALIDATE|unix.MAP_SYNC,
)

// With MAP_SYNC:
// - Writes are persistent on page fault
// - No need for explicit msync
// - Requires DAX-capable filesystem (ext4, xfs with dax option)
// - Requires persistent memory hardware or DAX emulation
```

#### Windows FlushViewOfFile
```go
// Flush dirty pages to disk
err := windows.FlushViewOfFile(
    uintptr(unsafe.Pointer(&data[0])),
    uintptr(len(data)),
)

// Flush specific range
err := windows.FlushViewOfFile(
    uintptr(unsafe.Pointer(&data[offset])),
    uintptr(length),
)
```

## License

MIT License - See LICENSE file

## Contributing

Contributions welcome! Please ensure:
- All tests pass on Linux, macOS, and Windows
- Benchmarks included for performance-critical code
- Documentation updated for API changes
- Platform-specific code properly isolated with build tags

## Related Projects

- [absfs](https://github.com/absfs/absfs) - Core filesystem abstraction
- [osfs](https://github.com/absfs/osfs) - Operating system filesystem wrapper
- [memfs](https://github.com/absfs/memfs) - In-memory filesystem
- [cachefs](https://github.com/absfs/cachefs) - Caching filesystem wrapper
