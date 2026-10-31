namespace Sbox;

public class ConfigRenderResult
{
    public required string Dockerfile { get; init; }
    public required string DockerCompose { get; init; }
    public required string SquidConf { get; init; }
    public required string SquidDomainAllowList { get; init; }
    public required string SquidIpAllowList { get; init; }
    public required string DevContainer { get; init; }
    public required string SandboxDevcontainerFeature { get; init; }
    public required string SandboxDevcontainerFeatureInstallScript { get; init; }
}