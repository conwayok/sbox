---
type: AFK
blocked_by: []
---

## What to build

Replace the stub `main()` — which hardcodes `sboxHome := "/tmp/sbox-home"`, renders an empty `Config`, and then
contains unreachable code after a `return` — with the real CLI dispatch and everything the renderer needs to run.

This is the first observable slice: it delivers `sbox --render` end to end. It resolves the host-side inputs once at
startup (`USER` with its `os/user.Current()` fallback, `HOME`, `SBOX_HOME` defaulting to `$HOME/.sbox`, both
absolutized with no trailing separator), derives every other path from them, loads `config.json` and `install.sh`
(each absent → zero value), renders the eight artifacts, and writes them under the build directory. It also creates
the squid log directory.

Two details are decision-rich enough to pin here, both from the plan:

- **The squid log directory's mode must be forced past umask.** `MkdirAll(dir, 0o777)` under the default `022` umask
  yields `0755`, which is the exact bug being fixed — squid runs as uid 13 and, relative to a host-owned mount, falls
  into "others", where `0755` grants `r-x` and nothing else. An explicit `Chmod(0o777)` after the `MkdirAll` is
  required. The failure is silent at creation time and fatal later: squid exits 1 with
  `Cannot open '/var/log/squid/egress.log' for writing` and the sandbox has no egress at all.
- **The artifact nesting is load-bearing.** The `devcontainer` CLI resolves the local feature ref
  `./features/sbox-feature` against the config file's directory and rejects anything outside
  `<workspaceFolder>/.devcontainer`, so the artifacts cannot sit directly in the build directory.

The error contract is inherited from C#'s broad catch filter and matters as much as the happy path: a malformed or
unreadable `config.json` becomes a one-line stderr message and exit 1, never a panic or a stack trace.

Argument handling also lands here, since `--help` and the mode-flag check are the two early returns that precede
path resolution: `--help` / `-h` print usage and exit 0, and a mode flag given extra arguments fails with
`sbox: '<mode>' takes no arguments`, exit 1.

`config.go`'s `NetworkConfig` gains json tags for consistency with `Config` — behaviorally a no-op, included here
rather than as its own slice.

## Acceptance criteria

- [ ] `SBOX_HOME=/tmp/x ./sbox --render` exits 0 and writes all eight artifacts under
      `<SBOX_HOME>/build/.devcontainer/`: `Dockerfile`, `docker-compose.yaml`, `devcontainer.json`,
      `features/sbox-feature/devcontainer-feature.json`, `features/sbox-feature/install.sh`, `squid/squid.conf`,
      `squid/acls/dst.allowlist.txt`, `squid/acls/dstdomain.allowlist.txt`
- [ ] `ls -ldn <SBOX_HOME>/logs/squid` reports `drwxrwxrwx` — the mode survives the default `022` umask, proving the
      `Chmod` follows the `MkdirAll`
- [ ] An absent `config.json` renders the zero-value config; a populated one renders its `volumes`, `privileged`,
      `network` and `remoteEnv`/`features`
- [ ] A malformed or unreadable `config.json` prints the bare `json`/`os` error to stderr and exits 1 — never a panic
      or a stack trace
- [ ] `sbox --help` and `sbox -h` print usage and exit 0; a mode flag followed by extra arguments prints
      `sbox: '<mode>' takes no arguments` and exits 1
- [ ] `HOME` unset or whitespace-only → `sbox: HOME is not set`, exit 1. No resolvable username →
      `sbox: cannot determine user`, exit 1
- [ ] `SboxTag` is the lowercased user; `SBOX_HOME` defaults to `$HOME/.sbox`; a relative `SBOX_HOME` resolves against
      the current directory; both `$HOME` and `$SBOX_HOME` are absolute with no trailing separator
- [ ] The render matches the C# renderer after trimming trailing whitespace, across an empty `{}` config, a populated
      config, and both `allowAll` values (plan §8 step 1). Residual differences are limited to the whitespace-only
      rows tabulated in plan §3a.5
- [ ] `devcontainer read-configuration --workspace-folder <build dir> --config <build dir>/.devcontainer/devcontainer.json`
      exits 0 with no diagnostics; `remoteUser` resolves to the invoking user rather than the literal
      `${localEnv:USER}`; the local feature ref resolves (plan §8 step 3)
- [ ] `SBOX_DEBUG=true` prints the render target to stdout; unset prints nothing
