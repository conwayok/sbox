using System.Text.Json;

namespace Sbox;

internal static class Program
{
    private const string DebugEnvironmentVariable = "SBOX_DEBUG";

    /// <summary>First tokens that name a mode, and therefore take no further arguments.</summary>
    private static readonly string[] ModeFlags =
    [
        "--build",
        "--render",
        "--squid-reconfigure",
        "--squid-logs"
    ];

    private static readonly JsonSerializerOptions ConfigDeserializeOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase
    };

    internal static async Task<int> Main(string[] args)
    {
        try
        {
            return await RunAsync(args);
        }
        catch (Exception ex) when (ex is JsonException or IOException or UnauthorizedAccessException)
        {
            // A malformed or unreadable config.json is reported, not thrown at the user.
            await Console.Error.WriteLineAsync(ex.Message);
            return 1;
        }
    }

    private static async Task<int> RunAsync(string[] args)
    {
        var mode = args.Length == 0 ? null : args[0];

        if (mode is "--help" or "-h")
        {
            PrintUsage();
            return 0;
        }

        if (mode is not null && ModeFlags.Contains(mode) && args.Length > 1)
        {
            return Fail($"sbox: '{mode}' takes no arguments");
        }

        if (!SboxPaths.TryResolve(out var paths, out var error))
        {
            return Fail(error);
        }

        if (mode == "--squid-logs")
        {
            return await FollowEgressLogAsync(paths);
        }

        await RenderAsync(paths);

        if (mode == "--render")
        {
            return 0;
        }

        if (mode == "--squid-reconfigure")
        {
            return await ReconfigureSquidAsync(paths);
        }

        if (!SboxPaths.OnPath("devcontainer"))
        {
            return Fail("sbox: 'devcontainer' not found on PATH");
        }

        if (await UpAsync(paths, mode == "--build") != 0)
        {
            return Fail("sbox: 'devcontainer up' failed (is Docker running?)");
        }

        if (mode == "--build")
        {
            return 0;
        }

        // Every remaining mode has returned by now, so all of args is the container command —
        // the first token included.
        return await ExecAsync(paths, args);
    }

    /// <summary>
    /// Regenerates the devcontainer project. Implicit for every mode that reaches here: the build
    /// directory is derived state, and a stale one fails inside the container in ways that are
    /// hard to connect back to an edited <c>config.json</c>.
    /// </summary>
    private static async Task RenderAsync(SboxPaths paths)
    {
        Debug($"rendering sbox configs to {paths.DevcontainerDir}");

        Directory.CreateDirectory(paths.DevcontainerDir);

        // Squid runs as uid 13 and must be able to write the egress log here.
        Directory.CreateDirectory(paths.SquidLogDir,
            UnixFileMode.UserRead | UnixFileMode.UserWrite | UnixFileMode.UserExecute);

        var config = await LoadConfigAsync(paths);

        var rendered = ConfigRenderer.Render(config.Config, config.InstallScriptContents, paths.SquidLogDir);

        await WriteFileAsync(Path.Combine(paths.DevcontainerDir, "Dockerfile"), rendered.Dockerfile);
        await WriteFileAsync(Path.Combine(paths.DevcontainerDir, "docker-compose.yaml"), rendered.DockerCompose);
        await WriteFileAsync(Path.Combine(paths.DevcontainerDir, "devcontainer.json"), rendered.DevContainer);

        await WriteFileAsync(
            Path.Combine(paths.DevcontainerDir, "features", "sbox-feature", "devcontainer-feature.json"),
            rendered.SandboxDevcontainerFeature);

        await WriteFileAsync(
            Path.Combine(paths.DevcontainerDir, "features", "sbox-feature", "install.sh"),
            rendered.SandboxDevcontainerFeatureInstallScript);

        await WriteFileAsync(Path.Combine(paths.DevcontainerDir, "squid", "squid.conf"), rendered.SquidConf);

        await WriteFileAsync(
            Path.Combine(paths.DevcontainerDir, "squid", "acls", "dst.allowlist.txt"),
            rendered.SquidIpAllowList);

        await WriteFileAsync(
            Path.Combine(paths.DevcontainerDir, "squid", "acls", "dstdomain.allowlist.txt"),
            rendered.SquidDomainAllowList);
    }

    private static async Task WriteFileAsync(string path, string contents)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        await File.WriteAllTextAsync(path, contents);
    }

    private static async Task<FullConfig> LoadConfigAsync(SboxPaths paths)
    {
        Config config;

        if (File.Exists(paths.ConfigPath))
        {
            var json = await File.ReadAllTextAsync(paths.ConfigPath);
            config = JsonSerializer.Deserialize<Config>(json, ConfigDeserializeOptions) ?? new Config();
        }
        else
        {
            config = new Config();
        }

        var installScript = File.Exists(paths.InstallScriptPath)
            ? await File.ReadAllTextAsync(paths.InstallScriptPath)
            : "";

        return new FullConfig
        {
            Config = config,
            InstallScriptContents = installScript
        };
    }

    private static async Task<int> UpAsync(SboxPaths paths, bool rebuild)
    {
        var result = await ProcessRunner.RunProcessAsync(
            StdioMode.Inherit,
            StdioMode.Suppress,
            StdioMode.Inherit,
            "devcontainer",
            DevcontainerArgs.Up(paths, rebuild),
            paths.ChildEnvironment);

        return result.ExitCode;
    }

    private static async Task<int> ExecAsync(SboxPaths paths, string[] command)
    {
        var hostCwd = Directory.GetCurrentDirectory();

        var arguments = command.Length == 0
            ? DevcontainerArgs.ExecShell(paths, hostCwd)
            : DevcontainerArgs.ExecCommand(paths, hostCwd, command);

        var result = await ProcessRunner.RunProcessAsync(
            StdioMode.Inherit,
            StdioMode.Inherit,
            StdioMode.Inherit,
            "devcontainer",
            arguments,
            paths.ChildEnvironment);

        return result.ExitCode;
    }

    private static async Task<int> ReconfigureSquidAsync(SboxPaths paths)
    {
        var result = await ProcessRunner.RunProcessAsync(
            StdioMode.Inherit,
            StdioMode.Capture,
            StdioMode.Capture,
            "docker",
            SquidArgs.Reconfigure(paths),
            paths.ChildEnvironment);

        return result.ExitCode == 0
            ? 0
            : Fail("sbox: 'squid -k reconfigure' failed (is the sandbox up?)");
    }

    private static async Task<int> FollowEgressLogAsync(SboxPaths paths)
    {
        var result = await ProcessRunner.RunProcessAsync(
            StdioMode.Inherit,
            StdioMode.Inherit,
            StdioMode.Inherit,
            "tail",
            SquidArgs.FollowEgressLog(paths),
            paths.ChildEnvironment);

        return result.ExitCode;
    }

    private static void Debug(string message)
    {
        if (Environment.GetEnvironmentVariable(DebugEnvironmentVariable) == "true")
        {
            Console.WriteLine(message);
        }
    }

    private static int Fail(string message)
    {
        Console.Error.WriteLine(message);
        return 1;
    }

    private static void PrintUsage()
    {
        Console.WriteLine(
            """
            sbox: shared agent sandbox shell

              sbox                       interactive shell in the sandbox
              sbox <command>...          run a command in the sandbox
              sbox --build               recreate the container and rebuild the image, then exit
              sbox --render              render the build dir, no Docker
              sbox --squid-reconfigure   reload squid's ACLs
              sbox --squid-logs          follow the egress log from the host
              sbox --help                this message
            """);
    }
}