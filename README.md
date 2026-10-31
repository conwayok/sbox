# sbox

A wrapper around [devcontainers](https://github.com/devcontainers/cli) for running AI agents in a
sandbox, with limited access to the filesystem and network.

**This is not a security boundary.** Its primary purpose is to prevent accidents, not to contain actively malicious
agents.

## Usage

```sh
# open a shell in the sandbox, cd's to the current directory
sbox

# run a command inside the sandbox, e.g. `sbox claude`
sbox <command>

# build / rebuild the container, use when changing configs
sbox --build

# generate devcontainer configs, written to $SBOX_HOME/build
sbox --render

# reload squid configs without needing to rebuild the container
sbox --squid-reconfigure

# follow squid egress log
sbox --squid-logs
```

## Configuration

`$SBOX_HOME` is where sbox keeps everything it owns: `config.json`, the generated configs under
`build/`, and the squid logs. It defaults to `~/.sbox`, and the `SBOX_HOME` environment variable
overrides it.

```json5
{
  // Docker Compose's "host:container" strings. ":ro" works. Default: empty.
  "volumes": [
    "${HOME}/.claude:${HOME}/.claude",
    "${HOME}/Workspace/api:${HOME}/Workspace/api",
    "${HOME}/.docker/daemon.json:/etc/docker/daemon.json:ro"
  ],
  // Only needed for Docker-in-Docker, the feature listed at the end of this
  // config; it also removes most of the container's protection. Default: false.
  "privileged": false,
  "network": {
    // Hostnames the agent may connect to. ".example.com" matches the domain and
    // its subdomains; "example.com" matches only that host. Everything else is
    // blocked, and the attempt shows up in the egress log. Default: empty.
    "allowDomains": [
      ".anthropic.com",
      "registry.npmjs.org"
    ],
    // The same, by IP address, for servers found by address and not by name.
    // Default: empty.
    "allowIps": [],
    // Allows every host. Squid still logs. Default: false.
    "allowAll": false,
    // Name resolution for squid itself. Pair with allowDomains.
    // Default: empty.
    "extraHosts": [
      "vm01.local:192.168.1.123"
    ]
  },
  // "${localEnv:...}" reads from the environment sbox was started in, so the
  // host variable can have a different name. Default: empty.
  "remoteEnv": {
    "ANTHROPIC_AUTH_TOKEN": "${localEnv:SBOX_ANTHROPIC_AUTH_TOKEN}"
  },
  // Devcontainer feature refs, installed as dependencies of the feature sbox
  // generates. Default: empty.
  "features": {
    "ghcr.io/devcontainers/features/node:2": {},
    // Needs "privileged": true above, with the caveat noted there.
    "ghcr.io/devcontainers/features/docker-in-docker:4": {}
  }
}
```

### install.sh

Optional. `$SBOX_HOME/install.sh` is read verbatim and becomes the `install.sh` of the devcontainer
feature sbox generates, so it runs as root during the image build. It exists for the one thing the
image cannot guess: the mount points under `$HOME` that Docker Compose creates as `root:root`,
which leave the agent unable to write there. Write it in POSIX `sh`; the feature contract provides
`$_REMOTE_USER` and `$_REMOTE_USER_HOME`.

```sh
#!/bin/sh
set -eu

# Keep this list in sync with the ${HOME}/... mounts in config.json.
mkdir -p "$_REMOTE_USER_HOME/.claude"
chown "$_REMOTE_USER" "$_REMOTE_USER_HOME/.claude"
```

### Minimal config for claude code

Reference: [code.claude.com/docs/en/devcontainer](https://code.claude.com/docs/en/devcontainer).

```json5
// config.json
{
  "volumes": [
    // The project to work on, then the host's Claude Code login.
    "${HOME}/Workspace/api:${HOME}/Workspace/api",
    "${HOME}/.claude:${HOME}/.claude",
    "${HOME}/.claude.json:${HOME}/.claude.json"
  ],
  "network": {
    "allowDomains": [
      ".anthropic.com",
      ".claude.ai",
      "registry.npmjs.org"
    ]
  },
  "remoteEnv": {
    // Node ignores the proxy variables without this, and squid is the only route out.
    "NODE_USE_ENV_PROXY": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "DISABLE_AUTOUPDATER": "1"
  },
  "features": {
    "ghcr.io/anthropics/devcontainer-features/claude-code:1": {}
  }
}
```

The mounts above need an [`install.sh`](#installsh) to make them writable by the agent.

