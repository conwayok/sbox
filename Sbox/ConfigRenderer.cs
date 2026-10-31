using System.Text.Json;
using Scriban;

namespace Sbox;

public static class ConfigRenderer
{
    private static readonly Template DockerfileTemplate = Template.Parse(
        """
        FROM mcr.microsoft.com/devcontainers/base:ubuntu
        ARG USERNAME
        ARG HOME_DIR
        RUN mkdir -p "$(dirname "$HOME_DIR")" && \
            useradd -m -d "$HOME_DIR" -s /bin/bash "$USERNAME"
        """
    );

    private static readonly Template SquidConfTemplate = Template.Parse(
        """
        http_port 3128
        {{ if allow_all }}
        http_access allow all
        {{ else }}
        acl allowlist_domains dstdomain "/acls/dstdomain.allowlist.txt"
        acl allowlist_ips dst "/acls/dst.allowlist.txt"
        http_access allow allowlist_domains
        http_access allow allowlist_ips
        http_access deny all
        {{ end }}
        strip_query_terms on
        cache deny all
        logfile_rotate 7

        # Fields: ISO8601 local time, Squid/HTTP status, method, URL (query stripped),
        # origin IP, User-Agent.
        logformat egress %{%Y-%m-%dT%H:%M:%S%z}tl %Ss/%>Hs %rm %ru %<a "%{User-Agent}>h"

        access_log stdio:/var/log/squid/egress.log egress
        """
    );

    private static readonly Template DockerComposeTemplate = Template.Parse(
        """
        name: sbox-${SBOX_TAG}
        services:
          sandbox:
            image: sbox-${SBOX_TAG}
            build:
              context: ..
              dockerfile: .devcontainer/Dockerfile
              args:
                USERNAME: ${USER}
                HOME_DIR: ${HOME}
            command: sleep infinity
            environment:
              HTTP_PROXY: "http://squid:3128"
              HTTPS_PROXY: "http://squid:3128"
              ALL_PROXY: "http://squid:3128"
              http_proxy: "http://squid:3128"
              https_proxy: "http://squid:3128"
              all_proxy: "http://squid:3128"
              NO_PROXY: "localhost,127.0.0.1,::1,squid"
              no_proxy: "localhost,127.0.0.1,::1,squid"
            {{ if volumes && volumes.size > 0 -}}
            volumes:
              {{- for vol in volumes }}    
              - {{ vol }}
              {{- end }}
            {{- end }}
            networks:
              - sandbox
              - sandbox_egress
            privileged: {{ privileged }}
          squid:
            image: ubuntu/squid:latest
            restart: unless-stopped
            networks:
              - sandbox_egress
              - internet
            {{ if extra_hosts && extra_hosts.size > 0 -}}
            extra_hosts:
              {{- for extra_host in extra_hosts }}    
              - {{ extra_host }}
              {{- end }}
            {{- end }}
            volumes:
              - ./squid/squid.conf:/etc/squid/squid.conf:ro
              - ./squid/acls:/acls:ro
              - {{ squid_log_directory }}:/var/log/squid
        networks:
          sandbox:
            internal: true
          sandbox_egress:
            internal: true
          internet: { }
        """
    );

    private static readonly JsonSerializerOptions JsonSerializeOptions = new()
    {
        WriteIndented = true
    };

    public static ConfigRenderResult Render(Config config, string installScript, string squidLogDirectory)
    {
        var dockerfile = DockerfileTemplate.Render();
        var dockerCompose = DockerComposeTemplate.Render(new
        {
            config.Volumes,
            Privileged = config.Privileged ? "true" : "false",
            SquidLogDirectory = squidLogDirectory,
            config.Network.ExtraHosts
        });
        var squidConf = SquidConfTemplate.Render(new
        {
            config.Network.AllowAll
        });

        var devContainerObj = new
        {
            name = "sbox",
            dockerComposeFile = "docker-compose.yaml",
            service = "sandbox",
            remoteUser = "${localEnv:USER}",
            remoteEnv = config.RemoteEnv,
            features = new Dictionary<string, object>
            {
                { "./features/sbox-feature", new { } }
            }
        };

        var sandboxDevcontainerFeatureObj = new
        {
            id = "sbox-feature",
            version = "0.0.1",
            name = "sbox-feature",
            dependsOn = config.Features
        };

        return new ConfigRenderResult
        {
            Dockerfile = dockerfile,
            DockerCompose = dockerCompose,
            SquidConf = squidConf,
            SquidDomainAllowList = string.Join("\n", config.Network.AllowDomains),
            SquidIpAllowList = string.Join("\n", config.Network.AllowIps),
            DevContainer = JsonSerializer.Serialize(devContainerObj, JsonSerializeOptions),
            SandboxDevcontainerFeature = JsonSerializer.Serialize(sandboxDevcontainerFeatureObj, JsonSerializeOptions),
            SandboxDevcontainerFeatureInstallScript = installScript
        };
    }
}