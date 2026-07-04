# sshc

[简体中文](README.zh-CN.md)

`sshc` is a small SSH helper CLI for managing hosts, running remote commands,
transferring files, executing local scripts on remote hosts, and keeping
per-host execution logs.

It is intended for lightweight deployment, troubleshooting, and day-to-day
remote operations where a full automation platform would be too heavy.

## Features

- Manage SSH hosts in `~/.config/sshc/sshc.config.json`
- Manage shared credential profiles with `auth`
- Inspect and edit local settings with `cfg`
- Read simple host entries from `~/.ssh/config`
- Encrypt saved passwords before writing `sshc.config.json`
- Verify SSH host keys with `known_hosts` by default
- Run remote commands by saved host name, IP, or unique partial match
- Execute local shell scripts on remote hosts
- Set remote working directory, timeout, environment variables, sudo, and sudo user
- Upload and download files or directories over SFTP
- Verify single-file transfers with SHA256
- Keep per-host JSONL run logs under `~/.config/sshc/logs/`
- Open an interactive remote PTY with `login/connect`

## Installation

### Download a release

1. **Recommended** Install by [eget](https://github.com/inherelab/eget): `eget install sshc`
2. Install by Golang: `go install github.com/inhere/sshc/cmd/sshc@latest`
3. Download the archive for your platform from GitHub Releases, extract it, and put the `sshc` binary on your `PATH`.

### Build from source

```bash
git clone https://github.com/inhere/sshc.git
cd sshc
go build -o sshc ./cmd/sshc
```

For local development on Windows, you can build into `tmp`:

```powershell
go build -o tmp\sshc.exe ./cmd/sshc
```

## Quick Start

```bash
sshc add --ip 192.168.1.10 --name devhost -u root -p password
sshc list
sshc run devhost -- uptime
sshc auth add dev-root -u root -p
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc run devhost --script ./deploy.sh
sshc scp -l ./dist -r /opt/app/dist devhost
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
sshc log devhost --tail 20
```

## Usage

```text
sshc add       Add or update a host
sshc list      List saved hosts
sshc cfg       Manage config
sshc auth      Manage credential profiles
sshc host      Manage hosts
sshc run       Run a remote command
sshc login     Open an interactive SSH shell
sshc scp       Upload files or directories
sshc download  Download files or directories
sshc log       Show or search run logs
```

Run command-specific help for full options:

```bash
sshc <command> --help
```

Aliases:

```text
list      ls
run       exec
login     connect
scp       upload
download  dl
cfg       config
auth      cred, creds
host      hosts, h
```

## Examples

### Add Hosts

```bash
sshc add --ip 192.168.1.10 -u root -p password
sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 22
sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
sshc add --ip 192.168.1.10 --name devhost --auth dev-root
sshc add -I
sshc add --from-clipboard
```

`sshc add -I` prompts for host fields interactively and hides password input.

`--from-clipboard` accepts either `key=value` lines or one CSV line:

```text
ip=192.168.1.10
user=root
password=password
name=devhost
port=22
```

```text
192.168.1.10,root,password,devhost,22
```

### Credential Profiles

Use `auth` profiles when multiple hosts share the same user, password, or key:

```bash
sshc auth add dev-root -u root -p
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
```

### Manage Hosts

```bash
sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
sshc host list --group testing --show-ip
sshc host list --match devhost
sshc host show devhost
sshc host rm devhost --yes
sshc host rename old-name new-name
```

Top-level `add`, `list`, and `ls` remain available for quick daily use.

### List Hosts

```bash
sshc list
sshc ls
sshc list --show-ip
```

`sshc list` shows the host name, group, address, authentication type, and remark.
IPv4 addresses are masked by default, for example `10.*.*.8`. Use `--show-ip`
when you need the full address.
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
```

Every `run` writes one JSON log line to `~/.config/sshc/logs/<host>.log` by
default.
Interactive `login` sessions only record connection metadata.

### Interactive Login

```bash
sshc login devhost
sshc connect devhost
sshc login --term xterm-256color devhost
```

`login` and `connect` open an interactive remote PTY. The terminal type defaults
to the local `TERM` value and falls back to `xterm-256color`.

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

Default config:

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
      "password_enc": "v1:..."
    }
  ],
  "hosts": [
    {
      "name": "devhost",
      "ip": "192.168.1.10",
      "auth_ref": "dev-root",
      "group": "testing"
    }
  ]
}
```

`logs_path` can be absolute, start with `~`, or be relative to
`~/.config/sshc`.

Default run logs:

```text
~/.config/sshc/logs/<host>.log
```

Use another config file:

```bash
SSHC_CONFIG=/path/to/sshc.config.json sshc list
```

Saved hosts override entries loaded from `~/.ssh/config` when the name or IP is
the same.
For compatibility, `~/.config/sshc/hosts.json` is still read when the new default
config file does not exist.

Config helpers:

```bash
sshc cfg path
sshc cfg show
sshc cfg show --raw
sshc cfg get logs_path
sshc cfg set logs_path ./runtime/logs
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
- If a host is not trusted yet, connect once with `ssh devhost` or add its key
  to `known_hosts` before using `sshc`.
- Set `host_key_check` to `insecure` only when you explicitly want to skip host key verification.
- With `--script --sudo-user`, the uploaded temporary script is readable by local
  remote users so the target sudo user can execute it. For sensitive scripts,
  use `--remote-script-dir` with a restricted remote directory.
- `login` opens an interactive PTY. Session logs record connection metadata only,
  not typed commands or terminal output.

## Documentation

- [Deployment examples](docs/deploy-examples.md)
- [Password encryption design](docs/password-encryption-design.md)

## Development

```bash
go test ./...
go build -o tmp/sshc ./cmd/sshc
```

On Windows:

```powershell
go test ./...
go build -o tmp\sshc.exe ./cmd/sshc
```

Release builds are driven by tags:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT
