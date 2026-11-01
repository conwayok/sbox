---
type: AFK
blocked_by: [001]
---

## What to build

The single shell-out point, plus the first mode that uses it. After this slice `sbox --build` creates the sandbox,
and every non-early-return mode has run `devcontainer up` on its way to doing anything else.

The process runner is the one place a child is started, and three of its behaviors are load-bearing rather than
incidental:

- **Ctrl-C.** Install a `signal.Notify` handler draining `os.Interrupt` for the duration of the call, and stop it
  afterwards — never `signal.Ignore`. POSIX propagates an *ignored* disposition across `exec`, so the child would
  inherit `SIG_IGN` and Ctrl-C would be dead inside the sandbox. `Notify` leaves the child with `SIG_DFL`, which is
  what C# does by cancelling only its own termination. sbox stays alive afterwards to mirror the child's exit code.
- **Environment overlay.** Seed `Cmd.Env` explicitly from the inherited environment plus the overlay. This is
  required, not convenient: the `devcontainer` CLI resolves `${localEnv:...}` from its own environment, and compose
  interpolates `${USER}` and `${HOME}` into the Dockerfile's build args from the one it is handed. Leaving `Cmd.Env`
  nil would also inherit, but then the overlay cannot be applied. `SBOX_TAG` is deliberately *not* exported — the
  template substitutes the tag at render time, so nothing reads the variable at run time.
- **Concurrent draining.** Captured streams must be read while waiting, not after, so a chatty child cannot fill a
  pipe and block forever.

The three stdio modes are `Inherit` (leave the stream attached), `Suppress` (detach and discard) and `Capture`
(detach and return). `Suppress` and `Capture` differ only in whether the piped bytes are kept — both matter, because
leaving a stream nil in Go means "inherit", which would leak output the C# deliberately hid. `devcontainer up` pins
`stdin=Inherit, stdout=Suppress, stderr=Inherit`: progress output never reaches the terminal, errors do.

`onPath` is implemented with `exec.LookPath` rather than C#'s bare `File.Exists`. That is a deliberate divergence,
and it is justified because it only changes an error path that already fires when something is wrong — but where the
current message actively misleads. `LookPath` requires the executable bit, and additionally rejects a *directory*
named `devcontainer`, which satisfies the x bit but is not runnable; a hand-rolled mode test would have let it
through and reintroduced the same confusing exec failure.

The `run` failure message for a non-zero `up` is `sbox: 'devcontainer up' failed (is Docker running?)`. A child that
cannot be *started* at all is a separate path: its `exec` error goes to stderr and the program exits 1. C# left that
unhandled, so this is a new (intended) surface.

## Acceptance criteria

- [ ] With a stub `devcontainer` on `PATH` that records its argv, `sbox --build` invokes
      `up --workspace-folder <build dir> --config <devcontainer.json> --remove-existing-container --build-no-cache`
- [ ] Without `--build`, the same argv is produced without the two rebuild flags; `sbox --build` exits 0 on a
      successful child and returns 0 immediately afterwards without reaching `exec`
- [ ] The stub's stdout never reaches the terminal while its stderr does, confirming `stdout=Suppress` and
      `stderr=Inherit`
- [ ] A stub that self-interrupts (`kill -INT $$`) dies rather than printing a survival marker, proving it inherited
      `SIG_DFL` and not `SIG_IGN`
- [ ] `devcontainer` absent from `PATH` → `sbox: 'devcontainer' not found on PATH`, exit 1. The same message and exit
      for a present-but-non-executable file, and for a directory named `devcontainer`
- [ ] A non-zero `devcontainer up` → `sbox: 'devcontainer up' failed (is Docker running?)`, exit 1
- [ ] A child that cannot be started at all → its `exec` error on stderr, exit 1 — not a panic
- [ ] Every child receives `USER` and `HOME` overlaid on the inherited environment, and does not receive `SBOX_TAG`
- [ ] The PATH check happens *after* render, so a render failure reports its own error first
