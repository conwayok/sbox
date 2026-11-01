---
type: AFK
blocked_by: [002]
---

## What to build

Entering the sandbox: `sbox` opens an interactive shell, `sbox <command>...` runs a command. After this slice the
sandbox is usable, which is the point of the whole tool.

Every mode flag has returned by the time this runs, so all of argv *is* the container command, first token included —
the program's own path is never mistaken for a command because `main` slices it off before calling `run`.

The in-container payload is a `cd` guard followed by the command, executed by `bash -lc`. The guard exists because
the mount model binds repositories at their real host paths: a directory that is not mounted has no counterpart
inside the sandbox, and the command must not silently run somewhere else. When the guard fails it prints the
`not mounted` message, lists what *is* mounted under `$HOME/Workspace`, hints at adding a volume to `config.json`,
and exits 1 — all written from inside the sandbox.

Two quoting behaviors are load-bearing and must survive exactly:

- **A lone argument is passed through untouched.** Shell syntax the caller quoted into it — pipes, redirects, an
  `sh -c` string — is still parsed on the far side. So `sbox sh -c 'exit 42'` reaches the shell as the caller wrote
  it.
- **Two or more arguments are each quoted before joining**, so an argument containing spaces stays one argument
  instead of being split by the shell. `sbox echo "a b"` prints `a b`.

The quoting function keeps a safe-character set (`a-zA-Z0-9._/:=@%+^,-`); values made only of those pass through
unquoted so ordinary paths read naturally, and everything else is single-quoted, which survives spaces, quotes, `$`,
backticks and newlines alike.

The host cwd is read once and threaded through. If reading it fails there is no sensible fallback — the guard's whole
purpose is to run where the caller stood — so the error is reported and the command exits 1.

The runner gives `exec` inherited stdio on all three streams, making the session fully interactive, and sbox mirrors
the child's exit code.

## Acceptance criteria

- [ ] `sbox` with no arguments produces
      `exec --workspace-folder <build dir> --config <devcontainer.json> bash -c "<cd guard>; exec bash"`
- [ ] `sbox <cmd>` produces the same argv with `bash -lc` and the payload `<cd guard>; <command>`
- [ ] A single argument reaches the shell untouched: `sbox sh -c 'exit 42'` exits 42
- [ ] Two or more arguments are quoted individually before joining: `sbox echo "a b"` prints `a b` as one argument
- [ ] Safe-only values pass unquoted; a value containing a space, a single quote, `$`, a backtick or a newline is
      single-quoted and arrives intact on the far side
- [ ] The guard changes to the host cwd; a command run from an unmounted directory prints the `not mounted` message
      with the mounted-under listing and the `config.json` hint, and exits 1, having run nothing on the host
- [ ] sbox mirrors the child's exit code in all cases, including non-zero ones
- [ ] Ctrl-C inside the sandbox interrupts the running command but not sbox, and sbox still reports the child's exit
      code — an interactive shell survives it, `tail -f` does not
- [ ] A failure to read the host cwd is reported and exits 1, with no fallback directory used
