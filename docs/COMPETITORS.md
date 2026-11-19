# Competitor research

Internal notes behind the "How it compares" section of the landing page
(`site/index.html`, `#compare`). Researched 20 November 2026 from each
product's own website, documentation, README and, for open-source tools, its
source code. Every cell on the landing page comes from this file. When a
competitor ships something new, update the table here first and then the page.

The section is plain HTML: each cell holds one of three indicators, a hidden
label for screen readers, and a short note. Keep the three levels consistent:

- **Supported**: documented and does the job.
- **Partly**: does some of it, or only in one mode, edition or platform.
- **Not a core focus, or not documented**: no evidence it does this. This is
  not a claim that it can't.

## The matrix

L = Leftover, DC = DevCleaner (devcleaner.app), Mo = Mole, np = npkill,
CMM = CleanMyMac, BB = BleachBit, WT = WizTree, CLI = ncdu / dust / gdu.
● supported, ◐ partly, ○ not a core focus or not documented.

| Capability | L | DC | Mo | np | CMM | BB | WT | CLI |
|---|---|---|---|---|---|---|---|---|
| General disk analysis | ○ | ○ | ● | ○ | ● | ○ | ● | ● |
| Developer artifact detection | ● | ● | ● | ● | ● | ◐ | ○ | ○ |
| Project-level awareness | ● | ● | ● | ● | ● | ○ | ○ | ○ |
| Contextual explanations | ● | ● | ◐ | ● | ◐ | ◐ | ○ | ○ |
| Regeneration guidance | ● | ● | ○ | ◐ | ◐ | ○ | ○ | ○ |
| AI model storage | ● | ● | ○ | ○ | ● | ◐ | ○ | ○ |
| Docker storage | ◐ | ● | ◐ | ○ | ◐ | ◐ | ○ | ○ |
| Reversible cleanup | ● | ◐ | ◐ | ○ | ○ | ○ | ○ | ○ |
| Privacy and local-first | ● | ◐ | ● | ● | ◐ | ● | ● | ● |
| Command line | ● | ○ | ● | ● | ● | ● | ◐ | ● |
| Cross-platform | ● | ◐ | ● | ● | ○ | ◐ | ● | ● |

CCleaner, WinDirStat, TreeSize, DaisyDisk, kondo, ClearDisk and the single-tool
commands (`du`, `df`, `docker system prune`, `npm cache clean`) are covered in
the notes below and in the category cards, but left out of the table to keep it
readable.

## Leftover, verified against the code

Checked in this repo, not taken from marketing copy.

- **Rules:** 66 rules for 51 tools in `internal/rules/catalog/*.json`. 54 are
  cleanable (`rebuild` or `redownload`). Every cleanable rule has a `comeback`
  text and a `source` link; `TestRejectsUnsourcedCleanableRule` enforces the
  source.
- **Projects:** `internal/scan/projects.go` walks the scan roots (home folder by
  default, `roots` in settings). It finds project folders like `node_modules`
  and `target` beside their marker files, groups them by project, and records
  idle days (`IdleDays` in `internal/scan/find.go`). The CLI filters by age with
  `--idle`.
- **Explanations:** each rule has `what`, `comeback`, `source`, and sometimes a
  `command`. `leftover why <path>` explains any folder (`cmd/leftover/cmd_why.go`).
- **AI models:** Hugging Face hub and datasets, Ollama, LM Studio, PyTorch Hub
  and Whisper (`ai.json`). Hugging Face and LM Studio use `each`, so every model
  is its own item. **Ollama is one item for the whole store, not per model.**
- **Docker:** `docker-disk` is `keep`. Leftover never moves Docker's disk image.
  It reads `docker system df` and shows each type's size, how much is
  reclaimable, and the matching prune command (`internal/scan/docker.go`). Paths
  cover Docker Desktop on macOS, Windows and Linux. Native Linux Docker
  (`/var/lib/docker`), OrbStack and Colima aren't covered.
- **Vault:** cleaning renames items into a vault on the same drive
  (`internal/vault/vault.go`), kept for 14 days by default (`DefaultKeepDays` in
  `internal/config/config.go`). Restore is exact. **Space only comes back when
  the vault is emptied or the items expire.** This is the same trade-off as the
  Trash, and the page should never imply otherwise.
- **Privacy:** no HTTP client anywhere. The only `net/http` use is the local UI
  server, bound to `127.0.0.1` with a token (`internal/ui/server.go`). No
  telemetry and no update checks.
- **CLI:** `scan`, `clean`, `why`, `vault`, `history`, `ignore`, `rules`,
  `where`, `app`, `version`; `--json` on scan, clean, why, vault, history and
  rules.
- **Platforms:** macOS (universal), Windows (x64, ARM64), Linux (.deb amd64 and
  arm64, plus archives). Every OS is exercised in CI.
- **Not present:**
  - general disk browsing or a treemap;
  - user-defined rules;
  - any check that the owning tool is running before moving its files;
  - hard-link folding on Windows (`stat_windows.go`).

## Competitors

### DevCleaner (devcleaner.app): the closest competitor

Sources: <https://devcleaner.app/>, [/pricing](https://devcleaner.app/pricing),
[/changelog](https://devcleaner.app/changelog), [/faq](https://devcleaner.app/faq),
[/docs/supported-tools](https://devcleaner.app/docs/supported-tools),
[/docs/staying-safe](https://devcleaner.app/docs/staying-safe).

- **Platforms:** macOS menu-bar app (1.17.2, 2026-11-09) and a Linux build
  (1.1.1, 2026-10-17; .deb, apt, AppImage). No Windows.
- **Coverage:** very broad.
  - Xcode in depth, plus simulators via `simctl`.
  - Android Studio and SDK, JetBrains, VS Code, CocoaPods, Homebrew.
  - npm, Yarn, pnpm, Bun, pip, uv, Flutter, Rust, Go, Maven, NuGet, Composer,
    Unity, version managers and Conda.
  - Many AI tools: Claude, Cursor, Codex, Windsurf, Zed, Kiro and others.
  - Ollama, LM Studio, Hugging Face and PyTorch.
  - Docker, OrbStack and Colima.
- **Projects:** artifacts are grouped by project with last-used dates. It skips
  running builds and Git-tracked files. It also finds "ghost projects", meaning
  DerivedData for workspaces that no longer exist.
- **Safety:** four risk levels (Safe, Warning, Danger, Danger+). Each category
  shows its paths, sizes and dates, and gives "rebuild hints". Credentials are
  never touched. AI tool cleanup is blocked while the tool is running.
- **Reversibility:**
  - Deletion defaults to "by risk": Safe items are deleted permanently, and
    Warning and Danger items go to the Trash.
  - Settings allow "always Trash" or "always delete".
  - Project hibernation archives source and lockfiles and can restore them.
  - It keeps cleanup history and JSON deletion manifests.
  - No undo is documented.
- **Docker:** `docker builder prune -af` and a guided `docker system prune -af`;
  volumes are Danger. Compose projects can be hibernated, with named volumes
  snapshotted.
- **CLI:** none documented; it offers `devcleaner://` URL schemes and a widget.
  Not open source, as far as the pages say.
- **Price:** free tier for safe caches. Pro costs $19.99 a year or $59 for life,
  with a 14-day trial.
- **Privacy:** an opt-out anonymous "space freed" counter, a Pro licence check
  about every 6 hours, and automatic updates.

Name clash: "DevCleaner for Xcode" (<https://github.com/vashpan/xcode-dev-cleaner>,
GPL-3.0, Mac App Store, 2.8.0 from 2025-11-11) is a different, older app.
It cleans Xcode only: device support, archives, DerivedData, documentation
cache and old logs. It has a dry run and a command-line mode, and deletes with
`FileManager.removeItem`, so removal is permanent.

### npkill

Sources: [README](https://github.com/voidcosmos/npkill),
[docs/profiles.md](https://github.com/voidcosmos/npkill/blob/main/docs/profiles.md),
[result descriptions](https://github.com/voidcosmos/npkill/blob/main/src/constants/result-descriptions.constants.ts).
v1.0.0, 2026-11-12, MIT.

- No longer only `node_modules`. It has 20 built-in profiles: node, python,
  java, android, dotnet, rust, elixir, haskell, scala, cpp, unity, unreal,
  godot, clojure, elm, php, dart, swift, zig and terraform. Many targets only
  count next to marker files, the same idea as Leftover's project rules.
- Each target has a description, and many say how to restore it ("run
  `npm install`").
- Shows "workspace" last-modified age. `--older-than` filters by it.
- Flags app-owned `node_modules` as sensitive (⚠) and hides them by default.
  **Leftover currently doesn't; see the Qoder issue below.**
- Deletion is permanent. It has `--dry-run`, `--json`, `--json-stream`, a Node
  API and `.npkillrc` for custom profiles.
- Runs anywhere Node runs.
- No global caches by default, no AI models and no Docker.

### Mole

Sources: [README](https://github.com/tw93/Mole),
[Windows README](https://github.com/tw93/Mole/tree/windows),
[clean-dev rules](https://github.com/tw93/Mole/blob/main/.claude/rules/clean-dev.md),
`lib/clean/dev.sh`. V1.73.0, 2026-11-18, GPL-3.0, about 75k stars.

- A broad Mac toolkit: `clean`, `uninstall`, `optimize`, `analyze` (disk
  explorer), `status`, `purge` and `installer`. Windows and Linux CLIs live on
  separate branches. There is also a separate native "Mole for Mac" app.
- Developer caches are classified by "recovery contract". It skips tools that
  are busy (for example "pnpm busy").
- `mo purge` groups build output by project. Anything active in the last
  7 days is left unselected.
- **Hugging Face, PyTorch and TensorFlow caches are deliberately kept off the
  delete path, and Ollama is whitelisted.** That is a design choice, not a gap.
- Docker: prunes build cache and dangling images through Docker's own
  commands, and only on a local engine.
- `clean`, `purge` and `installer` delete permanently. `uninstall` and
  `analyze` deletions go to the Trash. `--dry-run` is available everywhere,
  and an operations log is kept (`mo history`).
- No per-item explanation of what a folder is or how it comes back is
  documented.

### CleanMyMac (MacPaw)

Sources: <https://macpaw.com/news/cleanmymac-cli>,
<https://github.com/MacPaw/cleanmymac-cli>,
<https://macpaw.com/how-to/mac-disk-space-developer-cleanup-guide>,
<https://macpaw.com/support/cleanmymac/knowledgebase/system-junk-cleanup-list>,
<https://macpaw.com/support/cleanmymac/knowledgebase/ai-clutter>,
<https://cleanmymac.com/>.

- macOS only (macOS 11+). Commercial, from $3.33 a month. The app is at
  version 5.7.4.
- **App:** "Xcode Junk" covers derived data, module caches, device support,
  logs, archives and simulators. "AI Clutter" covers caches from Claude, Codex,
  Gemini and Cursor, plus "Local Models" from Ollama and LM Studio. Space Lens
  maps the drive. Per-item "Smart Insights" need macOS 26 with Apple
  Intelligence.
- **CLI:** launched July 2026 and still in public beta; proprietary, under a
  EULA.
  - `clean dev` covers Homebrew, npm, Yarn, pnpm, pip, Cargo, Go, CocoaPods,
    Docker, VS Code, JetBrains, Maven, Gradle, Poetry, uv, Bun, Deno and mise.
  - `purge` groups artifacts by project and preselects those older than
    7 days.
  - `analyze` explores disk usage.
  - It asks for confirmation before acting; `--force` skips that.
- Trash vs permanent deletion and undo are not documented. CLI analytics can be
  turned off; the default isn't stated.

### BleachBit

Sources: <https://www.bleachbit.org/>, <https://www.bleachbit.org/news>
(6.0.0 to 6.0.6), <https://www.bleachbit.org/download/mac>,
<https://docs.bleachbit.org/doc/command-line-interface/>,
<https://docs.bleachbit.org/doc/expert-mode.html>. GPL-3.0, free.

- Windows 10/11 and Linux. macOS is "experimental": there is no .dmg yet, and
  it runs from source.
- Recent developer cleaners:
  - Claude Code (6.0.1 onward);
  - VS Code, Codium, Cursor, Windsurf, Antigravity and Devin (6.0.2);
  - Android Studio and the Gradle cache (6.0.4);
  - Docker Desktop (6.0.5 on macOS, 6.0.6 on Windows and Linux).
- **Opt-in "deep scan"** for `node_modules`, `venv`, `__pycache__` and
  `.angular` (6.0.1 and 6.0.2). The docs don't say how it decides these are
  safe; the expert-mode doc warns the scan "may find files you want to keep".
- Cleaners have descriptions and warnings. Nothing is documented about how
  things come back.
- It has a preview, then deletes permanently; shredding is optional. No undo.
- AI: only Chrome's on-device model.
- CLI: `--list-cleaners`, `--preview`, `--clean`.
- No telemetry. It has an optional update check and a winapp2.ini download on
  Windows.

### WizTree

Sources: <https://diskanalyzer.com/>, [/guide](https://diskanalyzer.com/guide),
[/whats-new](https://diskanalyzer.com/whats-new).

- **Now on Windows, macOS (WizTreeMac 1.00, September 2026) and Linux
  (WizTreeNix 1.01, November 2026).** Earlier claims that it's Windows-only
  are out of date.
- Reads the NTFS MFT directly, so scans are very fast ("46x faster than
  WinDirStat", their claim). Has a treemap, a largest-files view and
  duplicates; its duplicates are matched by name, size and date, not content.
  CSV export.
- Command line: CSV export, treemap PNG export and an MFT dump.
- No developer awareness. Delete behaviour isn't documented on Windows or
  macOS; the Linux build uses the trash.
- Free for personal use; paid Supporter Code or Enterprise licence otherwise.
  Not open source.

### Other tools (in the category cards, not in the table)

- **CCleaner:** ccleaner.com refuses automated fetches (HTTP 403), so nothing
  was verified first-hand.
  - Third-party sources describe it as a Windows, Mac and Android system
    cleaner with a paid Professional tier.
  - They also say some data collection is on by default and can be controlled.
  - Treat these as unverified. Make no specific claims about it on the page
    beyond "system cleaner".
- **WinDirStat** (<https://windirstat.net/>,
  [README](https://github.com/windirstat/windirstat),
  [Cleaning-Up wiki](https://github.com/windirstat/windirstat/wiki/Cleaning-Up)):
  - Windows only, GPL-3.0.
  - Treemaps, sunbursts, flame graphs, content-hash duplicates.
  - CSV and JSON export and command-line reports (`/SaveTo`).
  - "Delete (to Recycle Bin)" plus deleted-file recovery.
  - No developer awareness.
- **TreeSize** (<https://www.jam-software.com/treesize>):
  - Windows. Free for private use; Personal is €20.40 per user per year, and
    Professional starts at €41.00.
  - Treemaps, pie and bar charts, duplicates (Personal and up).
  - Command line and scheduling in Professional only.
  - Says it runs "entirely on a local, on-premise basis".
  - No developer awareness.
- **DaisyDisk** (<https://daisydiskapp.com/>):
  - macOS only, $9.99 once.
  - Sunburst map, scans physically used space (clones and hard links counted
    once).
  - Deletion is permanent, with a 5-second cancel window.
  - "Doesn't collect or transmit anything". No CLI, no developer awareness.
- **kondo** ([README](https://github.com/tbillington/kondo)):
  - Rust CLI plus GUI, 20+ project types, `--older`, on winget, Homebrew and
    MacPorts.
  - "Essentially `rm -rf` with a prompt."
- **ClearDisk** (<https://github.com/bysiber/cleardisk>):
  - MIT macOS menu-bar app (about 760 stars).
  - Developer caches: Xcode, node_modules, CocoaPods, SPM, Docker, pip, Cargo.
- **ncdu, dust, gdu:**
  - ncdu: TUI for any POSIX system (<https://dev.yorhel.nl/ncdu>); it can
    delete.
  - dust: Linux, macOS, Windows; read-only.
  - gdu: Go TUI that runs everywhere; deletes, with `--no-delete` available,
    and exports JSON.
- **`du`/`df`, `docker system prune`, `npm cache clean`:**
  - Exact, built in, and permanent. Each covers one tool or one disk.
  - These are standard, well-documented behaviours; their docs weren't
    re-fetched for this review.

Also seen but not researched in depth: devclean (HuakunShen), Dev Cache
Cleaner, DevCacheCleaner, MegaCleaner, cleanmydev, CrunchyCleaner,
cache-vent. Several are new in 2026. The space is getting crowded.

## Findings

### Where Leftover is genuinely ahead

1. **Reversible by default.** Every clean goes to a vault with exact restore.
   No other tool researched makes undo the default for developer cleanup:
   - DevCleaner deletes Safe items permanently by default.
   - Mole's `clean`/`purge`, npkill, kondo and BleachBit delete permanently.
2. **Sourced explanations.** Each cleanable rule says how the item comes back
   and links to the owning tool's docs, and the tests enforce it. DevCleaner
   has rebuild hints and npkill has descriptions, but neither links a source.
   `leftover why <path>` has no direct equivalent.
3. **Local-only by construction.** It has no network code at all: no licence
   checks, no counters, no update pings. DevCleaner and CleanMyMac both
   phone home in some form.
4. **Free, open source, GUI plus CLI, and all three OSes.**
   - DevCleaner has no Windows version and no CLI.
   - CleanMyMac is Mac-only and paid.
   - Mole's GUI is Mac-only.
   - npkill and kondo are terminal-first.

### Where competitors are already better

1. **Coverage depth.** DevCleaner covers far more tools, especially:
   - AI coding tools;
   - version managers;
   - simulators through `simctl`;
   - OrbStack and Colima;
   - Unity.

   npkill covers project types Leftover doesn't: Unity, Unreal, Godot, Elixir,
   Haskell, Scala, Zig, CMake, coverage folders, `.angular`, `.nx`, `.vite`,
   Storybook and Docusaurus.
2. **Safety signals.**
   - npkill flags app-owned `node_modules`, and DevCleaner skips Git-tracked
     files and running builds.
   - Mole and DevCleaner check whether a tool is busy.
   - **Leftover has none of these, and currently lists `node_modules` inside
     editor extension folders such as `~/.qoder/extensions` as safe.** That is
     a real bug: `skipNames` in `internal/scan/projects.go` only covers
     `.vscode` and `.cursor`.
3. **Docker.** DevCleaner and Mole actually reclaim Docker space through
   Docker's own prune commands. Leftover only reports it.
4. **Disk analysis.** WizTree, WinDirStat, DaisyDisk, Mole and CleanMyMac show
   the whole disk. Leftover only shows what it recognises, and only under the
   scan roots.
5. **Speed.** WizTree's MFT scanning is in a different class on Windows.
6. **Freeing space now.** The vault means space isn't freed until it's emptied
   or expires. DevCleaner's by-risk policy frees cache space immediately.

### Missing features to consider (not built as part of this work)

In rough priority order:

1. **Skip editor and app extension folders.** Treat any folder with a
   `.vsixmanifest` or `package.json` inside an `extensions` directory as
   app-owned. Add Qoder, Windsurf, Kiro, Trae, VS Code Insiders, VSCodium,
   Positron and Zed to `skipNames`. This is a safety fix, so it comes first.
2. **Busy-tool checks.** Skip or warn while the owning tool is running, for
   example Gradle daemons, Xcode, Docker or Ollama.
3. **Per-model Ollama breakdown** by reading the manifests. The landing page's
   hero badge already implies this.
4. **More project types**, matching npkill's profiles.
5. **Docker:** cover native Linux Docker, OrbStack and Colima. Offer a guided
   build-cache and dangling-image prune, never volumes.
6. **A "free it now" option** for `rebuild` items, so the vault isn't the only
   path. Or at least show clearly how much space the vault is holding.
7. **Whole-drive view and other drives** (issue #20), probably a simple
   treemap.
8. **Simulators via `xcrun simctl delete unavailable`.**
9. **User-defined rules**, similar to `.npkillrc`.
10. **Project hibernation** (DevCleaner Pro's idea): archive an idle project's
    source and lockfiles and restore it later.

### What nobody handles well yet

- **Duplicate models across stores.** The same weights can sit in Hugging Face,
  Ollama and LM Studio at once. No tool detects this.
- **Docker volume ownership.** No tool explains which project a volume belongs
  to before you prune.
- **True reclaimable space.** pnpm's hard-linked store, APFS clones and Time
  Machine local snapshots make most "you'll free X GB" numbers optimistic.
  DaisyDisk counts physical space but isn't developer-aware.
- **Monorepos.** Tools treat each workspace package's `node_modules`
  separately, with no view of the whole repo.

### Differentiators worth leaning into

- Sourced, test-enforced explanations ("every safe item says how it comes back,
  with a link to the tool's docs").
- Undo as the default, honestly framed: space is freed when the vault empties.
- Zero network code, verifiable in the source.
- One tool for the GUI, the CLI and `--json` on all three OSes.

## Leftover claims on the landing page that the code doesn't fully back

- **Hero badge, "Breaks down Ollama and Hugging Face models":** Hugging Face
  and LM Studio are broken down per model. Ollama is one item for the whole
  store.
- **CLI demo (`#docs`):** "140.8 GB … in 2,184 folders, 4.2s", the 61.3 GB
  clean, and the scan window's sizes are illustrative, not measured. The
  `leftover clean --idle 90` command itself is real.
- **"Reversible by design. A 14-day vault":** accurate. But nowhere on the page
  does it say that vaulted items still use disk space until the vault is
  emptied.
- **"Works offline" and "no update checks":** verified. The binary has no HTTP
  client.
