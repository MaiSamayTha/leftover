package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MaiSamayTha/leftover/internal/app"
	"github.com/MaiSamayTha/leftover/internal/rules"
	"github.com/MaiSamayTha/leftover/internal/scan"
)

func init() {
	register(command{name: "report", short: "a shareable summary of the last scan (Markdown or JSON)", run: cmdReport})
}

// Report is a summary of one scan, safe to share: paths under your home
// folder are written with ~.
type Report struct {
	Scanned   time.Time       `json:"scanned"`
	Disk      *scan.Disk      `json:"disk,omitempty"`
	Totals    []ReportTotal   `json:"totals"`
	Tools     []ReportTool    `json:"tools"`
	Biggest   []ReportItem    `json:"biggest"`
	Idle      []ReportProject `json:"idle_projects"`
	Cleanable int64           `json:"cleanable_bytes"`
}

type ReportTotal struct {
	Verdict rules.Verdict `json:"verdict"`
	Label   string        `json:"label"`
	Bytes   int64         `json:"bytes"`
	Items   int           `json:"items"`
}

type ReportTool struct {
	Tool      string `json:"tool"`
	Bytes     int64  `json:"bytes"`
	Cleanable int64  `json:"cleanable_bytes"`
	Items     int    `json:"items"`
}

type ReportItem struct {
	Name    string        `json:"name"`
	Tool    string        `json:"tool"`
	Verdict rules.Verdict `json:"verdict"`
	Bytes   int64         `json:"bytes"`
	Path    string        `json:"path"`
}

type ReportProject struct {
	Project   string `json:"project"`
	IdleDays  int    `json:"idle_days"`
	Cleanable int64  `json:"cleanable_bytes"`
}

func cmdReport(args []string) error {
	fs := newFlags("report")
	asJSON := fs.Bool("json", false, "print as JSON")
	top := fs.Int("n", 10, "how many of the biggest items and idle projects to list")
	idle := fs.Int("idle", 90, "list projects untouched for at least this many days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := app.Open()
	if err != nil {
		return err
	}
	res := a.Last()
	if res == nil {
		if !*asJSON {
			fmt.Fprintln(os.Stderr, dim("No scan yet, so scanning first…"))
		}
		ctx, stop := interruptible()
		res, _, err = a.Scan(ctx, nil)
		stop()
		if err != nil {
			return err
		}
	}
	r := buildReport(res, *top, *idle)
	if *asJSON {
		return printJSON(r)
	}
	writeReport(os.Stdout, r, *idle)
	return nil
}

func buildReport(res *scan.Result, top, idleDays int) Report {
	r := Report{Scanned: res.Started, Disk: res.Disk}
	byVerdict := map[rules.Verdict]*ReportTotal{}
	byTool := map[string]*ReportTool{}
	byProject := map[string]*ReportProject{}
	for _, it := range res.Items {
		t := byVerdict[it.Verdict]
		if t == nil {
			t = &ReportTotal{Verdict: it.Verdict, Label: it.Verdict.Label()}
			byVerdict[it.Verdict] = t
		}
		t.Bytes += it.Bytes
		t.Items++
		tl := byTool[it.Tool]
		if tl == nil {
			tl = &ReportTool{Tool: it.Tool}
			byTool[it.Tool] = tl
		}
		tl.Bytes += it.Bytes
		tl.Items++
		if it.Cleanable() {
			tl.Cleanable += it.Bytes
			r.Cleanable += it.Bytes
			if it.Project != "" && it.IdleDays >= idleDays {
				p := byProject[it.Project]
				if p == nil {
					p = &ReportProject{Project: tilde(it.Project), IdleDays: it.IdleDays}
					byProject[it.Project] = p
				}
				p.Cleanable += it.Bytes
			}
		}
	}
	for _, v := range []rules.Verdict{rules.Rebuild, rules.Redownload, rules.Review, rules.Keep} {
		if t := byVerdict[v]; t != nil {
			r.Totals = append(r.Totals, *t)
		}
	}
	for _, t := range byTool {
		r.Tools = append(r.Tools, *t)
	}
	sort.Slice(r.Tools, func(i, j int) bool {
		if r.Tools[i].Bytes != r.Tools[j].Bytes {
			return r.Tools[i].Bytes > r.Tools[j].Bytes
		}
		return r.Tools[i].Tool < r.Tools[j].Tool
	})
	for _, it := range res.Items { // already biggest first
		if len(r.Biggest) == top {
			break
		}
		where := it.Paths[0]
		if it.Project != "" {
			where = it.Project
		}
		name := it.Name
		if it.Label != "" {
			name += " (" + it.Label + ")"
		}
		r.Biggest = append(r.Biggest, ReportItem{Name: name, Tool: it.Tool, Verdict: it.Verdict, Bytes: it.Bytes, Path: tilde(where)})
	}
	for _, p := range byProject {
		r.Idle = append(r.Idle, *p)
	}
	sort.Slice(r.Idle, func(i, j int) bool {
		if r.Idle[i].Cleanable != r.Idle[j].Cleanable {
			return r.Idle[i].Cleanable > r.Idle[j].Cleanable
		}
		return r.Idle[i].Project < r.Idle[j].Project
	})
	if len(r.Idle) > top {
		r.Idle = r.Idle[:top]
	}
	if r.Totals == nil {
		r.Totals = []ReportTotal{}
	}
	if r.Tools == nil {
		r.Tools = []ReportTool{}
	}
	if r.Biggest == nil {
		r.Biggest = []ReportItem{}
	}
	if r.Idle == nil {
		r.Idle = []ReportProject{}
	}
	return r
}

// mdCell keeps a value from breaking a Markdown table.
func mdCell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func writeReport(w io.Writer, r Report, idleDays int) {
	fmt.Fprintf(w, "# Leftover report\n\nScanned %s.", r.Scanned.Local().Format("2 Jan 2006, 15:04"))
	if d := r.Disk; d != nil {
		fmt.Fprintf(w, " %s free of %s.", human(d.Free), human(d.Total))
	}
	fmt.Fprintf(w, " **%s can be cleared safely.**\n\n", human(r.Cleanable))

	fmt.Fprintln(w, "## By verdict\n\n| Verdict | Size | Items |\n|---|--:|--:|")
	for _, t := range r.Totals {
		fmt.Fprintf(w, "| %s | %s | %d |\n", t.Label, human(t.Bytes), t.Items)
	}
	fmt.Fprintln(w, "\n## By tool\n\n| Tool | Size | Safe to clear | Items |\n|---|--:|--:|--:|")
	for _, t := range r.Tools {
		fmt.Fprintf(w, "| %s | %s | %s | %d |\n", mdCell(t.Tool), human(t.Bytes), human(t.Cleanable), t.Items)
	}
	if len(r.Biggest) > 0 {
		fmt.Fprintln(w, "\n## Biggest items\n\n| Item | Tool | Verdict | Size | Where |\n|---|---|---|--:|---|")
		for _, it := range r.Biggest {
			fmt.Fprintf(w, "| %s | %s | %s | %s | `%s` |\n", mdCell(it.Name), mdCell(it.Tool), it.Verdict.Label(), human(it.Bytes), mdCell(it.Path))
		}
	}
	if len(r.Idle) > 0 {
		fmt.Fprintf(w, "\n## Projects untouched for %d+ days\n\n| Project | Untouched | Safe to clear |\n|---|--:|--:|\n", idleDays)
		for _, p := range r.Idle {
			fmt.Fprintf(w, "| `%s` | %s | %s |\n", mdCell(p.Project), ago(p.IdleDays), human(p.Cleanable))
		}
	}
	fmt.Fprintln(w, "\n_Made with [Leftover](https://maisamaytha.github.io/leftover/)._")
}
