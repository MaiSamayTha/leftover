package app

import (
	"context"
	"strings"
	"testing"
)

func TestDockerPruneNeverTouchesVolumesOrContainers(t *testing.T) {
	a, _ := testApp(t)
	defer func(f func(context.Context, ...string) ([]byte, error)) { dockerRun = f }(dockerRun)
	var ran []string
	dockerRun = func(_ context.Context, args ...string) ([]byte, error) {
		ran = append(ran, strings.Join(args, " "))
		switch args[0] {
		case "builder":
			return []byte("ID\tRECLAIMABLE\tSIZE\nabc\ttrue\t1.2GB\nTotal:\t1.5GB\n"), nil
		case "image":
			return []byte("Deleted Images:\nuntagged: x\n\nTotal reclaimed space: 300MB\n"), nil
		}
		return []byte("27.0.1"), nil
	}
	steps, err := a.DockerPrune(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range ran {
		if strings.Contains(r, "volume") || strings.Contains(r, "container") || strings.Contains(r, "system prune") {
			t.Fatalf("ran %q; volumes and containers must never be pruned", r)
		}
	}
	if len(steps) != 2 || steps[0].Reclaimed != 1_500_000_000 || steps[1].Reclaimed != 300_000_000 {
		t.Fatalf("steps = %+v", steps)
	}
	if !strings.HasSuffix(ran[len(ran)-1], "-a") {
		t.Fatalf("unused images were asked for: %v", ran)
	}
	ev, _ := a.Vault.History(1)
	if len(ev) != 1 || ev[0].Action != "docker-prune" || ev[0].Bytes != 1_800_000_000 {
		t.Fatalf("history = %+v", ev)
	}
}
