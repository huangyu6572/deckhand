---
name: deckhand
description: Runs remote Linux SSH commands, SFTP copies, recipe deploys, and local serial I/O via Deckhand hub.exe on Windows. Use when the user asks to run a remote command, upload or download files, talk to a COM port, deploy an artifact, wait on hub jobs, or mentions hub, hubd, or Deckhand.
---

# Deckhand

Windows-only. `hub.exe` and `hubd.exe` must be in the same directory (usually `%LOCALAPPDATA%\Programs\Deckhand\`). Put flags after the verb. Prefer `--json`.

Start with `hub target list --json`. Use a listed name. Do not invent aliases.

## Resolve a target

In order: `connections.yaml` name, then exact `~/.ssh/config` `Host`, then `user@host` / `COM3`.

- Intranet: `hub run --json user@192.168.1.20 -- uname -a`
- Named: `hub run --json dev-web -- uname -a`
- Public IP: add `--allow-public`, or set `%LOCALAPPDATA%\LocalAIHub\settings.yaml` `network.scope: all` and restart hubd
- ssh_config: exact `Host` only (no wildcards, no `Match`). Same name as yaml: yaml wins.

## Default commands

| Task | Command |
|------|---------|
| Run | `hub run --json <target> -- <cmd>` |
| Upload | `hub cp --json <local> <target>:/remote/path` |
| Download | `hub cp --json <target>:/remote/path <local>` |
| Serial | `hub serial exec --json COM3 "version" --wait ">"` |
| Deploy | `hub deploy --json <target> --recipe <name> --artifact <file>` |
| Wait | `hub job wait --json <job_id>` |
| Follow | `hub job follow --jsonl <job_id>` |

Rules:

- `run` requires `--` before the remote command, or `--script-file <local>` plus `--shell bash` (Linux) / `--shell powershell` (Windows remote).
- Do not put `&&`, `$?`, `2>&1`, or `bash -c "..."` in PowerShell argv. If hub reports the command was rewritten, write a local `.sh`/`.ps1` and pass `--script-file`.
- Never pass `--password` (exit 2). Passwords: yaml `auth.type: password` then `hub secret set <name>`.
- Commands that may contain secrets: add `--sensitive`.
- Long jobs: `--detach`, then `hub job wait --json <id>`.
- Local Windows paths use `C:\...`. Remote paths are `<alias>:/unix/path`.
- Do not default to `hub session` / `hub shell` unless the user asks.

## Config

Data dir: `%LOCALAPPDATA%\LocalAIHub\` (not the install dir).

`connections.yaml` and `settings.yaml` load **once when hubd starts**. After editing:

```powershell
Stop-Process -Name hubd -Force
```

The next `hub` command starts a new hubd and drops pooled SSH.

- `connections.yaml` — named SSH/COM targets. Never put password or private-key text in yaml; `key_path` only.
- `settings.yaml` — `network.scope`: `intranet` (default) or `all`
- `recipes\<name>.yaml` — `--recipe` is the filename without `.yaml`
- `ssh\known_hosts` — Hub's file, not OpenSSH's

## Errors

| Code | Fix |
|------|-----|
| `SCOPE_NOT_INTRANET` | `--allow-public`, or `network.scope: all` + restart hubd |
| `AUTH_FAILED` | ssh-agent, `key_path`, or `hub secret set` |
| `HOST_KEY_CHANGED` | inspect `%LOCALAPPDATA%\LocalAIHub\ssh\known_hosts` |
| `TARGET_NOT_FOUND` | spelling; restart hubd after adding yaml |
| `INVALID_ARGUMENT` | flags after the verb; `run` needs `--` |
| `DAEMON_INSTANCE_CONFLICT` | `hub.exe` and `hubd.exe` in the same folder |
| `CAPABILITY_UNSUPPORTED` | do not `hub run` a COM port; use `serial exec` |
