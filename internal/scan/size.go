// Package scan finds the things described in the rule catalogue and measures
// how much space they take. Everything in this package is read-only.
package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Size is the result of measuring a folder.
type Size struct {
	// Bytes is the space on disk, counting each hard-linked file once.
	Bytes int64 `json:"bytes"`
	// Logical is the sum of file lengths, as a file manager would show.
	Logical int64 `json:"logical_bytes"`
	Files   int64 `json:"files"`
	Dirs    int64 `json:"dirs"`
	// Newest is the most recent modification time of anything inside.
	Newest time.Time `json:"newest"`
	// Skipped counts entries that couldn't be read (permissions, races).
	Skipped int64 `json:"skipped,omitempty"`
}

// Sizer measures folders in parallel. One Sizer can measure many folders at
// once; hard links are only folded within a single Measure call.
type Sizer struct {
	sem chan struct{}
	// Visited counts folders read so far across all Measure calls, for
	// progress reporting.
	Visited atomic.Int64
}

// NewSizer makes a sizer that reads at most workers folders at a time.
// Zero picks a value suited to the machine.
func NewSizer(workers int) *Sizer {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0) * 4
		if workers < 8 {
			workers = 8
		}
		if workers > 64 {
			workers = 64
		}
	}
	return &Sizer{sem: make(chan struct{}, workers)}
}

type measure struct {
	s       *Sizer
	ctx     context.Context
	rootDev uint64

	mu      sync.Mutex
	seen    map[[2]uint64]bool
	newest  int64
	bytes   atomic.Int64
	logical atomic.Int64
	files   atomic.Int64
	dirs    atomic.Int64
	skipped atomic.Int64
	wg      sync.WaitGroup
}

// Measure walks root and adds up everything below it. It never follows
// symbolic links or junctions and never crosses onto another device, so a
// mounted drive inside a cache folder isn't counted as part of it.
func (s *Sizer) Measure(ctx context.Context, root string) (Size, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return Size{}, err
	}
	m := &measure{s: s, ctx: ctx, rootDev: deviceOf(info), seen: map[[2]uint64]bool{}}
	if info.Mode()&fs.ModeSymlink != 0 {
		return Size{}, nil
	}
	if !info.IsDir() {
		m.file(info)
	} else {
		m.dirs.Add(1)
		m.touch(info.ModTime())
		m.wg.Add(1)
		m.walk(root)
		m.wg.Wait()
	}
	if err := ctx.Err(); err != nil {
		return Size{}, err
	}
	out := Size{
		Bytes:   m.bytes.Load(),
		Logical: m.logical.Load(),
		Files:   m.files.Load(),
		Dirs:    m.dirs.Load(),
		Skipped: m.skipped.Load(),
	}
	if m.newest > 0 {
		out.Newest = time.Unix(0, m.newest)
	}
	return out, nil
}

func (m *measure) touch(t time.Time) {
	n := t.UnixNano()
	m.mu.Lock()
	if n > m.newest {
		m.newest = n
	}
	m.mu.Unlock()
}

func (m *measure) file(info fs.FileInfo) {
	dev, ino, links, onDisk := fileStat(info)
	m.files.Add(1)
	m.logical.Add(info.Size())
	m.touch(info.ModTime())
	if links > 1 {
		key := [2]uint64{dev, ino}
		m.mu.Lock()
		dup := m.seen[key]
		m.seen[key] = true
		m.mu.Unlock()
		if dup {
			return
		}
	}
	m.bytes.Add(onDisk)
}

// walk reads one folder. Subfolders are handed to new goroutines while
// there is room in the semaphore, and walked inline when there isn't, so
// the pool can never deadlock on itself.
func (m *measure) walk(dir string) {
	defer m.wg.Done()
	if m.ctx.Err() != nil {
		return
	}
	m.s.sem <- struct{}{}
	entries, err := os.ReadDir(dir)
	<-m.s.sem
	m.s.Visited.Add(1)
	if err != nil {
		m.skipped.Add(1)
		return
	}
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink != 0 || e.Type()&fs.ModeIrregular != 0 {
			continue
		}
		info, err := e.Info()
		if err != nil {
			m.skipped.Add(1)
			continue
		}
		// Windows junctions and mount points report as irregular or as
		// symlinks above; anything else with an odd type is skipped too.
		if info.Mode()&(fs.ModeSymlink|fs.ModeDevice|fs.ModeNamedPipe|fs.ModeSocket) != 0 {
			continue
		}
		if !info.IsDir() {
			m.file(info)
			continue
		}
		if m.rootDev != 0 && deviceOf(info) != m.rootDev {
			continue // a different file system is mounted here
		}
		m.dirs.Add(1)
		m.touch(info.ModTime())
		child := filepath.Join(dir, e.Name())
		m.wg.Add(1)
		select {
		case m.s.sem <- struct{}{}:
			<-m.s.sem
			go m.walk(child)
		default:
			m.walk(child)
		}
	}
}
