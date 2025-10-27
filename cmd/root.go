package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "konflux-build-cli",
	Short: "A helper CLI tool for Konflux build pipelines",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	processedArgs := common.ExpandArrayParameters(os.Args[1:])
	rootCmd.SetArgs(processedArgs)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Common flags for all subcommands
	var logLevel string
	rootCmd.PersistentFlags().StringVar(&logLevel, "loglevel", "info", "Set the logging level (debug, info, warn, error, fatal)")

	cobra.OnInitialize(func() {
		if err := l.InitLogger(logLevel); err != nil {
			fmt.Printf("failed to init logger: %s", err.Error())
			os.Exit(2)
		}
	})
}
