package cliwrappers

import "testing"

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello", "hello"},
		{"with space", "hello world", "'hello world'"},
		{"with single quote", "it's", `'it'\''s'`},
		{"empty string", "", "''"},
		{"non-ascii", "čau", "'čau'"},
		{"dollar sign", "$HOME", "'$HOME'"},
		{"backtick", "`cmd`", "'`cmd`'"},
		{"pipe", "a|b", "'a|b'"},
		{"semicolon", "a;b", "'a;b'"},
		{"ampersand", "a&b", "'a&b'"},
		{"parentheses", "(cmd)", "'(cmd)'"},
		{"safe chars", "a-b_c+d,e.f/g:h=i@j%k", "a-b_c+d,e.f/g:h=i@j%k"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShellQuote(tt.input)
			if got != tt.expected {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShellJoin(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected string
	}{
		{
			"simple command",
			"buildah", []string{"build", "-t", "myimage"},
			"buildah build -t myimage",
		},
		{
			"arg with space",
			"buildah", []string{"build", "--build-arg", "FOO=hello world"},
			"buildah build --build-arg 'FOO=hello world'",
		},
		{
			"no args",
			"buildah", nil,
			"buildah",
		},
		{
			"multiple special args",
			"echo", []string{"it's", "a $var", ""},
			`echo 'it'\''s' 'a $var' ''`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellJoin(tt.cmd, tt.args...)
			if got != tt.expected {
				t.Errorf("shellJoin(%q, %v) = %q, want %q", tt.cmd, tt.args, got, tt.expected)
			}
		})
	}
}
