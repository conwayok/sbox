# sbox Go Port — #001 Render the build tree (`sbox --render`) Handoff

## What was built

`main.go` now has the real entrypoint and everything `sbox --render` needs end to end: `main` →
`run(os.Args[1:])`, `SboxPaths` resolution, config loading, rendering, and writing the eight artifacts
under `<SBOX_HOME>/build/.devcontainer`, plus the `0777` squid log directory. `config.go`'s
`NetworkConfig` gained json tags. All of issue 001's acceptance criteria are met and verified in the
dev sandbox (see the issue's return / commands in git history of this conversation, not repeated here).

## Key decisions made

None beyond the plan. The plan's §5 signatures are implemented as written. Error reporting is via
`run` returning `int` and printing through `fail`, so config errors surface as the bare `json`/`os`
message (plan #21).

## Files touched

- `main.go` — full rewrite of the stub into the CLI: `SboxPaths` + methods, `resolvePaths`,
  `run`/`render`/`loadConfig`, `RenderConfig(paths, setupData)`, `renderTemplate`/`marshalJSON`/
  `orEmptyMap`, `write`/`writeFile`, `ensureSquidLogDir`, `printUsage`/`debug`/`fail`, `isModeFlag`.
- `config.go` — json tags on `NetworkConfig`.

## Patterns to follow

- `RenderConfig`, `render`, `loadConfig`, `write` already take `SboxPaths` and are the seams 002–004
  build on. Do not re-introduce `EnvironmentInfo` — it is deleted.
- `SboxPaths.ChildEnvironment()` already exists and returns `USER` + `HOME` only (no `SBOX_TAG`, per
  plan #16). Issue 002's child-env criterion is satisfied by using it as the overlay.
- `isModeFlag` / `modeFlags` already handle the mode-flag list; reuse rather than re-listing.

## Gotchas

- **`run`'s tail is a placeholder.** It currently returns `0` after the `--render` check for *every*
  remaining mode. Issue 002/003/004 must replace that tail with the §7 flow (squid-logs early return,
  squid-reconfigure, `onPath`, `up`, `execDevcontainer`). Until then `sbox --build` renders and exits 0
  without building — expected for this slice, not a bug to preserve.
- **`--squid-logs` currently renders**, because its pre-render early return belongs to issue 004. Plan
  §7 puts that return *before* `render`; 004 must move the check up accordingly.
- `onPath` and `runProcess` do **not** exist yet — they are issues 002's. Nothing in 001 references them.
- `debug` prints the render target to stdout only when `SBOX_DEBUG == "true"` (exact match).

## Test coverage

None shipped — waived for this feature (plan §3.2 #7). Verification was the plan §8 manual checklist:
step 1 (render diff vs C#, both empty/populated configs and both `allowAll` values, only the §3a.5
whitespace residuals remain, plus a Dockerfile final-newline-only difference), step 2 (log dir
`drwxrwxrwx` under umask 022), step 3 (`devcontainer read-configuration` exit 0, `remoteUser` resolves
to the invoking user, local feature ref resolves).

## Suggested skills

- `/code-search` before grepping (global CLAUDE.md rule).
- `/tdd` is not applicable here — no automated tests ship with this port.
