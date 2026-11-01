---
type: AFK
blocked_by: [002]
---

## What to build

The two modes that operate on squid rather than on the sandbox session. After this slice the full CLI surface from
plan §6 is implemented.

`--squid-reconfigure` reloads squid's ACLs in place, so an edited allowlist takes effect without a `devcontainer up`
round-trip. It names the compose project and file explicitly, which keeps it independent of the current directory.
Its output is captured and discarded: nothing the child prints reaches the terminal on success, and a non-zero exit
is converted into sbox's own message rather than passing the raw child output through.

`--squid-logs` follows the host's copy of the egress log with `tail -n 500 -f`. The log directory is bind-mounted
into the squid service, so this is the same file the container writes, read without entering the container.

The one structural detail unique to this slice: `--squid-logs` returns **before** render. It is the only mode that
skips the render step, so it must not write or refresh the build directory — following a log should work even when
the config is currently broken. It runs after path resolution, so the `HOME` and username failures still apply to it.

## Acceptance criteria

- [x] `sbox --squid-logs` runs `tail -n 500 -f <SBOX_HOME>/logs/squid/egress.log` with inherited stdio on all three
      streams, and mirrors `tail`'s exit code
- [x] `--squid-logs` does not render: run against a `SBOX_HOME` with no build directory, confirm `tail` is invoked and
      no artifacts are written
- [x] With a stub `docker` recording its argv, `sbox --squid-reconfigure` invokes
      `compose --project-name sbox-<sboxTag> --file <build dir>/.devcontainer/docker-compose.yaml exec -T squid squid -k reconfigure`
- [x] On success, reconfigure prints nothing at all — the child's stdout and stderr are captured and discarded, not
      inherited
- [x] A non-zero reconfigure exits 1 with `sbox: 'squid -k reconfigure' failed (is the sandbox up?)`
- [x] A child that cannot be started at all → its `exec` error on stderr, exit 1 — not a panic
- [x] `--squid-logs` still resolves paths, so `HOME` unset → `sbox: HOME is not set`, exit 1
