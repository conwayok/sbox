# Plan: Port sbox from C# to Go

---

## References

Links taken from `README.md`, the generated compose template (`templates/docker-compose.yaml.tmpl`) and the source
under `Sbox/`.

- [devcontainers/cli](https://github.com/devcontainers/cli) — the CLI sbox wraps; source of the `up` / `exec` / feature-resolution contract
- [Claude Code devcontainer docs](https://code.claude.com/docs/en/devcontainer) — referenced by the README's minimal-config example
- [ubuntu/squid on Docker Hub](https://hub.docker.com/r/ubuntu/squid) — the egress proxy image; built `--with-default-user=proxy` (uid 13)

---

## 1. Overview

`sbox` is a wrapper around the `devcontainer` CLI that runs AI agents in a sandbox with restricted filesystem and
network access. The current implementation is C# (`Sbox/`, .NET 10, Scriban templates declared inline, `CliWrap`)
published as a platform-specific single-file binary. This plan ports it to Go.

The reason for the port is distribution and startup cost: a Go build is a single static binary with no .NET runtime
dependency and no publish-runtime matrix in the `justfile`.

The port is mostly mechanical — the C# tree is ten small files, and the behavior is fully settled by reading it. The
work is therefore about *equivalence*, not design: reproducing the CLI surface, the argv/shell-quoting contract, the
process-spawning semantics, and the generated config files faithfully, while fixing the two defects in §3.2 — the
squid log directory's mode (#2) and the `onPath` executable check (#12).

Scaffolding already exists and compiles: `main.go` (template rendering + file writing), `config.go` (data shapes),
and `templates/` (Go `text/template` rewrites of the three Scriban templates). `main()` is a stub — it hardcodes
`sboxHome := "/tmp/sbox-home"` and an empty `Config`, then contains unreachable code after a `return`. That stub is
replaced entirely by the CLI in §5.

---

## 2. Out of Scope

| Item | Why |
|------|-----|
| Deleting `Sbox/`, `Sbox.sln`, `out/` | Handled manually by the human, outside this change |
| Byte-for-byte identical output vs. the C# binary | Requirement is functional equivalence; see §3.2 #1. The §8 comparison does diff against the C# renderer, but as a one-off verification aid, not as the acceptance bar |
| Removing the dotnet `install-mac` justfile recipe | Belongs to the manual C# removal |
| Adding/removing config fields (`volumes`, `network.*`, `remoteEnv`, `features`) | Existing schema is carried over unchanged |
| The `sbox-feature` devcontainer feature's contents | `install.sh` is passed through verbatim from `$SBOX_HOME/install.sh` |
| Automated tests | Deferred out of this port — verification is the manual checklist in §8. The design for the suite is recorded in §3a.13 |

---

## 3. Decisions & Open Items

### 3.1 Open Items

IDs are shared with §3.2 and §3a — the complete set is #1–#25, split across the three tables by status. A gap here
means the item is resolved below, not that it never existed.

| # | Type | Description | Status | Notes | Owner / Due |
|---|------|-------------|--------|-------|-------------|
| 9 | Decision needed | Confirm the C# tree is removed, and the dotnet `install-mac` recipe dropped with it | Open | Deletion is a manual step owned by the human; this plan leaves `Sbox/`, `Sbox.sln` and `out/` untouched | Human / after this port lands |
| 11 | Open question | Whether the `0777` log directory behaves the same under Docker Desktop on macOS as it did under the C# build | Open | Verified only on native Linux (`§3a.2`). Docker Desktop remaps mount ownership, which is why the C# `0700` appeared to work. Needs spike on a macOS host — not runnable in the dev sandbox | Human / first macOS run |
| 13 | Decision needed | Build the automated test suite | Open | Deferred out of this port. The integration-first design is recorded in §3a.13 so a future plan starts from it rather than re-deriving it | Human / future plan |

### 3.2 Resolved / Closed Items

| # | Type | Description | Status | Resolution / Rationale | Owner / Due |
|---|------|-------------|--------|------------------------|-------------|
| 1 | Technical decision | Output fidelity requirement | Resolved | Functionally equivalent, not byte-identical — see §3a.1 | — |
| 2 | Technical decision | Permissions on the squid log directory | Resolved — see §3a.2 | `0777`, applied with an explicit `Chmod` after `MkdirAll` | — |
| 3 | Technical decision | Go file layout | Resolved | Two files: `main.go` and `config.go` — see §3a.3 | — |
| 4 | Technical decision | SIGINT handling in the process runner | Resolved — see §3a.4 | `signal.Notify` draining the signal; never `signal.Ignore` | — |
| 5 | Technical decision | Whether the Go templates need edits | Resolved — see §3a.5 | No template changes required; rendering verified equivalent | — |
| 6 | Technical decision | Value of `SboxTag` | Resolved | Lowercased `User`, matching C#'s `user.ToLowerInvariant()`. Where `User` itself comes from is #19 | — |
| 7 | Technical decision | Automated test coverage | Resolved | None in this port. The design for a future suite is recorded in §3a.13 | — |
| 8 | Technical decision | Build/install entrypoint | Resolved | Add an `install` recipe to the `justfile`, alongside the untouched dotnet `install-mac` — see #15 | — |
| 10 | Assumption | `devcontainer` and Docker are present on the target machine | Resolved — see §3a.10 | Both are available and exercised: Docker Compose v5.6.1, npm 11, and `@devcontainers/cli` 0.90.0 installs and runs | — |
| 12 | Technical decision | `onPath` executable-bit check | Resolved — see §3a.12 | Fix it: require the executable bit, diverging from C# | — |
| 14 | Technical decision | Location of the squid log directory | Resolved | `$SBOX_HOME/logs/squid`, unchanged from C# (`Sbox/SboxPaths.cs:39`). `build/` stays purely derived state and §9 loses its only backward-compatibility break | — |
| 15 | Technical decision | Name of the Go `justfile` recipe | Resolved | `install` — deliberately unqualified by platform. `go build` targets the host, so one recipe covers macOS, Linux and any future target; there is no runtime matrix to encode. The dotnet `install-mac` keeps its name and is untouched until §3.1 #9 deletes it | — |
| 16 | Technical decision | Whether `SBOX_TAG` is exported to children | Resolved | No. `ChildEnvironment()` exports `USER` and `HOME` only — those are the two the templates and the CLI actually read. See §3a.5 | — |
| 17 | Technical decision | Which argv slice becomes the container command | Resolved | `args`, the `run` parameter — equivalent to `os.Args[1:]` and to C#'s `args` (`Sbox/Program.cs:91`) | — |
| 18 | Technical decision | Normalizing `$HOME` and `$SBOX_HOME` | Resolved | `filepath.Abs` on both, mirroring C#'s `Path.GetFullPath` plus trailing-separator trim (`Sbox/SboxPaths.cs:95-96`) | — |
| 19 | Technical decision | Fallback when `USER` is unset | Resolved — see §3a.19 | `os/user.Current().Username`; if that errors or is empty, `sbox: cannot determine user`, exit 1 | — |
| 20 | Technical decision | `onPath` implementation | Resolved — see §3a.12 | `exec.LookPath`, which also rejects a directory named `devcontainer` | — |
| 21 | Technical decision | How errors reach stderr | Resolved | `run` returns `int` and prints through `fail`, so a config error prints the bare `json`/`os` message §6 promises | — |
| 22 | Technical decision | Meaning of `runProcess`'s `error` return | Resolved | Non-nil only when the child cannot be started; the caller reports it and exits 1 — see §6 | — |
| 23 | Technical decision | Whether a whitespace-only `HOME` fails | Resolved | Yes, matching C#'s `string.IsNullOrWhiteSpace` (`Sbox/SboxPaths.cs:71`) | — |
| 24 | Technical decision | `up`'s error return, and where the `--squid-logs` check sits | Resolved — issue 002 | `up` returns `(int, error)`, not the plan's bare `int`: Go reports a signal-killed child as exit `-1`, so an int sentinel could not tell "could not be started" apart from signal death. `--squid-logs` already returns before render (plan §7), so issue 004 only fills in the `followEgressLog` body | — |
| 25 | Technical decision | How `stdioSuppress` drops a stream | Resolved — see §3a.25 | A nil `io.Writer`, which `os/exec` connects to the null device. §5's "redirect the stream and discard it" and C#'s `ReadToEndAsync`-and-drop describe a drained pipe instead; the two are equivalent at both call sites, and the null device cannot block the way an undrained pipe could | — |

### 3a. Decision Notes

Headings are numbered by the decision ID they elaborate, not by position, so the numbering has gaps — those IDs are
either self-contained in §3.2 or open in §3.1.

#### 1. Output fidelity

Byte-identical output was rejected as the standard: `out/sbox` is a Mach-O arm64 binary that cannot run in the dev
sandbox (Linux aarch64), so a diff against *that* artifact is impossible. The C# implementation can still be compiled
and executed here via `dotnet` (10.0, and `Sbox/obj/Debug/net10.0/linux-arm64/` shows prior cross-builds), which is
how the template equivalence in §3a.5 was established. The remaining differences are inert whitespace, so functionally
equivalent is the right bar and the port does not fight template formatting.

#### 2. Permissions on the squid log directory

The C# code creates `SquidLogDir` with mode `0700`, commented "Squid runs as uid 13 and must be able to write the
egress log here". Measuring it shows that comment describes an intent the mode does not achieve. `ubuntu/squid` is
built `--with-default-user=proxy` (uid 13) and ships `/var/log/squid` owned `13:13`; the compose bind-mount replaces
that directory with a host-owned one, so the host directory's mode decides whether squid can write.

Run against the real `squid.conf` and a host-owned mount:

| mode of the log dir | Result |
|---------------------|--------|
| `0700` | `FATAL: Cannot open '/var/log/squid/egress.log' for writing.` — squid exits 1 |
| `0755` | Identical FATAL — squid exits 1 |
| `0777` | Squid starts; `egress.log` created, owned `13:13`, mode `0640` |

`0755` does not help because the relevant bit is *write for others*, and uid 13 falls into "others" relative to the
host-owned directory; `0755` grants `r-x` there, `0700` grants nothing. Only `0777` grants write. The failure is not
cosmetic — squid refuses to start, so the sandbox has no egress path at all.

This means the C# implementation is broken on native Linux and presumably only worked because Docker Desktop on
macOS remaps mount ownership. Chosen fix: create the directory `0777`. Rejected alternatives: keep `0700` faithful to
C# (leaves a startup crash-loop on native Linux), and `cache_effective_user root` in `squid.conf` (works, but removes
the privilege drop on the one component that holds internet access).

**Required implementation detail:** `os.MkdirAll(dir, 0o777)` is masked by umask — under the default `022` it yields
`0755`, silently reintroducing the exact bug. An explicit `os.Chmod(dir, 0o777)` must follow. Measured under umask
`022`: `MkdirAll(0o777)` → `0755`; `MkdirAll` + `Chmod(0o777)` → `0777`.

#### 3. Go file layout

Two files. One file per C# source was considered for a constant-time review against the original, but rejected in
favour of fewer files; reviewing against `Sbox/` means scrolling rather than switching files. Grouping by concern
instead of by source was also rejected, since it obscures the mapping back to the original.

The practical consequence: `main.go` grows to roughly 500–600 lines covering CLI dispatch, path resolution, argv
construction, process spawning and rendering. The sections in §5 mark the internal boundaries that would become file
boundaries if it is later split.

#### 4. SIGINT handling

C# installs a `ConsoleCancelKeyPress` handler that cancels its own termination, leaving the child to decide what
Ctrl-C means — an interactive shell survives it, `tail -f` does not — and keeping sbox alive to mirror the child's
exit code.

The obvious Go translation, `signal.Ignore(os.Interrupt)`, is wrong: POSIX propagates an *ignored* disposition across
`exec`, so the child would inherit `SIG_IGN` and Ctrl-C would be dead inside the sandbox. Measured by having a child
shell interrupt itself:

| Parent disposition | Child behaviour |
|--------------------|-----------------|
| `signal.Ignore(os.Interrupt)` | Child inherits `SIG_IGN`; `kill -INT $$` is a no-op, prints `SURVIVED` |
| `signal.Notify(ch, os.Interrupt)` | Child gets `SIG_DFL`; shell dies on SIGINT — matches C# |
| No signal call at all | Same as `Notify`, but sbox itself also dies and never mirrors the child's exit code |

Use `signal.Notify` and drain the channel. The third row is the reason a handler is needed at all: without it, Ctrl-C
in the sandbox shell tears down the wrapper.

#### 5. Templates verified equivalent

The Go templates were rendered against the same inputs as the C# renderer and diffed. The differences that remain are
all non-semantic:

| Case | C# output | Go output |
|------|-----------|-----------|
| `docker-compose.yaml`, populated | `volumes:    `, `- /a:/b    `, `extra_hosts:    ` (Scriban trailing spaces) | no trailing spaces |
| `docker-compose.yaml`, empty lists | emits a line containing four spaces where the `if` was | emits nothing |
| `docker-compose.yaml`, last line | `internet: { }` | `internet: {}` |
| `squid.conf` | 2-line comment, no trailing newline | 1-line comment, trailing newline |

`squid.conf` and `docker-compose.yaml` were each confirmed identical after trimming trailing whitespace, in both the
populated and empty-collection configurations. Note that the trimmed `{{- if }}` blocks — the suspected line-joining
bug — were the specific thing tested in the empty-collection case, and they render correctly.

Also confirmed a non-issue: C# renders `privileged` as the *string* `"true"`/`"false"` while Go passes a bool, but
`text/template` prints `true`/`false` identically.

One substantive difference remains and is intentional: C#'s template emits the literal `${SBOX_TAG}` for compose to
interpolate from the environment, while Go's substitutes `{{ .SboxTag }}` directly. Both yield the same project name
and image tag; Go's is self-describing and no longer depends on the variable being present.

Because the substitution now happens at render time, nothing reads `SBOX_TAG` at run time, so
`ChildEnvironment()` does not export it (#16). `USER` and `HOME` still are, and must be: compose
interpolates `${USER}` and `${HOME}` into the Dockerfile's build args, and the `devcontainer` CLI resolves
`${localEnv:USER}` from its own environment.

#### 10. `devcontainer` CLI and Docker availability

Measured in the dev sandbox: Docker Compose v5.6.1, node v26.13.0, npm 11.20.0, reachable network, and
`@devcontainers/cli` 0.90.0 installs to a temp prefix and runs. `devcontainer read-configuration` against the
C#-rendered config exits 0, resolves `remoteUser` from the literal `${localEnv:USER}` to the invoking user, resolves
the local feature ref against `<workspaceFolder>/.devcontainer`, and reports no diagnostics.

This is why the §8 manual checks can be run here, and why the §3a.13 tiers will be runnable when they are built. It
does *not* establish that Docker Desktop on macOS remaps mount ownership the same way — that remains open as §3.1
#11.

#### 12. `onPath` executable-bit check

C#'s `SboxPaths.OnPath` uses `File.Exists`, so a *non-executable* file named `devcontainer` on `PATH` passes the
check and then fails at exec with a confusing error, instead of reporting `sbox: 'devcontainer' not found on PATH`.
The port uses `exec.LookPath` instead. It splits `PATH` the same way C# splits it, but adds three rejections C# did
not have. First, the executable-bit requirement. Second, a *directory* named `devcontainer` satisfies the x bit but
is not runnable, and a hand-rolled mode test would have let it through — reintroducing the same misleading error.
Third, and narrower still, since Go 1.19 `LookPath` refuses an executable reached through a *relative* `PATH` element,
returning `exec.ErrDot`; C#'s `File.Exists` loop would have accepted it. The first two reject something already
unusable; the third rejects something that would have run, but only under a `PATH` containing a relative entry.

This is a deliberate divergence from C#, which is otherwise behavior-preserving. It is justified because it only
changes an error path that already fires when something is wrong, where the current message actively misleads.

#### 13. Test scope — deferred design of record

No automated tests ship with this port (§3.2 #7); verification is the manual checklist in §8, and a future plan covers
the suite. The design is recorded here so that plan starts from the three tiers, the harness and the probe gotchas
below rather than re-deriving them.

Integration-first was chosen over a unit-heavy tiering, whose layers would have been pure functions, render goldens,
filesystem writes, `PATH`-stub dispatch and the process runner, because the failures that actually matter are
emergent:

- squid exits 1 on a log directory it cannot write, which no unit test of `write` would notice unless it asserted the
  mode specifically;
- the local feature ref fails to resolve when the `.devcontainer` nesting is wrong, which only the real CLI reports;
- quoting breaks only at the shell boundary, where the difference between one argument and two is invisible until the
  command runs.

None of those surface in a unit test of the pure functions, which is the unit layer's whole claimed value.

Three integration tiers carry the weight, cheapest first: **tier 1**, config validation via
`devcontainer read-configuration` (seconds, no image build); **tier 2**, squid egress via `docker compose` (the only
coverage of the network restriction, and the only tier that catches the `0777` regression as an emergent symptom);
and **tier 3**, the full sandbox lifecycle via `devcontainer up` / `exec` (slowest, pulls images).

A thin unit layer is kept only where integration cannot reach: shell-quoting edge cases that are impractical to force
through a real shell, and the log-directory mode assertion, which fails silently and therefore needs an explicit
assertion rather than an emergent symptom.

**Harness, when it is built.** `TestMain` builds the binary once and every tier shells out to that path. Calling
`run(args)` in-process would be faster and give coverage for free, but it would not exercise exit-code mirroring, stdio
inheritance, SIGINT delivery, or argv as the OS sees it — which is most of what these tiers are for. Each test gets a
fresh `SBOX_HOME` under `t.TempDir()` and an `SboxTag` unique per test, so Compose project names cannot collide, plus
a `t.Cleanup` running `docker compose down -v`. Coverage across the process boundary needs `go build -cover` plus
`GOCOVERDIR`, not plain `-cover`.

Two probe gotchas, each of which makes a tier pass for the wrong reason:

- **Tier 3's cwd must be a mounted volume.** The `cd` guard fires whenever the host cwd has no counterpart in the
  sandbox, so a naive `sbox echo hi` from the test's working directory hits `not mounted` and the happy path is never
  exercised. The tier-3 config must mount the test directory and the test must run with cwd inside it; the `not
  mounted` case then deliberately runs from a cwd outside it, which is what makes that assertion meaningful.
- **Tier 2's probe must join `sandbox_egress`.** Both `sandbox` and `sandbox_egress` are `internal: true`, so the
  sandbox network has no route out. A probe on the wrong network sees the denied case succeed because *nothing was
  reachable*, not because the ACL blocked it — the worst kind of green, since that assertion is the one standing
  between the user and a silent allow-all.

#### 19. The `USER`-unset fallback

C# reads `USER`, and if that is blank falls back to `Environment.UserName` (`Sbox/SboxPaths.cs:77-82`), which never
fails. Go's equivalent can, and its behavior on a static build is worth pinning down before relying on it, since the
port ships as `CGO_ENABLED=0` with no cgo-backed `getpwuid_r`.

**Question:** with `CGO_ENABLED=0`, does `os/user.Current()` return a non-empty username in a plain environment, and
what does it do when `USER` is unset and when `/etc/passwd` has no entry for the uid?

**Probe:** Go 1.27.3 (linux/arm64); one ~20-line program in a `mktemp` dir printing `os.Getenv("USER")`,
`user.Current()` (Username + err) and `os.Getuid()`; built `CGO_ENABLED=0 go build -o probe .` and confirmed
`statically linked`; run normally, with `env -u USER`, with `USER=`, with `USER=bogusname`, and with `USER='  '`.

**Evidence:**

```
probe: ELF 64-bit LSB executable, ARM aarch64, statically linked, Go BuildID=... (not stripped)

=== (a) normal ===          USER env = "conway"    user.Current = "conway", err=<nil>   uid = 1001
=== (b) env -u USER ===     USER env = ""          user.Current = "conway", err=<nil>   uid = 1001
=== (c) USER= ===           USER env = ""          user.Current = "conway", err=<nil>   uid = 1001
=== (d) USER=bogusname ===  USER env = "bogusname" user.Current = "conway", err=<nil>   uid = 1001
=== (e) USER='  ' ===       USER env = "  "        user.Current = "conway", err=<nil>   uid = 1001
```

Run (d) is the discriminating one: `user.Current()` ignores `USER` entirely and resolves uid 1001 from `/etc/passwd`,
so the fallback works for reasons the environment cannot disturb.

**Verdict:** Confirmed for the main question. Inconclusive for the no-`/etc/passwd`-entry case — the running uid
1001 has a valid entry and Go hardcodes the `/etc/passwd` path, so a lookup miss cannot be forced without root.

**Recommendation** (adopted as #19): trim `USER`, use it when non-empty, else `os/user.Current().Username`. Treat an
error, or a nil or empty `Username`, as a failure rather than falling through to an empty tag: `SboxTag` is
`sbox-<tag>` and is the
Compose project name, so an empty one produces `sbox-` and silently collides across users. The failure path is
exactly the one the spike could not reproduce, so it is worth a glance at integration time; the message is
`sbox: cannot determine user`, exit 1.

#### 25. `stdioSuppress` and the null device

§5 describes `stdioSuppress` as "redirect the stream and discard it", and C#'s `ReadToEndAsync`-and-drop does exactly
that: it drains the child's pipe and throws the bytes away. The Go implementation is not that. `streamWriter` returns
a nil `io.Writer`, and `os/exec` connects a nil stream to the null device (`os.DevNull`) — the child writes into the
kernel's discard sink, and nothing on the parent side reads or buffers anything.

The difference is unobservable here. Both suppress the output identically at the two call sites — `up`'s progress on
stdout, `docker compose exec`'s output during reconfigure — because neither ever consults the dropped bytes. The null
device is also strictly safer: a pipe that is written but never read can fill and block the child, which is the exact
failure the concurrent-draining rule exists to avoid, whereas a null device has no capacity to fill. The nil writer is
kept deliberately, and this note records that the §5 wording is an analogy to C#'s mechanism, not a description of
Go's.

---

## 4. Change Surface

| Scope | Change | Notes |
|-------|--------|-------|
| `main.go` | Modify | Replace the stub `main()` and its dead code with the full CLI; add path resolution, argv construction, process running, squid commands, config loading |
| `config.go` | Modify | Add json tags to `NetworkConfig` for consistency with `Config` (behaviorally a no-op) |
| `templates/*.tmpl` | No change | Verified equivalent — see §3a.5 |
| `justfile` | Modify | Add an `install` recipe; the dotnet `install-mac` is untouched — see §3.2 #15 |
| `README.md` | No change | The log directory keeps its documented `$SBOX_HOME/logs/squid` location — see §3.2 #14 |
| `Sbox/`, `Sbox.sln`, `out/` | No change | Deletion is a manual step — see §3.1 #9 |
| `go.mod` | No change | Standard library only; `module sbox`, `go 1.27.3` |

No new dependencies. The `.NET` tree used `CliWrap` for process spawning and `Scriban` for templates; Go's
`os/exec` and `text/template` replace both.

The new recipe keeps the existing one's shape — build into `out/`, then install to `~/.local/bin` — and drops only
the `dotnet publish` and its runtime matrix. It carries no platform in its name: `go build` targets the host platform
on its own, so the runtime matrix the dotnet recipe needed has no Go counterpart to encode:

```just
install:
	go build -o out/sbox . && \
		cp out/sbox ~/.local/bin/sbox
```

---

## 5. Module Decomposition & Internal API

`package main`, two files. No new module boundary is introduced — this is a same-shape translation of an existing
one. The table describes internal boundaries that would become file boundaries if `main.go` is later split.

| Boundary | Responsibility | Depends on |
|----------|----------------|------------|
| Path resolution | Resolve and derive every host path from the environment; `PATH` lookup | `os`, `path/filepath` |
| CLI dispatch | Map argv to a mode, sequence render/up/exec, own exit codes and error reporting | Path resolution, rendering, process runner |
| Rendering | Produce the eight generated artifacts from `SetupData` | `text/template`, `encoding/json`, `templates/*` |
| Writing | Create directories and write the artifact tree | Rendering |
| Argv construction | Build argv for `devcontainer` and `docker compose`, plus the in-container bash payload and its quoting | Path resolution |
| Process runner | The single place a child process is started | `os/exec`, `os/signal` |

### Data shapes (`config.go`)

```go
package main

type Config struct {
	Volumes    []string          `json:"volumes"`
	Privileged bool              `json:"privileged"`
	Network    NetworkConfig     `json:"network"`
	RemoteEnv  map[string]string `json:"remoteEnv"`
	Features   map[string]any    `json:"features"`
}

type NetworkConfig struct {
	AllowAll     bool     `json:"allowAll"`
	AllowDomains []string `json:"allowDomains"`
	AllowIps     []string `json:"allowIps"`
	ExtraHosts   []string `json:"extraHosts"`
}

// SetupData replaces C#'s FullConfig: the parsed config plus the verbatim install.sh.
type SetupData struct {
	Config                Config
	InstallScriptContents string
}
```

### Path resolution

`EnvironmentInfo` and its lone `SboxHome` field are removed. `RenderConfig` and `Write` take `SboxPaths` instead, so
`SquidLogDir` is derived in one place rather than concatenated as a string at the call site. That call site today
reads `envInfo.SboxHome + "/build/logs/squid"` (`main.go:92`) — a path the scaffold invented, wrong on two counts:
wrong location (#14) and derived outside the type that owns it. Both go away with the move into `SboxPaths`.

```go
type SboxPaths struct {
	User     string // $USER, trimmed; else os/user.Current().Username
	SboxTag  string // strings.ToLower(User)
	Home     string // $HOME, absolute, no trailing separator
	SboxHome string // $SBOX_HOME, else $HOME/.sbox; absolute
}

func (p SboxPaths) BuildDir() string               { /* <SboxHome>/build */ }
func (p SboxPaths) DevcontainerDir() string        { /* <BuildDir>/.devcontainer */ }
func (p SboxPaths) ConfigPath() string             { /* <SboxHome>/config.json */ }
func (p SboxPaths) InstallScriptPath() string      { /* <SboxHome>/install.sh */ }
func (p SboxPaths) ComposeFilePath() string        { /* <DevcontainerDir>/docker-compose.yaml */ }
func (p SboxPaths) DevcontainerConfigPath() string { /* <DevcontainerDir>/devcontainer.json */ }
func (p SboxPaths) SquidLogDir() string            { /* <SboxHome>/logs/squid */ }
func (p SboxPaths) EgressLogPath() string          { /* <SquidLogDir>/egress.log */ }
func (p SboxPaths) ComposeProjectName() string     { /* sbox-<SboxTag> */ }
func (p SboxPaths) ChildEnvironment() map[string]string { /* USER, HOME overlay */ }

// Fails when HOME is unset or whitespace-only, or when no username can be
// determined, or when filepath.Abs fails. A relative SBOX_HOME resolves
// against the current directory. Returns a human-readable error for stderr.
func resolvePaths() (SboxPaths, error) { /* ... */ }

// Whether executable names a runnable file in a PATH directory. exec.LookPath,
// so the executable bit is required, diverging from C#'s bare existence check —
// see §3a.12.
func onPath(executable string) bool { /* ... */ }
```

`DevcontainerDir` nesting is load-bearing and must be preserved: the CLI resolves the local feature ref
`./features/sbox-feature` relative to the config file's directory and rejects anything outside
`<workspaceFolder>/.devcontainer`, so the artifacts cannot sit directly in `BuildDir`.

### Rendering and writing

`EnvironmentInfo` is dropped; `SboxTag` and the squid log path both come from `paths`.

```go
type SquidConfTmplData struct{ AllowAll bool }

type DockerComposeTmplData struct {
	SboxTag           string
	Volumes           []string
	Privileged        bool
	ExtraHosts        []string
	SquidLogDirectory string
}

// The two JSON artifacts, marshaled with marshalJSON. Fields and their json
// tags are unchanged from the scaffold (main.go:30-44).
type DevcontainerManifest struct { /* name, dockerComposeFile, service, remoteUser, remoteEnv, features */ }
type SboxFeatureManifest struct { /* id, version, name, dependsOn */ }

type RenderedConfig struct { /* the eight artifact strings, as today */ }

// Unchanged in behavior; signatures now take paths so SboxTag and the squid
// log directory are derived, not hardcoded.
func RenderConfig(paths SboxPaths, setupData SetupData) (RenderedConfig, error) { /* ... */ }
func renderTemplate(fileName string, data any) (string, error)                  { /* ... */ }
func marshalJSON(data any) (string, error)                                      { /* ... */ }
func orEmptyMap[V any](m map[string]V) map[string]V                             { /* ... */ }

// Writes the eight artifacts under <SboxHome>/build/. Creates the squid log
// directory separately (see below) — it is not part of the rendered tree.
func write(paths SboxPaths, result RenderedConfig) error { /* ... */ }
func writeFile(path string, contents string) error       { /* ... */ }
```

The squid log directory is created outside `write`'s per-file `MkdirAll`, because it is the one directory needing a
mode other than `0755`, and `MkdirAll` alone cannot express it (§3a.2):

```go
// Squid runs as uid 13 and writes egress.log here. MkdirAll applies umask, so
// the mode must be forced afterwards or the directory lands 0755 and squid
// fails to start with "Cannot open ... for writing".
func ensureSquidLogDir(paths SboxPaths) error { /* MkdirAll(0o777) then Chmod(0o777) */ }
```

### Argv construction

Pure string work, no I/O. Ports `DevcontainerArgs.cs` and `SquidArgs.cs`.

```go
// devcontainer
func devcontainerUpArgs(paths SboxPaths, rebuild bool) []string { /* ... */ }
func devcontainerExecCommandArgs(paths SboxPaths, hostCwd string, command []string) []string { /* ... */ }
func devcontainerExecShellArgs(paths SboxPaths, hostCwd string) []string { /* ... */ }
func commandPayload(paths SboxPaths, hostCwd string, command []string) string { /* ... */ }
func shellPayload(paths SboxPaths, hostCwd string) string { /* ... */ }
func cdGuard(paths SboxPaths, hostCwd string) string { /* cd -- <cwd> || { ...; exit 1; } */ }

// Quotes for a POSIX shell. Safe-only values pass through unquoted; everything
// else is single-quoted, surviving spaces, quotes, $, backticks and newlines.
func shellQuote(value string) string { /* ... */ }

// squid
func squidReconfigureArgs(paths SboxPaths) []string { /* compose --project-name <n> --file <f> exec -T squid squid -k reconfigure */ }
func followEgressLogArgs(paths SboxPaths) []string  { /* tail -n 500 -f <EgressLogPath> */ }
func composeExecArgs(paths SboxPaths, command ...string) []string { /* the above, minus the trailing command */ }
```

Two quoting behaviors are load-bearing and must be preserved exactly:

- `commandPayload` passes a **lone** argument through untouched, so shell syntax the caller quoted into it — pipes,
  redirects, an `sh -c` string — is still parsed on the far side. Two or more arguments are each quoted before
  joining, so an argument containing spaces stays one argument instead of being split by the shell.
- `shellQuote`'s safe-character set is `a-zA-Z0-9._/:=@%+^,-`. Unnecessary quotes are dropped so ordinary paths pass
  through unchanged.

The `cd` guard exists because mounts bind repos at their real host paths: a directory that is not mounted has no
counterpart inside the sandbox, and the command must not silently run somewhere else.

`hostCwd` is `os.Getwd()`, read once in `execDevcontainer` and threaded through, mirroring C#'s
`Directory.GetCurrentDirectory()` (`Sbox/Program.cs:182`). If it fails there is no sensible fallback — the guard's
whole purpose is to run where the caller stood — so the error is reported and the command exits 1.

### Process runner

```go
type stdioMode int

const (
	stdioInherit  stdioMode = iota // leave the stream attached to the parent process
	stdioSuppress                  // redirect the stream and discard it
	stdioCapture                   // redirect the stream and return it
)

type processResult struct {
	ExitCode       int
	Stdout, Stderr string // empty unless the mode was stdioCapture
}

// The single shell-out point: argv, paths and the environment overlay are all
// built elsewhere. Overlays env on the inherited environment. The error is
// non-nil only when the child could not be started; a child that runs and
// exits non-zero is a processResult, not an error — see §6.
func runProcess(stdin, stdout, stderr stdioMode, name string, args []string, env map[string]string) (processResult, error) { /* ... */ }
```

Three behaviors to reproduce, all from `ProcessRunner.cs`:

- **Ctrl-C.** Install a `signal.Notify` handler draining `os.Interrupt` for the duration of the call, and stop it
  after — never `signal.Ignore` (§3a.4). Go does not `exec`-replace itself (a .NET process cannot either), so sbox
  stays alive to mirror the child's exit code.
- **Environment overlay.** `os/exec` seeds `Cmd.Env` from the current process, so the host environment is inherited
  and `ChildEnvironment()`'s `USER` / `HOME` entries only overlay it. This is required, not merely convenient: the
  `devcontainer` CLI resolves `${localEnv:...}` from its own environment, and compose interpolates `${USER}` and
  `${HOME}` into the Dockerfile's build args from the one it is handed. Note that `Cmd.Env` must be set explicitly
  from `os.Environ()` plus the overlay — leaving `Cmd.Env` nil would also inherit, but then the overlay cannot be
  applied.
- **Concurrent draining.** Captured streams must be read concurrently with waiting — start the read before `Wait` —
  so a chatty child cannot fill a pipe and block. This is what `ProcessRunner.cs` does with `ReadToEndAsync`.

Each of the four call sites pins its three modes, carried over from the C# callers unchanged:

| Call site | stdin | stdout | stderr | Visible effect |
|-----------|-------|--------|--------|----------------|
| `devcontainer up` | Inherit | Suppress | Inherit | `up`'s progress output never reaches the terminal; its errors do |
| `devcontainer exec` | Inherit | Inherit | Inherit | Fully interactive — the sandbox session |
| `docker compose exec` (reconfigure) | Inherit | Capture | Capture | Nothing printed; squid's output is read only to be discarded |
| `tail -f` (squid logs) | Inherit | Inherit | Inherit | Fully interactive — follows the log |

`Suppress` and `Capture` differ only in whether the piped bytes are kept: both detach the stream from the parent, so
`up`'s stdout is unreadable and reconfigure's is read and dropped. Both matter, because leaving `Cmd.Stdout` nil in
Go means "inherit" — the default would leak output the C# deliberately hid.

### CLI dispatch

Ports `Program.cs`. Mode flags take no further arguments; render is implicit for every mode that gets past the
`--help` / `--squid-logs` early returns.

```go
func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int { /* dispatch; see flow in §7 */ }

// Renders the configs and creates the squid log directory. Implicit for every
// mode that reaches it: the build directory is derived state, and a stale one
// fails inside the container in ways that are hard to connect back to an
// edited config.json.
func render(paths SboxPaths) error { /* ... */ }

// Reads config.json (absent -> zero-value Config) and install.sh (absent -> "").
func loadConfig(paths SboxPaths) (SetupData, error) { /* ... */ }

func up(paths SboxPaths, rebuild bool) (int, error)       { /* devcontainer up; error only when the child could not be started */ }
func execDevcontainer(paths SboxPaths, args []string) int { /* devcontainer exec */ }
func reconfigureSquid(paths SboxPaths) int                { /* docker compose exec */ }
func followEgressLog(paths SboxPaths) int                 { /* tail -f */ }

func printUsage()
func debug(format string, args ...any) // prints to stdout only when SBOX_DEBUG == "true"
func fail(message string) int          // writes to stderr, returns 1
```

`execDevcontainer` takes its command from `args`, the whole of it, rather than from the mode variable: every mode flag
has already returned by that point, so all of argv is the container command, first token included. (`args` is exactly
`os.Args[1:]` — `main` slices before calling — so the program's own path is never mistaken for a command.)

`debug` is generic but has exactly one call site, in `render`, reporting the render target (`Sbox/Program.cs:101`
does the same). It is not a general logging facility.

Note the name: `execDevcontainer`, not `exec`, to avoid shadowing the `os/exec` package.

---

## 6. CLI Contract

The CLI is the external surface. It is recorded here because preserving it is the point of the exercise — with three
deliberate exceptions, each tagged *(new)* below and each traceable to a decision above.

| Invocation | Behaviour | Exit |
|------------|-----------|------|
| `sbox` | Interactive shell in the sandbox at the host cwd | Child's |
| `sbox <command>...` | Run a command in the sandbox | Child's |
| `sbox --build` | Recreate the container and rebuild the image, then exit | 0, or 1 on failure |
| `sbox --render` | Render the build directory; no Docker | 0, or 1 |
| `sbox --squid-reconfigure` | Reload squid's ACLs | 0, or 1 |
| `sbox --squid-logs` | Follow the egress log from the host | `tail`'s |
| `sbox --help` / `-h` | Print usage | 0 |
| `sbox <unknown-flag>` | Treated as a container command | Child's |

**Error semantics.** Rows marked *(new)* have no C# counterpart; the rest are preserved exactly.

| Condition | Behaviour |
|-----------|-----------|
| Mode flag given extra arguments | stderr `sbox: '<mode>' takes no arguments`, exit 1 |
| `HOME` unset or whitespace-only | stderr `sbox: HOME is not set`, exit 1 — C# tested `IsNullOrWhiteSpace` too (#23) |
| No username resolvable *(new)* | stderr `sbox: cannot determine user`, exit 1 — C#'s `Environment.UserName` cannot fail, so this path is unreachable there (#19) |
| Malformed or unreadable `config.json` | The `json`/`os` error message on stderr, exit 1 — reported, never a panic or stack trace |
| `devcontainer` not on `PATH` | stderr `sbox: 'devcontainer' not found on PATH`, exit 1 |
| `devcontainer` present but not executable *(new)* | The same message, exit 1. C# accepted a non-executable file here and failed later at exec (#20) |
| `devcontainer up` fails | stderr `sbox: 'devcontainer up' failed (is Docker running?)`, exit 1 |
| A child process cannot be started at all *(new)* | Its `exec` error message on stderr, exit 1. C# left this unhandled — a failed `process.Start()` raises `Win32Exception`, which is not in its catch filter (#22) |
| `squid -k reconfigure` fails | stderr `sbox: 'squid -k reconfigure' failed (is the sandbox up?)`, exit 1 |
| Command's cwd not mounted in the sandbox | The in-container `cd` guard's message, written inside the sandbox, exit 1 |

The C# `catch` filter is broad: any `JsonException`, `IOException` or `UnauthorizedAccessException` becomes a
one-line stderr message and exit 1. In Go those become error values returned by `loadConfig`/`render`/`write`, which
`run` prints through `fail` and turns into exit 1. `main` stays `os.Exit(run(os.Args[1:]))` and carries no error
handling of its own (#21). The notable consequence is that an unreadable `config.json` must not panic.

`fail` writes its argument verbatim, so a config error prints the bare `json`/`os` message, matching C#. Errors the
port wraps with context — a malformed embedded template, say — additionally carry that context, which C# would not
have produced; it would have thrown an unhandled Scriban exception out of the catch filter instead.

---

## 7. Data Flow

Mode dispatch, showing where render sits relative to the early returns:

```
run(args)
  |
  |-- args empty?              -> mode = ""
  |-- mode is --help or -h?    -> printUsage(); return 0
  |-- mode is a mode flag and len(args) > 1?
  |                            -> fail("<mode> takes no arguments"); return 1
  |
  |-- resolvePaths() == error? -> fail(err); return 1
  |
  |-- mode == --squid-logs?    -> followEgressLog(); return        <-- returns BEFORE render
  |
  |-- render(paths) == error?  -> fail(err); return 1
  |   (on success, and implicit for every mode below)
  |     |-- ensureSquidLogDir()   (0777)
  |     |-- loadConfig()          (config.json, install.sh; absent -> zero values)
  |     |-- RenderConfig()
  |     |-- write()               (8 artifacts under build/.devcontainer)
  |
  |-- mode == --render?        -> return 0
  |-- mode == --squid-reconfigure? -> return reconfigureSquid()   (prints its own failure)
  |
  |-- onPath("devcontainer") == false?
  |                            -> fail("not found on PATH"); return 1
  |
  |-- up(paths, mode == "--build") returns error?
  |                            -> fail(exec error); return 1
  |-- ... else != 0?           -> fail("devcontainer up failed"); return 1
  |-- mode == --build?         -> return 0
  |
  |-- execDevcontainer(args)   -> return child's exit code
```

Failure strings are abbreviated above; §6 has the exact text. Two error paths are handled inside the wrappers rather
than drawn in the flow:

- A `runProcess` error — the child could not be started (#22) — never reaches the flow's exit-code comparisons.
  Whichever wrapper made the call reports it through `fail` and exits 1.
- `reconfigureSquid` likewise converts a non-zero `squid -k reconfigure` into its own message, so its arm of the
  flow is just `return reconfigureSquid()`. `up` is the exception: it returns the bare exit code plus an error,
  leaving both messages to `run` — the exec error when the child could not be started, the Docker message
  otherwise.

The render step's render-before-validation ordering is inherited from C# and worth preserving: a render that fails
reports its own error before the `devcontainer` PATH check can mask it, which is why the C# build in this sandbox
wrote its tree and *then* reported `devcontainer not found on PATH`.

Rendering, in detail:

```
SetupData ──┬─> squid.conf.tmpl      ─> squid.conf
            ├─> docker-compose.yaml.tmpl ─> docker-compose.yaml
            │      (SboxTag, Volumes, Privileged, ExtraHosts, SquidLogDirectory)
            ├─> Dockerfile.tmpl      ─> Dockerfile  (copied verbatim)
            ├─ Network.AllowDomains  ─> strings.Join("\n") ─> acls/dstdomain.allowlist.txt
            ├─ Network.AllowIps      ─> strings.Join("\n") ─> acls/dst.allowlist.txt
            ├─ devcontainer.json     ─> marshalJSON ─> devcontainer.json
            │     name=sbox, dockerComposeFile=docker-compose.yaml, service=sandbox,
            │     remoteUser=${localEnv:USER}, remoteEnv, features{"./features/sbox-feature": {}}
            ├─ devcontainer-feature.json ─> marshalJSON ─> features/sbox-feature/devcontainer-feature.json
            │     id/version/name=sbox-feature, dependsOn = Features
            └─ install.sh contents   ─> features/sbox-feature/install.sh  (verbatim)
```

Two details that must not be lost:

- `remoteUser` is the **literal string** `${localEnv:USER}`, not a substituted value — the `devcontainer` CLI
  resolves it, which is why `USER` must be in the child environment.
- The generated `install.sh` runs as root during image build and exists for the one thing the image cannot guess: the
  mount points under `$HOME` that Compose creates as `root:root`, leaving the agent unable to write there.

Runtime flow once inside the sandbox:

```
sbox <cmd> ──> devcontainer exec ──> bash -lc "<cd guard>; <quoted cmd>"
                                          |
                                          +-- cwd not mounted -> guard prints the
                                              "not mounted" message + exit 1

agent ──> HTTP(S)_PROXY=http://squid:3128 ──> squid (uid 13)
                                                 |-- allowlist hit  -> egress allowed
                                                 |-- miss           -> denied, logged to
                                                 |                     egress.log as an
                                                 |                     allowlist-miss line
                                                 +-- /var/log/squid bind-mounted from
                                                     $SBOX_HOME/logs/squid
sbox --squid-logs ──> tail -n 500 -f <that host path>
```

Network topology from the generated compose file: `sandbox` sits on an internal network plus `sandbox_egress`;
`squid` alone bridges `sandbox_egress` and `internet`. The sandbox therefore has no route out except through squid.

---

## 8. Verification — Manual Checklist

No automated tests in this port (§3.2 #7); a future plan covers them, and §3a.13 is the design of record. Verification
is a manual checklist, cheapest first. Steps 1–3 run in the dev sandbox and were executed while writing this plan;
step 4 needs a machine with the `devcontainer` CLI and Docker.

### 1. Render comparison against C#

Run the C# and Go renderers over the same `$SBOX_HOME` inputs and diff, trimming trailing whitespace. Expected
residual differences are limited to those tabulated in §3a.5. Already executed while writing this plan; the templates
pass.

Matrix: an empty `{}` config, a populated config, and both `allowAll` values.

### 2. Squid log directory mode

The regression is silent — the directory simply lands `0755` and squid dies later — so it needs a direct check, plus a
start to confirm the real failure is not somewhere else:

```sh
go build -o /tmp/sbox .
SBOX_HOME=/tmp/sbox-check /tmp/sbox --render
ls -ldn /tmp/sbox-check/logs/squid                # expect drwxrwxrwx

cd /tmp/sbox-check/build/.devcontainer
docker compose -f docker-compose.yaml up -d squid
sleep 10
docker compose -f docker-compose.yaml ps          # expect "Up", NOT "Restarting (1)"
docker compose -f docker-compose.yaml logs squid | grep -i fatal
docker compose -f docker-compose.yaml down -v
```

Confirmed while writing this plan: with `0700` the container reports `Restarting (1)` and `FATAL: Cannot open
'/var/log/squid/egress.log' for writing`; with `0777` it reports `Up` and `Accepting HTTP Socket connections`.

### 3. Rendered config accepted by the real CLI

`devcontainer` installs with `npm i -g @devcontainers/cli` if it is not already present.

```sh
devcontainer read-configuration \
  --workspace-folder /tmp/sbox-check/build \
  --config /tmp/sbox-check/build/.devcontainer/devcontainer.json
```

Confirm exit 0 with no diagnostics, that `remoteUser` resolves to the invoking user rather than the literal
`${localEnv:USER}` (proving `USER` reaches the child environment), that the local feature ref `./features/sbox-feature`
resolves (proving the `.devcontainer` nesting in §5), and that `dockerComposeFile` resolves.

### 4. End-to-end on macOS

In order: `sbox --render`, `sbox --build`, then `sbox`, `sbox <command>`, `sbox --squid-reconfigure`,
`sbox --squid-logs`. Behaviours to confirm, each mapping to a decision above:

| Check | Decision |
|---|---|
| Ctrl-C inside the sandbox interrupts the running command but not sbox, and sbox still mirrors the child's exit code | §3a.4 |
| A command run from an unmounted directory prints the `not mounted` message and exits 1, rather than running somewhere else | §5 |
| `sbox sh -c 'exit 42'` returns 42; `sbox echo "a b"` prints `a b` as one argument | §5 quoting |
| A host outside the allowlist is denied, and the attempt appears in the egress log on the host | §3a.2 |
| An allowlisted host is reachable through the proxy | §6 |

When probing allow/deny by hand, put the client on the `sandbox_egress` network. Both `sandbox` and `sandbox_egress`
are `internal: true`, so a client on the wrong network reports "denied" because *nothing was reachable* rather than
because the ACL blocked it — the failure mode that looks like a pass.

### Not covered, and why

- **Docker Desktop's mount-ownership remapping on macOS** (§3.1 #11). Host-runtime specific, and the reason the C#
  `0700` ever appeared to work.
- **True terminal Ctrl-C semantics.** Sending SIGINT to a process group is not the same as the terminal driver's
  treatment of a foreground process group, which needs a pty to reproduce.

---

## 9. Non-Functional Considerations

- **Backward compatibility.** Every on-disk path is unchanged, including the squid log directory at
  `$SBOX_HOME/logs/squid` (#14), the CLI surface is unchanged (§6), and the README needs no edit. Two things a user
  can observe do change, both deliberately: the log directory's mode goes from `0700` to `0777` (#2), which is the
  point of that fix, and `onPath` now rejects a non-executable or directory `devcontainer` where C# accepted it
  (#20), converting a confusing exec failure into a clear message. Neither breaks an existing workflow.
- **Security.** The log directory is `0777`: world-writable on the host. It holds only squid's egress log, and
  `sbox` is explicitly documented as *not* a security boundary — its purpose is preventing accidents, not containing
  malicious agents. A stricter alternative (owning the directory as uid 13) needs root on the host and does not work
  with Docker Desktop on macOS, so it was rejected. Note that squid writes its own files `0640` regardless.
- **Failure mode to watch.** A log directory that is not writable does not degrade — squid exits 1 and the sandbox
  has no egress at all. The `0777` + explicit `Chmod` is the mitigation, and §8 step 2 is the manual guard.
- **Observability.** `SBOX_DEBUG=true` prints the render target to stdout, replacing `Console.WriteLine`. Everything
  else reports through stderr and exit codes, as today. Squid's own egress log is the primary observability surface
  for what the agent tried to reach, and its format — ISO8601 local time, Squid/HTTP status, method, URL with query
  stripped, origin IP, User-Agent — is unchanged.
- **Scalability.** Not applicable: a single-user CLI that runs one child process per invocation.
- **Portability.** The port removes the .NET runtime requirement and the `--runtime osx-arm64` publish step. The
  `justfile` gains a platform-agnostic `install` recipe alongside the existing dotnet `install-mac`, so both builds
  work until the C# tree is deleted. Building is a plain `go build` anywhere the toolchain is present — the platform
  is picked by the toolchain, not declared by the recipe, which is why the dotnet recipe's runtime matrix has no Go
  counterpart.
