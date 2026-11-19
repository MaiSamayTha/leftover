package scan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func init() {
	for _, id := range []string{"docker-disk", "docker-engine", "orbstack-data", "colima-disk"} {
		id := id
		detailers[id] = func(string) []Detail {
			if dockerOwner(dockerContext()) != id {
				return nil
			}
			return dockerDetails("")
		}
	}
}

// DockerBreakdown asks Docker what its data is made of, as shown by
// docker system df. Nil means Docker isn't installed or isn't running.
func DockerBreakdown() []Detail { return dockerDetails("") }

// ParseDockerSize reads a size the way Docker prints it, like "1.24GB".
func ParseDockerSize(s string) (int64, bool) { return parseDockerSize(s) }

// DockerRules are the rules for places Docker keeps its data.
var DockerRules = map[string]bool{"docker-disk": true, "docker-engine": true, "orbstack-data": true, "colima-disk": true}

// dockerContext is swapped out in tests. It names the engine the docker
// command is talking to right now, or "" when Docker isn't there.
var dockerContext = func() string {
	path, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "context", "show").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// dockerOwner maps a docker context to the rule for where that engine
// keeps its data, so a breakdown is shown once, on the right item.
func dockerOwner(context string) string {
	switch {
	case context == "":
		return ""
	case context == "orbstack":
		return "orbstack-data"
	case strings.HasPrefix(context, "colima"):
		return "colima-disk"
	case context == "default" && runtime.GOOS == "linux":
		return "docker-engine"
	}
	return "docker-disk"
}

// dockerRow is one line of `docker system df --format '{{json .}}'`.
type dockerRow struct {
	Type        string `json:"Type"`
	TotalCount  string `json:"TotalCount"`
	Size        string `json:"Size"`
	Reclaimable string `json:"Reclaimable"`
}

// dockerDF is swapped out in tests.
var dockerDF = func(ctx context.Context) ([]byte, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, path, "system", "df", "--format", "{{json .}}").Output()
}

// dockerDetails asks Docker itself what's inside its disk image. Leftover
// never moves that file, so this is how people find out what Docker's own
// prune commands would give back. If Docker isn't installed or running,
// there are simply no details.
func dockerDetails(string) []Detail {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := dockerDF(ctx)
	if err != nil {
		return nil
	}
	return parseDockerDF(out)
}

var dockerCommands = map[string]string{
	"Images":        "docker image prune -a",
	"Containers":    "docker container prune",
	"Local Volumes": "docker volume prune",
	"Build Cache":   "docker builder prune",
}

func parseDockerDF(out []byte) []Detail {
	var details []Detail
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		var r dockerRow
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Type == "" {
			continue
		}
		size, ok := parseDockerSize(r.Size)
		if !ok {
			continue
		}
		d := Detail{Name: r.Type, Bytes: size, Command: dockerCommands[r.Type]}
		if f := strings.Fields(r.Reclaimable); len(f) > 0 {
			d.Reclaimable, _ = parseDockerSize(f[0])
		}
		details = append(details, d)
	}
	return details
}

// parseDockerSize reads sizes the way Docker prints them: decimal units,
// like "1.24GB", "512MB", "12.5kB" or "0B".
func parseDockerSize(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	units := []struct {
		suffix string
		mult   float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			f, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			if err != nil {
				return 0, false
			}
			return int64(f * u.mult), true
		}
	}
	return 0, false
}
