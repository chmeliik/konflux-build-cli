package cmd

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/cmd/image"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "A sub command group to work with container images",
}

func init() {
	imageCmd.AddCommand(image.ApplyTagsCmd)
	imageCmd.AddCommand(image.BuildCmd)
}
