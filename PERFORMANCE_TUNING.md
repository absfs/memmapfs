# Performance Tuning Guide

This guide covers platform-specific optimizations and performance tuning techniques for memmapfs.

## Table of Contents

1. [Configuration Options](#configuration-options)
2. [Platform-Specific Features](#platform-specific-features)
3. [Access Pattern Hints](#access-pattern-hints)
4. [Performance Profiles](#performance-profiles)
5. [Benchmarking](#benchmarking)

## Configuration Options

### MapFullFile vs Windowed Mapping

**Full File Mapping** (`MapFullFile: true`):
- Maps entire file into address space
- Best performance for files smaller than available RAM
- Lower overhead (no window sliding)
- May fail on 32-bit systems with large files

```go
config := &memmapfs.Config{
    MapFullFile: true,
}
```

**Windowed Mapping** (`MapFullFile: false`):
- Maps only a portion of the file at a time
- Necessary for files larger than address space
- Automatically slides window on access
- Slightly lower performance due to remapping overhead

```go
config := &memmapfs.Config{
    MapFullFile: false,
    WindowSize:  1 << 30, // 1GB window
}
```

**Recommendation**:
- Use full-file mapping for files < 1GB on 64-bit systems
- Use windowed mapping for very large files (>4GB) or on 32-bit systems

### Preloading Strategies

#### Preload (madvise-based)

Uses `madvise(MADV_WILLNEED)` to hint that pages should be loaded:

```go
config := &memmapfs.Config{
    Preload:      true,
    PreloadAsync: true, // Non-blocking
}
```

**Characteristics**:
- Non-blocking (hint to OS)
- OS may ignore under memory pressure
- Lower overhead than PopulatePages
- Good for files you'll access soon

#### PopulatePages (MAP_POPULATE)

Uses `MAP_POPULATE` flag to eagerly load pages (Linux-specific):

```go
config := &memmapfs.Config{
    PopulatePages: true,
}
```

**Characteristics**:
- Blocks until pages loaded (synchronous)
- Guarantees pages are in RAM
- Higher overhead at open time
- Eliminates page faults during access
- **Linux-only**, ignored on other platforms

**Recommendation**:
- Use `PopulatePages` for small-medium files (<100MB) accessed immediately
- Use `Preload` for larger files or when you want non-blocking behavior
- Use neither for very large files or sparse access patterns

### Sync Modes

Control when dirty pages are written to disk:

| Mode | Behavior | Use Case |
|------|----------|----------|
| `SyncImmediate` | msync after every write | Critical data, crash safety |
| `SyncPeriodic` | Background sync at intervals | Balanced durability/performance |
| `SyncLazy` | Sync only on close | Maximum performance, lazy durability |
| `SyncNever` | Let OS handle sync | Read-only or disposable data |

```go
// Maximum performance (read-write)
config := &memmapfs.Config{
    Mode:     memmapfs.ModeReadWrite,
    SyncMode: memmapfs.SyncLazy,
}

// Maximum durability
config := &memmapfs.Config{
    Mode:     memmapfs.ModeReadWrite,
    SyncMode: memmapfs.SyncImmediate,
}
```

## Platform-Specific Features

### Linux: Huge Pages

Huge pages can significantly improve TLB (Translation Lookaside Buffer) performance for large files by reducing the number of page table entries.

#### Using MAP_HUGETLB

```go
config := &memmapfs.Config{
    UseHugePages: true,
}
```

**Requirements**:
- Huge pages must be configured on the system
- File size should be a multiple of huge page size (typically 2MB)
- May require elevated privileges

**Check huge page availability**:
```bash
# Linux
cat /proc/meminfo | grep Huge
grep -i huge /proc/meminfo

# Configure huge pages (as root)
echo 128 > /proc/sys/vm/nr_hugepages
```

#### Transparent Huge Pages (THP)

```go
file, _ := mfs.Open("largefile.dat")
mf := file.(*memmapfs.MappedFile)

// Hint to use THP
mf.AdviseHugePage()
```

**Enable THP** (Linux):
```bash
# Check current setting
cat /sys/kernel/mm/transparent_hugepage/enabled

# Enable always
echo always > /sys/kernel/mm/transparent_hugepage/enabled

# Enable madvise (recommended)
echo madvise > /sys/kernel/mm/transparent_hugepage/enabled
```

**Performance Impact**:
- Can improve performance by 10-30% for large files
- Most beneficial for random access workloads
- Requires sufficient contiguous physical memory

### Linux: Advanced madvise Hints

#### MADV_FREE (Linux 4.5+)

Allows kernel to reclaim pages without writing dirty data:

```go
mf.AdviseFree() // Data will be lost if reclaimed!
```

**Use case**: Temporary buffers, caches

#### MADV_REMOVE

Frees backing storage for pages:

```go
mf.AdviseRemove()
```

**Use case**: Sparse files, hole punching

## Access Pattern Hints

Providing access pattern hints helps the OS optimize page cache behavior:

### Sequential Access

```go
mf := file.(*memmapfs.MappedFile)
mf.AdviseSequential()

// OS will:
// - Readahead more aggressively
// - Free pages behind current position
// - Optimize for streaming workloads
```

**Best for**:
- Log file processing
- Sequential scans
- Video/media playback

### Random Access

```go
mf.AdviseRandom()

// OS will:
// - Disable or reduce readahead
// - Keep more pages in cache
// - Optimize for random I/O
```

**Best for**:
- Database index lookups
- B-tree operations
- Random sampling

### WillNeed / DontNeed

```go
// Preload pages you'll access soon
mf.AdviseWillNeed()

// Read data...

// Release pages when done
mf.AdviseDontNeed()
```

**Best for**:
- Working set management
- Predictable access patterns
- Memory-constrained environments

## Performance Profiles

### Database / Index Files

High-performance random access with durability:

```go
config := &memmapfs.Config{
    Mode:          memmapfs.ModeReadWrite,
    SyncMode:      memmapfs.SyncPeriodic,
    SyncInterval:  5 * time.Second,
    MapFullFile:   true,
    PopulatePages: true,  // Load index into RAM
    UseHugePages:  true,  // If available
}

mfs := memmapfs.New(osFS, config)
file, _ := mfs.OpenFile("index.db", os.O_RDWR, 0644)

mf := file.(*memmapfs.MappedFile)
mf.AdviseRandom()      // Random access pattern
mf.AdviseHugePage()    // Use THP if available
```

**Expected performance**:
- Random reads: 10-100x faster than traditional I/O
- Reduced page faults with PopulatePages
- Better TLB hit rate with huge pages

### Log Analysis

Memory-efficient sequential processing of large files:

```go
config := &memmapfs.Config{
    Mode:          memmapfs.ModeReadOnly,
    SyncMode:      memmapfs.SyncNever,
    MapFullFile:   false,
    WindowSize:    100 * 1024 * 1024, // 100MB window
    PopulatePages: false, // Lazy loading
}

mfs := memmapfs.New(osFS, config)
file, _ := mfs.Open("application.log")

mf := file.(*memmapfs.MappedFile)
mf.AdviseSequential() // Optimize for sequential scan
```

**Benefits**:
- Processes multi-GB files with minimal RAM
- OS readahead optimization
- Automatic page eviction behind read position

### High-Throughput Data Processing

Maximum performance, lazy durability:

```go
config := &memmapfs.Config{
    Mode:          memmapfs.ModeReadWrite,
    SyncMode:      memmapfs.SyncLazy,
    MapFullFile:   true,
    PopulatePages: true,
    UseHugePages:  true,
}

mfs := memmapfs.New(osFS, config)
file, _ := mfs.OpenFile("data.bin", os.O_RDWR, 0644)

mf := file.(*memmapfs.MappedFile)
mf.AdviseHugePage()

// Process data at maximum speed
// Sync explicitly at checkpoints if needed
mf.Sync()
```

**Trade-offs**:
- Maximum read/write performance
- Data at risk until explicit sync or close
- Requires checkpoint logic for crash recovery

### Memory-Constrained Environment

Minimize memory usage while maintaining reasonable performance:

```go
config := &memmapfs.Config{
    Mode:          memmapfs.ModeReadOnly,
    SyncMode:      memmapfs.SyncNever,
    MapFullFile:   false,
    WindowSize:    10 * 1024 * 1024, // 10MB window
    PopulatePages: false,
    Preload:       false,
}

mfs := memmapfs.New(osFS, config)
file, _ := mfs.Open("data.bin")

mf := file.(*memmapfs.MappedFile)
mf.AdviseSequential()

// Actively manage working set
mf.AdviseDontNeed() // Release pages when done
```

**Benefits**:
- Small memory footprint
- Works well under memory pressure
- Cooperative with other applications

## Benchmarking

### Measuring Performance

Use the included benchmarks to measure performance on your system:

```bash
# Run all benchmarks
go test -bench=. -benchtime=5s

# Benchmark specific operations
go test -bench=BenchmarkWindowedSequentialRead
go test -bench=BenchmarkWindowedRandomRead

# Profile memory
go test -bench=. -benchmem

# Generate CPU profile
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

### Key Metrics

**Sequential Read Throughput**:
```bash
go test -bench=BenchmarkSequentialRead
```

Expected: 25-30 GB/s (full file) vs 5-10 GB/s (windowed)

**Random Read Latency**:
```bash
go test -bench=BenchmarkRandomRead
```

Expected: 10-100x improvement over traditional I/O

**Window Sliding Overhead**:
```bash
go test -bench=BenchmarkWindowSize
```

Compares different window sizes to find optimal trade-off

### System-Level Monitoring

Monitor system behavior during operation:

```bash
# Watch page cache usage
watch -n1 'cat /proc/meminfo | grep -i mapped'

# Monitor page faults
perf stat -e page-faults,minor-faults,major-faults <your-app>

# Check THP usage
grep -i anon /proc/meminfo

# Monitor file I/O
iostat -x 1
```

## Platform Compatibility

| Feature | Linux | macOS | Windows |
|---------|-------|-------|---------|
| MAP_POPULATE | ✓ | ✗ | ✗ |
| MAP_HUGETLB | ✓ | ✗ | ✗ |
| Transparent Huge Pages | ✓ | ✗ | ✗ |
| madvise(MADV_SEQUENTIAL) | ✓ | ✓ | ✗ |
| madvise(MADV_RANDOM) | ✓ | ✓ | ✗ |
| madvise(MADV_WILLNEED) | ✓ | ✓ | ✗ |
| madvise(MADV_DONTNEED) | ✓ | ✓ | ✗ |
| madvise(MADV_FREE) | ✓ (4.5+) | ✗ | ✗ |
| madvise(MADV_HUGEPAGE) | ✓ (2.6.38+) | ✗ | ✗ |

Non-supported features are silently ignored on incompatible platforms.

## Troubleshooting

### Huge Pages Allocation Fails

**Symptom**: File open fails when `UseHugePages: true`

**Solutions**:
1. Check huge page availability:
   ```bash
   cat /proc/meminfo | grep HugePages_Free
   ```
2. Increase huge page pool:
   ```bash
   echo 256 > /proc/sys/vm/nr_hugepages
   ```
3. Consider using `AdviseHugePage()` instead (THP)

### High Memory Usage

**Symptom**: Process using more memory than expected

**Solutions**:
1. Disable `PopulatePages` for large files
2. Use windowed mapping instead of full-file
3. Call `AdviseDontNeed()` when done with regions
4. Reduce `WindowSize`

### Slow Window Sliding

**Symptom**: Performance degradation with windowed mapping

**Solutions**:
1. Increase `WindowSize` (trade memory for speed)
2. Use sequential access patterns when possible
3. Consider full-file mapping if RAM permits
4. Use `PopulatePages` to preload window

## Best Practices

1. **Start with defaults**: Use `DefaultConfig()` and tune based on profiling
2. **Match sync mode to durability requirements**: Don't over-sync
3. **Use access pattern hints consistently**: Help the OS optimize
4. **Monitor system metrics**: Watch page faults, memory usage
5. **Test on target platform**: Performance varies by OS and hardware
6. **Benchmark your workload**: Standard benchmarks may not match your use case
7. **Consider huge pages for large files**: Can provide significant speedup
8. **Profile before optimizing**: Measure to identify actual bottlenecks

## References

- [Linux mmap(2) man page](https://man7.org/linux/man-pages/man2/mmap.2.html)
- [Linux madvise(2) man page](https://man7.org/linux/man-pages/man2/madvise.2.html)
- [Transparent Huge Pages](https://www.kernel.org/doc/html/latest/admin-guide/mm/transhuge.html)
- [Huge Pages Documentation](https://www.kernel.org/doc/html/latest/admin-guide/mm/hugetlbpage.html)
