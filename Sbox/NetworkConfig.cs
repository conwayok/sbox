namespace Sbox;

public class NetworkConfig
{
    public bool AllowAll { get; set; }
    public List<string> AllowDomains { get; set; } = [];
    public List<string> AllowIps { get; set; } = [];
    public List<string> ExtraHosts { get; set; } = [];
}