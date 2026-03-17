package cmd

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/cmd/internal"
)

var internalCmdGroup = &cobra.Command{
	Use:    "internal",
	Short:  "Internal subcommands, not intended for direct use",
	Hidden: true,
}

func init() {
	internalCmdGroup.AddCommand(internal.InUserNamespaceCmd)
}
