---
type: AFK
blocked_by: [001, 002, 003, 004]
---

## What to build

The distribution deliverable — the reason the port exists at all. Add a `just install` recipe that builds `out/sbox`
with `go build` and copies it to `~/.local/bin/sbox`.

The recipe is deliberately **not** platform-qualified. `go build` targets the host platform on its own, so one recipe
covers macOS, Linux and any future target, and the runtime matrix the dotnet recipe needed has no Go counterpart to
encode. This is the whole point: the port removes the .NET runtime dependency and the
`--runtime osx-arm64` publish step.

The existing dotnet `install-mac` recipe is untouched and keeps its name, so both builds work until the C# tree is
deleted.

## Acceptance criteria

- [ ] `just install` builds `out/sbox` and copies it to `~/.local/bin/sbox`, exiting 0
- [ ] The recipe declares no platform and no runtime matrix — it is a plain host-targeted `go build`
- [ ] The installed binary runs `--help` (exit 0) and `--render` (exit 0, writing the artifact tree)
- [ ] The dotnet `install-mac` recipe is byte-for-byte unchanged
- [ ] A clean `go build` from a fresh checkout with no network access succeeds — no new module dependencies
