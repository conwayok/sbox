# sbox Go Port — Issues Index

## Overview

`sbox` is a wrapper around the `devcontainer` CLI that runs AI agents in a sandbox with restricted filesystem and
network access. This set of issues ports it from C# (.NET 10, Scriban templates, CliWrap) to a single static Go
binary, preserving the CLI surface, the argv and shell-quoting contract, and the generated config files — while
fixing two defects: the squid log directory's mode, which currently makes squid refuse to start on native Linux, and
the `onPath` executable check, which accepts a non-runnable `devcontainer`.

Full context and the living decisions log: see [plan.md](plan.md).

> To resume: find the first issue where `status` is not `done` or `dropped` and whose `blocked_by` are all `done`, then read its issue file.
> If `in-progress`, check which acceptance criteria are already ticked and continue from there.
> Read [000-decisions.md](000-decisions.md) before starting any issue — mid-flight decisions may have reshaped downstream issues.
>
> **Range check (read before anything else):** If the user specified a range of issues (e.g. "implement issues 010–020"), STOP — do not implement anything yet. Go directly to [Range execution mode](#range-execution-mode) and follow it exactly. Do not attempt to implement multiple issues in a single pass.

| # | File | Title | Type | Status | Blocked by | Remark |
|---|------|-------|------|--------|------------|--------|
| 001 | [001-render-build-tree.md](001-render-build-tree.md) | Render the build tree (`sbox --render`) | AFK | done | None | |
| 002 | [002-devcontainer-up.md](002-devcontainer-up.md) | Process runner and `devcontainer up` (`--build`) | AFK | done | 001 | |
| 003 | [003-devcontainer-exec.md](003-devcontainer-exec.md) | Enter the sandbox (`sbox`, `sbox <command>`) | AFK | done | 002 | no handoff needed |
| 004 | [004-squid-modes.md](004-squid-modes.md) | Squid modes (`--squid-reconfigure`, `--squid-logs`) | AFK | done | 002 | no handoff needed |
| 005 | [005-just-install.md](005-just-install.md) | Platform-agnostic `just install` | AFK | done | 001, 002, 003, 004 | no handoff needed |
| 006 | [006-macos-e2e-verification.md](006-macos-e2e-verification.md) | End-to-end verification on macOS | HITL | todo | 005 | Needs a macOS host with Docker Desktop; resolves open item #11 |
| 007 | [007-remove-csharp-tree.md](007-remove-csharp-tree.md) | Remove the C# tree and the dotnet recipe | HITL | todo | 006 | Plan §2 lists this as out of scope for the port's own change; kept so open item #9 stays executable |

Issues 003 and 004 are independent of each other and can be taken in either order once 002 is done.

## Status key

- `todo` — not started
- `in-progress` — actively being worked
- `blocked` — cannot proceed; put the blocker in `Remark` (e.g. "waiting on 004", "needs API key")
- `done` — all DoD criteria met
- `dropped` — intentionally skipped; reason required in `Remark` (e.g. "superseded by 005", "scope cut")

## Status transition protocol

1. **Before starting** — verify all `blocked_by` issues are `done`, then set status to `in-progress`
2. **After completing** — if decisions were made that affect issues beyond the next one, append to [000-decisions.md](000-decisions.md) (date, issue #, decision, rationale, affected issues), then set status to `done`
3. **Before next issue** — ask the user for permission before proceeding (range mode overrides this — see "Range execution mode")
4. **When dropping** — set `Remark`, set status to `dropped`, surface any issues that listed this one in `Blocked by` and ask the user how to update them

## Definition of done (shared — applies to every issue)

Slice-specific behavior lives in each issue's Acceptance criteria; this list is the universal gate.

- [ ] All acceptance criteria in the issue file are met
- [ ] All existing tests pass (no regressions)
- [ ] ~~New behavior is covered by at least one meaningful test~~ — **waived for this feature.** Plan §3.2 #7 ships no
      automated tests; verification is the manual checklist in plan §8, and each issue's Remark names the step that
      covers it. The suite is deferred design-of-record in plan §3a.13, tracked as open item #13
- [ ] Self code review passed (see [Self code review](#self-code-review) below) — in range mode this is deferred to the end-of-range review
- [ ] Ask the user if any final modifications are needed before marking done
- [ ] Handoff doc (`NNN-this-slug-handoff.md`) written **if it would save the next agent time** (see handoff triggers below); otherwise note "no handoff needed" in `Remark`

## Agent implementation guidance

Use `/tdd` if available. Before starting, read previous handoff docs and explore unfamiliar code areas.

Spawn subagents for exploration that would burn significant context — one subagent per concern:
- Reading a handoff doc from a prior issue
- Locating where a concept is implemented before touching it
- Scanning test coverage in adjacent areas

Each subagent returns a **structured summary only** (file paths, key decisions, patterns, gotchas) — not raw file contents.

Before confirming final modifications with the user, run the self code review (see [Self code review](#self-code-review) below) and fix its Critical/Major findings. Then write a handoff doc before marking `done`.

**Feature-specific note.** This is a port, not a design exercise: the behavior is already settled by the C# tree under
`Sbox/` and by `plan.md` §3. When the plan and the C# disagree, the plan wins — the divergences are deliberate and
each is traceable to a decision ID. Do not "improve" behavior the plan marks as preserved, and do not reintroduce a
C# behavior the plan marks as fixed (#2, #12, #19, #20, #22).

## Self code review

Before an issue is considered finished, its changes must be reviewed by a fresh-context subagent. Purpose: catch what the implementing agent can't see — its own blind spots.

- **Who runs it** — the implementing agent (single-issue mode) or the orchestrator (range mode, deferred to the end of the range). The reviewer must be a freshly spawned subagent given NO conversation context from the implementation — only the artifacts below.
- **What it's given** — the issue file(s), this index (for the shared DoD and the issue's intent), and the issue's (or range's) working-tree changes (git diff). Not the implementer's reasoning, decisions, or summaries. Explicitly list any **out-of-scope or intentionally-excluded items** (known limitations, deferred pieces, deliberate shortcuts) in the review prompt — so the reviewer doesn't report things that will be discarded immediately. For this feature that list includes: no automated tests (plan §3.2 #7), and the deliberate behavioral divergences in plan §3.2 #2, #12, #19, #20 and #22.
- **How it reviews** — load the `/code-review` skill if available; otherwise do the same shape of review manually: structured, severity-graded feedback grounded in the issue's intent.
- **Findings handling** — Critical/Major findings must be fixed before marking the issue `done`. Minor findings are at the implementing agent's discretion. Re-run the review only if the fixes were substantive (no mandatory re-run for nits).

## Range execution mode

Triggered when the user specifies a range of issues (e.g., "implement issues 010-020").

> **Anti-pattern — DO NOT do this:** Reading all issue files and implementing multiple issues in a single agent pass, even if the context can hold it. This is wrong regardless of range size. Each issue MUST be handed to a dedicated subagent.

**Differences from single-issue mode:**
- Do **not** ask for permission between issues — the range specification is the permission grant
- MUST spawn a **dedicated subagent per issue** — the orchestrating agent only drives the loop; no implementation code runs in the orchestrator's context
- MUST implement strictly **sequentially** — start issue N+1 only after issue N's subagent returns and reports success

**Per-issue loop:**

1. Check all `blocked_by` issues are `done` — if not, pause and surface the blocker to the user before continuing
2. Set the issue status to `in-progress` in this index file
3. Spawn a subagent (see "Subagent prompt must include" below for the full spec)
4. Wait for the subagent to complete
5. If the subagent reports failure or a new blocker, pause and ask the user how to proceed before continuing the range
6. Move to the next issue in the range

Per-issue subagents do NOT run the self code review — it is deferred until the whole range is implemented (see [Self code review](#self-code-review)).

**Subagent prompt must include:**
- Feature name, issue number, and path to the issue file
- Path to this index file (so it can update the status column)
- Paths to prior handoff docs to read before starting (ordered oldest → newest)
- Explicit instruction to implement the issue and update this index's status column before returning, and to write `NNN-this-slug-handoff.md` only if the handoff triggers apply (see "Handoff doc structure")
- Explicit instruction NOT to run the self code review — it is deferred to the end-of-range review (see [Self code review](#self-code-review))
- Return format: plain markdown — issue number, status (`done`/`blocked`), files touched (one per line), key decisions (one per line), out-of-scope / intentional exclusions (one per line, or "none"), and handoff path if written (otherwise "no handoff").

The return already carries the summary to the orchestrator, so a separate handoff file is warranted only when a *future* subagent (not the orchestrator) needs the context — omit it otherwise.

**After the full range completes:**

1. Run the self code review (see [Self code review](#self-code-review)) once over the whole range's changes — spawn a fresh subagent given the index, all range issue files, the combined working-tree diff, and the out-of-scope / intentional notes collected from each per-issue subagent's return; use the `/code-review` skill if available.
2. For each Critical/Major finding, set the affected issue's status back to `in-progress`, spawn a fix subagent, and re-mark it `done` after the fix. The fix subagent should check whether downstream issues in the range built on the broken code.
3. Re-run the review only if the fixes were substantive; otherwise proceed.
4. Surface a one-paragraph summary of what was built across all issues, and note any decisions appended to `000-decisions.md`.

---

### Handoff doc structure

**Write one only when at least one of these holds** (otherwise skip the file and note "no handoff needed" in `Remark`):
- A later un-done issue depends on this one *and* needs non-obvious context (a pattern to follow, a gotcha)
- You hit something surprising or fragile worth warning the next agent about
- Decisions were made that affect only the next issue (cross-cutting ones go in `000-decisions.md` instead)

**Filename**: `NNN-this-slug-handoff.md`  
**Header**: `# <Feature Name> — #NNN <Issue Title> Handoff`

Don't restate what's already in the issue file, commits, or diffs — reference those by path/URL. Add only what isn't recorded elsewhere.

- **What was built** — one paragraph: what this slice delivered and how it fits the feature
- **Key decisions made** — non-obvious choices with rationale; decisions affecting issues beyond the next one must also go in `000-decisions.md`
- **Files touched** — path + what changed, one line each
- **Patterns to follow** — specific patterns to copy/extend, with file paths and line numbers
- **Gotchas** — anything surprising or fragile that would waste time to rediscover
- **Test coverage** — which test files now cover this slice
- **Suggested skills** — skills the next agent should load, with one-line reason each
