package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MaiSamayTha/leftover/internal/rules"
)

func TestRunningAs(t *testing.T) {
	xcode := rules.Busy{Name: "Xcode", Process: []string{"Xcode"}}
	gradle := rules.Busy{Name: "Gradle", Args: []string{"GradleDaemon"}}
	idea := rules.Busy{Name: "IDE", Process: []string{"idea64", "idea"}}
	ollama := rules.Busy{Name: "Ollama", Process: []string{"ollama", "ollama app"}}
	cases := []struct {
		cmd  string
		bz   rules.Busy
		want bool
	}{
		{"/Applications/Xcode.app/Contents/MacOS/Xcode", xcode, true},
		{"/Applications/Xcode.app/Contents/Developer/usr/bin/xcodebuild -scheme App", xcode, false},
		{"/usr/bin/java -Xmx2g -cp gradle-launcher.jar org.gradle.launcher.daemon.bootstrap.GradleDaemon 8.5", gradle, true},
		{"/usr/bin/java -jar server.jar", gradle, false},
		{`"C:\Program Files\JetBrains\IntelliJ IDEA\bin\idea64.exe"`, idea, true},
		{"/Applications/IntelliJ IDEA.app/Contents/MacOS/idea", idea, true},
		{"/usr/bin/vim ideas.txt", idea, false},
		{"/Applications/Ollama.app/Contents/Resources/ollama serve", ollama, true},
		{`C:\Users\sam\AppData\Local\Programs\Ollama\ollama app.exe`, ollama, true},
		{"ollama.exe", ollama, true},
	}
	for _, c := range cases {
		if got := runningAs(c.cmd, c.bz); got != c.want {
			t.Errorf("runningAs(%q, %s) = %v, want %v", c.cmd, c.bz.Name, got, c.want)
		}
	}
}

// Nothing is moved while the program that uses it is running.
func TestCleanWaitsForRunningTools(t *testing.T) {
	a, home := testApp(t)
	put(t, filepath.Join(home, ".gradle/caches/modules-2/a.jar"), 20_000)
	defer func(f func(context.Context) ([]string, error)) { listProcesses = f }(listProcesses)
	running := []string{"/usr/bin/java org.gradle.launcher.daemon.bootstrap.GradleDaemon 8.5"}
	listProcesses = func(context.Context) ([]string, error) { return running, nil }

	res, _, err := a.Scan(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var gradle, web string
	for _, it := range res.Items {
		switch it.Rule {
		case "gradle-caches":
			gradle = it.ID
		case "node-modules":
			web = it.ID
		}
	}
	if gradle == "" || web == "" {
		t.Fatal("fixture items missing")
	}
	out := a.Clean([]string{gradle, web})
	if !strings.Contains(out[0].Error, "Gradle daemon is running") || out[0].Entry != nil {
		t.Fatalf("gradle cache moved while the daemon runs: %+v", out[0])
	}
	if out[1].Error != "" {
		t.Fatalf("node_modules should still move: %+v", out[1])
	}
	running = nil
	if out := a.Clean([]string{gradle}); out[0].Error != "" {
		t.Fatalf("once the daemon stops it should move: %+v", out[0])
	}
}
