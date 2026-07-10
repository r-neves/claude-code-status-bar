# claude-code-status-bar

A single-binary statusline for [Claude Code](https://claude.com/claude-code) that shows the same
information as a conventional multi-line status bar, in **three rows instead of nine**.

The 5-hour and 7-day rate-limit windows sit side by side rather than stacked, the blank separator
rows are gone, and session statistics share a row with the working directory.

```
💻 Newton │ Opus 4.8 │ 🪟 126.0k/200k (38%) 💾 44% cache │ ⏱ 42m │ 💰$3.45 │ ● high
📂 ~/code/proj (main* +3 ~2) abc1234 #42 │ 📊 +482 -37 lines │ 24.5k out │ 3m18s api
current ━━━─────── 30% ⟳ 2:30pm │ weekly ━━━━━━──── 62% ⟳ jul 7
```

## Layouts

| Layout | Rows | What it drops |
|--------|------|---------------|
| `compact` (default) | 3 | Blank separator rows |
| `ultra` | 2 | Hostname, tokens-out, api time; folds rate limits beside the directory |
| `full` | 6 | Nothing — the original stacked arrangement, for comparison |

Every row is fitted to the terminal width. As space runs short, segments drop in a fixed
priority order (api time first, then the commit sha, then tokens-out, and so on), and the
rate-limit row degrades by shrinking its bars, then dropping the reset times, then shortening
`current`/`weekly` to `5h`/`7d`. It never wraps, because a wrapped row would give back the
vertical space this exists to save.

## Install

```bash
git clone https://github.com/r-neves/claude-code-status-bar.git
cd claude-code-status-bar
make install
```

That builds and copies the binary to `~/.claude/`, then prints the snippet to add to
`~/.claude/settings.json`:

```json
"statusLine": { "type": "command", "command": "~/.claude/claude-code-status-bar", "padding": 0 }
```

To have the installer edit `settings.json` for you (it backs the file up first and needs `jq`):

```bash
make install CONFIGURE=1
```

Restart Claude Code afterwards.

## Preview

Run the binary from a shell and it renders sample data, so you can try themes and layouts
without waiting for a real session:

```bash
./claude-code-status-bar --layout ultra --theme dracula --bar block
make demo   # renders all three layouts
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--layout` | `compact` | `compact`, `ultra`, or `full` |
| `--theme` | `default` | `default`, `dracula`, `catppuccin`, `solarized`, `mono`, `plain` |
| `--bar` | `line` | `line`, `classic`, `block`, `braille`, `geometric` |
| `--bar-width` | `10` | Progress bar width in cells |
| `--width` | auto | Terminal width override |
| `--extra` | off | Fetch pay-as-you-go credit usage (see below) |
| `--dump <path>` | — | Write the raw stdin JSON somewhere for debugging |
| `--themes`, `--bars`, `--version` | — | Print and exit |

`--theme`, `--bar`, and `--layout` also read `CLAUDE_STATUSLINE_THEME`, `CLAUDE_STATUSLINE_BAR`,
and `CLAUDE_STATUSLINE_LAYOUT`. Setting `NO_COLOR` forces the `plain` theme. `COLUMNS` overrides
the detected terminal width.

Bar styles:

| Name | Rendering |
|------|-----------|
| `line` | `━━━───────` |
| `classic` | `●●●○○○○○○○` |
| `block` | `███░░░░░░░` |
| `braille` | `⣿⣿⣿⠄⠄⠄⠄⠄⠄⠄` |
| `geometric` | `▰▰▰▱▱▱▱▱▱▱` |

## Where the data comes from

Claude Code pipes a JSON payload to the statusline command on stdin. Almost everything is read
straight from it — no network calls, no keychain access, no `gh` subprocess:

- **Rate limits** — `rate_limits.five_hour` and `.seven_day`, including `resets_at`. Present only
  for Pro/Max subscribers, and only after the first API response of the session.
- **Effort level** — `effort.level`, reflecting mid-session `/effort` changes.
- **Pull request** — `pr.number`, `pr.url`, `pr.review_state`.
- **Context, cost, model, cwd** — `context_window`, `cost`, `model`, `cwd`.

The only subprocess is `git`, called once for `status --branch` and once for the short sha, each
with a 300ms timeout so a slow repository degrades to "no git info" instead of stalling the bar.

### Extra usage (`--extra`)

Pay-as-you-go credit usage is *not* on stdin, so `--extra` fetches it from the OAuth usage API,
resolving a token from `CLAUDE_CODE_OAUTH_TOKEN`, the macOS Keychain, `~/.claude/.credentials.json`,
or `secret-tool`. Responses are cached for 60 seconds in `$TMPDIR/claude-status-bar/`. It adds a
third rate-limit window with a daily burn rate and a projected monthly total:

```
extra ━───────── $4.50/$20.00 ⟳ aug 1  📈 $0.64/day → $19.84 proj
```

It is off by default because it costs a network round trip on every render. Any failure —
no token, no network, bad response — silently falls back to a cached value or omits the window.

## Development

```bash
make check   # gofmt, go vet, go test
```
