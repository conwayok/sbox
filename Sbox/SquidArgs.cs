namespace Sbox;

/// <summary>
/// The argv for entering the squid container and for reading its log from the host. Pure string
/// work — no process, no I/O.
/// </summary>
public static class SquidArgs
{
    /// <summary>
    /// Reloads squid's config in place, re-reading the bind-mounted ACL allowlists. Naming the
    /// project and file keeps this independent of the current directory and avoids a
    /// <c>devcontainer up</c> round-trip.
    /// </summary>
    public static IReadOnlyList<string> Reconfigure(SboxPaths paths) =>
        ComposeExec(paths, "squid", "-k", "reconfigure");

    /// <summary>
    /// Follows the host's copy of the egress log. The directory is bind-mounted into the squid
    /// service, so this is the same file, without entering the container.
    /// </summary>
    public static IReadOnlyList<string> FollowEgressLog(SboxPaths paths) =>
        ["-n", "500", "-f", paths.EgressLogPath];

    public static IReadOnlyList<string> ComposeExec(SboxPaths paths, params string[] command) =>
    [
        "compose",
        "--project-name", paths.ComposeProjectName,
        "--file", paths.ComposeFilePath,
        "exec", "-T", "squid",
        .. command
    ];
}
