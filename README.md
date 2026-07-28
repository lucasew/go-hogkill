# go-hogkill (`hk`)

Go port of [hogkill](https://github.com/igorfelipeduca/hogkill): find and kill the processes eating your machine. One row per app, live CPU, held ranking, risk labels.

See [SPEC.md](./SPEC.md) for decisions and feature parity.

## Install

```bash
mise install          # go + release tools
mise run build        # → bin/hk
# or
go install github.com/lucasew/go-hogkill/cmd/hk@latest
```

## Usage

```bash
hk                     # interactive TUI
hk -m --me             # memory sort, your processes
hk top -n 15           # print once
hk top --json          # machine readable
hk kill Slack -y       # non-interactive kill
hk kill chrome --dry-run
```

### Subcommands

| Command | Aliases | Role |
|---------|---------|------|
| *(root)* | | interactive TUI |
| `top` | `ls`, `list` | one-shot table / JSON |
| `kill` | `rm` | non-interactive kill |
| `version` | | print version |

### Keys (TUI)

| Key | Action |
|-----|--------|
| ↑↓ / kj | move |
| →← / lh | expand / collapse |
| space | select |
| d / D | kill / force kill |
| / | filter |
| s c m | sort cycle / cpu / mem |
| click header | sort by NAME / CPU / MEMORY / PROCS |
| p | pin order |
| g G | top / bottom |
| ? q | help / quit |

## Development

```bash
mise run ci      # install, fmt, lint, test, build
mise run smoke   # hk top -n 5
mise run fmt     # fmt:* (go fmt)
```

## Release

Tools: `go`, `goreleaser`, `svu` in `mise.toml`. Tags via svu (`.svu.yml`: no `v` prefix, `v0`).

```bash
mise install
mise release          # svu next + goreleaser (needs GITHUB_TOKEN)
mise release patch    # or major | minor | next
```

CI: `.github/workflows/autorelease.yml` runs `mise run ci` on push/PR to `master`. Manual workflow_dispatch with patch/minor/major runs `mise release <bump>`.

## License

MIT
