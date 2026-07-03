# sshc

`sshc` is a small SSH command runner.

## Usage

Add or update a host:

```bash
sshc add --ip 192.168.1.10 -u root -p password
sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 22
sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
sshc add -I
sshc add --from-clipboard
```

Run a remote command by IP or saved name:

```bash
sshc run 192.168.1.10 -- uptime
sshc run devhost -- docker ps
sshc run devhost --cwd /opt/app -- python -m app
sshc run devhost --script ./deploy.sh
sshc run devhost --script ./deploy.sh --remote-script-dir /opt/app/tmp
sshc run devhost --sudo -- apt-get update
sshc run devhost --sudo-user app --cwd /opt/app -- whoami
sshc run devhost --timeout 30s --kill-after 5s -e APP_ENV=prod -e DEBUG=1 -- printenv APP_ENV
sshc run devhost --efile ./remote.env -- env
```

Upload a file or directory:

```bash
sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost --sha256
sshc scp -l ./local-dir -r /tmp/remote-dir devhost
sshc scp -l ./dist -r /opt/app/dist devhost --remove-dir
sshc scp -l "./dist/*.jar" -r /opt/app/lib devhost
sshc scp -l ./a.jar -l ./b.jar -r /opt/app/lib/ devhost
sshc scp --map ./config/app.yml=/etc/app/app.yml --map ./scripts/deploy.sh=/opt/app/deploy.sh devhost
```

Download a file or directory:

```bash
sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost
sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost --sha256
sshc download -r /var/log/my-app/app.log -l tmp/logs/ devhost --sha256
sshc dl -r /tmp/remote-dir -l ./local-dir devhost
```

List saved hosts:

```bash
sshc list
```

Show run logs:

```bash
sshc log
sshc log devhost
sshc log devhost --match uptime
sshc log devhost --tail 50
```

Open an interactive remote shell:

```bash
sshc login devhost
sshc connect devhost
sshc login --term xterm-256color devhost
```

`login` and `connect` open an interactive remote PTY. The terminal type defaults
to the local `TERM` value and falls back to `xterm-256color`; use `--term` when a
remote full-screen program or color handling needs a specific terminal type.
Session logs only record connection metadata, not typed commands or terminal
output.

Deployment examples:

```text
docs/deploy-examples.md
```

## Config

The default config file is:

```text
~/.config/sshc/hosts.json
```

Run logs are stored per host under:

```text
~/.config/sshc/logs/
```

Set `SSHC_CONFIG` to use a different file:

```bash
SSHC_CONFIG=/path/to/hosts.json sshc add --ip 192.168.1.10 -u root -p password
```

The config file stores passwords in plain text. Keep the file private.
