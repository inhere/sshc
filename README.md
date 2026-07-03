# sshc

`sshc` is a small SSH command runner.

## Usage

Add or update a host:

```bash
sshc add --ip 192.168.1.10 -u root -p password
sshc add --ip 192.168.1.10 --name dev -u root -p password --port 22
```

Run a remote command by IP or saved name:

```bash
sshc run 192.168.1.10 -- uptime
sshc run dev -- docker ps
```

List saved hosts:

```bash
sshc list
```

## Config

The default config file is:

```text
~/.config/sshc/hosts.json
```

Set `SSHC_CONFIG` to use a different file:

```bash
SSHC_CONFIG=/path/to/hosts.json sshc add --ip 192.168.1.10 -u root -p password
```

The config file stores passwords in plain text. Keep the file private.
