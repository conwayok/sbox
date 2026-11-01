---
type: HITL
blocked_by: [006]
---

## What to build

Nothing is implemented here. This is the manual cleanup that closes the port: with the Go binary verified end to end,
the C# tree has no remaining job.

Plan §2 lists the deletion as out of scope for the port's own change, and §3.1 #9 tracks it as a decision owned by the
human. It is still a slice here so the open item stays executable rather than living only in a table.

The `Sbox/` directory, `Sbox.sln` and `out/` go, and the dotnet `install-mac` recipe goes with them — which is what
leaves `install` as the single build entrypoint.

## Acceptance criteria

- [ ] `Sbox/`, `Sbox.sln` and `out/` are removed from the repository
- [ ] The dotnet `install-mac` recipe is removed from the `justfile`, leaving `install` as the only build entrypoint
- [ ] `just install` still works from a clean checkout after the removal
- [ ] No remaining reference to dotnet, Scriban, CliWrap or the C# tree in `README.md` or `plan.md` describes
      something that no longer exists
- [ ] `plan.md` §3.1 open item #9 is marked resolved in the companion doc
