# go-hogkill (hk) — specification

Go port of [igorfelipeduca/hogkill](https://github.com/igorfelipeduca/hogkill): interactive terminal killer for CPU and RAM hogs, one row per app.

## Reader

Developer installing or hacking on `hk`. After this document you can implement, test, and release without reopening the TypeScript original for product rules.

## Product promise

- Show apps (grouped processes), not a raw PID dump.
- Rank by live CPU or memory.
- Kill with one key (or `hk kill`), with risk explained, never blocked.
- List does not re-sort under the cursor while you navigate.

---

## Grilled decisions (resolved)

Each decision was stress-tested; recommendations locked below.

### 1. Binary name

| Option | Tradeoff |
|--------|----------|
| `hk` only | Short; rare PATH collisions (legacy Heroku CLI, hakuin) |
| `hogkill` only | Clear; longer to type |
| Both | Upstream parity; dual install noise |

**Lock:** binary `hk` only. Module/repo may stay `go-hogkill`. Document collisions in README.

### 2. CLI shape (no magic argparse)

| Option | Tradeoff |
|--------|----------|
| Flat flags only (upstream) | Familiar to hogkill users; hard to extend |
| kubectl-style subcommands | Explicit modes; root can still default to TUI |
| Subcommands only, no root TUI | Forces `hk top` every time — worse UX |

**Lock:**

```
hk [filter] [global flags]     # interactive TUI (default)
hk top|ls|list [flags]         # one-shot table / json
hk kill|rm <pattern> [flags]   # non-interactive kill
```

No implicit “any unknown positional becomes filter on every command.” Only root takes optional filter arg (same as `--filter`).

### 3. Flags placement

Shared on root as **persistent** flags (inherited by `top` / `kill` where useful). Mode-specific flags stay local:

| Scope | Flags |
|-------|--------|
| Persistent | `--sort`, `--mem`, `--interval`, `--min-cpu`, `--min-mem`, `--user`, `--me`, `--filter`, `--safe-only`, `--no-color`, `--dry-run` |
| `top` | `--top`/`-n`, `--flat`, `--json` |
| `kill` | `--yes`/`-y`, `--force`/`-9` |
| Root only extra | positional filter |

`--json` implies list mode (only on `top`). Interval is `time.Duration` (default `1500ms`).

### 4. TUI stack

| Option | Tradeoff |
|--------|----------|
| Raw terminal like upstream | Zero TUI deps; more code, fragile |
| Bubble Tea + Lip Gloss + Bubbles textinput | Matches agreed stack; modes/async kill fit Elm loop |

**Lock:** `github.com/charmbracelet/bubbletea`, `lipgloss`, `bubbles/textinput`. Hand-roll the process table (not `bubbles/list`).

### 5. Process source

| Option | Tradeoff |
|--------|----------|
| Shell out to `ps` | Upstream-faithful; brittle parsing |
| `/proc` only | Linux-only |
| gopsutil/v4 | Cross-platform, no cgo; owned grouping above it |

**Lock:** `github.com/shirou/gopsutil/v4`. Live CPU = delta of process CPU times between samples, EMA smooth (α=0.6), same idea as upstream. Cold start: two samples ~600ms apart for non-interactive; TUI improves after first tick.

### 6. Platforms

| Option | Tradeoff |
|--------|----------|
| Linux only v1 | Faster ship; incomplete port |
| Linux + macOS + Windows | Matches upstream claims |

**Lock:** build all three. Windows: kill is terminate-only; hide USER / reject `--user`/`--me` with a clear error if ownership is unavailable cheaply (same honesty as upstream). Risk tables per OS.

### 7. Package layout (simple)

```
cmd/hk/                 main + version ldflags
internal/cmd/           cobra only
internal/proc/          sample, group, name, risk
internal/kill/          signal policy
internal/render/        format, table, json (non-TUI + helpers)
internal/tui/           bubbletea model
```

No separate packages for group/name/risk until files hurt.

### 8. Release

| Piece | Choice |
|-------|--------|
| Version tags | svu (`.svu.yml`: no `v` prefix, `always`, `v0`) |
| Artifacts | goreleaser → `hk` binary, linux/darwin/windows amd64+arm64 |
| Toolchain | mise (`go`, `goreleaser`, `svu`) |

### 9. Kill policy

- Never refuse a kill; show risk, then obey.
- SIGTERM then wait 4s then SIGKILL (unless `--force` / `-9` or Windows).
- `own` (self + ancestor shell/terminal) last in a batch.
- Pattern kill (`hk kill`) never includes `own` processes (argv self-match).
- EPERM → message “permission denied — rerun with sudo”.
- `--dry-run` reports without signalling.

### 10. Held list

Re-rank only when **not held**:

`held = pinned || cursor > 0 || selection non-empty || mode != list`

While held: keep group/proc order keys; update numbers only. Sort key change or `g`/unpin at top re-ranks. Explicit sort always re-ranks.

### 11. Out of scope (v1)

- Daemon / background agent
- Remote process management
- Custom risk config files (code tables only; PRs welcome)
- Dual binary name `hogkill`
- Docker image publish (goreleaser archives only unless added later)

---

## Feature parity checklist

### Interactive (root)

| Key | Action |
|-----|--------|
| ↑↓ / k j | move |
| →← / l h | expand / collapse |
| space | select |
| a / x | select all groups / clear |
| d | kill (term→kill) |
| D | force kill |
| / | filter |
| s / c / m | cycle sort / cpu / mem |
| p | pin order |
| g / G | top / bottom |
| r | refresh + re-rank |
| ? | help |
| q / Esc | quit |

Header: proc count, cpu bar, ram bar, load, live|held|pinned, sort, filter.

### `hk top`

Table or `--json`. `--flat` nests PIDs. `-n` limits rows.

### `hk kill <pattern>`

Match group name or command line. Confirm unless `-y` or `--dry-run`. Non-TTY requires `-y`.

### Risk column

| Tag | Level |
|-----|--------|
| critical | OS leans on it |
| system | daemon / system user |
| you | hk itself or session lineage |

---

## Data model

```text
Proc: pid, ppid, rss, cpu%, cpuSeconds, elapsed, user, command, exe, name, risk, riskReason
Group: key, name, procs[], cpu, rss, user, risk, riskReason
SortKey: cpu | mem | count | name
```

Group key: `"$user $groupName"`. Group name: macOS `.app` bundle, else interpreter+script, else exe basename.

---

## Success criteria

1. `mise run ci` green.
2. `mise run test:smoke` prints top apps on this machine.
3. `hk top --json` is valid JSON with cpu/rss/risk fields.
4. `hk kill --dry-run` of a harmless pattern prints plan, signals nothing.
5. Interactive: list holds order when cursor moves; `g` re-ranks.
6. Goreleaser config builds `hk` for linux/darwin/windows without network release dry-run locally: `mise exec -- goreleaser build --snapshot --clean` (when tools installed).

---

## Implementation order

1. `proc` sampler + group + risk + unit tests for name/group/sort
2. `kill` + dry-run
3. `render` + `hk top` / `hk kill`
4. `tui`
5. release files (already scaffolded)

---

## Terminology

| Concept | Approved term | Avoid |
|---------|---------------|--------|
| Binary | hk | hogkill (except product prose) |
| Folded processes | app / group | job, service |
| Live ranking off | held | frozen, locked |
| Force pin | pinned | sticky |
| Danger label | risk | severity, priority |
