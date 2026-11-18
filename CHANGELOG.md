# Changelog

Every release of Leftover, newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). The release workflow publishes each version's section below as its GitHub release notes.

## [Unreleased]

## [0.1.0] - 2026-11-19

The first release.

### Finds and explains

- Scans caches at their fixed locations and dependencies and build output inside your projects: JavaScript, Python, AI models, Rust, Go, the JVM, Apple and mobile tooling, Ruby, PHP, .NET, Dart/Flutter, Terraform and more.
- Every item has a plain-language explanation, the tool that made it, how it comes back, and a link to the tool's documentation. Anything without a documented source is shown as *worth a look* and is never offered for cleaning.
- Breaks big folders down: each model in Ollama's folder (and which share layers), each Hugging Face model, each Xcode version, and what `docker system df` says is reclaimable inside Docker's disk image.
- Ties dependencies and build output to their project and shows how long since you last worked on it, so `--idle 90` clears only what you've stopped using.
- `leftover why <path>` explains any folder, even ones a scan skipped. The desktop window has the same thing under **What's this?**

### Cleans safely

- Cleaning moves items into a vault on the same drive instead of deleting them. Everything can be restored for 14 days (configurable), and the space comes back when the vault is emptied.
- A manifest is written before anything moves, items with several folders move all-or-nothing, and if a failed move can't be undone the files stay in the vault, restorable, instead of being lost.
- Restoring next to a folder your tools already rebuilt puts it beside the new copy, never over it.
- `clean` always shows its plan and asks first. Without a terminal it does nothing unless you pass `--yes`.
- Every move, restore and deletion is logged; see `leftover history`.

### Desktop window and command line

- `leftover` with no arguments opens the desktop window, served only to your machine on `127.0.0.1` with a per-launch key.
- Commands: `scan`, `why`, `clean`, `vault`, `history`, `ignore`, `rules`, `where`, `version`. Most take `--json`.

### Install

- Single binaries for macOS, Linux and Windows (x86-64 and ARM64), with SHA-256 checksums.
- Install scripts that verify the checksums and add a launcher; Homebrew and Scoop installs from this repository.

[Unreleased]: https://github.com/MaiSamayTha/leftover/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/MaiSamayTha/leftover/releases/tag/v0.1.0
