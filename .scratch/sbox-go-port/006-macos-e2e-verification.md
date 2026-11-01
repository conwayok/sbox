---
type: HITL
blocked_by: [005]
---

## What to build

Nothing is implemented here. This slice is the manual end-to-end pass on a machine that can actually run the tool —
a macOS host with the `devcontainer` CLI and Docker Desktop — because the dev sandbox cannot run Docker Desktop's
mount-ownership remapping, which is the one behavior the whole `0777` fix depends on.

It exists as a slice because it is the only thing standing between "the port compiles and its argv is right" and
"sbox works", and because it resolves two of plan §3.1's open items.

The probe gotcha worth repeating: when checking allow/deny by hand, put the client on the `sandbox_egress` network.
Both `sandbox` and `sandbox_egress` are `internal: true`, so a client on the wrong network reports "denied" because
*nothing was reachable* rather than because the ACL blocked it — a failure mode that looks exactly like a pass, on
the one assertion standing between the user and a silent allow-all.

## Acceptance criteria

- [ ] On macOS with Docker Desktop, in order: `sbox --render`, `sbox --build`, `sbox`, `sbox <command>`,
      `sbox --squid-reconfigure`, `sbox --squid-logs` all behave as plan §6 specifies
- [ ] `ls -ldn $SBOX_HOME/logs/squid` reports `drwxrwxrwx`, and squid reports `Up` rather than `Restarting (1)` with
      no `FATAL: Cannot open ... for writing` in its logs — resolving open item #11
- [ ] Ctrl-C inside the sandbox interrupts the running command but not sbox, and sbox still mirrors the child's exit
      code
- [ ] A command run from an unmounted directory prints the `not mounted` message and exits 1, rather than running
      somewhere else
- [ ] `sbox sh -c 'exit 42'` returns 42; `sbox echo "a b"` prints `a b`
- [ ] A host outside the allowlist is denied *with the probe on the `sandbox_egress` network*, and the attempt appears
      in the egress log on the host; an allowlisted host is reachable through the proxy
- [ ] Anything that diverges is filed as a new issue or recorded in `plan.md` — not worked around silently
