package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
)

//go:embed templates/*
var templateFs embed.FS

const debugEnvironmentVariable = "SBOX_DEBUG"

// modeFlags are the first tokens that name a mode, and therefore take no further
// arguments.
var modeFlags = []string{
	"--build",
	"--render",
	"--squid-reconfigure",
	"--squid-logs",
}

type SquidConfTmplData struct {
	AllowAll bool
}

type DockerComposeTmplData struct {
	SboxTag           string
	Volumes           []string
	Privileged        bool
	ExtraHosts        []string
	SquidLogDirectory string
}

type DevcontainerManifest struct {
	Name              string            `json:"name"`
	DockerComposeFile string            `json:"dockerComposeFile"`
	Service           string            `json:"service"`
	RemoteUser        string            `json:"remoteUser"`
	RemoteEnv         map[string]string `json:"remoteEnv"`
	Features          map[string]any    `json:"features"`
}

type SboxFeatureManifest struct {
	Id        string         `json:"id"`
	Version   string         `json:"version"`
	Name      string         `json:"name"`
	DependsOn map[string]any `json:"dependsOn"`
}

type RenderedConfig struct {
	SquidConf                            string
	SquidIpAcl                           string
	SquidDomainAcl                       string
	Dockerfile                           string
	DockerCompose                        string
	DevcontainerManifest                 string
	DevcontainerSboxFeatureManifest      string
	DevcontainerSboxFeatureInstallScript string
}

// SboxPaths holds the host-side values every mode needs, resolved once at
// startup: the user and sandbox tag exported to child processes, and the sbox
// home directory everything else is derived from.
type SboxPaths struct {
	User     string // $USER, trimmed; else os/user.Current().Username
	SboxTag  string // strings.ToLower(User)
	Home     string // $HOME, absolute, no trailing separator
	SboxHome string // $SBOX_HOME, else $HOME/.sbox; absolute
}

// BuildDir is the devcontainer project root, and the workspace folder handed to
// the CLI.
func (p SboxPaths) BuildDir() string { return filepath.Join(p.SboxHome, "build") }

// DevcontainerDir is where everything the renderer emits lives. The nesting is
// load-bearing: the CLI resolves a local feature ref against the config file's
// directory and rejects anything that is not a child of
// <workspaceFolder>/.devcontainer, so the artifacts cannot sit directly in
// BuildDir.
func (p SboxPaths) DevcontainerDir() string { return filepath.Join(p.BuildDir(), ".devcontainer") }

func (p SboxPaths) ConfigPath() string { return filepath.Join(p.SboxHome, "config.json") }

func (p SboxPaths) InstallScriptPath() string {
	return filepath.Join(p.SboxHome, "install.sh")
}

func (p SboxPaths) ComposeFilePath() string {
	return filepath.Join(p.DevcontainerDir(), "docker-compose.yaml")
}

func (p SboxPaths) DevcontainerConfigPath() string {
	return filepath.Join(p.DevcontainerDir(), "devcontainer.json")
}

func (p SboxPaths) SquidLogDir() string { return filepath.Join(p.SboxHome, "logs", "squid") }

func (p SboxPaths) EgressLogPath() string {
	return filepath.Join(p.SquidLogDir(), "egress.log")
}

func (p SboxPaths) ComposeProjectName() string { return "sbox-" + p.SboxTag }

// ChildEnvironment is the environment overlaid on the inherited one for every
// child process. Required, not just convenient: the devcontainer CLI resolves
// ${localEnv:...} from its own environment, and compose interpolates ${USER}
// and ${HOME} from the one it is handed. SBOX_TAG is deliberately not exported
// — the template substitutes the tag at render time.
func (p SboxPaths) ChildEnvironment() map[string]string {
	return map[string]string{
		"USER": p.User,
		"HOME": p.Home,
	}
}

// resolvePaths resolves from the current environment. It fails when HOME is
// unset or whitespace-only, or when no username can be determined, or when
// filepath.Abs fails. An absent SBOX_HOME falls back to $HOME/.sbox, and a
// relative one resolves against the current directory. Both Home and SboxHome
// are absolute with no trailing separator (filepath.Abs cleans them, mirroring
// Path.GetFullPath plus the trailing-separator trim).
func resolvePaths() (SboxPaths, error) {
	home := os.Getenv("HOME")

	if strings.TrimSpace(home) == "" {
		return SboxPaths{}, errors.New("sbox: HOME is not set")
	}

	name := strings.TrimSpace(os.Getenv("USER"))

	if name == "" {
		current, err := user.Current()

		if err != nil || current.Username == "" {
			return SboxPaths{}, errors.New("sbox: cannot determine user")
		}

		name = current.Username
	}

	sboxHome := os.Getenv("SBOX_HOME")

	if strings.TrimSpace(sboxHome) == "" {
		sboxHome = filepath.Join(home, ".sbox")
	}

	absHome, err := filepath.Abs(home)

	if err != nil {
		return SboxPaths{}, err
	}

	absSboxHome, err := filepath.Abs(sboxHome)

	if err != nil {
		return SboxPaths{}, err
	}

	return SboxPaths{
		User:     name,
		SboxTag:  strings.ToLower(name),
		Home:     absHome,
		SboxHome: absSboxHome,
	}, nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	mode := ""

	if len(args) > 0 {
		mode = args[0]
	}

	if mode == "--help" || mode == "-h" {
		printUsage()
		return 0
	}

	if mode != "" && isModeFlag(mode) && len(args) > 1 {
		return fail(fmt.Sprintf("sbox: '%s' takes no arguments", mode))
	}

	paths, err := resolvePaths()

	if err != nil {
		return fail(err.Error())
	}

	// --squid-logs returns before rendering: it only follows a log file on the
	// host and needs neither the build tree nor Docker.
	if mode == "--squid-logs" {
		return followEgressLog(paths)
	}

	if err := render(paths); err != nil {
		return fail(err.Error())
	}

	if mode == "--render" {
		return 0
	}

	if mode == "--squid-reconfigure" {
		return reconfigureSquid(paths)
	}

	// The PATH check comes after render on purpose: a render that fails reports
	// its own error before this can mask it.
	if !onPath("devcontainer") {
		return fail("sbox: 'devcontainer' not found on PATH")
	}

	if code, err := up(paths, mode == "--build"); err != nil {
		// The child could not be started at all: a distinct path, reported with
		// the exec error rather than the Docker message.
		return fail(err.Error())
	} else if code != 0 {
		return fail("sbox: 'devcontainer up' failed (is Docker running?)")
	}

	if mode == "--build" {
		return 0
	}

	// Every remaining mode has returned by now, so all of args is the container
	// command — the first token included.
	return execDevcontainer(paths, args)
}

// render regenerates the devcontainer project and creates the squid log
// directory. It is implicit for every mode that reaches it: the build directory
// is derived state, and a stale one fails inside the container in ways that are
// hard to connect back to an edited config.json.
func render(paths SboxPaths) error {
	debug("rendering sbox configs to %s", paths.DevcontainerDir())

	if err := ensureSquidLogDir(paths); err != nil {
		return err
	}

	setupData, err := loadConfig(paths)

	if err != nil {
		return err
	}

	renderedConfig, err := RenderConfig(paths, setupData)

	if err != nil {
		return err
	}

	return write(paths, renderedConfig)
}

// loadConfig reads config.json (absent -> zero-value Config) and install.sh
// (absent -> empty string).
func loadConfig(paths SboxPaths) (SetupData, error) {
	setupData := SetupData{Config: Config{}}

	configContents, err := os.ReadFile(paths.ConfigPath())

	switch {
	case err == nil:
		if err := json.Unmarshal(configContents, &setupData.Config); err != nil {
			return SetupData{}, err
		}
	case errors.Is(err, os.ErrNotExist):
		// Absent config.json renders the zero-value Config.
	default:
		return SetupData{}, err
	}

	installScript, err := os.ReadFile(paths.InstallScriptPath())

	switch {
	case err == nil:
		setupData.InstallScriptContents = string(installScript)
	case errors.Is(err, os.ErrNotExist):
		// Absent install.sh renders an empty script.
	default:
		return SetupData{}, err
	}

	return setupData, nil
}

func RenderConfig(paths SboxPaths, setupData SetupData) (RenderedConfig, error) {
	renderedConfig := RenderedConfig{}

	squidConfTmplData := SquidConfTmplData{
		AllowAll: setupData.Config.Network.AllowAll,
	}

	squidConf, err := renderTemplate("squid.conf.tmpl", squidConfTmplData)

	if err != nil {
		return RenderedConfig{}, fmt.Errorf("render squid.conf template: %w", err)
	}

	renderedConfig.SquidConf = squidConf

	renderedConfig.SquidIpAcl = strings.Join(setupData.Config.Network.AllowIps, "\n")
	renderedConfig.SquidDomainAcl = strings.Join(setupData.Config.Network.AllowDomains, "\n")

	dockerfile, err := templateFs.ReadFile("templates/Dockerfile.tmpl")

	if err != nil {
		return RenderedConfig{}, fmt.Errorf("read Dockerfile from embed fs: %w", err)
	}

	renderedConfig.Dockerfile = string(dockerfile)

	dockerComposeTmplData := DockerComposeTmplData{
		SboxTag:           paths.SboxTag,
		Volumes:           setupData.Config.Volumes,
		Privileged:        setupData.Config.Privileged,
		ExtraHosts:        setupData.Config.Network.ExtraHosts,
		SquidLogDirectory: paths.SquidLogDir(),
	}

	dockerCompose, err := renderTemplate("docker-compose.yaml.tmpl", dockerComposeTmplData)

	if err != nil {
		return RenderedConfig{}, fmt.Errorf("render docker-compose.yaml template: %w", err)
	}

	renderedConfig.DockerCompose = dockerCompose

	devcontainerManifest, err := marshalJSON(DevcontainerManifest{
		Name:              "sbox",
		DockerComposeFile: "docker-compose.yaml",
		Service:           "sandbox",
		RemoteUser:        "${localEnv:USER}",
		RemoteEnv:         orEmptyMap(setupData.Config.RemoteEnv),
		Features: map[string]any{
			"./features/sbox-feature": struct{}{},
		},
	})

	if err != nil {
		return RenderedConfig{}, fmt.Errorf("marshal devcontainer.json: %w", err)
	}

	renderedConfig.DevcontainerManifest = devcontainerManifest

	sboxFeatureManifest, err := marshalJSON(SboxFeatureManifest{
		Id:        "sbox-feature",
		Version:   "0.0.1",
		Name:      "sbox-feature",
		DependsOn: orEmptyMap(setupData.Config.Features),
	})

	if err != nil {
		return RenderedConfig{}, fmt.Errorf("marshal devcontainer-feature.json: %w", err)
	}

	renderedConfig.DevcontainerSboxFeatureManifest = sboxFeatureManifest
	renderedConfig.DevcontainerSboxFeatureInstallScript = setupData.InstallScriptContents

	return renderedConfig, nil
}

func renderTemplate(fileName string, data any) (string, error) {
	tmpl, err := template.ParseFS(templateFs, "templates/"+fileName)

	if err != nil {
		return "", fmt.Errorf("load template: %w", err)
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, data)

	if err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func marshalJSON(data any) (string, error) {
	marshaled, err := json.MarshalIndent(data, "", "  ")

	if err != nil {
		return "", err
	}

	return string(marshaled), nil
}

func orEmptyMap[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}

	return m
}

// write writes the eight generated artifacts under <SboxHome>/build/. The squid
// log directory is created separately by ensureSquidLogDir — it is not part of
// the rendered tree.
func write(paths SboxPaths, result RenderedConfig) error {
	devcontainerDir := paths.DevcontainerDir()
	sboxFeatureDir := filepath.Join(devcontainerDir, "features", "sbox-feature")
	squidDir := filepath.Join(devcontainerDir, "squid")

	files := []struct {
		path     string
		contents string
	}{
		{filepath.Join(devcontainerDir, "Dockerfile"), result.Dockerfile},
		{filepath.Join(devcontainerDir, "docker-compose.yaml"), result.DockerCompose},
		{filepath.Join(devcontainerDir, "devcontainer.json"), result.DevcontainerManifest},
		{filepath.Join(sboxFeatureDir, "devcontainer-feature.json"), result.DevcontainerSboxFeatureManifest},
		{filepath.Join(sboxFeatureDir, "install.sh"), result.DevcontainerSboxFeatureInstallScript},
		{filepath.Join(squidDir, "squid.conf"), result.SquidConf},
		{filepath.Join(squidDir, "acls", "dst.allowlist.txt"), result.SquidIpAcl},
		{filepath.Join(squidDir, "acls", "dstdomain.allowlist.txt"), result.SquidDomainAcl},
	}

	for _, file := range files {
		if err := writeFile(file.path, file.contents); err != nil {
			return err
		}
	}

	return nil
}

func writeFile(path string, contents string) error {
	err := os.MkdirAll(filepath.Dir(path), 0o755)

	if err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}

	err = os.WriteFile(path, []byte(contents), 0o644)

	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// ensureSquidLogDir creates the squid log directory with mode 0777. Squid runs
// as uid 13 and writes egress.log here, falling into "others" relative to the
// host-owned mount, so only write-for-others lets it start. MkdirAll applies
// umask — under the default 022 it yields 0755 — so the mode must be forced
// afterwards with an explicit Chmod, or the directory lands 0755 and squid dies
// with "Cannot open '/var/log/squid/egress.log' for writing".
func ensureSquidLogDir(paths SboxPaths) error {
	dir := paths.SquidLogDir()

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}

	return os.Chmod(dir, 0o777)
}

func isModeFlag(mode string) bool {
	for _, flag := range modeFlags {
		if flag == mode {
			return true
		}
	}

	return false
}

// stdioMode selects how one of a child's standard streams is wired up.
type stdioMode int

const (
	// stdioInherit leaves the stream attached to the parent process.
	stdioInherit stdioMode = iota
	// stdioSuppress redirects the stream and discards it.
	stdioSuppress
	// stdioCapture redirects the stream and returns it.
	stdioCapture
)

// processResult is a child's exit code plus whatever stdioCapture streams
// produced. Streams that were inherited or suppressed come back empty.
type processResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// runProcess is the single place a child process is started: argv, paths and
// the environment overlay are all built elsewhere. The returned error is
// non-nil only when the child could not be started; a child that runs and exits
// non-zero is a processResult, not an error.
func runProcess(stdin, stdout, stderr stdioMode, name string, args []string, env map[string]string) (processResult, error) {
	cmd := exec.Command(name, args...)

	// Cmd.Env must be set explicitly from the inherited environment plus the
	// overlay: leaving it nil would also inherit, but then the overlay could not
	// be applied. The devcontainer CLI resolves ${localEnv:...} from its own
	// environment, and compose interpolates ${USER} and ${HOME} from the one it
	// is handed.
	cmd.Env = overlayEnvironment(os.Environ(), env)

	switch stdin {
	case stdioInherit:
		cmd.Stdin = os.Stdin
	default:
		// Redirected and closed: the child reads EOF immediately.
		cmd.Stdin = nil
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.Stdout = streamWriter(stdout, os.Stdout, &stdoutBuf)
	cmd.Stderr = streamWriter(stderr, os.Stderr, &stderrBuf)

	// Ctrl-C reaches the whole foreground process group, so the child sees it
	// too. Draining the signal here — rather than signal.Ignore — leaves the
	// child with SIG_DFL, the disposition C# gives it: an ignored disposition
	// would survive exec and kill Ctrl-C inside the sandbox. It also keeps sbox
	// alive to mirror the child's exit code.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	drained := make(chan struct{})
	defer close(drained)

	go func() {
		for {
			select {
			case <-interrupts:
				// Swallow it; the child decides what Ctrl-C means.
			case <-drained:
				return
			}
		}
	}()

	// Assigning the buffers makes os/exec drain both pipes concurrently with
	// Wait, so a chatty child cannot fill one and block forever.
	err := cmd.Run()

	result := processResult{}

	if stdout == stdioCapture {
		result.Stdout = stdoutBuf.String()
	}

	if stderr == stdioCapture {
		result.Stderr = stderrBuf.String()
	}

	if err != nil {
		var exitErr *exec.ExitError

		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()

			// Go reports -1 when the child was killed by a signal, which
			// os.Exit would truncate to 255; the C# original (via .NET's
			// Process.ExitCode) reports 128+signal instead. Mirror that so a
			// SIGINT'd child yields 130. syscall.WaitStatus is the wait(2)
			// status struct, available on the macOS and Linux targets this
			// tool ships for; a failed cast leaves the -1 untouched rather
			// than panicking.
			if result.ExitCode < 0 {
				if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					result.ExitCode = 128 + int(ws.Signal())
				}
			}

			return result, nil
		}

		return processResult{}, err
	}

	return result, nil
}

// streamWriter wires one child stream. Inherit passes the parent's matching
// file through; Capture collects into buf; Suppress returns a nil Writer, which
// os/exec connects to the null device — leaving it nil must not be mistaken for
// inheriting.
func streamWriter(mode stdioMode, inherit *os.File, buf *bytes.Buffer) io.Writer {
	switch mode {
	case stdioInherit:
		return inherit
	case stdioCapture:
		return buf
	default:
		return nil
	}
}

// overlayEnvironment seeds a child's environment from the inherited environment
// plus the overlay, with the overlay winning over any inherited entry of the
// same name.
func overlayEnvironment(inherited []string, overlay map[string]string) []string {
	if len(overlay) == 0 {
		return inherited
	}

	merged := make([]string, 0, len(inherited)+len(overlay))

	for _, entry := range inherited {
		name, _, _ := strings.Cut(entry, "=")

		if _, overridden := overlay[name]; overridden {
			continue
		}

		merged = append(merged, entry)
	}

	for name, value := range overlay {
		merged = append(merged, name+"="+value)
	}

	return merged
}

// onPath reports whether executable names a runnable file in a PATH directory.
// It uses exec.LookPath, so the executable bit is required and a directory
// named devcontainer is rejected — a deliberate divergence from C#'s bare
// existence check, which would let both through and fail later at exec with a
// confusing error (plan §3a.12).
func onPath(executable string) bool {
	_, err := exec.LookPath(executable)

	return err == nil
}

// devcontainerUpArgs is the argv for devcontainer up. rebuild adds the two
// flags that recreate the container and bypass the image cache.
func devcontainerUpArgs(paths SboxPaths, rebuild bool) []string {
	args := []string{
		"up",
		"--workspace-folder", paths.BuildDir(),
		"--config", paths.DevcontainerConfigPath(),
	}

	if rebuild {
		args = append(args, "--remove-existing-container", "--build-no-cache")
	}

	return args
}

// up runs devcontainer up with stdin inherited, its progress output suppressed
// and its errors inherited. It returns the child's exit code, or a non-nil error
// when the child could not be started at all — a distinct path the caller
// reports rather than the "up failed" message. The error return is a deliberate
// departure from the plan's `int` signature: Go reports a signal-killed child as
// exit code -1, so a sentinel inside the int could not be told apart from it.
func up(paths SboxPaths, rebuild bool) (int, error) {
	result, err := runProcess(
		stdioInherit, stdioSuppress, stdioInherit,
		"devcontainer",
		devcontainerUpArgs(paths, rebuild),
		paths.ChildEnvironment(),
	)

	if err != nil {
		return 0, err
	}

	return result.ExitCode, nil
}

// devcontainerExecCommandArgs is the argv that runs command at hostCwd in a
// login shell.
func devcontainerExecCommandArgs(paths SboxPaths, hostCwd string, command []string) []string {
	return devcontainerExecArgs(paths, "-lc", commandPayload(paths, hostCwd, command))
}

// devcontainerExecShellArgs is the argv that opens an interactive shell at
// hostCwd.
func devcontainerExecShellArgs(paths SboxPaths, hostCwd string) []string {
	return devcontainerExecArgs(paths, "-c", shellPayload(paths, hostCwd))
}

func devcontainerExecArgs(paths SboxPaths, bashFlag string, payload string) []string {
	return []string{
		"exec",
		"--workspace-folder", paths.BuildDir(),
		"--config", paths.DevcontainerConfigPath(),
		"bash", bashFlag, payload,
	}
}

// commandPayload is the command to run once the sandbox is entered: the cd guard
// followed by the caller's command.
//
// A lone argument is passed through untouched, so shell syntax the caller quoted
// into it — pipes, redirects, an sh -c string — is still parsed on the far side.
// Two or more arguments are each quoted before joining, so an argument
// containing spaces stays one argument instead of being split by the shell.
func commandPayload(paths SboxPaths, hostCwd string, command []string) string {
	return cdGuard(paths, hostCwd) + "; " + joinCommand(command)
}

func joinCommand(command []string) string {
	if len(command) == 1 {
		return command[0]
	}

	quoted := make([]string, len(command))

	for i, argument := range command {
		quoted[i] = shellQuote(argument)
	}

	return strings.Join(quoted, " ")
}

func shellPayload(paths SboxPaths, hostCwd string) string {
	return cdGuard(paths, hostCwd) + "; exec bash"
}

// cdGuard changes to the host path the caller is standing in, or reports why
// nothing ran. The mount model binds repos at their real host paths, so a
// directory that is not mounted has no counterpart inside the sandbox and the
// command must not silently run somewhere else.
func cdGuard(paths SboxPaths, hostCwd string) string {
	quotedCwd := shellQuote(hostCwd)

	notMounted := "echo 'sbox: not mounted: '" + quotedCwd + "' — nothing ran on the host' >&2; " +
		"echo 'sbox: mounted under $HOME/Workspace:' >&2; " +
		"ls -1 $HOME/Workspace 2>/dev/null >&2; " +
		"echo 'sbox: add a volume to '" + shellQuote(paths.SboxHome) + "'/config.json, then re-run' >&2; " +
		"exit 1"

	return "cd -- " + quotedCwd + " || { " + notMounted + "; }"
}

// safeCharacters are the characters a shell reads literally, so a value made
// only of them can be passed through unquoted.
const safeCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._/:=@%+^,-"

// shellQuote quotes for safe consumption by a shell. Unnecessary quotes are
// dropped so ordinary paths pass through unchanged; everything else is
// single-quoted, which survives spaces, quotes, $, backticks and newlines alike.
func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !strings.ContainsRune(safeCharacters, r)
	}) == -1 {
		return value
	}

	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// execDevcontainer runs devcontainer exec with all three streams inherited,
// making the session fully interactive, and mirrors the child's exit code. The
// host cwd is read once and threaded into the payload; if it cannot be read
// there is no sensible fallback — the guard's whole purpose is to run where the
// caller stood — so the error is reported and the command exits 1.
func execDevcontainer(paths SboxPaths, args []string) int {
	hostCwd, err := os.Getwd()

	if err != nil {
		return fail(err.Error())
	}

	arguments := devcontainerExecShellArgs(paths, hostCwd)

	if len(args) > 0 {
		arguments = devcontainerExecCommandArgs(paths, hostCwd, args)
	}

	result, err := runProcess(
		stdioInherit, stdioInherit, stdioInherit,
		"devcontainer",
		arguments,
		paths.ChildEnvironment(),
	)

	if err != nil {
		return fail(err.Error())
	}

	return result.ExitCode
}

// composeExecArgs is the argv for entering the squid service with docker compose
// exec. Naming the project and file keeps it independent of the current
// directory. The trailing command is what runs inside the service.
func composeExecArgs(paths SboxPaths, command ...string) []string {
	args := []string{
		"compose",
		"--project-name", paths.ComposeProjectName(),
		"--file", paths.ComposeFilePath(),
		"exec", "-T", "squid",
	}

	return append(args, command...)
}

// squidReconfigureArgs reloads squid's config in place, re-reading the
// bind-mounted ACL allowlists, without a devcontainer up round-trip.
func squidReconfigureArgs(paths SboxPaths) []string {
	return composeExecArgs(paths, "squid", "-k", "reconfigure")
}

// followEgressLogArgs follows the host's copy of the egress log. The directory
// is bind-mounted into the squid service, so this is the same file, read without
// entering the container.
func followEgressLogArgs(paths SboxPaths) []string {
	return []string{"-n", "500", "-f", paths.EgressLogPath()}
}

// reconfigureSquid runs docker compose exec with both output streams captured
// and discarded: nothing the child prints reaches the terminal on success. A
// non-zero exit becomes sbox's own message rather than the raw child output; a
// child that cannot be started reports its exec error instead.
func reconfigureSquid(paths SboxPaths) int {
	result, err := runProcess(
		stdioInherit, stdioCapture, stdioCapture,
		"docker",
		squidReconfigureArgs(paths),
		paths.ChildEnvironment(),
	)

	if err != nil {
		return fail(err.Error())
	}

	if result.ExitCode != 0 {
		return fail("sbox: 'squid -k reconfigure' failed (is the sandbox up?)")
	}

	return 0
}

// followEgressLog follows the egress log with all three streams inherited, so it
// is fully interactive, and mirrors tail's exit code. A child that cannot be
// started reports its exec error and exits 1.
func followEgressLog(paths SboxPaths) int {
	result, err := runProcess(
		stdioInherit, stdioInherit, stdioInherit,
		"tail",
		followEgressLogArgs(paths),
		paths.ChildEnvironment(),
	)

	if err != nil {
		return fail(err.Error())
	}

	return result.ExitCode
}

func printUsage() {
	fmt.Println(`sbox: shared agent sandbox shell

  sbox                       interactive shell in the sandbox
  sbox <command>...          run a command in the sandbox
  sbox --build               recreate the container and rebuild the image, then exit
  sbox --render              render the build dir, no Docker
  sbox --squid-reconfigure   reload squid's ACLs
  sbox --squid-logs          follow the egress log from the host
  sbox --help                this message`)
}

func debug(format string, args ...any) {
	if os.Getenv(debugEnvironmentVariable) == "true" {
		fmt.Printf(format+"\n", args...)
	}
}

func fail(message string) int {
	fmt.Fprintln(os.Stderr, message)
	return 1
}
