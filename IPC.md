# Inter-Process Communication (IPC) Guide

This guide covers using memmapfs for inter-process communication through shared memory-mapped files.

## Table of Contents

1. [Overview](#overview)
2. [SharedMemory API](#sharedmemory-api)
3. [SIGBUS Protection](#sigbus-protection)
4. [Best Practices](#best-practices)
5. [Examples](#examples)

## Overview

Memory-mapped files provide an efficient mechanism for inter-process communication (IPC). Multiple processes can map the same file into their address spaces, allowing them to share data with zero-copy semantics.

### Advantages

- **Zero-copy**: No data copying between processes
- **Fast**: Direct memory access, no syscalls for data access
- **Simple**: Just read/write to memory
- **Persistent**: Data survives process crashes
- **Large data**: Can share gigabytes of data efficiently

### Limitations

- **Synchronization required**: No built-in locks (use external synchronization)
- **Platform-dependent**: Behavior varies across OS
- **File-backed**: Requires filesystem space
- **No built-in messaging**: Need to implement protocols on top

## SharedMemory API

The `SharedMemory` type provides a high-level API for creating and using shared memory regions.

### Creating Shared Memory

```go
config := &memmapfs.SharedMemoryConfig{
    Path:          "/tmp/myapp-shared.dat",
    Size:          1024 * 1024, // 1MB
    Mode:          memmapfs.ModeReadWrite,
    SyncMode:      memmapfs.SyncImmediate,
    PopulatePages: true, // Preload pages
    Permissions:   0644,
}

sm, err := memmapfs.CreateSharedMemory(config)
if err != nil {
    log.Fatal(err)
}
defer sm.Close()
```

### Opening Existing Shared Memory

```go
// Open for reading
sm, err := memmapfs.OpenSharedMemory("/tmp/myapp-shared.dat", false)

// Open for writing
sm, err := memmapfs.OpenSharedMemory("/tmp/myapp-shared.dat", true)
```

### Accessing Data

```go
// Get direct access to shared memory
data := sm.Data()

// Write data
copy(data, []byte("Hello from process A"))

// Read data
message := make([]byte, 100)
copy(message, data)

// Sync to ensure visibility
sm.Sync()
```

### Cleanup

```go
// Close but keep file
sm.Close()

// Close and delete file
sm.Remove()
```

## SharedMemory Configuration

### Path

The filesystem path where the shared memory file will be created:

```go
config.Path = "/tmp/myapp-shared.dat"
```

**Recommendations**:
- Use `/tmp` for temporary shared memory
- Use `/dev/shm` on Linux for true RAM-based shared memory (tmpfs)
- Use unique names to avoid conflicts

### Size

Size of the shared memory region in bytes:

```go
config.Size = 1024 * 1024 // 1MB
```

**Recommendations**:
- Pre-allocate exact size needed
- Consider page alignment (multiples of 4KB)
- Larger sizes may benefit from huge pages

### Mode

Access mode for the mapping:

```go
// Read-write shared (changes visible to all processes)
config.Mode = memmapfs.ModeReadWrite

// Read-only
config.Mode = memmapfs.ModeReadOnly

// Copy-on-write (changes private to process)
config.Mode = memmapfs.ModeCopyOnWrite
```

**For IPC**: Use `ModeReadWrite` with `MAP_SHARED`

### Sync Mode

Controls when changes are written to disk:

```go
// Immediate sync (maximum visibility, lower performance)
config.SyncMode = memmapfs.SyncImmediate

// Lazy sync (higher performance, manual sync required)
config.SyncMode = memmapfs.SyncLazy

// Let OS handle sync
config.SyncMode = memmapfs.SyncNever
```

**For IPC**: Use `SyncImmediate` if you need guarantees, or `SyncLazy` with manual `Sync()` calls

### PopulatePages

Eagerly load all pages into RAM:

```go
config.PopulatePages = true
```

**Benefits**:
- Eliminates page faults during access
- Predictable performance
- Good for real-time requirements

**Trade-offs**:
- Slower initialization
- Higher memory usage upfront

## SIGBUS Protection

SIGBUS signals occur when accessing memory-mapped files that have been truncated or have I/O errors.

### Enabling Protection

```go
file, _ := mfs.Open("data.dat")
mf := file.(*memmapfs.MappedFile)

// Enable SIGBUS monitoring
mf.EnableSIGBUSProtection()
defer mf.DisableSIGBUSProtection()
```

### Handling SIGBUS

Register a handler to respond to SIGBUS events:

```go
handler := memmapfs.GetSIGBUSHandler()

handler.OnSIGBUS(func(mf *memmapfs.MappedFile, err error) {
    log.Printf("SIGBUS detected: %v", err)

    // Attempt recovery
    if err := mf.RemapAfterTruncation(); err != nil {
        log.Printf("Recovery failed: %v", err)
    } else {
        log.Printf("Successfully remapped file")
    }
})
```

### What Causes SIGBUS

1. **File Truncation**: File is truncated while mapped
2. **I/O Errors**: Disk errors when reading pages
3. **Out of Bounds**: Accessing beyond mapped region

### Recovery Strategies

#### Automatic Remap

```go
// Attempt to remap with current file size
if err := mf.RemapAfterTruncation(); err != nil {
    // Recovery failed, handle error
}
```

#### Graceful Degradation

```go
handler.OnSIGBUS(func(mf *memmapfs.MappedFile, err error) {
    // Log error
    log.Error("File became invalid:", err)

    // Notify application
    notifyError(err)

    // Close file gracefully
    mf.DisableSIGBUSProtection()
    mf.Close()
})
```

## Best Practices

### 1. Synchronization

Memory-mapped files don't provide built-in synchronization. Use external mechanisms:

**File Locks (fcntl/flock)**:
```go
import "golang.org/x/sys/unix"

// Acquire exclusive lock
fd := sm.MappedFile().Fd()
err := unix.Flock(int(fd), unix.LOCK_EX)
defer unix.Flock(int(fd), unix.LOCK_UN)

// Access shared data
data := sm.Data()
// ...
```

**Atomic Operations**:
```go
import "sync/atomic"

// Treat data as atomic values
data := sm.Data()
val := (*uint64)(unsafe.Pointer(&data[0]))

// Atomic read
atomic.LoadUint64(val)

// Atomic write
atomic.StoreUint64(val, 42)
```

**Mutex in Shared Memory** (advanced):
```go
// Place a mutex at the start of shared memory
type SharedData struct {
    mu    sync.Mutex // WARNING: Only works within same process!
    value int64
}

// For cross-process, need OS-level mutexes (pthread_mutex, etc.)
```

### 2. Data Layout

Design your shared memory layout carefully:

```go
// Define structure at compile time
type SharedBuffer struct {
    Version   uint32   // Protocol version
    Size      uint32   // Data size
    Flags     uint64   // Status flags
    Reserved  [16]byte // Future use
    Data      [1024 * 1024]byte // Actual data
}

// Map and use
sm, _ := memmapfs.CreateSharedMemory(config)
buf := (*SharedBuffer)(unsafe.Pointer(&sm.Data()[0]))

buf.Version = 1
buf.Size = 100
copy(buf.Data[:], []byte("payload"))
```

### 3. Versioning

Include version information for compatibility:

```go
const SharedMemVersion = 2

type Header struct {
    Magic   [4]byte // "MYAP"
    Version uint32
    // ...
}

// Writer
copy(header.Magic[:], []byte("MYAP"))
header.Version = SharedMemVersion

// Reader
if string(header.Magic[:]) != "MYAP" {
    return errors.New("invalid shared memory")
}
if header.Version != SharedMemVersion {
    return errors.New("version mismatch")
}
```

### 4. Error Handling

Always handle errors and implement recovery:

```go
sm, err := memmapfs.OpenSharedMemory(path, false)
if err != nil {
    if os.IsNotExist(err) {
        // Create if doesn't exist
        config := &memmapfs.SharedMemoryConfig{
            Path: path,
            Size: 1024,
        }
        sm, err = memmapfs.CreateSharedMemory(config)
    }
    if err != nil {
        return err
    }
}
```

### 5. Cleanup

Ensure proper cleanup to avoid resource leaks:

```go
// Use defer for automatic cleanup
sm, _ := memmapfs.CreateSharedMemory(config)
defer sm.Close()

// Or explicit cleanup with error handling
if err := sm.Close(); err != nil {
    log.Printf("Failed to close shared memory: %v", err)
}

// Remove file when done
defer sm.Remove()
```

### 6. Testing IPC

Test with actual multiple processes:

```go
func TestIPC(t *testing.T) {
    // Create shared memory in parent
    sm, _ := memmapfs.CreateSharedMemory(config)
    defer sm.Remove()

    // Write test data
    copy(sm.Data(), []byte("test message"))
    sm.Sync()

    // Fork child process (or use exec.Command)
    cmd := exec.Command("./child-process", sm.Path())
    if err := cmd.Run(); err != nil {
        t.Fatal(err)
    }
}
```

## Examples

### Example 1: Simple Message Queue

```go
package main

import (
    "encoding/binary"
    "github.com/absfs/memmapfs"
)

type MessageQueue struct {
    sm   *memmapfs.SharedMemory
    head uint64 // Read position
    tail uint64 // Write position
}

func NewMessageQueue(path string, size int64) (*MessageQueue, error) {
    config := &memmapfs.SharedMemoryConfig{
        Path:     path,
        Size:     size,
        Mode:     memmapfs.ModeReadWrite,
        SyncMode: memmapfs.SyncImmediate,
    }

    sm, err := memmapfs.CreateSharedMemory(config)
    if err != nil {
        return nil, err
    }

    return &MessageQueue{sm: sm}, nil
}

func (mq *MessageQueue) Write(msg []byte) error {
    data := mq.sm.Data()

    // Write length
    binary.LittleEndian.PutUint32(data[mq.tail:], uint32(len(msg)))
    mq.tail += 4

    // Write message
    copy(data[mq.tail:], msg)
    mq.tail += uint64(len(msg))

    return mq.sm.Sync()
}

func (mq *MessageQueue) Read() ([]byte, error) {
    data := mq.sm.Data()

    // Read length
    length := binary.LittleEndian.Uint32(data[mq.head:])
    mq.head += 4

    // Read message
    msg := make([]byte, length)
    copy(msg, data[mq.head:])
    mq.head += uint64(length)

    return msg, nil
}
```

### Example 2: Shared Counter

```go
package main

import (
    "sync/atomic"
    "unsafe"
    "github.com/absfs/memmapfs"
)

type SharedCounter struct {
    sm *memmapfs.SharedMemory
}

func NewSharedCounter(path string) (*SharedCounter, error) {
    config := &memmapfs.SharedMemoryConfig{
        Path: path,
        Size: 8, // uint64
        Mode: memmapfs.ModeReadWrite,
    }

    sm, err := memmapfs.CreateSharedMemory(config)
    if err != nil {
        return nil, err
    }

    return &SharedCounter{sm: sm}, nil
}

func (sc *SharedCounter) Increment() uint64 {
    data := sc.sm.Data()
    counter := (*uint64)(unsafe.Pointer(&data[0]))
    return atomic.AddUint64(counter, 1)
}

func (sc *SharedCounter) Get() uint64 {
    data := sc.sm.Data()
    counter := (*uint64)(unsafe.Pointer(&data[0]))
    return atomic.LoadUint64(counter)
}
```

### Example 3: Configuration Sharing

```go
package main

import (
    "encoding/json"
    "github.com/absfs/memmapfs"
)

type AppConfig struct {
    DatabaseURL string `json:"database_url"`
    CacheSize   int    `json:"cache_size"`
    Debug       bool   `json:"debug"`
}

func WriteConfig(path string, config *AppConfig) error {
    data, err := json.Marshal(config)
    if err != nil {
        return err
    }

    smConfig := &memmapfs.SharedMemoryConfig{
        Path:     path,
        Size:     int64(len(data)),
        Mode:     memmapfs.ModeReadWrite,
        SyncMode: memmapfs.SyncImmediate,
    }

    sm, err := memmapfs.CreateSharedMemory(smConfig)
    if err != nil {
        return err
    }
    defer sm.Close()

    copy(sm.Data(), data)
    return sm.Sync()
}

func ReadConfig(path string) (*AppConfig, error) {
    sm, err := memmapfs.OpenSharedMemory(path, false)
    if err != nil {
        return nil, err
    }
    defer sm.Close()

    var config AppConfig
    if err := json.Unmarshal(sm.Data(), &config); err != nil {
        return nil, err
    }

    return &config, nil
}
```

## Platform Considerations

### Linux

- Use `/dev/shm` for RAM-based shared memory (tmpfs)
- Support for `flock()` and `fcntl()` for file locking
- Transparent huge pages available for large regions

### macOS

- Uses standard POSIX mmap
- File locks via `flock()`
- Limited huge page support

### Windows

- Uses `CreateFileMapping` and `MapViewOfFile`
- Named shared memory via `CreateFileMapping` with name
- Different synchronization primitives (mutexes, semaphores)

## Security Considerations

1. **Permissions**: Set appropriate file permissions
   ```go
   config.Permissions = 0600 // Owner only
   ```

2. **Validation**: Validate all data read from shared memory
   ```go
   if header.Magic != ExpectedMagic {
       return errors.New("corrupted shared memory")
   }
   ```

3. **Trust**: Only share memory with trusted processes
4. **Encryption**: Consider encrypting sensitive data
5. **Cleanup**: Remove shared memory files when done

## Troubleshooting

### Data Not Visible to Other Process

**Cause**: Sync not called or sync mode set to `SyncNever`

**Solution**:
```go
sm.Sync() // Explicit sync
// Or use SyncImmediate mode
```

### SIGBUS Signal

**Cause**: File truncated, I/O error, or out of bounds access

**Solution**:
- Enable SIGBUS protection
- Register recovery handler
- Validate file size before access

### Race Conditions

**Cause**: Concurrent access without synchronization

**Solution**:
- Use file locks (`flock`)
- Use atomic operations
- Implement mutex protocol

## References

- [POSIX mmap](https://pubs.opengroup.org/onlinepubs/9699919799/functions/mmap.html)
- [Linux Shared Memory](https://www.kernel.org/doc/html/latest/admin-guide/mm/sharedmem.html)
- [Advanced IPC Techniques](https://beej.us/guide/bgipc/)
