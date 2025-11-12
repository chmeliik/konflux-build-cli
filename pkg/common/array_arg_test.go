package common

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestExpandArrayParameters(t *testing.T) {

	setupTestCommands := func() (*cobra.Command, *cobra.Command) {
		// Clear existing array params
		arrayParamsInCommands = map[*cobra.Command][]string{}

		rootCmd := &cobra.Command{Use: "root"}
		subCmd := &cobra.Command{Use: "subcmd"}
		rootCmd.AddCommand(subCmd)

		recordArrayParamForCommand(subCmd, "--array-param")
		recordArrayParamForCommand(subCmd, "-a")
		recordArrayParamForCommand(subCmd, "--multi")

		return rootCmd, subCmd
	}

	t.Run("should handle command without array parameters", func(t *testing.T) {
		g := NewWithT(t)

		argv := []string{"root", "command", "--param", "value"}
		result := ExpandArrayParameters(argv)

		expected := []string{"root", "command", "--param", "value"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should handle non-array parameters normally", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"root", "subcmd", "--normal-param", "value", "--other-flag"}
		result := ExpandArrayParameters(argv)

		expected := []string{"root", "subcmd", "--normal-param", "value", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should expand array parameters with multiple values", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--array-param", "val1", "val2", "val3", "--other-flag"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--array-param", "val1", "--array-param", "val2", "--array-param", "val3", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should expand short form array parameters", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "-a", "val1", "val2", "--other-flag"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "-a", "val1", "-a", "val2", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should handle array parameters with equals syntax", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--flag", "--array-param=val1", "val2", "val3", "--other-flag"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--flag", "--array-param", "val1", "--array-param", "val2", "--array-param", "val3", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should handle short form array parameters with equals syntax", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--flag", "-a=val1", "val2", "val3", "--other-flag"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--flag", "-a", "val1", "-a", "val2", "-a", "val3", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should handle array parameters with equals syntax when a paramter with equals syntax exists", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--param=val", "--array-param=val1", "val2", "val3", "-p=v"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--param=val", "--array-param", "val1", "--array-param", "val2", "--array-param", "val3", "-p=v"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should handle empty array parameters", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--param", "val", "--array-param", "--other-flag"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--param", "val", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should handle empty array parameters at the end", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--other-flag", "--array-param"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should stop processing after -- sentinel", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--array-param", "val1", "val2", "--", "other"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--array-param", "val1", "--array-param", "val2", "--", "other"}
		g.Expect(result).To(Equal(expected))
	})

	t.Run("should handle mixed array and non-array parameters", func(t *testing.T) {
		g := NewWithT(t)

		setupTestCommands()

		argv := []string{"subcmd", "--array-param", "val1", "val2", "--normal", "value", "--multi", "m1", "m2", "--other-flag"}
		result := ExpandArrayParameters(argv)

		expected := []string{"subcmd", "--array-param", "val1", "--array-param", "val2", "--normal", "value", "--multi", "m1", "--multi", "m2", "--other-flag"}
		g.Expect(result).To(Equal(expected))
	})

	// TODO improve array parameters expansion.
	// Note, that:
	// - persistant flags could have a value unless the flag is of boolen type.
	// - subcommands can also have persistant flags that could be passed even before the subcommand itself.
	// - the root command has list of only own persistant flags.
	// t.Run("should handle array parameters when global flag is passed before subcommand", func(t *testing.T) {
	// 	g := NewWithT(t)

	// 	setupTestCommands()

	// 	argv := []string{"--loglevel", "info", "subcmd", "--normal", "value", "--array-param", "val1", "val2", "--other-flag"}
	// 	result := ExpandArrayParameters(argv)

	// 	expected := []string{"--loglevel", "info", "subcmd", "--normal", "value", "--array-param", "val1", "--array-param", "val2", "--other-flag"}
	// 	g.Expect(result).To(Equal(expected))
	// })
}

func TestRecordArrayParamForCommand(t *testing.T) {

	t.Run("should record array parameter for command", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: "root"}

		// Clear any existing entries for this command
		delete(arrayParamsInCommands, cmd)

		recordArrayParamForCommand(cmd, "--test-param")
		recordArrayParamForCommand(cmd, "-t")

		params := arrayParamsInCommands[cmd]
		g.Expect(params).To(ContainElement("--test-param"))
		g.Expect(params).To(ContainElement("-t"))
		g.Expect(len(params)).To(Equal(2))
	})
}

func TestBuildArrayParamsData(t *testing.T) {

	t.Run("should build array params data correctly", func(t *testing.T) {
		g := NewWithT(t)

		// Clear existing data
		arrayParamsInCommands = map[*cobra.Command][]string{}

		rootCmd := &cobra.Command{Use: "root"}
		subCmd := &cobra.Command{Use: "sub"}
		rootCmd.AddCommand(subCmd)

		recordArrayParamForCommand(subCmd, "--array-param")
		recordArrayParamForCommand(subCmd, "-a")

		data := buildArrayParamsData()

		g.Expect(data).To(HaveLen(1))
		g.Expect(data).To(HaveKey("sub"))
		g.Expect(data["sub"]).To(ContainElement("--array-param"))
		g.Expect(data["sub"]).To(ContainElement("-a"))
	})

	t.Run("should handle nested subcommands", func(t *testing.T) {
		g := NewWithT(t)

		// Clear existing data
		arrayParamsInCommands = map[*cobra.Command][]string{}

		rootCmd := &cobra.Command{Use: "root"}
		subCmd := &cobra.Command{Use: "sub"}
		nestedCmd := &cobra.Command{Use: "nested"}
		rootCmd.AddCommand(subCmd)
		subCmd.AddCommand(nestedCmd)

		recordArrayParamForCommand(nestedCmd, "--nested-array")

		data := buildArrayParamsData()

		g.Expect(data).To(HaveLen(1))
		g.Expect(data).To(HaveKey("sub nested"))
		g.Expect(data["sub nested"]).To(ContainElement("--nested-array"))
	})
}
