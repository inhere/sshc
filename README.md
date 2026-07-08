# sshc

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/inhere/sshc?style=flat-square)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/inhere/sshc)](https://github.com/inhere/sshc)
[![Unit-Tests](https://github.com/inhere/sshc/actions/workflows/go.yml/badge.svg)](https://github.com/inhere/sshc)

---

English | [简体中文](./README.zh-CN.md)

`sshc` is a small SSH helper CLI for managing hosts, running remote commands,
transferring files, executing local scripts on remote hosts, and keeping per-host execution logs.

It is intended for lightweight deployment, troubleshooting, and day-to-day
remote operations where a full automation platform would be too heavy.

## Positioning

`sshc` is designed as a small SSH operations CLI for developers and AI agents.
It keeps the common remote workflow in one local tool: host and credential
profiles, audited remote commands, batch script execution, file transfer,
known-host trust helpers, and an optional local Web console.

It does not try to replace OpenSSH, Ansible, or a full infrastructure
orchestration platform. It focuses on the operational gap between ad hoc
`ssh`/`scp` commands and heavier automation systems.

## Features

- Manage SSH hosts in `~/.config/sshc/sshc.config.json`
  - Encrypt saved passwords before writing to disk
- Manage shared credential profiles with `auth`
- Inspect and edit local settings with `cfg`
- Read simple host entries from `~/.ssh/config`
- Verify SSH host keys with `known_hosts` by default
- Run remote commands by saved host name, IP, or unique partial match
  - Execute local shell scripts on remote hosts
- Connect through a single jump host with host config or `--jump`
- Register command-proxy hosts for PVE/LXC, Docker, or vhost command execution
- Run commands or scripts across multiple hosts with `batch-run/brun`
- Set remote working directory, timeout, environment variables, sudo, and sudo user
- Upload files with `upload` and download files with `download` over SFTP
  - Verify single-file transfers with SHA256
- Keep per-host JSONL run logs under `~/.config/sshc/logs/`
- Open an interactive remote PTY with `login/connect`
- Start a local Web console with `serve` for host/auth/config/log management and browser terminals

## Installation

### Download a release

1. **Recommended** Install by [eget](https://github.com/inherelab/eget): `eget install inhere/sshc`
2. Install by Golang: `go install github.com/inhere/sshc/cmd/sshc@latest`
3. Download the archive for your platform from [GitHub Releases](https://github.com/inhere/sshc/releases), extract it, and put the `sshc` binary on your `PATH`.

### Build from source

```bash
git clone https://github.com/inhere/sshc.git
cd sshc
go build -o sshc ./cmd/sshc
# Windows
go build -o sshc.exe ./cmd/sshc
```

## Quick Start

```bash
sshc add --ip 192.168.1.10 --name devhost -u root -p password
sshc list
sshc run devhost -- uptime
sshc auth add dev-root -u root -p --remark "shared root login"
sshc group set testing auth=dev-root port=22
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root # use auth refer
sshc host set devhost tags=app,testing remark="app server"
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
sshc host add --name lxc-app --backend command_proxy --via pve-host --run-template "pct exec 101 -- sh -lc {{cmd}}" --login-command "pct enter 101"
sshc run devhost --script ./deploy.sh
sshc run inner-db --jump bastion -- hostname
sshc run lxc-app -- hostname
sshc batch-run --hosts devhost,web-2 -- uptime
sshc scp -l ./dist -r /opt/app/dist devhost
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
sshc log devhost --tail 20
sshc serve
```

## Usage

```text
sshc add                Add or update a host
sshc list|ls            List saved hosts
sshc cfg|config         Manage config
sshc auth|cred          Manage credential profiles
sshc host|hosts         Manage hosts
sshc group|groups       Manage group defaults
sshc run|exec           Run a remote command
sshc batch-run|brun     Run a command or script on multiple hosts
sshc login              Open an interactive SSH shell
sshc scp|upload         Upload files or directories
sshc download|dl        Download files or directories
sshc log                Show or search run logs
sshc serve              Start local Web console
```

Run command-specific help for full options:

```bash
sshc <command> --help
```

## Examples

### Add Hosts

```bash
sshc add --ip 192.168.1.10 -u root -p password
sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 22
sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
sshc add --ip 192.168.1.10 --name devhost --auth dev-root --tags app,testing
sshc add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
sshc add -I
sshc add --from-clipboard
```

`sshc add -I` prompts for host fields interactively and hides password input.

`--from-clipboard` accepts either `key=value`/`key: value` lines or one CSV line:

```text
ip=192.168.1.10
user=root
password=password
name=devhost
port=22
tags=app,testing
```

```text
192.168.1.10,root,password,devhost,22
```

### Credential Profiles

Use `auth` profiles when multiple hosts share the same user, password, or key:

```bash
sshc auth add dev-root -u root -p --remark "shared root login"
sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519
sshc auth list
sshc auth show dev-root
sshc auth rm old-profile --yes
```

`sshc auth add -p` prompts for a hidden password. It intentionally does not
accept `-p secret` or `--password secret`.

Attach a profile to a host:

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
```

### Manage Hosts

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
sshc host add --name lxc-app --backend command_proxy --via pve-host --run-template "pct exec 101 -- sh -lc {{cmd}}" --login-command "pct enter 101"
sshc host set devhost user=root port=22 group=testing tags=app,gpu
sshc host unset devhost tags remark jump
sshc host list --group testing --show-ip
sshc host list --tag app,gpu
sshc host list --match devhost
sshc host show devhost
sshc host trust devhost
sshc host rm devhost --yes
sshc host rename old-name new-name
```

Top-level `add`, `list`, and `ls` remain available for quick daily use.

### Group Defaults

Use group defaults when many hosts share the same auth profile, jump host, port,
timeouts, or host key policy:

```bash
sshc group set testing auth=dev-root jump=bastion port=22
sshc group set testing connect_timeout=10s run_timeout=60s remote_script_dir=/tmp
sshc group list
sshc group show testing
sshc group unset testing jump port
sshc group rm testing --yes
```

Hosts inherit defaults from their `group`. Values set directly on the host still
take priority over group defaults.

### Import Hosts

```bash
sshc host import -f ips.txt --format ips --auth dev-root --group testing --tags imported,testing --yes
sshc host import -f hosts.txt --format plain --dry-run
sshc host import -f hosts.csv --format csv --overwrite --yes
sshc host import --from-clipboard --format plain --auth dev-root
```

`ips` is a simple one-target-per-line format:

```text
10.0.0.8
10.0.0.9
web.internal
```

`plain` reuses the same `key=value`/`key: value` style as `add --from-clipboard`.
Separate multiple hosts with a blank line:

```text
ip=10.0.0.8
name=devhost
auth=dev-root
group=testing
tags=app,testing

ip: 10.0.0.9
name: dbhost
user: root
password: secret
group: testing
tags: db,testing
```

CSV imports must include a header row:

```csv
name,ip,auth,group,tags,remark,port
devhost,10.0.0.8,dev-root,testing,"app,testing",app server,22
```

By default, conflicts fail without saving. Use `--skip-existing` to ignore saved
hosts or `--overwrite` to update them. `--dry-run` previews the plan. Imported
passwords are encrypted before saving and are not printed.

### List Hosts

```bash
sshc list
sshc ls
sshc list --show-ip
sshc list --tag testing
```

`sshc list` shows the host name, group, tags, address, authentication type, and remark.
IPv4 addresses are masked by default, for example `10.*.*.8`. Use `--show-ip`
when you need the full address. Use `--tag app,gpu` to list hosts that have all
specified tags.

Hosts from `~/.ssh/config` are also listed when they have `HostName`, `User`, and
`IdentityFile`.

### Run Commands

```bash
sshc run 192.168.1.10 -- uptime
sshc run devhost -- docker ps
sshc run devhost --cwd /opt/app -- python -m app
sshc run devhost --timeout 30s --kill-after 5s -- systemctl status nginx
sshc run devhost -e APP_ENV=prod -e DEBUG=1 -- printenv APP_ENV
sshc run devhost --efile ./remote.env -- env
```

Remote commands must be placed after `--`.

Environment files support comments, blank lines, plain `KEY=value`, and
`export KEY=value` lines:

```text
APP_ENV=prod
export DEBUG=1
NAME="hello world"
```

### Jump Hosts

Set `jump` on the target host when it normally needs a bastion:

```bash
sshc host add --ip 1.2.3.4 --name bastion --auth dev-root
sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
```

Equivalent config:

```json
{
  "hosts": [
    {
      "name": "bastion",
      "ip": "1.2.3.4",
      "auth_ref": "dev-root"
    },
    {
      "name": "inner-db",
      "ip": "10.0.0.8",
      "auth_ref": "dev-root",
      "jump": "bastion"
    }
  ]
}
```

Then use the target host as usual:

```bash
sshc run inner-db -- hostname
sshc login inner-db
sshc scp -l app.jar -r /tmp/app.jar inner-db
sshc download -r /var/log/app.log -l tmp/logs inner-db
```

Use `--jump` to override the configured jump host for one command:

```bash
sshc run inner-db --jump bastion -- hostname
sshc login inner-db --jump bastion
sshc scp -l app.jar -r /tmp/app.jar inner-db --jump bastion
sshc download -r /var/log/app.log -l tmp/logs inner-db --jump bastion
```

The initial implementation supports one jump host. Nested jump hosts,
`ProxyCommand`, and nested jump chains are not supported yet. Host key checking
still happens on the local machine for both the jump host and the target host.

### Command Proxy Hosts

Use `command_proxy` when the target is a logical host that cannot be reached by
SSH directly, such as a PVE LXC container, Docker container, or vhost. sshc
connects to the real SSH host in `via`, renders `run_template`, and records logs
under the logical host name.

```bash
sshc host add --ip 192.168.1.20 --name pve-host --auth dev-root
sshc host add --name lxc-app \
  --backend command_proxy \
  --via pve-host \
  --run-template "pct exec 101 -- sh -lc {{cmd}}" \
  --login-command "pct enter 101" \
  --group lxc \
  --remark "PVE CT 101"

sshc run lxc-app -- hostname
sshc run lxc-app --cwd /opt/app -e APP_ENV=prod -- ./init.sh
sshc batch-run --hosts devhost,lxc-app -- uptime
sshc login lxc-app
```

Equivalent config:

```json
{
  "name": "lxc-app",
  "backend": "command_proxy",
  "via": "pve-host",
  "run_template": "pct exec 101 -- sh -lc {{cmd}}",
  "login_command": "pct enter 101",
  "group": "lxc",
  "remark": "PVE CT 101"
}
```

`{{cmd}}` is replaced with sshc's shell-quoted final command after applying
`--cwd`, `--env`, `--sudo`, and timeout options. `login_command` is a complete
interactive command executed inside a PTY on the `via` host.

`command_proxy` is not OpenSSH `ProxyCommand`: it does not proxy a TCP stream and
the logical target does not need sshd. Initial command-proxy support covers
`run`, `batch-run`, and `login`. `scp/upload/download` return a clear unsupported
error for command-proxy hosts. `run --script` is also not supported yet because
the temporary script is uploaded to the `via` host, not into the logical target.

### Run Scripts

```bash
sshc run devhost --script ./deploy.sh
sshc run devhost --script ./deploy.sh --cwd /opt/app
sshc run devhost --script ./deploy.sh --remote-script-dir /opt/app/tmp
sshc run devhost --script ./deploy.sh --keep-remote-script
```

Use `--script` for multiline shell, here-doc, `source` or virtualenv activation,
or commands that require heavy quoting.

Script mode uploads the local file to `/tmp` by default and runs it with `bash`.
Use `--remote-script-dir` when `/tmp` has restrictive mount options, permissions,
or cleanup policies.

### Batch Run

```bash
sshc batch-run --hosts devhost,web-2 -- uptime
sshc brun --hosts devhost,web-2 -- hostname
sshc batch-run --group testing --parallel 5 --script ./deploy.sh
sshc batch-run --hosts-file hosts.txt -- hostname
sshc batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh
```

`--hosts` accepts a comma-separated list. `--hosts-file` reads one host target per
line and ignores blank lines and full-line comments. Saved hosts are resolved
first; unresolved IP or hostname targets can be used with shared auth options
such as `--auth`, `-u`, `--key`, or `-p`.

Use `--parallel` to limit concurrency. With `--fail-fast`, sshc stops starting
new hosts after the first failure and waits for already running hosts to finish.

### Sudo

```bash
sshc run devhost --sudo -- apt-get update
sshc run devhost --sudo-user app --cwd /opt/app -- whoami
sshc run devhost --script ./deploy.sh --sudo-user app --remote-script-dir /opt/app/tmp
```

`--sudo` and `--sudo-user` require passwordless sudo, or an SSH user that already
has the required privileges.

### Upload Files

```bash
sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost --sha256
sshc scp -l ./local-dir -r /tmp/remote-dir devhost
sshc scp -l ./dist -r /opt/app/dist devhost --remove-dir
sshc scp -l "./dist/*.jar" -r /opt/app/lib devhost
sshc scp -l ./a.jar -l ./b.jar -r /opt/app/lib/ devhost
sshc scp --map ./config/app.yml=/etc/app/app.yml --map ./scripts/deploy.sh=/opt/app/deploy.sh devhost
```

Use repeatable `-l/--local` when multiple local paths should go into one remote
directory. Use repeatable `--map local=remote` when each local path needs an
explicit remote destination.

### Download Files

```bash
sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost
sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost --sha256
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
sshc dl -r /tmp/remote-dir -l ./local-dir devhost
```

Existing local directories receive the remote base name. Local paths ending with
`/` or `\` are also treated as directories.

### View Logs

```bash
sshc log
sshc log devhost
sshc log devhost --match uptime
sshc log devhost --tail 50
sshc log devhost -m error --tail 50
sshc log --id 20260704-173012-a1b2c3
sshc log --id 20260704-173012-a1b2c3 --tail 80
sshc log --id 20260704-173012-a1b2c3 --lines 120,180
sshc log devhost --lines 20,80
```

Every `run` writes one JSON log line to `~/.config/sshc/logs/<host>.log` by
default. Each run has a `task_id`. Short output is kept inline in JSONL; larger
output is stored under `~/.config/sshc/logs/yyyyMMdd/<task_id>.out.log` and can
be opened with `sshc log --id <task_id>`.
Interactive `login` sessions only record connection metadata.

### Interactive Login

```bash
sshc login
sshc login devhost
sshc connect devhost
sshc login --term xterm-256color devhost
sshc login lxc-app
```

`login` and `connect` open an interactive remote PTY. The terminal type defaults
to the local `TERM` value and falls back to `xterm-256color`.
When no target is provided, or when a target matches multiple hosts, `sshc`
opens an interactive host selector.

### Web Console

```bash
sshc serve
sshc serve --addr 127.0.0.1:8822
sshc serve --addr 127.0.0.1:0 --no-open
sshc serve --readonly
sshc serve --web-dir ./web/dist
sshc serve --addr 0.0.0.0:8822 --token random
sshc serve --addr 0.0.0.0:8822 --token "change-me"
```

`sshc serve` starts a local Web console for managing hosts, credential
profiles, config summary, run logs, and browser-based SSH terminals. It listens
on `127.0.0.1:8822` by default and opens the browser automatically. Use
`--no-open` when running in a terminal-only environment.

Browser terminals use the same saved host config as `sshc login`, including
single jump hosts. Web Terminal does not prompt inside the browser for unknown
host keys. Trust the host first with:

```bash
sshc host trust devhost
```

If the browser terminal reports a host key mismatch, verify the target machine
first, then replace the stale entry with `sshc host trust -f devhost`.

`command_proxy` browser terminals are not supported in v1. CLI `sshc login
lxc-app` remains available for command-proxy hosts.

When binding to a non-loopback address such as `0.0.0.0`, `--token` is required.
Use `--token random` to generate a one-time access token printed at startup, or
pass an explicit token. The server stores only a token hash in memory and uses a
session cookie plus `X-SSHC-CSRF` for Web write requests.

Terminal audit metadata is written to:

```text
{logs_path}/terminal/{yyyyMMdd}.jsonl
```

The audit log records session lifecycle events such as start, resize, and close.
It does not record full terminal input or output.

## Host Matching

Most commands accept a saved host name or IP. `sshc` resolves exact matches
first, then a unique partial match across host name, IP, remark, and group.

For example, if a saved host has name `testing-web`, group `testing`, and remark
`gpu runner`, these can match when the result is unique:

```bash
sshc run "testing web" -- hostname
sshc run "testing gpu" -- uptime
```

If multiple hosts match, `sshc` returns the candidate list instead of guessing.

## Configuration

Default config file:

```text
~/.config/sshc/sshc.config.json
```

Example config:

```json
{
  "version": 1,
  "logs_path": "logs",
  "defaults": {
    "user": "root",
    "port": 22,
    "connect_timeout": "20s",
    "remote_script_dir": "/tmp",
    "host_key_check": "known_hosts",
    "known_hosts_path": "~/.ssh/known_hosts"
  },
  "auth_profiles": [
    {
      "name": "dev-root",
      "user": "root",
      "password_enc": "v1:...",
      "remark": "shared root login"
    }
  ],
  "hosts": [
    {
      "name": "devhost",
      "ip": "192.168.1.10",
      "auth_ref": "dev-root",
      "group": "testing"
    },
    {
      "name": "inner-db",
      "ip": "10.0.0.8",
      "auth_ref": "dev-root",
      "jump": "devhost",
      "group": "testing"
    },
    {
      "name": "lxc-app",
      "backend": "command_proxy",
      "via": "devhost",
      "run_template": "pct exec 101 -- sh -lc {{cmd}}",
      "login_command": "pct enter 101",
      "group": "lxc"
    }
  ]
}
```

`logs_path` can be absolute, start with `~`, or be relative to
`~/.config/sshc`.

Default remote run log directory:

```text
~/.config/sshc/logs/<host>.log
```

sshc also writes its own runtime log with `log/slog`:

```text
~/.config/sshc/logs/runtime/sshc.log
```

The runtime log records command start/end, exit code, duration, process
metadata, and serve lifecycle events. Sensitive command argument values such as
passwords, tokens, keys, and export keys are masked before writing.

Use another config file:

```bash
SSHC_CONFIG=/path/to/sshc.config.json sshc list
```

Use another config directory while keeping the default file names:

```bash
SSHC_CONFIG_DIR=/tmp/sshc-test sshc cfg path
SSHC_CONFIG_DIR=/tmp/sshc-test sshc list
```

Path priority:

```text
SSHC_CONFIG file path > SSHC_CONFIG_DIR directory > ~/.config/sshc
```

`SSHC_CONFIG_DIR` changes the default config root used for
`sshc.config.json`, the local password encryption key, and relative
`logs_path` values. It is useful for tests, temporary sandboxes, and running
multiple isolated sshc configurations.

Saved hosts override entries loaded from `~/.ssh/config` when the name or IP is
the same.

### Export And Import Config

Use `cfg export/import` to move a complete sshc config to another machine:

```bash
sshc cfg export -o sshc-export.enc
sshc cfg import -f sshc-export.enc --key "sshc-v1:..."
sshc cfg import -f sshc-export.enc --key "sshc-v1:..." --overwrite
sshc cfg import -f sshc-export.enc --key "sshc-v1:..." --replace
```

`cfg export` writes an encrypted export package and prints a one-time export key.
Save that key separately; it is not stored in the export file or local config.

`cfg import` backs up the current config before writing. The default `merge`
strategy rejects conflicting host names, host IPs, and auth profile names.
Use `--overwrite` to update conflicting entries, or `--replace` to replace the
current config with the imported config.

Passwords from the export package are re-encrypted with the target machine's
local `~/.config/sshc/key` when the imported config is saved. Importing plain IP
lists, CSV files, or pasted host snippets is handled by `sshc host import`, not
`sshc cfg import`.

Config helpers:

```bash
sshc cfg path
sshc cfg show
sshc cfg show --raw
sshc cfg get logs_path
sshc cfg set logs_path ./runtime/logs
sshc cfg set defaults.user root
sshc cfg set defaults.port 2222
sshc cfg set defaults.host_key_check known_hosts
sshc cfg unset logs_path
sshc cfg doctor
```

`cfg show` masks passwords and encrypted password values. `cfg show --raw`
prints the config file as stored on disk and is intended for local debugging.

## Security Notes

- Saved passwords are encrypted before being written to `sshc.config.json`.
- The local encryption key is stored at `~/.config/sshc/key`; keep both files private.
- Legacy plaintext `password` fields are still readable for compatibility.
- Prefer SSH keys over passwords when possible.
- If both password and `--key` are provided, key authentication is tried first.
- SSH host keys are checked against `~/.ssh/known_hosts` by default.
- If an interactive command sees an unknown host key, sshc asks whether to append
  it to `known_hosts` and then continues the same connection.
- Use `sshc host trust devhost` or `sshc host trust 192.168.1.10 --port 2222`
  to pre-scan and add a host key to `known_hosts`.
- In non-interactive scripts, pre-add host keys to `known_hosts` before running
  sshc, or explicitly configure `host_key_check=insecure` for trusted temporary environments.
- If a known host key changes, sshc fails by default. After verifying the host
  identity, run `sshc host trust -f devhost` to replace the stale entry.
- Set `host_key_check` to `insecure` only when you explicitly want to skip host key verification.
- With `--script --sudo-user`, the uploaded temporary script is readable by local
  remote users so the target sudo user can execute it. For sensitive scripts,
  use `--remote-script-dir` with a restricted remote directory.
- `login` opens an interactive PTY. Session logs record connection metadata only,
  not typed commands or terminal output.
- `sshc serve` binds to localhost by default. Binding to `0.0.0.0` exposes a Web
  SSH management entry point to the network and must use `--token`.
- Do not expose `sshc serve` directly to the public Internet. Put it behind a
  trusted tunnel or access-controlled network if remote access is required.
- Web APIs and UI responses mask `password` and `password_enc`; passwords are not
  displayed by the Web console.

## Documentation

- [Deployment examples](docs/deploy-examples.md)
- [Password encryption design](docs/password-encryption-design.md)

## Related Projects

- [OpenSSH](https://www.openssh.com/) remains the foundation. `sshc` builds a
  workflow around SSH instead of replacing the SSH protocol or client ecosystem.
- [assh](https://github.com/moul/assh) focuses on making SSH configuration
  smarter with aliases, gateways, templates, inheritance, and ProxyCommand
  integration. `sshc` focuses more on operational commands, logs, transfer, and
  host/auth management.
- [lazyssh](https://github.com/Adembc/lazyssh),
  [dssh](https://github.com/madLinux7/dssh), and
  [susshi](https://github.com/yatoub/susshi) are SSH connection managers with a
  stronger TUI-oriented experience. `sshc` keeps a scriptable CLI-first surface
  and adds run logs, batch execution, transfer, and a local Web console.
- [pssh](https://github.com/lilydjwg/pssh) focuses on parallel SSH execution.
  `sshc batch-run` covers lightweight batch runs while also reusing saved hosts,
  auth profiles, jump hosts, command-proxy hosts, and per-host logs.
- [hop](https://github.com/danmartuszewski/hop) and
  [purple](https://github.com/erickochen/purple) are modern SSH managers with
  broader TUI, MCP, or cloud-sync goals. `sshc` intentionally stays smaller and
  local-config driven.

## Development

```bash
go test ./...
go build -o tmp/sshc ./cmd/sshc
npm --prefix web run build
go build -tags embed_web -o tmp/sshc ./cmd/sshc
```

On Windows:

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
npm --prefix web run build
go build -tags embed_web -o tmp\sshc.exe ./cmd/sshc
```

`web/dist` is generated by the frontend build and is not committed. Release
builds that need the Web UI embedded must run `npm --prefix web run build`
before `go build -tags embed_web`.

Release builds are driven by tags:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT
