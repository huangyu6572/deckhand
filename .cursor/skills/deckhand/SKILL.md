---
name: deckhand
description: Runs remote Linux SSH commands, SFTP copies, recipe deploys, and local serial I/O via Deckhand hub.exe on Windows. Use when the user asks to run a remote command, upload or download files, talk to a COM port, deploy an artifact, wait on hub jobs, or mentions hub, hubd, or Deckhand.
---

# Deckhand

Windows-only. `hub.exe` and `hubd.exe` must be in the same directory (usually `%LOCALAPPDATA%\Programs\Deckhand\`). Put flags after the verb. Prefer `--json`.

Start with `hub target list --json`. Use a listed name. Do not invent aliases.

## Resolve a target

In order: `connections.yaml` name, then exact `~/.ssh/config` `Host`, then `user@host` / `COM3`.

- Intranet: open a session on `user@192.168.1.20`, then `session exec`
- Named: open a session on `dev-web`, then `session exec`
- Public IP: add `--allow-public` on `session open` (and on `run`/`cp`/`deploy` if used), or set `%LOCALAPPDATA%\LocalAIHub\settings.yaml` `network.scope: all` and restart hubd
- ssh_config: exact `Host` only (no wildcards, no `Match`). Same name as yaml: yaml wins.

## Persistent session (default remote shell)

Everyday SSH commands use a persistent remote PTY held by `hubd`. Local `hub` is only the CLI. This is a remote shell, not a mounted workspace: files still go through `hub cp`.

1. Name: target plus the last segment of `--workdir`, letters/digits/hyphens only. Example: `--workdir ~/sub-2-api` on `dev-web` → `--name dev-web-sub-2-api`. No `--workdir` → `--name dev-web`. If two directories share a basename in the same chat, include another path segment.
2. Open once (same name while `open` is reused; do not open a second PTY):

```text
hub session open --json <target> --name <name>
```

3. Run commands on that name. Pass `--workdir` every time when the user named a remote directory. Do not `cd` in the command.

```text
hub session exec --json <name> --workdir ~/sub-2-api -- uname -a
```

4. Do not `session close` unless the user asks. CLI exit does not kill the PTY.
5. Do not default to `hub shell`, `session attach`, or `--no-sentinel` (no TTY here).

`session exec` has no `--script-file`. If PowerShell would rewrite the command, write a local `.sh`, `hub cp` it under `--workdir`, then `session exec` `bash` that file.

## Default commands

| Task | Command |
|------|---------|
| Open shell | `hub session open --json <target> --name <name>` |
| Run | `hub session exec --json <name> [--workdir <dir>] -- <cmd>` |
| Detach / long job | `hub run --json <target> --detach -- <cmd>` then `hub job wait` |
| Upload | `hub cp --json <local> <target>:/remote/path` |
| Download | `hub cp --json <target>:/remote/path <local>` |
| Serial | `hub serial exec --json COM3 "version" --wait ">"` |
| Deploy | `hub deploy --json <target> --recipe <name> --artifact <file>` |
| Wait | `hub job wait --json <job_id>` |
| Follow | `hub job follow --jsonl <job_id>` |

Rules:

- `session exec` and `run` require `--` before the remote command. `run` may use `--script-file <local>` plus `--shell bash` (Linux) / `--shell powershell` (Windows remote).
- Do not put `&&`, `$?`, `2>&1`, or `bash -c "..."` in PowerShell argv. If hub reports the command was rewritten, write a local `.sh`/`.ps1` and pass `--script-file` (`run` only) or upload and `bash` it (`session exec`).
- Never pass `--password` (exit 2). Passwords: yaml `auth.type: password` then `hub secret set <name>`.
- Commands that may contain secrets: add `--sensitive`.
- Long jobs that must outlive the CLI: `hub run --detach`, then `hub job wait --json <id>`. Do not use `session exec` for `--detach`.
- Local Windows paths use `C:\...`. Remote paths are `<alias>:/unix/path`.

## Safety

When the user names a remote working directory, pass `--workdir` on `session exec` / `run` / `cp` / `deploy`. Example: `--workdir ~/sub-2-api`. Hub cds there itself. Do not put `cd /home/...` in the command.

- Allowed outside that directory: `ls`, `cat`, `find` (no `-delete`), `stat`, and other read-only lookup.
- Forbidden outside that directory: `rm`, `mkdir`, `mv`, `cp`, `touch`, `chmod`, `cd ..` / `cd /elsewhere`, and `>` / `>>` redirects. Hub returns `FILE_OUTSIDE_WORKSPACE`.
- `hub cp` remote paths must stay under `--workdir`.

Never run `rm` / `rmdir` / `Remove-Item` for the user. Hub refuses them unless a person types `DELETE <target>` in a real terminal. There is no `--yes`. `--script-file` and recipes cannot confirm a delete. If you get `DESTROY_NEEDS_HUMAN`, stop and tell the user to run it themselves.

## Config

Data dir: `%LOCALAPPDATA%\LocalAIHub\` (not the install dir).

`connections.yaml` and `settings.yaml` load **once when hubd starts**. After editing:

```powershell
Stop-Process -Name hubd -Force
```

The next `hub` command starts a new hubd and drops pooled SSH (open PTYs are gone; `session open` again).

- `connections.yaml` — named SSH/COM targets. Never put password or private-key text in yaml; `key_path` only.
- `settings.yaml` — `network.scope`: `intranet` (default) or `all`
- `recipes\<name>.yaml` — `--recipe` is the filename without `.yaml`
- `ssh\known_hosts` — Hub's file, not OpenSSH's

## Errors

| Code | Fix |
|------|-----|
| `SCOPE_NOT_INTRANET` | `--allow-public`, or `network.scope: all` + restart hubd |
| `AUTH_FAILED` | Read offered-key fingerprints in the message. Fix `key_path` / ssh-agent, or `hub secret set <name>` if the host also allows a password. |
| `HOST_KEY_CHANGED` | inspect `%LOCALAPPDATA%\LocalAIHub\ssh\known_hosts` |
| `TARGET_NOT_FOUND` | spelling; restart hubd after adding yaml |
| `SESSION_NOT_FOUND` | `hubd` restarted or name is wrong; `session open` again, then retry exec |
| `SESSION_BUSY` | wait; do not open a second PTY for the same name |
| `INVALID_ARGUMENT` | flags after the verb; `run` / `session exec` need `--` |
| `DAEMON_INSTANCE_CONFLICT` | `hub.exe` and `hubd.exe` in the same folder |
| `CAPABILITY_UNSUPPORTED` | do not `hub run` / `session` a COM port; use `serial exec` |
| `DESTROY_NEEDS_HUMAN` | Do not confirm. Ask the user to run the `rm` in their own terminal. |
| `FILE_OUTSIDE_WORKSPACE` | Query-only outside `--workdir`; do not write or delete there. |
