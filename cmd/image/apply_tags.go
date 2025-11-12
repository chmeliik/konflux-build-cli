package image

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var ApplyTagsCmd = &cobra.Command{
	Use:   "apply-tags",
	Short: "Creates more tags for the provided image",
	Long: `Creates more tags for the provided image.

It might be useful when, for example, the build produces hash based tag, but 'latest' or some other tags needed.

Tags can be defined in two ways:
 - via tags parameter
 - via image label in the base image (see --tags-from-image-label parameter)
Both ways can be used together.
`,
	Run: func(cmd *cobra.Command, args []string) {
		l.Logger.Debug("Starting apply-tags")
		applyTags, err := commands.NewApplyTags(cmd)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := applyTags.Run(); err != nil {
			l.Logger.Fatal(err)
		}
		l.Logger.Debug("Finished apply-tags")
	},
}

func init() {
	common.RegisterParameters(ApplyTagsCmd, commands.ApplyTagsParamsConfig)
}
