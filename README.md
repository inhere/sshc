# sshc

`sshc` is a small SSH command runner.

## Usage

Add or update a host:

```bash
sshc add --ip 192.168.1.10 -u root -p password
sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 22
```

Run a remote command by IP or saved name:

```bash
sshc run 192.168.1.10 -- uptime
sshc run devhost -- docker ps
sshc run devhost --cwd /opt/app -- python -m app
sshc run devhost --script ./deploy.sh
sshc run devhost --timeout 30s -e APP_ENV=prod -e DEBUG=1 -- printenv APP_ENV
sshc run devhost --efile ./remote.env -- env
```

Upload a file or directory:

```bash
sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
sshc scp -l ./local-dir -r /tmp/remote-dir devhost
```

Download a file or directory:

```bash
sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost
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
