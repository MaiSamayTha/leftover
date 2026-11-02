package scan

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Detail is one named part inside an item, such as a single model inside
// the Ollama folder. Details are for understanding; cleaning still works on
// the whole item, and the tool's own command handles single parts.
type Detail struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	// Shared is set when some of the space is also used by other parts,
	// so removing this part alone frees less than Bytes.
	Shared bool `json:"shared,omitempty"`
	// Reclaimable is how much the tool's own command says it could free.
	Reclaimable int64 `json:"reclaimable,omitempty"`
	// Command removes just this part with the tool's own command.
	Command string `json:"command,omitempty"`
}

// detailers break some items into their parts.
var detailers = map[string]func(path string) []Detail{
	"ollama-models": ollamaModels,
}

// ollama manifest, as stored under models/manifests.
type ollamaManifest struct {
	Config ollamaLayer   `json:"config"`
	Layers []ollamaLayer `json:"layers"`
}

type ollamaLayer struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// ollamaModels lists every pulled model with the size of the layers it uses.
// Layers shared between models (a base model and its fine-tune, say) are
// counted for each and the models are marked as sharing.
func ollamaModels(root string) []Detail {
	manifests := filepath.Join(root, "manifests")
	type model struct {
		name    string
		digests []string
		size    int64
	}
	var models []model
	uses := map[string]int{}
	filepath.WalkDir(manifests, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil || len(raw) > 1<<20 {
			return nil
		}
		var m ollamaManifest
		if json.Unmarshal(raw, &m) != nil || len(m.Layers) == 0 {
			return nil
		}
		rel, _ := filepath.Rel(manifests, p)
		md := model{name: ollamaName(filepath.ToSlash(rel))}
		for _, l := range append(m.Layers, m.Config) {
			if l.Digest == "" {
				continue
			}
			md.digests = append(md.digests, l.Digest)
			md.size += l.Size
			uses[l.Digest]++
		}
		models = append(models, md)
		return nil
	})
	out := make([]Detail, 0, len(models))
	for _, m := range models {
		d := Detail{Name: m.name, Bytes: m.size, Command: "ollama rm " + m.name}
		for _, dg := range m.digests {
			if uses[dg] > 1 {
				d.Shared = true
				break
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// ollamaName turns a manifest path into the name people type:
// registry.ollama.ai/library/llama3.1/8b -> llama3.1:8b, while models from
// other namespaces keep them: registry.ollama.ai/sam/coder/latest -> sam/coder:latest.
func ollamaName(rel string) string {
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return rel
	}
	tag := parts[len(parts)-1]
	name := parts[len(parts)-2]
	ns := parts[:len(parts)-2]
	if len(ns) >= 1 && ns[0] == "registry.ollama.ai" {
		ns = ns[1:]
	}
	if len(ns) == 1 && ns[0] == "library" {
		ns = nil
	}
	full := strings.Join(append(ns, name), "/")
	return full + ":" + tag
}
