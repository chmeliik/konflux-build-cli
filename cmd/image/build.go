package image

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var BuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a container image",
	Long: `Build a container image using buildah.

Optionally, push the built image to a registry using the --push flag.

The command outputs the image URL and optionally the image digest (if pushing).

Examples:
  # Build using auto-detected Containerfile/Dockerfile in current directory
  konflux-build-cli image build -t quay.io/myorg/myimage:latest

  # Build and push to registry
  konflux-build-cli image build -t quay.io/myorg/myimage:latest --push

  # Build with explicit Containerfile and context
  konflux-build-cli image build -f ./Containerfile -c ./myapp -t quay.io/myorg/myimage:v1.0.0

  # Build with additional buildah arguments
  konflux-build-cli image build -t quay.io/myorg/myimage:latest -- --compat-volumes --force-rm
`,
	Run: func(cmd *cobra.Command, args []string) {
		l.Logger.Debug("Starting build")
		build, err := commands.NewBuild(cmd, args)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := build.Run(); err != nil {
			l.Logger.Fatal(err)
		}
		l.Logger.Debug("Finished build")
	},
}

func init() {
	common.RegisterParameters(BuildCmd, commands.BuildParamsConfig)
}
