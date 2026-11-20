# Writing rules

Everything Leftover recognises comes from a rule: a short JSON description of where a tool keeps something, what it is, and whether it comes back. Rules live in [`internal/rules/catalog`](../internal/rules/catalog), grouped by ecosystem, and are compiled into the binary so the app never needs the network.

Adding a tool means adding a rule. No Go code is needed.

## The one rule about rules

**Nothing is offered for cleaning without a source.** A rule with the verdict `rebuild` or `redownload` must link to the tool's own documentation (or the closest thing to it) and explain how the item comes back. The loader refuses to start otherwise, and the tests check it.

If you can't find a source, use `review`. Leftover will still show the item, explain it and point to the tool's own cleanup command. It just won't move it.

## Verdicts

| Verdict | Shown as | Meaning | Cleanable |
|---|---|---|---|
| `rebuild` | Rebuilds itself | Your tools recreate it: build output, bytecode, indexes | yes |
| `redownload` | Downloads again | Fetched again when needed: package caches, models, browser builds | yes |
| `review` | Worth a look | Known, but can't be promised to come back | no |
| `keep` | Your data | Belongs to you; shown so people understand the space | no |

When in doubt, choose the more careful verdict.

## A location rule

For things at a fixed place, like a package cache:

```json
{
  "id": "pip-cache",
  "tool": "pip",
  "kind": "Package cache",
  "name": "pip cache",
  "verdict": "redownload",
  "what": "Wheels and downloads pip keeps to make installs faster.",
  "comeback": "pip downloads packages again when it needs them.",
  "source": "https://pip.pypa.io/en/stable/topics/caching/",
  "command": "pip cache purge",
  "env": [{ "var": "PIP_CACHE_DIR" }],
  "paths": {
    "darwin": ["{cache}/pip"],
    "linux": ["{cache}/pip"],
    "windows": ["{localappdata}/pip/Cache"]
  }
}
```

| Field | |
|---|---|
| `id` | A unique slug. Shown in `leftover rules`. |
| `tool`, `kind`, `name` | What people see. `name` is the item; `kind` is a short category. |
| `what` | One or two plain sentences: what this is. Write for someone who has never used the tool. |
| `comeback` | How it comes back, or for `review` items, what to do before removing it. |
| `source` | An `https` link backing the verdict. Required for `rebuild` and `redownload`. |
| `command` | The tool's own cleanup command, if it has one. Shown next to the item. |
| `paths` | Locations by OS: `darwin`, `linux`, `windows`, `unix` (macOS and Linux) or `all`. |
| `env` | Environment variables that move the location. The first one set replaces `paths`. `join` adds a sub-path, e.g. `{"var": "HF_HOME", "join": "hub"}`. |
| `each` | List the location's children as separate items (one per model, per IDE version, per OS version). |
| `min_age_days` | Only count entries untouched for this many days. |

### Placeholders

Paths start with `~` or a placeholder, never a relative path:

| Placeholder | macOS | Linux | Windows |
|---|---|---|---|
| `~` | home folder | home folder | `%USERPROFILE%` |
| `{cache}` | `~/Library/Caches` | `$XDG_CACHE_HOME` or `~/.cache` | `%LOCALAPPDATA%` |
| `{xdgcache}` | `$XDG_CACHE_HOME` or `~/.cache` | same | — |
| `{data}` | `~/Library/Application Support` | `$XDG_DATA_HOME` or `~/.local/share` | `%APPDATA%` |
| `{config}` | `~/Library/Application Support` | `$XDG_CONFIG_HOME` or `~/.config` | `%APPDATA%` |
| `{localappdata}` | — | — | `%LOCALAPPDATA%` |
| `{appdata}` | — | — | `%APPDATA%` |
| `{temp}` | `$TMPDIR` | `$TMPDIR` or `/tmp` | `%TEMP%` |
| `{tmp}` | `/private/tmp` | `/tmp` | — |
| `{uid}` | your user id, anywhere in a path, e.g. `{tmp}/claude-{uid}` | same | — |

Absolute paths like `/usr/share/ollama` work on macOS and Linux.

## A project rule

For folders that sit inside projects, like `node_modules` or Cargo's `target`:

```json
{
  "id": "cargo-target",
  "tool": "Cargo",
  "kind": "Build output",
  "name": "target",
  "verdict": "rebuild",
  "what": "Everything Cargo compiled for this project, for every profile and target.",
  "comeback": "Cargo compiles it again on the next build.",
  "source": "https://doc.rust-lang.org/cargo/reference/build-cache.html",
  "command": "cargo clean",
  "project": { "dirs": ["target"], "beside": ["Cargo.toml"], "inside": ["CACHEDIR.TAG", ".rustc_info.json"] }
}
```

| `project` field | |
|---|---|
| `dirs` | Folder names to look for. |
| `beside` | Files, any one of which must sit next to the folder. Globs work: `"*.csproj"`. This is what tells Cargo's `target` from a folder you happened to call `target`. |
| `inside` | Files, any one of which must exist inside the folder. |
| `group` | Fold every match in one project into a single item. Use it for things scattered in many small pieces, like `__pycache__`. |

Be strict with `beside` and `inside`. A folder name alone is almost never enough evidence.

Rules are checked in file order, then in order within a file, and the first match wins. If two rules can match the same folder, put the stricter one first (see `python-venv` and `python-venv-unlisted`).

## Testing a rule

```sh
go test ./internal/rules        # the catalogue loads and every rule is valid
leftover rules --here           # rules that apply on this machine
leftover why <path>             # what Leftover thinks a folder is
leftover scan --tool <name>     # scan, showing only that tool
```

If the rule needs logic a rule can't express, add a test case to `internal/scan/scan_test.go` with a small fake folder layout. Tests always use temporary folders, never your real home.

## Pull requests

Include in the description the tool version you checked against, the OS, and a link to where the documentation says the folder is safe to remove. If you only verified the path on one OS, say so; it's better to ship a rule for one OS than to guess at the others.
