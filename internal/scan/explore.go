package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Node is one folder or file in a disk map.
type Node struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
	// Children are the largest things inside, biggest first. Anything
	// smaller than the tree's threshold is only counted in Rest.
	Children []*Node `json:"children,omitempty"`
	// Rest is the space taken by entries too small to list.
	Rest int64 `json:"rest,omitempty"`
	// Unread counts folders inside that couldn't be read.
	Unread int64 `json:"unread,omitempty"`
	// Private names the personal library this is (Photos, Contacts,
	// Calendar) when Leftover didn't open it, so macOS never asks.
	Private string `json:"private,omitempty"`
}

// TreeOptions controls Tree.
type TreeOptions struct {
	// Keep is the smallest size listed as its own entry; smaller ones are
	// added to Rest. Zero means 1 MB.
	Keep int64
	// MaxChildren caps how many entries each folder keeps. Zero means 200.
	MaxChildren int
	// Progress, if set, is called from time to time with the number of
	// folders read so far and the one being read.
	Progress func(folders int64, current string)
}

type treeWalk struct {
	s    *Sizer
	ctx  context.Context
	opt  TreeOptions
	dev  uint64
	mu   sync.Mutex
	seen map[[2]uint64]bool
	last time.Time
}

// Tree measures root and returns a map of what's inside, for exploring a
// disk by size. Like Measure it never follows links, never leaves the
// drive root is on, and counts hard-linked files once. Only large entries
// are kept, so even a whole home folder fits in a few megabytes.
func (s *Sizer) Tree(ctx context.Context, root string, opt TreeOptions) (*Node, error) {
	if opt.Keep <= 0 {
		opt.Keep = 1 << 20
	}
	if opt.MaxChildren <= 0 {
		opt.MaxChildren = 200
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	t := &treeWalk{s: s, ctx: ctx, opt: opt, dev: deviceOf(info), seen: map[[2]uint64]bool{}}
	n := &Node{Name: filepath.Base(root), Path: root, Dir: info.IsDir()}
	if info.Mode()&fs.ModeSymlink != 0 {
		return n, nil
	}
	if !n.Dir {
		n.Bytes, n.Files = t.file(info), 1
		return n, nil
	}
	t.walk(n)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return n, nil
}

func (t *treeWalk) file(info fs.FileInfo) int64 {
	dev, ino, links, onDisk := fileStat(info)
	if links > 1 {
		key := [2]uint64{dev, ino}
		t.mu.Lock()
		dup := t.seen[key]
		t.seen[key] = true
		t.mu.Unlock()
		if dup {
			return 0
		}
	}
	return onDisk
}

// walk fills in n, a folder. Subfolders run in parallel while the sizer
// has room and inline when it doesn't, the same way Measure does.
func (t *treeWalk) walk(n *Node) {
	if t.ctx.Err() != nil {
		return
	}
	t.s.sem <- struct{}{}
	entries, err := os.ReadDir(n.Path)
	<-t.s.sem
	t.s.Visited.Add(1)
	if err != nil {
		n.Unread++
		return
	}
	if t.opt.Progress != nil {
		t.mu.Lock()
		if time.Since(t.last) > 120*time.Millisecond {
			t.last = time.Now()
			t.mu.Unlock()
			t.opt.Progress(t.s.Visited.Load(), n.Path)
		} else {
			t.mu.Unlock()
		}
	}
	var kids []*Node
	var wg sync.WaitGroup
	for _, e := range entries {
		if e.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Mode()&(fs.ModeSymlink|fs.ModeDevice|fs.ModeNamedPipe|fs.ModeSocket) != 0 {
			continue
		}
		child := &Node{Name: e.Name(), Path: filepath.Join(n.Path, e.Name()), Dir: info.IsDir()}
		kids = append(kids, child)
		if !child.Dir {
			child.Bytes, child.Files = t.file(info), 1
			continue
		}
		if t.dev != 0 && deviceOf(info) != t.dev {
			continue // another drive is mounted here
		}
		if child.Private = Private(child.Path); child.Private != "" {
			continue
		}
		select {
		case t.s.sem <- struct{}{}:
			<-t.s.sem
			wg.Add(1)
			go func() { defer wg.Done(); t.walk(child) }()
		default:
			t.walk(child)
		}
	}
	wg.Wait()

	for _, k := range kids {
		n.Bytes += k.Bytes
		n.Files += k.Files
		n.Unread += k.Unread
	}
	sort.Slice(kids, func(i, j int) bool {
		if kids[i].Bytes != kids[j].Bytes {
			return kids[i].Bytes > kids[j].Bytes
		}
		return kids[i].Name < kids[j].Name
	})
	for _, k := range kids {
		if (k.Bytes >= t.opt.Keep || k.Private != "") && len(n.Children) < t.opt.MaxChildren {
			n.Children = append(n.Children, k)
		} else {
			n.Rest += k.Bytes
		}
	}
}

// Find returns the node at path inside n, or nil if it wasn't kept.
func (n *Node) Find(path string) *Node {
	path = filepath.Clean(path)
	if filepath.Clean(n.Path) == path {
		return n
	}
	rel, err := filepath.Rel(n.Path, path)
	if err != nil || rel == "." || startsWithParent(rel) {
		return nil
	}
	cur := n
	for _, part := range splitPath(rel) {
		var next *Node
		for _, c := range cur.Children {
			if c.Name == part {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func startsWithParent(rel string) bool {
	return rel == ".." || len(rel) > 2 && rel[:2] == ".." && os.IsPathSeparator(rel[2])
}

func splitPath(rel string) []string {
	return strings.Split(filepath.ToSlash(rel), "/")
}
