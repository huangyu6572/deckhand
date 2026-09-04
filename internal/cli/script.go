package cli

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"strings"
	"unicode/utf16"

	"localaihub/internal/wire"
)

const maxScriptBytes = 256 * 1024

func prepareRunCommand(args []string, scriptFile, shell string) (target, command, warn string, err error) {
	if len(args) < 1 {
		return "", "", "", wire.E("INVALID_ARGUMENT", "target is required")
	}
	target = args[0]
	var body string
	if scriptFile != "" {
		if len(args) > 1 {
			return "", "", "", wire.E("INVALID_ARGUMENT", "do not combine --script-file with -- command")
		}
		b, rerr := os.ReadFile(scriptFile)
		if rerr != nil {
			return "", "", "", wire.Ef("INVALID_ARGUMENT", "script-file: %v", rerr)
		}
		body = strings.TrimPrefix(string(b), "\ufeff")
	} else {
		if len(args) < 2 {
			return "", "", "", wire.E("INVALID_ARGUMENT", "command is required after --")
		}
		rest := args[1:]
		var extracted string
		extracted, warn = extractBashC(rest)
		if extracted != "" {
			body = extracted
			if strings.TrimSpace(shell) == "" {
				shell = "bash"
			}
		} else {
			body = strings.Join(rest, " ")
		}
	}
	if len(body) > maxScriptBytes {
		return "", "", "", wire.E("INVALID_ARGUMENT", "script too large; hub cp the file then run the remote path")
	}
	if looksPowerShellMangled(body) {
		return "", "", "", wire.E("INVALID_ARGUMENT", "command looks rewritten by PowerShell ($?, 2>&1, quotes); use --script-file and --shell bash or --shell powershell")
	}
	if strings.TrimSpace(shell) == "" {
		shell = shellFromShebang(body)
	}
	wrapped, werr := wrapRemoteCommand(shell, body)
	if werr != nil {
		return "", "", "", werr
	}
	return target, wrapped, warn, nil
}

func extractBashC(rest []string) (script, warn string) {
	if len(rest) < 2 {
		return "", ""
	}
	i := 0
	switch rest[0] {
	case "bash", "/bin/bash", "/usr/bin/bash", "sh", "/bin/sh":
		i = 1
	default:
		return "", ""
	}
	if i >= len(rest) {
		return "", ""
	}
	switch rest[i] {
	case "-c", "-lc", "-ic":
		i++
	default:
		return "", ""
	}
	if i >= len(rest) {
		return "", "bash -c had no script (PowerShell may have eaten quoted argv)"
	}
	script = strings.Join(rest[i:], " ")
	if looksPowerShellMangled(script) || script == "set" {
		return script, "bash -c script looks truncated or rewritten by PowerShell"
	}
	return script, "rewrote bash -c through hub --shell bash (avoids local quoting)"
}

func looksPowerShellMangled(s string) bool {
	t := strings.TrimSpace(s)
	if t == "set" || t == "set;" {
		return true
	}
	if strings.Contains(s, "exit=True") {
		return true
	}
	if strings.Contains(s, "echo True") || strings.Contains(s, "echo True\n") {
		return true
	}
	return false
}

func shellFromShebang(body string) string {
	line, _, _ := strings.Cut(strings.TrimPrefix(body, "\ufeff"), "\n")
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if !strings.HasPrefix(line, "#!") {
		return ""
	}
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "pwsh"):
		return "pwsh"
	case strings.Contains(l, "powershell"):
		return "powershell"
	case strings.Contains(l, "bash"), strings.Contains(l, "/sh"):
		return "bash"
	default:
		return ""
	}
}

func wrapRemoteCommand(shell, body string) (string, error) {
	body = strings.TrimRight(body, "\r\n")
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "", "raw", "none":
		return body, nil
	case "bash", "sh":
		b64 := base64.StdEncoding.EncodeToString([]byte(body + "\n"))
		return "printf '%s' " + b64 + " | base64 -d | bash -s", nil
	case "powershell":
		return "powershell.exe -NoProfile -NonInteractive -EncodedCommand " + powershellEncoded(body), nil
	case "pwsh":
		return "pwsh -NoProfile -NonInteractive -EncodedCommand " + powershellEncoded(body), nil
	case "cmd":
		return "", wire.E("INVALID_ARGUMENT", "--shell cmd is not supported; use --shell powershell on Windows remotes")
	default:
		return "", wire.Ef("INVALID_ARGUMENT", "unknown --shell %q (bash, sh, powershell, pwsh, raw)", shell)
	}
}

func powershellEncoded(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	return base64.StdEncoding.EncodeToString(b)
}
