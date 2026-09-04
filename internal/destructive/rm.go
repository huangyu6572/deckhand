package destructive

import (
	"encoding/base64"
	"path"
	"regexp"
	"strings"
	"unicode"

	"localaihub/internal/wire"
)

var bashWrapRe = regexp.MustCompile(`printf '%s' ([A-Za-z0-9+/=]+) \| base64 -d \| bash -s`)
var cmdSplitRe = regexp.MustCompile(`&&|\|\||[\n;&]`)

var rmNames = map[string]bool{
	"rm":          true,
	"rmdir":       true,
	"unlink":      true,
	"remove-item": true,
}

func LooksLikeRm(text string) bool {
	return Match(text) != ""
}

func RefuseUnlessConfirmed(text string, confirmed bool) error {
	hit := Match(text)
	if hit == "" {
		return nil
	}
	if confirmed {
		return nil
	}
	if len(hit) > 80 {
		hit = hit[:80] + "…"
	}
	hit = strings.ReplaceAll(hit, `"`, `'`)
	return wire.E("DESTROY_NEEDS_HUMAN", "refused to run rm without a person at a real keyboard (matched \""+hit+"\"); there is no --yes flag; a human must run hub in a terminal and type DELETE <target>. Scripts and agents cannot confirm")
}

func Match(text string) string {
	if text == "" {
		return ""
	}
	if hit := matchBlob(text); hit != "" {
		return hit
	}
	if decoded := unwrapHubScript(text); decoded != "" && decoded != text {
		return matchBlob(decoded)
	}
	return ""
}

func unwrapHubScript(command string) string {
	m := bashWrapRe.FindStringSubmatch(command)
	if len(m) < 2 {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		return ""
	}
	return string(raw)
}

func matchBlob(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if hit := scanSegments(s); hit != "" {
		return hit
	}
	for _, inner := range extractSubs(s) {
		if hit := scanSegments(inner); hit != "" {
			return hit
		}
	}
	return ""
}

func extractSubs(s string) []string {
	var out []string
	rest := s
	for {
		i := strings.Index(rest, "$(")
		if i < 0 {
			break
		}
		rest = rest[i+2:]
		depth := 1
		j := 0
		for j < len(rest) && depth > 0 {
			switch rest[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		if depth == 0 && j > 1 {
			out = append(out, rest[:j-1])
		}
	}
	return out
}

func scanSegments(s string) string {
	for _, p := range cmdSplitRe.Split(s, -1) {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		p = strings.TrimLeft(p, "({")
		fields := strings.FieldsFunc(p, unicode.IsSpace)
		if hit := matchWords(fields); hit != "" {
			return hit
		}
	}
	return ""
}

func matchWords(fields []string) string {
	skip := map[string]bool{
		"sudo": true, "command": true, "busybox": true, "nohup": true,
		"nice": true, "time": true, "then": true, "do": true, "else": true, "if": true,
	}
	i := 0
	for i < len(fields) {
		w := strings.Trim(fields[i], "(){}")
		lw := strings.ToLower(w)
		if skip[lw] {
			i++
			for i < len(fields) && strings.HasPrefix(fields[i], "-") {
				if lw == "sudo" && (fields[i] == "-u" || fields[i] == "-g") && i+1 < len(fields) {
					i += 2
					continue
				}
				i++
			}
			continue
		}
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.ContainsAny(w, "/\\") {
			i++
			continue
		}
		base := commandBase(w)
		if rmNames[base] {
			return strings.Join(fields, " ")
		}
		if base == "xargs" || base == "find" {
			for _, f := range fields[i+1:] {
				if rmNames[commandBase(f)] || strings.EqualFold(f, "-delete") {
					return strings.Join(fields, " ")
				}
			}
		}
		return ""
	}
	return ""
}

func commandBase(w string) string {
	w = strings.Trim(w, `"'`)
	w = strings.ReplaceAll(w, `\`, "/")
	return strings.ToLower(path.Base(w))
}
