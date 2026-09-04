package workspace

import (
	"encoding/base64"
	"path"
	"regexp"
	"strings"
	"unicode"

	"localaihub/internal/wire"
)

func Confine(cliWorkdir, yamlRoot string) (string, error) {
	cliWorkdir = strings.TrimSpace(cliWorkdir)
	yamlRoot = strings.TrimSpace(yamlRoot)
	if cliWorkdir == "" {
		return yamlRoot, nil
	}
	if yamlRoot != "" && !Under(cliWorkdir, yamlRoot) {
		return "", wire.E("FILE_OUTSIDE_WORKSPACE", "--workdir is outside workspace_root")
	}
	return cliWorkdir, nil
}

func Under(child, root string) bool {
	if strings.TrimSpace(root) == "" {
		return true
	}
	c := cleanUnix(child)
	r := cleanUnix(root)
	if c == r {
		return true
	}
	if r != "/" && strings.HasPrefix(c, r+"/") {
		return true
	}
	if r == "/" && strings.HasPrefix(c, "/") && c != "/.." {
		return !strings.HasPrefix(path.Clean(c), "/..")
	}
	return false
}

func CheckCommand(inspect, workdir string) error {
	if strings.TrimSpace(workdir) == "" {
		return nil
	}
	inspect = strings.ReplaceAll(inspect, "\r\n", "\n")
	inspect = strings.ReplaceAll(inspect, "\r", "\n")
	if err := checkRedirects(inspect, workdir); err != nil {
		return err
	}
	for _, stmt := range stmtSplit.Split(inspect, -1) {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "#") {
			continue
		}
		if err := checkStatement(stmt, workdir); err != nil {
			return err
		}
	}
	return nil
}

var stmtSplit = regexp.MustCompile(`&&|\|\||[\n;]`)
var redirectRe = regexp.MustCompile(`(?:\d*)(>>?)(\s*)([^\s|&;]+)`)

var queryVerbs = map[string]bool{
	"ls": true, "dir": true, "cat": true, "less": true, "more": true,
	"head": true, "tail": true, "file": true, "stat": true, "du": true, "df": true,
	"wc": true, "grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true,
	"tree": true, "readlink": true, "realpath": true, "pwd": true, "echo": true,
	"printf": true, "which": true, "type": true, "uname": true, "hostname": true,
	"id": true, "whoami": true, "env": true, "printenv": true, "date": true,
	"true": true, "false": true, "basename": true, "dirname": true,
	"md5sum": true, "sha256sum": true, "sha1sum": true, "diff": true,
	"test": true, "[": true, "command": true, "find": true,
	"awk": true, "cut": true, "sort": true, "uniq": true, "tr": true, "jq": true,
	"column": true, "od": true, "hexdump": true, "xxd": true, "strings": true,
	"nl": true, "tac": true, "zcat": true, "cmp": true, "comm": true,
}

func checkRedirects(inspect, workdir string) error {
	for _, m := range redirectRe.FindAllStringSubmatch(inspect, -1) {
		if len(m) < 4 {
			continue
		}
		dest := strings.Trim(m[3], `"'`)
		if dest == "" || strings.HasPrefix(dest, "&") || allowlisted(dest) {
			continue
		}
		if looksAbsOrHome(dest) && !Under(dest, workdir) {
			return wire.E("FILE_OUTSIDE_WORKSPACE", "redirect "+dest+" is outside --workdir")
		}
		if err := rejectEscapingRel(dest); err != nil {
			return err
		}
	}
	return nil
}

func checkStatement(stmt, workdir string) error {
	for _, stage := range strings.Split(stmt, "|") {
		stage = strings.TrimSpace(stage)
		if stage == "" {
			continue
		}
		fields := strings.FieldsFunc(stage, unicode.IsSpace)
		verb, args := firstVerb(fields)
		if verb == "" {
			continue
		}
		if !isQuery(verb, args) {
			for _, tok := range args {
				tok = strings.Trim(tok, `"'`)
				if err := checkMutatePath(tok, workdir); err != nil {
					return err
				}
			}
			if verb == "cd" && len(args) > 0 {
				if err := checkMutatePath(strings.Trim(args[0], `"'`), workdir); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func firstVerb(fields []string) (verb string, args []string) {
	skip := map[string]bool{"sudo": true, "nohup": true, "nice": true, "time": true, "command": true, "busybox": true, "then": true, "do": true, "else": true}
	i := 0
	for i < len(fields) {
		w := strings.Trim(fields[i], "(){}")
		lw := strings.ToLower(pathBase(w))
		if skip[lw] {
			i++
			for i < len(fields) && strings.HasPrefix(fields[i], "-") {
				i++
			}
			continue
		}
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.ContainsAny(w, "/\\") {
			i++
			continue
		}
		return lw, fields[i+1:]
	}
	return "", nil
}

func isQuery(verb string, args []string) bool {
	switch verb {
	case "find":
		for _, a := range args {
			if a == "-delete" || a == "-exec" || a == "-ok" {
				return false
			}
		}
		return true
	case "git":
		if len(args) == 0 {
			return true
		}
		sub := strings.ToLower(strings.Trim(args[0], "-"))
		switch sub {
		case "status", "log", "diff", "show", "ls-files", "rev-parse", "branch", "remote", "describe", "cat-file", "version":
			return true
		default:
			return false
		}
	case "tar":
		joined := strings.Join(args, " ")
		if strings.Contains(joined, " -x") || strings.HasPrefix(joined, "-x") || strings.Contains(joined, "--extract") || strings.Contains(joined, " -c") || strings.Contains(joined, "--create") {
			return false
		}
		return true
	case "sed":
		for _, a := range args {
			if a == "-i" || strings.HasPrefix(a, "-i") {
				return false
			}
		}
		return true
	case "cd":
		return false
	}
	if queryVerbs[verb] {
		return true
	}
	return false
}

func checkMutatePath(tok, workdir string) error {
	if tok == "" || strings.HasPrefix(tok, "-") || strings.Contains(tok, "://") {
		return nil
	}
	if allowlisted(tok) {
		return nil
	}
	if err := rejectEscapingRel(tok); err != nil {
		return err
	}
	if looksAbsOrHome(tok) && !Under(tok, workdir) {
		return wire.E("FILE_OUTSIDE_WORKSPACE", "cannot modify "+tok+" outside --workdir "+workdir+"; listing/reading other dirs is allowed")
	}
	return nil
}

func rejectEscapingRel(tok string) error {
	if !looksRelative(tok) && tok != ".." && !strings.HasPrefix(tok, "../") {
		return nil
	}
	c := cleanUnix(tok)
	if c == ".." || strings.HasPrefix(c, "../") {
		return wire.E("FILE_OUTSIDE_WORKSPACE", "path "+tok+" escapes --workdir")
	}
	return nil
}

func pathBase(w string) string {
	w = strings.ReplaceAll(w, `\`, "/")
	i := strings.LastIndex(w, "/")
	if i >= 0 {
		return w[i+1:]
	}
	return w
}

func Bind(body, workdir string) string {
	wp := ShellPath(workdir)
	script := "mkdir -p -- " + wp + " && cd -- " + wp + " || { echo hub: cannot cd to workdir >&2; exit 1; }\n" + strings.TrimRight(body, "\r\n") + "\n"
	b64 := base64.StdEncoding.EncodeToString([]byte(script))
	return "printf '%s' " + b64 + " | base64 -d | bash -s"
}

func ShellPath(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, `\`, "/"))
	switch {
	case s == "~" || s == "$HOME":
		return `"$HOME"`
	case strings.HasPrefix(s, "~/"):
		return `"$HOME/` + escapeDouble(s[2:]) + `"`
	case strings.HasPrefix(s, "$HOME/"):
		return `"$HOME/` + escapeDouble(s[len("$HOME/"):]) + `"`
	default:
		return ShellQuote(s)
	}
}

func escapeDouble(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "$", `\$`)
	return s
}

func cleanUnix(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, `\`, "/")
	if p == "~" || p == "$HOME" {
		return "~"
	}
	if strings.HasPrefix(p, "$HOME/") {
		p = "~/" + strings.TrimPrefix(p, "$HOME/")
	}
	if strings.HasPrefix(p, "~/") {
		return "~/" + path.Clean(strings.TrimPrefix(p, "~/"))
	}
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	return path.Clean(p)
}

func looksAbsOrHome(tok string) bool {
	return strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "~") || strings.HasPrefix(tok, "$HOME")
}

func looksRelative(tok string) bool {
	return tok == ".." || strings.HasPrefix(tok, "../") || strings.HasPrefix(tok, "./") || strings.Contains(tok, "/../")
}

func allowlisted(tok string) bool {
	c := cleanUnix(tok)
	switch c {
	case "/dev/null", "/dev/stdin", "/dev/stdout", "/dev/stderr":
		return true
	}
	for _, p := range []string{"/bin/", "/usr/bin/", "/usr/local/bin/", "/sbin/", "/usr/sbin/", "/lib/", "/usr/lib/"} {
		if c == strings.TrimSuffix(p, "/") || strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
