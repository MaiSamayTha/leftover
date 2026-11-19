package scan

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// gitPath is swapped out in tests; empty means git isn't installed and the
// check is skipped.
var gitPath = func() string {
	p, _ := exec.LookPath("git")
	return p
}

// markTracked asks git whether any project folder that looks generated
// actually holds files committed to the repository, like a vendor folder
// someone checked in. Those aren't purely generated, so they're never
// offered for cleaning. It only reads; when git is missing or a repository
// can't be read, nothing is marked.
func markTracked(ctx context.Context, cands []candidate) {
	git := gitPath()
	if git == "" {
		return
	}
	byRepo := map[string][]int{}
	roots := map[string]string{} // folder -> repository root, cached
	for i, c := range cands {
		if c.rule.Project == nil || !c.rule.Verdict.Cleanable() {
			continue
		}
		if root := repoRoot(filepath.Dir(c.paths[0]), roots); root != "" {
			byRepo[root] = append(byRepo[root], i)
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	var mu sync.Mutex
	for root, idx := range byRepo {
		wg.Add(1)
		sem <- struct{}{}
		go func(root string, idx []int) {
			defer wg.Done()
			defer func() { <-sem }()
			counts := trackedCounts(ctx, git, root, idx, cands)
			mu.Lock()
			for i, n := range counts {
				cands[i].tracked = n
			}
			mu.Unlock()
		}(root, idx)
	}
	wg.Wait()
}

// repoRoot finds the folder holding .git above dir, if any.
func repoRoot(dir string, cache map[string]string) string {
	var walked []string
	root := ""
	for p := dir; ; {
		if r, ok := cache[p]; ok {
			root = r
			break
		}
		walked = append(walked, p)
		if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
			root = p
			break
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	for _, p := range walked {
		cache[p] = root
	}
	return root
}

// trackedCounts runs git ls-files for the candidate folders of one
// repository and counts the committed files inside each.
func trackedCounts(ctx context.Context, git, root string, idx []int, cands []candidate) map[int]int {
	type target struct {
		i   int
		rel string // slash-separated, relative to root, with a trailing slash
	}
	var targets []target
	for _, i := range idx {
		for _, p := range cands[i].paths {
			rel, err := filepath.Rel(root, p)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				continue
			}
			targets = append(targets, target{i, filepath.ToSlash(rel) + "/"})
		}
	}
	counts := map[int]int{}
	const chunk = 200
	for start := 0; start < len(targets); start += chunk {
		end := min(start+chunk, len(targets))
		args := []string{"-C", root, "-c", "core.quotepath=off", "ls-files", "-z", "--"}
		for _, t := range targets[start:end] {
			args = append(args, ":(literal)"+t.rel)
		}
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		out, err := exec.CommandContext(cctx, git, args...).Output()
		cancel()
		if err != nil {
			return nil // can't tell, so don't guess
		}
		for _, f := range bytes.Split(out, []byte{0}) {
			if len(f) == 0 {
				continue
			}
			name := string(f)
			for _, t := range targets[start:end] {
				if strings.HasPrefix(name, t.rel) {
					counts[t.i]++
					break
				}
			}
		}
	}
	return counts
}
