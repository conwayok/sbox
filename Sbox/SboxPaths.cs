using System.Diagnostics.CodeAnalysis;

namespace Sbox;

/// <summary>
/// The host-side values every mode needs, resolved once at startup: the user and sandbox tag
/// exported to child processes, and the sbox home directory everything else is derived from.
/// Pure computation — the only thing it touches outside itself is <c>PATH</c>.
/// </summary>
public sealed record SboxPaths
{
    public required string User { get; init; }

    public required string SboxTag { get; init; }

    public required string Home { get; init; }

    public required string SboxHome { get; init; }

    /// <summary>The devcontainer project root, and the workspace folder handed to the CLI.</summary>
    public string BuildDir => Path.Combine(SboxHome, "build");

    /// <summary>
    /// Everything the renderer emits lives here. The nesting is load-bearing: the CLI resolves a
    /// local feature ref against the config file's directory and rejects anything that is not a
    /// child of <c>&lt;workspaceFolder&gt;/.devcontainer</c>, so the artifacts cannot sit directly
    /// in <see cref="BuildDir"/>.
    /// </summary>
    public string DevcontainerDir => Path.Combine(BuildDir, ".devcontainer");

    public string ConfigPath => Path.Combine(SboxHome, "config.json");

    public string InstallScriptPath => Path.Combine(SboxHome, "install.sh");

    public string ComposeFilePath => Path.Combine(DevcontainerDir, "docker-compose.yaml");

    public string DevcontainerConfigPath => Path.Combine(DevcontainerDir, "devcontainer.json");

    public string SquidLogDir => Path.Combine(SboxHome, "logs", "squid");

    public string EgressLogPath => Path.Combine(SquidLogDir, "egress.log");

    public string ComposeProjectName => $"sbox-{SboxTag}";

    /// <summary>
    /// The environment overlaid on the inherited one for every child process. Required, not just
    /// convenient: the <c>devcontainer</c> CLI resolves <c>${localEnv:...}</c> from its own
    /// environment, and compose interpolates <c>${USER}</c>, <c>${HOME}</c> and <c>${SBOX_TAG}</c>
    /// from the one it is handed.
    /// </summary>
    public Dictionary<string, string> ChildEnvironment => new()
    {
        ["USER"] = User,
        ["HOME"] = Home,
        ["SBOX_TAG"] = SboxTag
    };

    /// <summary>
    /// Resolves from the current environment. Fails only when <c>HOME</c> is unset; an absent
    /// <c>SBOX_HOME</c> falls back to <c>$HOME/.sbox</c>, and a relative one resolves against the
    /// current directory.
    /// </summary>
    public static bool TryResolve(
        [NotNullWhen(true)] out SboxPaths? paths,
        [NotNullWhen(false)] out string? error)
    {
        paths = null;

        var home = Environment.GetEnvironmentVariable("HOME");

        if (string.IsNullOrWhiteSpace(home))
        {
            error = "sbox: HOME is not set";
            return false;
        }

        var user = Environment.GetEnvironmentVariable("USER");

        if (string.IsNullOrWhiteSpace(user))
        {
            user = Environment.UserName;
        }

        var sboxHome = Environment.GetEnvironmentVariable("SBOX_HOME");

        if (string.IsNullOrWhiteSpace(sboxHome))
        {
            sboxHome = Path.Combine(home, ".sbox");
        }

        paths = new SboxPaths
        {
            User = user,
            SboxTag = user.ToLowerInvariant(),
            Home = Path.TrimEndingDirectorySeparator(Path.GetFullPath(home)),
            SboxHome = Path.TrimEndingDirectorySeparator(Path.GetFullPath(sboxHome))
        };

        error = null;
        return true;
    }

    /// <summary>
    /// Whether <paramref name="executable"/> names a file in one of the <c>PATH</c> directories.
    /// </summary>
    public static bool OnPath(string executable)
    {
        var path = Environment.GetEnvironmentVariable("PATH");

        if (string.IsNullOrEmpty(path))
        {
            return false;
        }

        foreach (var directory in path.Split(
                     Path.PathSeparator,
                     StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries))
        {
            if (File.Exists(Path.Combine(directory, executable)))
            {
                return true;
            }
        }

        return false;
    }
}
