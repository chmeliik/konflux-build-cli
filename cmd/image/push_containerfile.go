package image

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var PushContainerfileCmd = &cobra.Command{
	Use:   "push-containerfile",
	Short: "Discover Containerfile from source code and push it to registry as an OCI artifact.",
	Long: `Pushes Containerfile to image registry as an OCI artifact.

Containerfile is auto-detected from the source by default. It is searched
firstly from build context, then the source directory. Dockerfile is supported
as a fallback. If neither is found command exits as normal without pushing
anyting. The search is highly customizable with arguments --source, --context
and --containerfile.`,
	Example: `
  # Push source/Containerfile as artifact quay.io/org/app:sha256-1234567.containerfile
  konflux-build-cli image push-containerfile --image-url quay.io/org/app --image-digest sha256:1234567 --source source

  # Push source/Containerfile as artifact quay.io/org/app:sha256-1234567.containerfile with custom artifact type
  konflux-build-cli image push-containerfile --image-url quay.io/org/app --image-digest sha256:1234567 \
    --source source --artifact-type application/vnd.my-org.containerfile

  # Push source/db/Containerfile as artifact quay.io/org/app:sha256-1234567.containerfile, build context is db/
  konflux-build-cli image push-containerfile --image-url quay.io/org/app --image-digest sha256:1234567 \
    --source source --context db

  # Push source/containerfiles/db as artifact quay.io/org/app:sha256-1234567.containerfile, build context is db/
  konflux-build-cli image push-containerfile --image-url quay.io/org/app --image-digest sha256:1234567 \
    --source source --context db --containerfile containerfiles/db

  # Push source/Dockerfile as artifact quay.io/org/app:sha256-1234567.dockerfile
  konflux-build-cli image push-containerfile --image-url quay.io/org/app --image-digest sha256:1234567 \
    --source source --tag-suffix .dockerfile

  # Push source/Containerfile as artifact quay.io/org/app:sha256-1234567.containerfile by passing absolute source path
  konflux-build-cli image push-containerfile --image-url quay.io/org/app --image-digest sha256:1234567 \
    --source /path/to/source
`,
	Run: func(cmd *cobra.Command, args []string) {
		l.Logger.Debug("Starting push-containerfile")
		pushContainerfile, err := commands.NewPushContainerfile(cmd)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := pushContainerfile.Run(); err != nil {
			l.Logger.Fatal(err)
		}
		l.Logger.Debug("Finished push-containerfile")
	},
}

func init() {
	common.RegisterParameters(PushContainerfileCmd, commands.PushContainerfileParamsConfig)
}
