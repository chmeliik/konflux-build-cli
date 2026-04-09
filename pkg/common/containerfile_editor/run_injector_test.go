package containerfileeditor

import (
	"errors"
	"math"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

const defaultInjection = "echo INJECTED && "

// Strip common leading whitespace from a raw string literal, similar to Python's textwrap.dedent
// (but also strips the leading newline).
func dedent(s string) string {
	s = strings.TrimPrefix(s, "\n")
	lines := strings.Split(s, "\n")

	minIndent := math.MaxInt
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if len(trimmed) == 0 {
			continue
		}
		if indent := len(line) - len(trimmed); indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent == math.MaxInt {
		minIndent = 0
	}

	for i, line := range lines {
		if len(line) >= minIndent {
			lines[i] = line[minIndent:]
		} else {
			lines[i] = ""
		}
	}

	return strings.Join(lines, "\n")
}

func TestInject(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   string
		toInject string // defaults to defaultInjection
	}{
		// Basic
		{
			name: "simple-run",
			input: dedent(`
				FROM alpine:latest
				RUN echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && echo hello
			`),
		},
		{
			name: "no-run",
			input: dedent(`
				FROM alpine:latest
				COPY . /app
				LABEL foo=bar
			`),
			output: dedent(`
				FROM alpine:latest
				COPY . /app
				LABEL foo=bar
			`),
		},
		{
			name: "multiple-run",
			input: dedent(`
				FROM alpine:latest

				RUN echo first

				RUN echo second
				RUN echo third
			`),
			output: dedent(`
				FROM alpine:latest

				RUN echo INJECTED && echo first

				RUN echo INJECTED && echo second
				RUN echo INJECTED && echo third
			`),
		},
		{
			name: "lowercase-run",
			input: dedent(`
				FROM alpine:latest
				run echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				run echo INJECTED && echo hello
			`),
		},

		// Flags (--mount, --network)
		{
			name: "run-with-mount",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/var/cache echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/var/cache echo INJECTED && echo hello
			`),
		},
		{
			name: "run-with-multiple-flags",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=type=secret,id=mysecret --network=host echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=type=secret,id=mysecret --network=host echo INJECTED && echo hello
			`),
		},
		{
			name: "run-with-mount-continuation",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=from=base,src=/etc/os-release,dst=/tmp/os-release \
				    echo hello && \
				    echo world
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=from=base,src=/etc/os-release,dst=/tmp/os-release \
				    echo INJECTED && echo hello && \
				    echo world
			`),
		},
		{
			name: "run-with-multiple-flags-continuation",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=type=secret,id=mysecret \
				    --network=host \
				    echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=type=secret,id=mysecret \
				    --network=host \
				    echo INJECTED && echo hello
			`),
		},
		{
			name: "run-with-quoted-flag",
			input: dedent(`
				FROM alpine:latest
				RUN --mount="type=cache, target=/var/cache" echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount="type=cache, target=/var/cache" echo INJECTED && echo hello
			`),
		},
		{
			name: "run-with-single-quoted-flag",
			input: dedent(`
				FROM alpine:latest
				RUN --mount='type=cache, target=/var/cache' echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount='type=cache, target=/var/cache' echo INJECTED && echo hello
			`),
		},
		{
			name: "run-with-escaped-flag",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,\ target=/var/cache echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,\ target=/var/cache echo INJECTED && echo hello
			`),
		},

		// Heredocs
		{
			name: "heredoc-with-interpreter",
			input: dedent(`
				FROM alpine:latest
				RUN sh <<EOF
				echo hello
				EOF
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && sh <<EOF
				echo hello
				EOF
			`),
		},
		{
			name: "heredoc-without-interpreter",
			input: dedent(`
				FROM alpine:latest
				RUN <<EOF
				echo hello
				EOF
			`),
			output: dedent(`
				FROM alpine:latest
				RUN <<EOF
				echo hello
				EOF
			`),
		},
		{
			name: "heredoc-quoted-marker",
			input: dedent(`
				FROM alpine:latest

				RUN <<'EOF'
				echo bare single-quoted
				EOF

				RUN sh <<"EOF"
				echo with interpreter double-quoted
				EOF
			`),
			output: dedent(`
				FROM alpine:latest

				RUN <<'EOF'
				echo bare single-quoted
				EOF

				RUN echo INJECTED && sh <<"EOF"
				echo with interpreter double-quoted
				EOF
			`),
		},
		{
			name: "heredoc-in-copy",
			input: dedent(`
				FROM alpine:latest
				COPY <<example.Containerfile /tmp/example.Containerfile
				RUN echo this should not be injected
				example.Containerfile
				RUN echo this should be injected
			`),
			output: dedent(`
				FROM alpine:latest
				COPY <<example.Containerfile /tmp/example.Containerfile
				RUN echo this should not be injected
				example.Containerfile
				RUN echo INJECTED && echo this should be injected
			`),
		},
		{
			name: "heredoc-in-run-body",
			input: dedent(`
				FROM alpine:latest

				RUN sh <<'EOF'
				function RUN() {
				    echo "Run: $*"
				}
				RUN echo inside heredoc
				EOF

				RUN echo outside heredoc
			`),
			output: dedent(`
				FROM alpine:latest

				RUN echo INJECTED && sh <<'EOF'
				function RUN() {
				    echo "Run: $*"
				}
				RUN echo inside heredoc
				EOF

				RUN echo INJECTED && echo outside heredoc
			`),
		},
		{
			name: "heredoc-with-comments",
			input: dedent(`
				FROM alpine:latest
				RUN sh <<EOF
				# this comment should be preserved
				echo hello # inline comment too
				EOF
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && sh <<EOF
				# this comment should be preserved
				echo hello # inline comment too
				EOF
			`),
		},
		{
			name: "copy-and-run-heredocs",
			input: dedent(`
				FROM alpine:latest

				COPY <<example.Containerfile \
				     <<"example2.Containerfile" /tmp
				RUN echo hi
				example.Containerfile
				RUN echo $FOO
				example2.Containerfile

				RUN sh <<'EOF'
				function RUN() {
				    echo "Run: $*"
				}
				RUN echo inside heredoc
				EOF

				RUN echo outside heredoc
			`),
			output: dedent(`
				FROM alpine:latest

				COPY <<example.Containerfile \
				     <<"example2.Containerfile" /tmp
				RUN echo hi
				example.Containerfile
				RUN echo $FOO
				example2.Containerfile

				RUN echo INJECTED && sh <<'EOF'
				function RUN() {
				    echo "Run: $*"
				}
				RUN echo inside heredoc
				EOF

				RUN echo INJECTED && echo outside heredoc
			`),
		},

		// Exec form
		{
			name: "exec-form",
			input: dedent(`
				FROM alpine:latest
				RUN ["echo", "hello"]
			`),
			output: dedent(`
				FROM alpine:latest
				RUN ["echo", "hello"]
			`),
		},

		// No-op RUN instructions
		{
			name: "run-comment-only",
			input: dedent(`
				FROM alpine:latest
				RUN # just a comment
			`),
			output: dedent(`
				FROM alpine:latest
				RUN # just a comment
			`),
		},
		{
			name: "run-flag-then-comment",
			input: dedent(`
				FROM alpine:latest
				RUN --network=host # flag with no command is a no-op
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --network=host # flag with no command is a no-op
			`),
		},
		{
			name: "run-all-comment-continuation",
			input: dedent(`
				FROM alpine:latest
				RUN # a line that goes [real token] -> # -> [line continuation] \
				    also makes the next line a comment \
				    and this one too, so this whole thing is a no-op
			`),
			output: dedent(`
				FROM alpine:latest
				RUN # a line that goes [real token] -> # -> [line continuation] \
				    also makes the next line a comment \
				    and this one too, so this whole thing is a no-op
			`),
		},

		// Comments in continuations
		{
			name: "comment-after-bare-continuation",
			input: dedent(`
				FROM alpine:latest
				RUN \
				    # a comment-only line that ends with a continuation is ignored \
				    echo so this is an actual command
			`),
			output: dedent(`
				FROM alpine:latest
				RUN \
				    # a comment-only line that ends with a continuation is ignored \
				    echo INJECTED && echo so this is an actual command
			`),
		},
		{
			name: "comment-in-middle",
			input: dedent(`
				FROM alpine:latest
				RUN echo && \
				    # comment between commands
				    echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && echo && \
				    # comment between commands
				    echo hello
			`),
		},
		{
			name: "comment-between-flags",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/cache \
				    # comment between flags
				    --network=host \
				    echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/cache \
				    # comment between flags
				    --network=host \
				    echo INJECTED && echo hello
			`),
		},
		{
			name: "trailing-inline-comment",
			input: dedent(`
				FROM alpine:latest
				RUN echo "hello world" # inline comment after command
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && echo "hello world" # inline comment after command
			`),
		},

		// Mixed supported/unsupported
		{
			name: "mixed",
			input: dedent(`
				FROM alpine:latest
				RUN echo first

				RUN <<EOF
				echo heredoc
				EOF

				RUN ["echo", "exec"]

				RUN echo second
				RUN # no-op
				RUN echo third
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && echo first

				RUN <<EOF
				echo heredoc
				EOF

				RUN ["echo", "exec"]

				RUN echo INJECTED && echo second
				RUN # no-op
				RUN echo INJECTED && echo third
			`),
		},

		// Multi-stage builds
		{
			name: "multi-stage",
			input: dedent(`
				FROM alpine:latest AS builder
				RUN echo building

				FROM scratch
				COPY --from=builder /app /app

				FROM alpine:latest
				RUN echo running
			`),
			output: dedent(`
				FROM alpine:latest AS builder
				RUN echo INJECTED && echo building

				FROM scratch
				COPY --from=builder /app /app

				FROM alpine:latest
				RUN echo INJECTED && echo running
			`),
		},

		// Escape directive (can't use dedent + raw strings — they contain backticks)
		{
			name: "escape-backtick",
			input: strings.Join([]string{
				"# escape=`",
				"FROM alpine:latest",
				"RUN --mount=type=cache` target=/cache `",
				"    echo hello",
				"",
			}, "\n"),
			output: strings.Join([]string{
				"# escape=`",
				"FROM alpine:latest",
				"RUN --mount=type=cache` target=/cache `",
				"    echo INJECTED && echo hello",
				"",
			}, "\n"),
		},
		{
			name:     "escape-backtick-multiline-inject",
			toInject: ". /tmp/prefetch.env && \\\n    ",
			input: strings.Join([]string{
				"# escape=`",
				"FROM alpine:latest",
				"RUN echo hello",
				"",
			}, "\n"),
			output: strings.Join([]string{
				"# escape=`",
				"FROM alpine:latest",
				"RUN . /tmp/prefetch.env && `",
				"    echo hello",
				"",
			}, "\n"),
		},

		// Multi-line injection
		{
			name:     "multiline-inject",
			toInject: ". /tmp/prefetch.env && \\\n    ",
			input: dedent(`
				FROM alpine:latest
				RUN echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN . /tmp/prefetch.env && \
				    echo hello
			`),
		},
		{
			name:     "multiline-inject-bare-newlines",
			toInject: ". /tmp/prefetch.env &&\n    ",
			input: dedent(`
				FROM alpine:latest
				RUN echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN . /tmp/prefetch.env &&\
				    echo hello
			`),
		},
		{
			name:     "multiline-inject-with-mount",
			toInject: ". /tmp/prefetch.env && \\\n    ",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/cache \
				    echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/cache \
				    . /tmp/prefetch.env && \
				    echo hello
			`),
		},

		// Edge cases
		{
			name: "bare-continuation-outdented",
			input: dedent(`
				FROM alpine:latest
				RUN \
				echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN \
				echo INJECTED && echo hello
			`),
		},
		{
			name: "flag-then-continuation-outdented",
			input: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/cache \
				echo hello
			`),
			output: dedent(`
				FROM alpine:latest
				RUN --mount=type=cache,target=/cache \
				echo INJECTED && echo hello
			`),
		},
		{
			name: "trailing-continuation-at-eof",
			input: dedent(`
				FROM alpine:latest
				RUN echo hello \
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && echo hello \
			`),
		},
		{
			name: "trailing-continuation-then-comment",
			input: dedent(`
				FROM alpine:latest
				RUN echo hello \
				    # nothing follows
			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && echo hello \
				    # nothing follows
			`),
		},
		{
			name:   "missing-newline-at-eof",
			input:  "FROM alpine:latest\nRUN echo hello",
			output: "FROM alpine:latest\nRUN echo INJECTED && echo hello\n",
		},
		{
			name: "trailing-blank-lines",
			input: dedent(`
				FROM alpine:latest
				RUN echo hello


			`),
			output: dedent(`
				FROM alpine:latest
				RUN echo INJECTED && echo hello


			`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			toInject := tc.toInject
			if toInject == "" {
				toInject = defaultInjection
			}

			injector := RunInjector{}
			result, err := injector.Inject(tc.input, toInject)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(Equal(tc.output))
		})
	}
}

func TestInject_InvalidEscapeDirective(t *testing.T) {
	g := NewWithT(t)

	injector := RunInjector{}
	_, err := injector.Inject(dedent(`
		# escape=abc
		FROM alpine:latest
		RUN echo hello
	`), defaultInjection)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid escape token 'abc'"))
}

func TestInject_EmptyContainerfile(t *testing.T) {
	g := NewWithT(t)

	injector := RunInjector{}
	_, err := injector.Inject("", defaultInjection)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("file with no instructions"))
}

type unsupportedCall struct {
	Line  int
	Error error
}

func TestInject_OnUnsupported(t *testing.T) {
	g := NewWithT(t)

	input := dedent(`
		FROM alpine:latest

		RUN echo supported

		RUN <<EOF
		echo heredoc
		EOF

		RUN ["echo", "exec"]

		# no-op
		RUN
		RUN --network=host # also no-op

		RUN echo also supported
	`)

	var calls []unsupportedCall
	injector := RunInjector{OnUnsupported: func(lineno int, err error) {
		calls = append(calls, unsupportedCall{Line: lineno, Error: err})
	}}

	result, err := injector.Inject(input, "INJECTED ")
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(result).To(Equal(dedent(`
		FROM alpine:latest

		RUN INJECTED echo supported

		RUN <<EOF
		echo heredoc
		EOF

		RUN ["echo", "exec"]

		# no-op
		RUN
		RUN --network=host # also no-op

		RUN INJECTED echo also supported
	`)))

	g.Expect(calls).To(HaveLen(4))

	g.Expect(calls[0].Line).To(Equal(5))
	g.Expect(errors.Is(calls[0].Error, ErrRunHeredoc)).To(BeTrue())

	g.Expect(calls[1].Line).To(Equal(9))
	g.Expect(errors.Is(calls[1].Error, ErrRunExec)).To(BeTrue())

	g.Expect(calls[2].Line).To(Equal(12))
	g.Expect(errors.Is(calls[2].Error, ErrRunNoOp)).To(BeTrue())

	g.Expect(calls[3].Line).To(Equal(13))
	g.Expect(errors.Is(calls[3].Error, ErrRunNoOp)).To(BeTrue())
}
