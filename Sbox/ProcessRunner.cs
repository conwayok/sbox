using System.Diagnostics;

namespace Sbox;

public enum StdioMode
{
    /// <summary>Leave the stream attached to the parent's.</summary>
    Inherit,

    /// <summary>Redirect the stream and discard its contents.</summary>
    Suppress,

    /// <summary>Redirect the stream and return its contents.</summary>
    Capture
}

/// <summary>
/// A child's exit code, plus whatever <see cref="StdioMode.Capture"/> streams produced. Streams
/// that were inherited or suppressed come back empty.
/// </summary>
public readonly record struct ProcessResult(int ExitCode, string Stdout, string Stderr);

/// <summary>
/// The one place a child process is started. Everything a mode needs to run — argv, paths, the
/// environment overlay — is built elsewhere, so this stays a thin shell-out.
/// </summary>
public static class ProcessRunner
{
    public static async Task<ProcessResult> RunProcessAsync(
        StdioMode stdin,
        StdioMode stdout,
        StdioMode stderr,
        string fileName,
        IReadOnlyList<string> arguments,
        IReadOnlyDictionary<string, string> environment)
    {
        var startInfo = new ProcessStartInfo
        {
            FileName = fileName,
            UseShellExecute = false,
            RedirectStandardInput = stdin != StdioMode.Inherit,
            RedirectStandardOutput = stdout != StdioMode.Inherit,
            RedirectStandardError = stderr != StdioMode.Inherit
        };

        foreach (var argument in arguments)
        {
            startInfo.ArgumentList.Add(argument);
        }

        // UseShellExecute=false seeds this from the current process, so the full host environment
        // is inherited and these entries only overlay it.
        foreach (var (name, value) in environment)
        {
            startInfo.Environment[name] = value;
        }

        using var process = new Process { StartInfo = startInfo };

        // Ctrl-C reaches the whole foreground process group, so the child sees it too. Cancelling
        // this process's own termination leaves the child to decide what it means — an
        // interactive shell survives it, `tail -f` does not — and keeps us alive to mirror the
        // child's exit code, which is as close as a .NET process gets to exec.
        ConsoleCancelEventHandler cancelKeyPress = (_, eventArgs) => eventArgs.Cancel = true;
        Console.CancelKeyPress += cancelKeyPress;

        try
        {
            process.Start();

            if (stdin != StdioMode.Inherit)
            {
                process.StandardInput.Close();
            }

            // Started before waiting so a chatty child cannot fill a pipe and block.
            var stdoutTask = stdout == StdioMode.Inherit ? null : process.StandardOutput.ReadToEndAsync();
            var stderrTask = stderr == StdioMode.Inherit ? null : process.StandardError.ReadToEndAsync();

            await process.WaitForExitAsync();

            var capturedStdout = stdoutTask is null ? "" : await stdoutTask;
            var capturedStderr = stderrTask is null ? "" : await stderrTask;

            return new ProcessResult(
                process.ExitCode,
                stdout == StdioMode.Capture ? capturedStdout : "",
                stderr == StdioMode.Capture ? capturedStderr : "");
        }
        finally
        {
            Console.CancelKeyPress -= cancelKeyPress;
        }
    }
}
