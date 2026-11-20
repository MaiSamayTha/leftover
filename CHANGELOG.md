# Changelog

Every release of Leftover, newest first. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/). The release workflow publishes each version's section below as its GitHub release notes.

## [Unreleased]

## [0.1.4] - 2026-11-20

No more Photos prompt on macOS, coding agent scratch folders, sixteen more tools, and a better terminal.

### Fixed

- On macOS, Explore and the project scan no longer open your Photos library, Contacts or Calendars, so macOS no longer asks for access to your photos. Explore lists them as not opened.
- When everything found is under `--min`, `leftover scan` now says nothing is over that size and how many smaller items are hidden, instead of "0 B can be cleared safely in 5 folders".

### Added

- **Coding agent scratch folders:** Claude Code's per-session scratch folders in `/tmp` are listed, one per project, as worth a look. Leftover never moves them, and says when Claude Code is running, since a task may still be using them.
- **Old temporary files on macOS and Linux:** anything in your temporary folder untouched for over a week is listed as worth a look, never cleaned.
- **More tools:** ccache, sccache and Bazel output, NuGet's HTTP cache, Hypothesis's `.hypothesis` database, coverage.py's `htmlcov`, nyc's `.nyc_output` and Xcode Simulator runtimes. Visual Studio's `.vs`, Cloudflare Wrangler's `.wrangler`, Flatpak, Snap, and local models from llama.cpp, GPT4All, Jan and ComfyUI are shown as worth a look or as your data, never cleaned.
- `leftover vault restore --last` puts back everything from the most recent clean.
- `leftover report` prints a Markdown summary of the last scan to share: totals by verdict and tool, the biggest items and idle projects, with paths under your home written as `~`. `--json` gives the same for scripts.
- `leftover completion bash|zsh|fish|powershell` prints a tab-completion script for commands, flags, item ids and vault ids. Homebrew and the `.deb` install it for you.
- `leftover scan` shows your disk's free space and how much clearing would add, and `--json` includes it.
- Documented exit codes for scripts, cron and CI: 0 done, 1 error, 2 bad usage, 3 partly done, 130 interrupted.

## [0.1.3] - 2026-11-20

A new look for the app.

### Changed

- A new look for the app: an overview with a ring chart of what can be cleared, five categories (AI models, Containers, Packages, Build output, Caches) with a switch on each item, a details panel, the disk's free space in the sidebar, and Undo right after cleaning.
- Cleaning everything from the overview first shows a summary by category, warns about projects you changed this week and large re-downloads, and says when the space actually frees up.

## [0.1.2] - 2026-11-20

Explore any folder, Docker beyond Docker Desktop, and updates from inside the app.

### Added

- **Explore:** `leftover explore [path]` and a new Explore view in the app show what any folder holds, biggest first, with what Leftover recognises in each folder and how much of it is safe to clear. A whole home folder is measured in seconds.
- **Docker:** OrbStack, Colima and Docker Engine on Linux are recognised alongside Docker Desktop. `leftover docker` shows what Docker is holding, and `leftover docker prune` (or the button in the app) asks Docker to remove its build cache and dangling images, plus unused images with `--images`. Containers and volumes are never touched, and every prune is in the history.
- **Updates:** Settings has a **Check for updates** button, and the terminal has `leftover update`. If a newer version is out, **Update now** downloads it, checks it against the release's signature and checksum, puts it in place and restarts. Homebrew, Scoop and `.deb` installs are told the command to run instead. Leftover only goes online when you click the button; it never checks by itself.
- **New project types:** Unity's `Library`, Elixir's `_build` and `deps`, Haskell's `.stack-work` and `dist-newstyle`, Zig's `.zig-cache` and `zig-out`, sbt's `target`, Clojure's `.cpcache`, Angular's `.angular`, `.docusaurus`, `storybook-static` and test coverage reports. Unreal's `Intermediate` and `DerivedDataCache` and Godot's `.godot` are shown as worth a look.

### Changed

- Nothing is moved while the tool that uses it is running. Gradle daemons, Android Studio, Xcode, the iOS Simulator, Android emulators, Ollama, JetBrains IDEs and Homebrew are checked first, and Leftover says how to stop them.
- Project folders that hold files committed to git (a checked-in `vendor`, say) are shown as worth a look and never offered for cleaning.
- New icon: a cookie with a bite taken out.

### Fixed

- `node_modules` inside extensions installed in Qoder, Windsurf, Kiro, Trae and other VS Code-based editors were listed as safe to clear, and clearing them broke the extensions. Editor extensions are now never treated as projects; they're shown as your data.

## [0.1.1] - 2026-11-19

Leftover is now a normal download for people who don't use a terminal.

### Added

- **macOS:** `Leftover.dmg` with Leftover.app, one universal app for Apple silicon and Intel. Open it, drag Leftover onto Applications.
- **Windows:** `Leftover-Setup.exe`, a regular installer. It installs for your user without admin rights, adds a Start menu entry (plus an optional desktop shortcut) and the `leftover` command, and uninstalls from Settings → Apps. The Start menu entry opens Leftover without a console window.
- **Linux:** `.deb` packages for x86-64 and ARM64 with an app-menu entry.
- The website's Download button picks the right one for your computer and shows what to click the first time you open it.

### Fixed

- The macOS install script stopped with "unbound variable" before downloading anything.
- The install scripts no longer hang on a stalled download, and the Windows one no longer fails when the Start menu folder can't be found.

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

[Unreleased]: https://github.com/MaiSamayTha/leftover/compare/v0.1.4...HEAD
[0.1.4]: https://github.com/MaiSamayTha/leftover/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/MaiSamayTha/leftover/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/MaiSamayTha/leftover/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/MaiSamayTha/leftover/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/MaiSamayTha/leftover/releases/tag/v0.1.0
