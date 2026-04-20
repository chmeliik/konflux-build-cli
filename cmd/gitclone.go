package cmd

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands/gitclone"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var gitCloneCmd = &cobra.Command{
	Use:   "git-clone",
	Short: "Clone a git repository",
	Long: `Clone a git repository with support for submodules, sparse checkout,
authentication, and optional merge with a target branch.`,
	Example: `  # Clone a repository
  kbc git-clone --url https://github.com/user/repo.git

  # Clone a specific revision with shallow depth
  kbc git-clone --url https://github.com/user/repo.git --revision main --depth 1

  # Clone with sparse checkout (only specific directories)
  kbc git-clone --url https://github.com/user/repo.git --sparse-checkout-directories "src,docs"

  # Clone and merge target branch (for PR testing)
  kbc git-clone --url https://github.com/user/repo.git --revision feature-branch --merge-target-branch --target-branch main`,
	Run: func(cmd *cobra.Command, args []string) {
		l.Logger.Debug("Starting git-clone")
		gitClone, err := gitclone.New(cmd)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := gitClone.Run(); err != nil {
			l.Logger.Fatal(err)
		}
		l.Logger.Debug("Finished git-clone")
	},
}

func init() {
	common.RegisterParameters(gitCloneCmd, gitclone.ParamsConfig)
}
