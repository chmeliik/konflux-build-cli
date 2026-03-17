package cmd

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands/prefetch_dependencies"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var prefetchDependenciesCmd = &cobra.Command{
	Use:   "prefetch-dependencies",
	Short: "Prefetch project dependencies",
	Long:  "Prefetch project dependencies using Hermeto to enable hermetic container builds",
	Run: func(cmd *cobra.Command, args []string) {
		logger.Logger.Debug("Starting prefetch-dependencies")
		prefetchDependencies, err := prefetch_dependencies.New(cmd)
		if err != nil {
			logger.Logger.Fatal(err)
		}
		if err := prefetchDependencies.Run(); err != nil {
			logger.Logger.Fatal(err)
		}
		logger.Logger.Debug("Finished prefetch-dependencies")
	},
}

func init() {
	common.RegisterParameters(prefetchDependenciesCmd, prefetch_dependencies.ParamsConfig)
}
