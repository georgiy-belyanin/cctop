# cctop — Claude Code top

`cctop` is a Go CLI that discovers all running Claude Code instances on this machine and renders
them in a live-updating terminal UI (one row per instance): **status, effort level,
model, working directory**, plus session name and uptime.

Target platforms: **macOS and Linux** (Windows is out of scope for now).

## Hard constraints

- **Go standard library only.** The single allowed exception is `golang.org/x/term`
  (and transitively `golang.org/x/sys`) for raw terminal mode — these are maintained
  by the Go team. No bubbletea, no lipgloss, no fsnotify, no third-party TUI kits.
- TUI is rendered with raw ANSI escape sequences (alternate screen, cursor
  positioning, SGR colors). Keep the renderer small and hand-rolled.
- No daemon, no config file, no network. The tool is read-only: it must never write
  into `~/.claude/`.
- Platform-specific code goes in build-tagged files (`_darwin.go` / `_linux.go`),
  shared logic stays portable.

## Where the data comes from (verified against Claude Code v2.1.170)

Claude Code itself maintains everything we need under `~/.claude/`. Do **not** parse
`ps` output to find instances — there is a proper registry.

### 1. Session registry — primary source

`~/.claude/sessions/<PID>.json` — one file per running instance, updated live:

```json
{
  "pid": 84330,
  "sessionId": "76dbaebe-8d96-420a-99d5-ae7c7a2f5f66",
  "cwd": "/Users/g.belyanin/Work/Repositories/tt-vkdoc",
  "startedAt": 1781084834599,
  "procStart": "Wed Jun 10 09:47:09 2026",
  "version": "2.1.170",
  "peerProtocol": 1,
  "kind": "interactive",
  "entrypoint": "cli",
  "status": "busy",
  "updatedAt": 1781093739769,
  "name": "migrate-rst-docs-markdown"
}
```

- `status` values observed so far: `idle`, `busy`, `shell`. Treat the field as an
  open enum — render unknown values verbatim rather than failing.
- `name` is the optional session title; absent for untitled sessions.
- `startedAt` / `updatedAt` are Unix milliseconds.

**Liveness:** files can go stale after a crash. An entry counts as alive only if
`syscall.Kill(pid, 0)` succeeds (works on both platforms; `EPERM` still means alive).
Guard against PID reuse by comparing the file's `procStart` with the actual process
start time (`ps -o lstart= -p PID` on macOS, `/proc/<pid>/stat` field 22 on Linux).
A dead-PID entry is rendered as **error** (or dropped after a grace period), never
silently trusted.

### 2. Per-session transcript — model (and fallback metadata)

`~/.claude/projects/<munged-cwd>/<sessionId>.jsonl` — append-only JSONL transcript.
The munged dir name is the cwd with `/` and `.` replaced by `-`
(e.g. `/Users/g.belyanin/Work/x` → `-Users-g-belyanin-Work-x`). Prefer locating the
file by globbing `~/.claude/projects/*/<sessionId>.jsonl` over re-implementing the
munging.

- Assistant entries carry `message.model` (e.g. `"claude-fable-5"`,
  `"claude-opus-4-8"`) — the **per-session model** is the model of the most recent
  assistant entry.
- Entries also carry `cwd`, `gitBranch`, `version`, `timestamp` as fallbacks.
- These files get large (MBs). Never read them whole on every tick: read the **last
  ~64 KB**, split on newlines, scan backwards for the newest line containing
  `"message"` with a `"model"` key. Cache the result per session and only re-read
  when the file's mtime/size changes.

### 3. Global settings — effort level and default model

`~/.claude/settings.json`:

```json
{ "effortLevel": "high", "model": "claude-fable-5[1m]" }
```

- `effortLevel` (`low`/`medium`/`high`/`xhigh`/`max`) is the effort shown for all
  instances unless a per-session source is found (none is known today — see Open
  questions). Missing key means the Claude Code default (display as `high (default)`
  or `—`).
- `model` here is the configured default, including display suffixes like `[1m]`;
  the transcript value is authoritative for a live session.
- Also check the project-level `.claude/settings.json` / `.claude/settings.local.json`
  inside each instance's cwd — project settings override user settings.

## Status mapping

The UI shows four user-facing statuses. Mapping from raw data:

| Displayed  | Condition                                                              |
|------------|------------------------------------------------------------------------|
| `idle`     | registry `status == "idle"`                                            |
| `auto`     | registry `status == "busy"` or `"shell"` (agent is actively working)   |
| `ask`      | see Open questions — likely a distinct registry status or a busy entry whose `updatedAt` has stalled while a permission prompt is pending |
| `error`    | PID dead but registry file present, registry JSON unparsable, or the newest transcript entry is an API error |

Color them: idle = dim/grey, auto = green, ask = yellow (and consider a terminal
bell/flash, since this is the state the user actually needs to act on), error = red.

## Architecture

```
cctop/
├── AGENTS.md
├── go.mod                      # module cctop; no third-party requires
├── main.go                     # flag parsing, wiring, main loop
└── internal/
    ├── instance/               # Instance struct, Status type, sorting
    ├── discover/               # registry scan, liveness, transcript+settings enrichment
    │   ├── discover.go
    │   ├── proc_darwin.go      # process start time via ps
    │   └── proc_linux.go       # process start time via /proc
    └── tui/
        ├── screen.go           # ANSI: alt-screen, clear, move, SGR, table layout
        ├── term_unix.go        # raw mode via x/term (or ioctl if x/term is dropped)
        └── loop.go             # ticker + keyboard select loop
```

- **Poll, don't watch.** Rescan `~/.claude/sessions/` every 1 s (configurable via
  `-interval`). The directory holds a handful of small files — this is cheap and
  avoids a watcher dependency.
- **Event loop:** one goroutine reads stdin keys, one ticker triggers
  rescan+redraw; both feed a `select` in `tui.Loop`. Keys: `q`/`Ctrl-C` quit,
  later: `j/k` select row, `enter` could print the cwd for shell integration.
- **Redraw, not append:** repaint the full table each tick using cursor-home +
  clear-to-end; this avoids flicker without diffing. Truncate cwd from the left
  (`…/Repositories/foo`) to fit terminal width (`term.GetSize`).
- Always restore the terminal (cooked mode, main screen, cursor visible) on exit,
  including on SIGINT/SIGTERM — use `defer` plus a signal handler.

## Development

```sh
make build                     # go build -o cctop .
make test
make lint                      # golangci-lint v2 (config: .golangci.yml)
make fmt                       # golangci-lint fmt (gofmt + goimports)
make ci                        # vet + lint + test + build — run before pushing
GOOS=linux go build ./...      # cross-compile check from macOS
```

CI (`.github/workflows/ci.yml`) runs format check + golangci-lint, then
test/build on both ubuntu and macos runners, uploading the binaries as
artifacts. Tags matching `v*` trigger `.github/workflows/release.yml`, which
cross-compiles linux/darwin × amd64/arm64 and publishes a GitHub release.
Lint findings are fixed in code, not silenced — `.golangci.yml` exclusions
are reserved for whole-class decisions (e.g. errcheck in tests).

- Tests for `discover` must not touch the real `~/.claude` — the scanner takes a
  root dir parameter; fixtures live in `testdata/` (copy the JSON shapes from the
  examples above).
- Rendering functions return strings/`[]byte` so they're testable without a TTY.
- Manual smoke test: run `claude` in another terminal and check the row appears,
  flips idle↔auto as you submit prompts, and disappears (or goes red) when the
  process exits.

## Style

- Plain Go, no cleverness: small packages, exported types documented, errors
  wrapped with `fmt.Errorf("...: %w", err)`.
- Tolerate schema drift: unknown JSON fields ignored, missing fields rendered as
  `—`, a single bad file must never crash the whole scan.
- The schemas above are **undocumented internals** of Claude Code and may change
  between versions — keep all parsing in `internal/discover`, behind small structs,
  so a format change is a one-package fix.

## Open questions (verify empirically before building on them)

1. **`ask` detection.** Does the registry `status` field have a dedicated value when
   Claude Code is blocked on a permission prompt or an AskUserQuestion? Reproduce a
   permission prompt and inspect `~/.claude/sessions/<pid>.json`. If not, fall back
   to the stalled-`updatedAt` heuristic.
2. **Per-session effort.** Is `effortLevel` ever recorded per session (transcript
   request metadata contains `"effort": "high"` strings worth tracing), or only in
   settings files?
3. **Full `status` enum.** Watch values during compaction, plan mode, and subagent
   activity; extend the mapping table as values are observed.
