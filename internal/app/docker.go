package app

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/MaiSamayTha/leftover/internal/scan"
	"github.com/MaiSamayTha/leftover/internal/vault"
)

// PruneStep is one Docker command run by DockerPrune.
type PruneStep struct {
	Command   string `json:"command"`
	Reclaimed int64  `json:"reclaimed"`
	Error     string `json:"error,omitempty"`
}

// dockerRun is swapped out in tests.
var dockerRun = func(ctx context.Context, args ...string) ([]byte, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, ErrNoDocker
	}
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

// ErrNoDocker means the docker command isn't installed.
var ErrNoDocker = errors.New("Docker isn't installed, or the docker command isn't on your PATH")

// PrunePlan lists the Docker commands DockerPrune runs. They only remove
// what Docker re-creates on demand: build cache, and images no container
// uses. Containers and volumes are never touched, since volumes can hold
// databases and containers can hold changes nobody saved.
func PrunePlan(allUnusedImages bool) [][]string {
	images := []string{"image", "prune", "-f"}
	if allUnusedImages {
		images = append(images, "-a")
	}
	return [][]string{{"builder", "prune", "-f"}, images}
}

var reclaimedRe = regexp.MustCompile(`(?i)total(?: reclaimed space)?:\s*([0-9.]+\s*[kMGT]?B)`)

// DockerPrune asks Docker to remove its build cache and dangling images,
// plus every unused image when allUnusedImages is set. This is Docker's own
// cleanup, so unlike the vault it can't be undone: the cache is rebuilt and
// images are pulled again when they're next needed. It's recorded in the
// history like everything else Leftover changes.
func (a *App) DockerPrune(ctx context.Context, allUnusedImages bool) ([]PruneStep, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if _, err := dockerRun(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		if errors.Is(err, ErrNoDocker) {
			return nil, err
		}
		return nil, errors.New("Docker isn't running. Start it and try again")
	}
	var steps []PruneStep
	var total int64
	var cmds []string
	for _, args := range PrunePlan(allUnusedImages) {
		st := PruneStep{Command: "docker " + strings.Join(args, " ")}
		out, err := dockerRun(ctx, args...)
		if err != nil {
			st.Error = strings.TrimSpace(firstLine(string(out)))
			if st.Error == "" {
				st.Error = err.Error()
			}
		}
		if m := reclaimedRe.FindStringSubmatch(string(out)); m != nil {
			st.Reclaimed, _ = scan.ParseDockerSize(strings.ReplaceAll(m[1], " ", ""))
		}
		total += st.Reclaimed
		cmds = append(cmds, st.Command)
		steps = append(steps, st)
	}
	name := "Docker build cache and dangling images"
	if allUnusedImages {
		name = "Docker build cache and unused images"
	}
	a.Vault.Record(vault.Event{Action: "docker-prune", Name: name, Bytes: total, Paths: cmds})
	return steps, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
