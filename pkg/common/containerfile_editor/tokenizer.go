package containerfileeditor

import (
	"unicode"
)

type token struct {
	start int    // token starts at line[start]
	raw   string // content of line[start : <end of token>]
}

// Split a containerfile line into tokens, handling quoting and escaping.
func tokenize(line string, escapeToken byte) []token {
	tokenStart := -1
	var openingQuote byte

	// Assuming the character at index i is the escapeToken, check if it forms a valid
	// escape sequence in this context or if it should be treated as literal.
	isAnEscape := func(i int) bool {
		switch openingQuote {
		case '\'':
			// Always literal inside single quotes
			// https://github.com/moby/buildkit/blob/fa19659fc7b7af25fcac96e4c6314b2146994e8c/frontend/dockerfile/shell/lex.go#L247-L248
			return false
		case '"':
			// Inside double quotes, only recognized before ", $ or the escape char itself
			// https://github.com/moby/buildkit/blob/fa19659fc7b7af25fcac96e4c6314b2146994e8c/frontend/dockerfile/shell/lex.go#L325-L330
			if i+1 >= len(line) {
				return false
			}
			switch line[i+1] {
			case '"', '$', escapeToken:
				return true
			default:
				return false
			}
		default:
			// Always a valid escape outside quotes
			return true
		}
	}

	var tokens []token

	for i := 0; i < len(line); i++ {
		c := line[i]
		isSpace := unicode.IsSpace(rune(c))

		if !isSpace && tokenStart < 0 {
			// Any non-whitespace character starts a token
			tokenStart = i
		}

		if isSpace {
			// An unquoted, unescaped whitespace character terminates a token
			// (we skip over escaped whitespace, so only need to check for quotes)
			if openingQuote == 0 && tokenStart >= 0 {
				tokens = append(tokens, token{start: tokenStart, raw: line[tokenStart:i]})
				tokenStart = -1
			}
		} else if c == escapeToken {
			if isAnEscape(i) {
				i++
			}
		} else if c == '\'' || c == '"' {
			switch openingQuote {
			case 0:
				openingQuote = c
			case c:
				openingQuote = 0
			}
		}
	}

	// Note: the opening quote may never have been closed.
	// Rather than erroring, return what we have in case it's somehow valid.

	if tokenStart >= 0 {
		tokens = append(tokens, token{start: tokenStart, raw: line[tokenStart:]})
	}

	return tokens
}
