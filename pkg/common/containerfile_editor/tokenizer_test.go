package containerfileeditor

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		escapeToken byte
		expected    []token
	}{
		// Basic tokenization
		{
			name: "single-token",
			line: "hello",
			expected: []token{
				{start: 0, raw: "hello"},
			},
		},
		{
			name: "multiple-tokens",
			line: "echo hello world",
			expected: []token{
				{start: 0, raw: "echo"},
				{start: 5, raw: "hello"},
				{start: 11, raw: "world"},
			},
		},
		{
			name: "multiple-spaces",
			line: "echo   hello",
			expected: []token{
				{start: 0, raw: "echo"},
				{start: 7, raw: "hello"},
			},
		},
		{
			name: "tab-whitespace",
			line: "echo\thello",
			expected: []token{
				{start: 0, raw: "echo"},
				{start: 5, raw: "hello"},
			},
		},
		{
			name: "leading-and-trailing-whitespace",
			line: "  echo hello  ",
			expected: []token{
				{start: 2, raw: "echo"},
				{start: 7, raw: "hello"},
			},
		},
		{
			name:     "empty-string",
			line:     "",
			expected: nil,
		},
		{
			name:     "whitespace-only",
			line:     "   ",
			expected: nil,
		},

		// Quoting
		{
			name: "double-quoted-string",
			line: `echo "hello world"`,
			expected: []token{
				{start: 0, raw: "echo"},
				{start: 5, raw: `"hello world"`},
			},
		},
		{
			name: "single-quoted-string",
			line: "echo 'hello world'",
			expected: []token{
				{start: 0, raw: "echo"},
				{start: 5, raw: "'hello world'"},
			},
		},
		{
			name: "quotes-mid-token",
			line: `he"llo wor"ld`,
			expected: []token{
				{start: 0, raw: `he"llo wor"ld`},
			},
		},
		{
			name: "single-quotes-inside-double-quotes",
			line: `"it's"`,
			expected: []token{
				{start: 0, raw: `"it's"`},
			},
		},
		{
			name: "double-quotes-inside-single-quotes",
			line: `'say "hi"'`,
			expected: []token{
				{start: 0, raw: `'say "hi"'`},
			},
		},
		{
			name: "unclosed-double-quote",
			line: `echo "hello`,
			expected: []token{
				{start: 0, raw: "echo"},
				{start: 5, raw: `"hello`},
			},
		},
		{
			name: "unclosed-single-quote",
			line: "echo 'hello",
			expected: []token{
				{start: 0, raw: "echo"},
				{start: 5, raw: "'hello"},
			},
		},

		// Escaping (backslash)
		{
			name: "escaped-space-outside-quotes",
			line: `hello\ world`,
			expected: []token{
				{start: 0, raw: `hello\ world`},
			},
		},
		{
			name: "escape-before-double-quote",
			line: `"hello\" world"`,
			expected: []token{
				{start: 0, raw: `"hello\" world"`},
			},
		},
		{
			name: "double-escape-before-quote-closes-token",
			// \\ escapes to \, then " closes the quote, then space separates tokens
			line: `"hello\\" world`,
			expected: []token{
				{start: 0, raw: `"hello\\"`},
				{start: 10, raw: "world"},
			},
		},
		// Most of the escape scenarios below would only be relevant if we were lexing the tokens.
		// They're kept for completeness.
		{
			name: "escape-before-dollar-in-double-quotes",
			line: `"hello\$world"`,
			expected: []token{
				{start: 0, raw: `"hello\$world"`},
			},
		},
		{
			name: "escape-literal-inside-single-quotes",
			line: `'hello\tworld'`,
			expected: []token{
				{start: 0, raw: `'hello\tworld'`},
			},
		},
		{
			name: "non-escape-inside-double-quotes",
			line: `"hello\tworld"`,
			expected: []token{
				{start: 0, raw: `"hello\tworld"`},
			},
		},
		{
			name: "escape-at-end-of-line",
			line: `hello\`,
			expected: []token{
				{start: 0, raw: `hello\`},
			},
		},
		{
			name: "escape-at-end-inside-double-quotes",
			// The \" escapes the ", so the quote is never closed
			line: `"hello\"`,
			expected: []token{
				{start: 0, raw: `"hello\"`},
			},
		},
		{
			name: "escape-at-start-of-token",
			line: `\xhello`,
			expected: []token{
				{start: 0, raw: `\xhello`},
			},
		},
		{
			name: "escape-regular-char-outside-quotes",
			line: `hello\xworld`,
			expected: []token{
				{start: 0, raw: `hello\xworld`},
			},
		},

		// Backtick escape token
		{
			name:        "backtick-escape-outside-quotes",
			line:        "hello` world",
			escapeToken: '`',
			expected: []token{
				{start: 0, raw: "hello` world"},
			},
		},
		{
			name:        "backtick-before-double-quote",
			line:        "\"hello`\" world\"",
			escapeToken: '`',
			expected: []token{
				{start: 0, raw: "\"hello`\" world\""},
			},
		},
		{
			name: "double-backtick-before-quote-closes-token",
			// `` escapes to `, then " closes the quote, then space separates tokens
			line: "\"hello``\" world",
			expected: []token{
				{start: 0, raw: "\"hello``\""},
				{start: 10, raw: "world"},
			},
		},
		{
			name:        "backtick-literal-inside-single-quotes",
			line:        "'hello`' world",
			escapeToken: '`',
			expected: []token{
				{start: 0, raw: "'hello`'"},
				{start: 9, raw: "world"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			escapeToken := tc.escapeToken
			if escapeToken == 0 {
				escapeToken = '\\'
			}

			result := tokenize(tc.line, escapeToken)
			g.Expect(result).To(Equal(tc.expected))
		})
	}
}
