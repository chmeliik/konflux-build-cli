package image

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var BuildImageIndexCmd = &cobra.Command{
	Use:   "build-image-index",
	Short: "Build a multi-architecture image index",
	Long: `Build a multi-architecture image index (manifest list) from multiple platform-specific images.

This command combines multiple container images into a single image index, enabling
multi-platform container image support.

Examples:
  # Build an image index from multiple platform images
  konflux-build-cli image build-image-index \
    --image quay.io/myorg/myapp:latest \
    --images quay.io/myorg/myapp@sha256:amd64digest... quay.io/myorg/myapp@sha256:arm64digest...

  # Build and push to additional tags (e.g., TaskRun name, commit SHA)
  konflux-build-cli image build-image-index \
    --image quay.io/myorg/myapp:latest \
    --images quay.io/myorg/myapp@sha256:amd64digest... quay.io/myorg/myapp@sha256:arm64digest... \
    --additional-tags taskrun-xyz-12345 commit-abc123

  # Write results to files (useful for Tekton tasks)
  konflux-build-cli image build-image-index \
    --image quay.io/myorg/myapp:latest \
    --images quay.io/myorg/myapp@sha256:amd64digest... quay.io/myorg/myapp@sha256:arm64digest... \
    --result-path-image-digest /tekton/results/IMAGE_DIGEST \
    --result-path-image-url /tekton/results/IMAGE_URL \
    --result-path-image-ref /tekton/results/IMAGE_REF \
    --result-path-images /tekton/results/IMAGES
`,
	Run: func(cmd *cobra.Command, args []string) {
		l.Logger.Debug("Starting build-image-index")
		buildImageIndex, err := commands.NewBuildImageIndex(cmd)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := buildImageIndex.Run(); err != nil {
			l.Logger.Fatal(err)
		}
		l.Logger.Debug("Finished build-image-index")
	},
}

func init() {
	common.RegisterParameters(BuildImageIndexCmd, commands.BuildImageIndexParamsConfig)
}
