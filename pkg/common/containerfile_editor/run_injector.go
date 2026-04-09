package containerfileeditor

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	dfparser "github.com/moby/buildkit/frontend/dockerfile/parser"
)

var (
	ErrRunHeredoc = errors.New("heredoc")
	ErrRunExec    = errors.New("exec-form")
	ErrRunNoOp    = errors.New("no-op")
)

type RunInjector struct {
	// Called when encountering an unsupported RUN instruction, see [RunInjector.Inject]
	OnUnsupported func(lineno int, err error)
}

// Prepend toInject at the beginning of supported RUN instructions, after any options like --mount.
//
// Does not inject into RUN instructions that:
//   - are effectively no-ops (e.g. RUN # just a comment)
//   - are in exec-form (e.g. RUN ["echo", "hello"])
//   - start with a heredoc (e.g. RUN <<EOF)
//     -- heredocs appearing later in the instruction are OK ('RUN sh <<EOF' works)
//
// When encountering an unsupported RUN instruction, calls the OnUnsupported function
// with the RUN instruction's line number and an error value that gives the unsupported reason;
// one of [ErrRunHeredoc], [ErrRunExec], [ErrRunNoOp].
//
// The toInject text can contain multiple lines. This function automatically adds the appropriate
// line continuation character if needed. If toInject already has line continuations, they must use
// backslash regardless of the actual escape character used in the containerfile.
func (r *RunInjector) Inject(containerfileContent string, toInject string) (string, error) {
	injector, err := newInternalInjector(containerfileContent)
	if err != nil {
		return "", err
	}
	onUnsupported := r.OnUnsupported
	if onUnsupported == nil {
		onUnsupported = func(int, error) {}
	}
	return injector.inject(toInject, onUnsupported), nil
}

// Tracks the internal state necessary to inject text into RUN instructions in a containerfile.
// Single-use, won't produce sensible results after the first inject() call.
type internalInjector struct {
	physicalLines []string
	parsed        *dfparser.Result
	escapeToken   byte
}

func newInternalInjector(content string) (*internalInjector, error) {
	inj := &internalInjector{physicalLines: splitLines(content)}

	result, err := dfparser.Parse(strings.NewReader(content))
	if err != nil {
		return nil, err
	}

	// Don't try to support non-ASCII escape tokens (only \ and ` should be valid anyway)
	if rune(byte(result.EscapeToken)) != result.EscapeToken {
		return nil, fmt.Errorf("unsupported escape token: %c", result.EscapeToken)
	}

	inj.parsed = result
	inj.escapeToken = byte(result.EscapeToken)

	return inj, nil
}

// Perform the injection as described in [RunInjector.Inject].
// Should not be called more than once on the same internalInjector instance.
func (inj *internalInjector) inject(toInject string, onUnsupported func(lineno int, err error)) string {
	toInject = inj.fixInjection(toInject)

	for _, node := range inj.parsed.AST.Children {
		if strings.ToUpper(node.Value) != "RUN" {
			continue
		}
		if node.Attributes["json"] {
			onUnsupported(node.StartLine, ErrRunExec)
			continue
		}

		lineIndices := inj.getPhysicalLines(node)
		logicalLine := inj.joinToLogicalLine(lineIndices)
		injectionIndex, unsupportedErr := inj.findInjectionIndex(logicalLine)

		if unsupportedErr != nil {
			onUnsupported(node.StartLine, unsupportedErr)
		}

		if injectionIndex >= 0 {
			inj.injectIntoPhysicalLine(lineIndices, injectionIndex, toInject)
		}
	}

	return strings.Join(inj.physicalLines, "\n") + "\n"
}

// Adjust the injection so that newlines are escaped as appropriate.
// The input injection may contain unescaped newlines or backslash-escaped newlines,
// which will be converted to line continuations using the proper escape character.
func (inj *internalInjector) fixInjection(toInject string) string {
	lines := splitLines(toInject)
	for i, line := range lines {
		lines[i] = trimContinuation(line, '\\')
	}
	return strings.Join(lines, string(inj.escapeToken)+"\n")
}

// Get the physical line indices that form the instruction represented by the node,
// i.e. the starting line and any lines joined with continuations.
// This does not include heredocs that may be attached to this instruction,
// but that's OK, we don't want to inject into heredocs.
// Skips comment-only lines within continuations.
//
// Example:
// 0| FROM alpine:latest
// 1| RUN echo hi && \
// 2|     # this is a comment
// 3|     echo hello && \
// 4|     sh <<EOF
// 5| echo bye
// 6| EOF
// getPhysicalLines(RUN node) => [1, 3, 4]
func (inj *internalInjector) getPhysicalLines(node *dfparser.Node) []int {
	var lines []int

	// startLine is 1-indexed, our physical lines are 0-indexed
	for i := node.StartLine - 1; i < len(inj.physicalLines); i++ {
		line := inj.physicalLines[i]
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		lines = append(lines, i)

		if !hasContinuation(line, inj.escapeToken) {
			break
		}
	}

	return lines
}

// Join the physical lines at the given indices into a single logical line, stripping continuations.
func (inj *internalInjector) joinToLogicalLine(lineIndices []int) string {
	var b strings.Builder
	for _, idx := range lineIndices {
		b.WriteString(trimContinuation(inj.physicalLines[idx], inj.escapeToken))
	}
	return b.String()
}

// Given a logical RUN line, find the index at which we should insert the injection.
// If this RUN line isn't supported for injection, return (-1, error).
func (inj *internalInjector) findInjectionIndex(line string) (int, error) {
	tokens := tokenize(line, inj.escapeToken)
	if len(tokens) < 1 {
		// Shouldn't be possible, there's always at least RUN. Let's be safe though.
		return -1, ErrRunNoOp
	}

	for _, tok := range tokens[1:] {
		if strings.HasPrefix(tok.raw, "--") {
			continue
		}
		if strings.HasPrefix(tok.raw, "#") {
			return -1, ErrRunNoOp
		}
		if dfparser.MustParseHeredoc(tok.raw) != nil {
			return -1, ErrRunHeredoc
		}
		// ErrExecForm already handled in the main loop
		return tok.start, nil
	}

	return -1, ErrRunNoOp
}

// Find the physical line that corresponds to injectionIndex
// (which is an offset into the joined logical line with continuations stripped)
// and insert toInject into this line at the corresponding offset.
func (inj *internalInjector) injectIntoPhysicalLine(logicalLine []int, injectionIndex int, toInject string) {
	for _, i := range logicalLine {
		physicalLine := inj.physicalLines[i]
		contributionToLogicalLine := len(trimContinuation(physicalLine, inj.escapeToken))

		if contributionToLogicalLine > injectionIndex {
			inj.physicalLines[i] = physicalLine[:injectionIndex] + toInject + physicalLine[injectionIndex:]
			return
		}

		// Subtract this physical line's contribution, so that
		// injectionIndex becomes relative to the next physical line.
		injectionIndex -= contributionToLogicalLine
	}
	panic("injection index out of range")
}

// Check whether a line ends with an unescaped escape character (possibly followed by whitespace).
func hasContinuation(line string, escapeToken byte) bool {
	return len(trimContinuation(line, escapeToken)) != len(line)
}

// Remove the trailing escape character (and any trailing whitespace after it) from a line,
// if it ends with an unescaped continuation. Otherwise returns the line unchanged.
//
// Matches buildkit behavior:
//   - the escape character may be followed by tabs or spaces
//   - the escape character must not be preceded by an escape character
//     (ignores the fact that the preceding escape may itself be escaped)
//   - see https://github.com/moby/buildkit/blob/fa19659fc7b7af25fcac96e4c6314b2146994e8c/frontend/dockerfile/parser/parser.go#L168
func trimContinuation(line string, escapeToken byte) string {
	trimmed := strings.TrimRight(line, " \t")
	length := len(trimmed)
	if length > 0 && trimmed[length-1] == escapeToken {
		if length == 1 || trimmed[length-2] != escapeToken {
			return trimmed[:length-1]
		}
	}
	return line
}

func splitLines(s string) []string {
	var lines []string
	for line := range strings.Lines(s) {
		lines = append(lines, strings.TrimSuffix(line, "\n"))
	}
	return lines
}
