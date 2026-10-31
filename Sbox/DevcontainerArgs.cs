namespace Sbox;

/// <summary>
/// The argv for <c>devcontainer up</c> / <c>devcontainer exec</c>, the bash payload that runs
/// inside the sandbox, and the quoting those two need. Pure string work — no process, no I/O.
/// </summary>
public static class DevcontainerArgs
{
    /// <summary>
    /// Characters a shell reads literally, so a path made only of them can be passed through
    /// unquoted. Anything else gets the conservative single-quote treatment in <see cref="ShellQuote"/>.
    /// </summary>
    private const string SafeCharacters =
        "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._/:=@%+^,-";

    public static IReadOnlyList<string> Up(SboxPaths paths, bool rebuild)
    {
        var args = new List<string>
        {
            "up",
            "--workspace-folder", paths.BuildDir,
            "--config", paths.DevcontainerConfigPath
        };

        if (rebuild)
        {
            args.Add("--remove-existing-container");
            args.Add("--build-no-cache");
        }

        return args;
    }

    /// <summary>Runs <paramref name="command"/> at <paramref name="hostCwd"/> in a login shell.</summary>
    public static IReadOnlyList<string> ExecCommand(
        SboxPaths paths,
        string hostCwd,
        IReadOnlyList<string> command) =>
        Exec(paths, "-lc", CommandPayload(paths, hostCwd, command));

    /// <summary>Opens an interactive shell at <paramref name="hostCwd"/>.</summary>
    public static IReadOnlyList<string> ExecShell(SboxPaths paths, string hostCwd) =>
        Exec(paths, "-c", ShellPayload(paths, hostCwd));

    /// <summary>
    /// The command to run once the sandbox is entered.
    ///
    /// A lone argument is passed through untouched, so shell syntax the caller quoted into it —
    /// pipes, redirects, an <c>sh -c</c> string — is still parsed on the far side. Two or more
    /// arguments are each quoted before joining, so an argument containing spaces stays one
    /// argument instead of being split into words by the shell.
    /// </summary>
    public static string CommandPayload(SboxPaths paths, string hostCwd, IReadOnlyList<string> command) =>
        $"{Cd(paths, hostCwd)}; {JoinCommand(command)}";

    private static string JoinCommand(IReadOnlyList<string> command) =>
        command.Count == 1
            ? command[0]
            : string.Join(" ", command.Select(ShellQuote));

    public static string ShellPayload(SboxPaths paths, string hostCwd) =>
        $"{Cd(paths, hostCwd)}; exec bash";

    /// <summary>
    /// Quotes for safe consumption by a shell. Unnecessary quotes are dropped so ordinary paths
    /// pass through unchanged; everything else is single-quoted, which survives spaces, quotes,
    /// <c>$</c>, backticks and newlines alike.
    /// </summary>
    public static string ShellQuote(string value)
    {
        if (value.Length > 0 && value.All(SafeCharacters.Contains))
        {
            return value;
        }

        return $"'{value.Replace("'", "'\\''")}'";
    }

    private static IReadOnlyList<string> Exec(SboxPaths paths, string bashFlag, string payload) =>
    [
        "exec",
        "--workspace-folder", paths.BuildDir,
        "--config", paths.DevcontainerConfigPath,
        "bash", bashFlag, payload
    ];

    /// <summary>
    /// Change to the host path the caller is standing in, or report why nothing ran. The mount
    /// model binds repos at their real host paths, so a directory that is not mounted has no
    /// counterpart inside the sandbox and the command must not silently run somewhere else.
    /// </summary>
    private static string Cd(SboxPaths paths, string hostCwd)
    {
        var quotedCwd = ShellQuote(hostCwd);

        var fail =
            $"echo 'sbox: not mounted: '{quotedCwd}' — nothing ran on the host' >&2; " +
            "echo 'sbox: mounted under $HOME/Workspace:' >&2; " +
            "ls -1 $HOME/Workspace 2>/dev/null >&2; " +
            $"echo 'sbox: add a volume to '{ShellQuote(paths.SboxHome)}'/config.json, then re-run' >&2; " +
            "exit 1";

        return $"cd -- {quotedCwd} || {{ {fail}; }}";
    }
}
