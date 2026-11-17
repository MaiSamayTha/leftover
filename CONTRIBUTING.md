# Contributing

Thanks for helping. A few things keep Leftover trustworthy, so please read this before opening a pull request.

## The ground rules

1. **Never delete.** Cleaning moves into the vault (`internal/vault`). Only emptying the vault removes files, and only files inside a vault.
2. **Scanning only reads.** Nothing in `internal/scan` may write, and it never follows links or crosses drives.
3. **No source, no cleaning.** A rule can only be `rebuild` or `redownload` with a documented source. Otherwise it's `review`.
4. **Standard library only.** Ask in an issue before adding a dependency. CI fails if `go.mod` gains a `require`.
5. **Tests use temporary folders.** Never the real home folder. Use `t.TempDir()` and a fake `rules.Host`.
6. **`--json` output is a contract.** Add fields; don't rename or remove them.
7. **No network.** The app must keep working with the network unplugged.

## Getting started

```sh
go build ./cmd/leftover
go test ./...
sh scripts/e2e.sh
./leftover app --no-open   # prints a local address for the window
```

The window's front end is plain HTML, CSS and JavaScript in `internal/ui/web`, embedded into the binary. There's no build step; rebuild the Go binary to see changes.

## Good first contributions

- **A new rule.** See [docs/RULES.md](docs/RULES.md). Include the tool version and OS you checked, and the documentation link.
- **A wrong verdict.** If Leftover says something comes back and it doesn't, or calls something yours that's really a cache, open an issue with the path and tool version.
- **Windows and Linux paths.** Many rules were written from documentation; confirming them on real machines is very valuable.

## Style

- `gofmt`, and keep functions small and named for what they do.
- User-facing text is plain, short and specific. Say what something is and what will happen. Avoid jargon, hype and exclamation marks.
- Commit messages say what changed and why, in a sentence or two.

By contributing you agree that your work is released under the [MIT License](LICENSE).
