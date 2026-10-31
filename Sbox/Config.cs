namespace Sbox;

public class Config
{
    public List<string> Volumes { get; set; } = [];

    public bool Privileged { get; set; }

    public NetworkConfig Network { get; set; } = new();

    public Dictionary<string, string> RemoteEnv { get; set; } = new();

    public Dictionary<string, object> Features { get; set; } = new();
}