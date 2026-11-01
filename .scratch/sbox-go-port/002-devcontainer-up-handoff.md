# sbox Go Port — #002 Process runner and `devcontainer up` (`--build`) Handoff

## What was built

`main.go` now has the single process runner (`runProcess` + `stdioMode` + `processResult` +
`streamWriter` + `overlayEnvironment`), `onPath` via `exec.LookPath`, `devcontainerUpArgs` and `up`,
and `run`'s tail follows plan §7 exactly: `--squid-logs` early return, render, `--render` return,
`--squid-reconfigure`, `onPath`, `up`, `--build` return, `execDevcontainer`. `sbox --build` creates
the sandbox through the real runner; every non-early-return mode runs `up` first.

## Key decisions made

- **`up` returns `(int, error)`, not `int`.** Go's `ProcessState.ExitCode()` returns `-1` for a
  signal-terminated child, so a negative sentinel inside a bare `int` cannot be told apart from a
  real signal death. The error return is the clean way to keep the child-cannot-start path separate
  from the child-exits-non-zero path without double-printing. Plan §5/§7 updated to match. Only
  `run` calls `up`, so issues 003/004 are unaffected.
- **The `--squid-logs` pre-render early return is already in place** (plan §7). Issue 004's handoff
  note said it still had to move that check up; it no longer does — 004 only fills in
  `followEgressLog`/`reconfigureSquid` bodies.
- **`stdioSuppress` maps to a nil `io.Writer`**, which `os/exec` wires to the null device, rather
  than C#'s redirect-and-discard-pipe. Functionally identical (output hidden) and it needs no
  concurrent drain. `stdioCapture` assigns a `*bytes.Buffer`, which `os/exec` drains concurrently
  with `Wait`, so the "captured streams must be read while waiting" requirement is met by
  construction.
- **`overlayEnvironment` strips inherited entries the overlay replaces**, then appends the overlay.
  Appending alone would leave duplicate `USER=`/`HOME=` entries and let the inherited value win in
  some libc getenv implementations.

## Files touched

- `main.go` — imports (`io`, `os/exec`, `os/signal`); `run` tail (full §7 dispatch); the process
  runner / argv / `onPath` block inserted after `isModeFlag`; stubs for `execDevcontainer`
  (003), `reconfigureSquid` and `followEgressLog` (004).
- `.scratch/sbox-go-port/plan.md` — §5 `up` signature and §7 flow/notes updated for the error return.
- `.scratch/sbox-go-port/000-index.md` — issue 002 → `done`.

## Patterns to follow

- Call `runProcess(stdin, stdout, stderr, name, args, paths.ChildEnvironment())` — do not rebuild the
  environment or spawn directly. Call sites (plan §5 table): `up` = Inherit/Suppress/Inherit;
  `execDevcontainer` = Inherit/Inherit/Inherit; `reconfigureSquid` = Inherit/Capture/Capture;
  `followEgressLog` = Inherit/Inherit/Inherit.
- On a `runProcess` error, the wrapper reports it with `fail(err.Error())` and exits 1 — that is the
  "child could not be started" path; a non-zero child is a `processResult`, not an error.
- `config.go` and the path helpers are unchanged from 001; `paths.ChildEnvironment()` is the overlay.

## Gotchas

- **`exec.ExitError.ExitCode()` is `-1` for a signal-killed child.** Do not treat `-1` as a sentinel.
- **`exec.LookPath` rejects a directory named `devcontainer` and a non-executable file** (plan
  §3a.12 divergence). `onPath` is exactly `_, err := exec.LookPath(name); err == nil`.
- **`devcontainer` cannot be started after `onPath` passes** — e.g. an executable text file with no
  shebang gives `fork/exec ...: exec format error`. That is the only realistic way to hit the
  start-error path, since `LookPath` already covers absent/non-exec/dir.
- **`--squid-logs` now returns before render** (stub). It no longer renders the build tree, which is
  the intended plan §7 ordering, not a regression.

## Test coverage

None shipped — waived (plan §3.2 #7). Verified manually with PATH stubs recording argv: `--build`
argv with the two rebuild flags, bare-`sbox` argv without them, stdout suppressed / stderr
inherited, `kill -INT $$` stub dying with no survival marker, absent / non-executable / directory
`devcontainer` all giving `sbox: 'devcontainer' not found on PATH` exit 1, non-zero `up` giving the
Docker message exit 1, bad-format executable giving only the exec error exit 1, `USER`/`HOME`
overlaid with `SBOX_TAG` unset, and a malformed `config.json` reporting its own error before the
PATH check.

## Suggested skills

- `/code-search` before grepping (global CLAUDE.md rule).
- `/tdd` not applicable — no automated tests in this port.
