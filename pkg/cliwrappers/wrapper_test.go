package cliwrappers_test

import (
	"runtime"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

func TestWrapperCmd_Wrap(t *testing.T) {
	g := NewWithT(t)

	t.Run("should wrap with separator", func(t *testing.T) {
		w := cliwrappers.NewWrapperCmd("unshare", "--user")
		name, args := w.Wrap("buildah", []string{"build", "."})

		g.Expect(name).To(Equal("unshare"))
		g.Expect(args).To(Equal([]string{"--user", "--", "buildah", "build", "."}))
	})

	t.Run("wrapper with no args should still add separator", func(t *testing.T) {
		w := cliwrappers.NewWrapperCmd("sudo")
		name, args := w.Wrap("buildah", []string{"build"})

		g.Expect(name).To(Equal("sudo"))
		g.Expect(args).To(Equal([]string{"--", "buildah", "build"}))
	})

	t.Run("empty wrapper should be a no-op", func(t *testing.T) {
		var w cliwrappers.WrapperCmd
		name, args := w.Wrap("buildah", []string{"build"})

		g.Expect(name).To(Equal("buildah"))
		g.Expect(args).To(Equal([]string{"build"}))
	})
}

func TestWrapperCmd_WithArgs(t *testing.T) {
	g := NewWithT(t)

	t.Run("should append extra args", func(t *testing.T) {
		w := cliwrappers.NewWrapperCmd("unshare", "--user")
		w2 := w.WithArgs("--map-auto")

		name, args := w2.Wrap("buildah", []string{"build"})

		g.Expect(name).To(Equal("unshare"))
		g.Expect(args).To(Equal([]string{"--user", "--map-auto", "--", "buildah", "build"}))
	})

	t.Run("should not mutate the original", func(t *testing.T) {
		w := cliwrappers.NewWrapperCmd("unshare", "--user")
		_ = w.WithArgs("--map-auto")

		name, args := w.Wrap("buildah", []string{"build"})

		g.Expect(name).To(Equal("unshare"))
		g.Expect(args).To(Equal([]string{"--user", "--", "buildah", "build"}))
	})
}

func TestWrapperCmd_CheckExists(t *testing.T) {
	g := NewWithT(t)

	t.Run("should succeed for existing tool", func(t *testing.T) {
		var executable string
		if runtime.GOOS == "windows" {
			executable = "cmd"
		} else {
			executable = "sh"
		}
		w := cliwrappers.NewWrapperCmd(executable)

		err := w.CheckExists()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should error for nonexistent tool", func(t *testing.T) {
		w := cliwrappers.NewWrapperCmd("nonexistent-executable")
		err := w.CheckExists()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("executable not found in PATH: nonexistent-executable"))
	})
}

func TestJoinWrappers(t *testing.T) {
	g := NewWithT(t)

	t.Run("two wrappers", func(t *testing.T) {
		w1 := cliwrappers.NewWrapperCmd("buildah", "unshare")
		w2 := cliwrappers.NewWrapperCmd("unshare", "--user")
		nested := cliwrappers.JoinWrappers(w1, w2)

		name, args := nested.Wrap("buildah", []string{"build"})

		g.Expect(name).To(Equal("buildah"))
		g.Expect(args).To(Equal([]string{"unshare", "--", "unshare", "--user", "--", "buildah", "build"}))
	})

	t.Run("single wrapper", func(t *testing.T) {
		w := cliwrappers.NewWrapperCmd("unshare", "--user")
		nested := cliwrappers.JoinWrappers(w)

		name, args := nested.Wrap("buildah", []string{"build"})
		g.Expect(name).To(Equal("unshare"))
		g.Expect(args).To(Equal([]string{"--user", "--", "buildah", "build"}))
	})

	t.Run("empty wrappers nested with non-empty", func(t *testing.T) {
		var empty cliwrappers.WrapperCmd
		w := cliwrappers.NewWrapperCmd("unshare", "--user")
		nested := cliwrappers.JoinWrappers(empty, w, empty)

		name, args := nested.Wrap("buildah", []string{"build"})
		g.Expect(name).To(Equal("unshare"))
		g.Expect(args).To(Equal([]string{"--user", "--", "buildah", "build"}))
	})
}
