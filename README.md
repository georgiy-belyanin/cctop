# cctop

`top` for Claude Code: a live terminal view of every Claude Code instance
running on your machine — what it's doing, which model and effort level it's
on, and where.

```
cctop — 5 sessions                                          15:34:39  q quit

PID     STATUS  MODEL            EFFORT  UP      NAME             CWD
90064   auto    claude-fable-5   high    16m                      ~/work/cctop
84330   auto    claude-opus-4-8  high    2h47m   migrate-rst-do…  ~/work/tt-vkdoc
43058   idle    claude-opus-4-8  high    3h38m                    ~/work/emmylua
```

Statuses: `idle`, `auto` (working), `ask` (needs your attention), `error`
(stale or crashed session). Rows are sorted by urgency.

macOS and Linux. Read-only — it never touches your `~/.claude` data.

## Install

```sh
go install github.com/georgiy-belyanin/cctop@latest
```

or grab a binary from [releases](../../releases), or build from source
(Go 1.26+):

```sh
make build    # produces ./cctop
```

## Usage

```sh
cctop                 # interactive TUI, refreshes every second; q to quit
cctop -once           # print the table once and exit (also when piped)
cctop -interval 5s    # custom refresh interval
```
