package cliwrappers

import (
	"regexp"
	"strings"
)

// Use an explicit ASCII set instead of \w, which in Go matches Unicode letters.
// Non-ASCII characters should be quoted to avoid locale-dependent shell behavior.
var shellUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_%+,\-./:=@]`)

// Quotes command arguments as needed and joins them with spaces.
// The output should be human-readable and copy-paste-able into a POSIX shell.
// Try to avoid using this to execute shell commands, the intended use case is logging.
func shellJoin(cmdName string, args ...string) string {
	cmd := make([]string, len(args)+1)
	cmd[0] = shellQuote(cmdName)
	for i, arg := range args {
		cmd[i+1] = shellQuote(arg)
	}
	return strings.Join(cmd, " ")
}

func shellQuote(arg string) string {
	if arg == "" || shellUnsafe.MatchString(arg) {
		return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return arg
}
