// -> https://hermetoproject.github.io/hermeto <-

package cliwrappers

import (
	"errors"
	"os"

	"github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var log = logger.Logger.WithField("logger", "HermetoCli")

type HermetoCliInterface interface {
	Version() error
	FetchDeps(params *HermetoFetchDepsParams) error
	GenerateEnv(params *HermetoGenerateEnvParams) error
	InjectFiles(params *HermetoInjectFilesParams) error
}

type HermetoCli struct {
	Executor CliExecutorInterface
	Env      []string // constructed as expected by exec.Cmd.Env
}

func NewHermetoCli(executor CliExecutorInterface, env []string) (*HermetoCli, error) {
	hermetoCliAvailable, err := CheckCliToolAvailable("hermeto")
	if err != nil {
		return nil, err
	}

	if !hermetoCliAvailable {
		return nil, errors.New("hermeto CLI is not available")
	}

	return &HermetoCli{Executor: executor, Env: env}, nil
}

// Print the Hermeto version.
func (hc *HermetoCli) Version() error {
	args := []string{"--version"}
	_, _, _, err := hc.Executor.Execute(Cmd{Name: "hermeto", Args: args, LogOutput: true})
	return err
}

type HermetoFetchDepsParams struct {
	Input      string
	SourceDir  string
	OutputDir  string
	ConfigFile string
	SBOMFormat string
	Mode       string
}

// Run the Hermeto fetch-deps command.
func (hc *HermetoCli) FetchDeps(params *HermetoFetchDepsParams) error {
	logLevel := logger.Logger.GetLevel().String()

	args := []string{
		"--log-level",
		logLevel,
		"--mode",
		params.Mode,
	}

	// Make the config file optional.
	if params.ConfigFile != "" {
		args = append(args, "--config-file", params.ConfigFile)
	}

	args = append(
		args,
		"fetch-deps",
		params.Input,
		"--sbom-output-type",
		params.SBOMFormat,
		"--source",
		params.SourceDir,
		"--output",
		params.OutputDir,
	)

	log.Debugf("Executing %s", shellJoin("hermeto", args...))
	extendedEnv := append(os.Environ(), hc.Env...)
	_, _, _, err := hc.Executor.Execute(Cmd{Name: "hermeto", Args: args, LogOutput: true, Env: extendedEnv})
	return err
}

type HermetoGenerateEnvParams struct {
	OutputDir    string
	ForOutputDir string
	Output       string
}

// Run the Hermeto generate-env command.
func (hc *HermetoCli) GenerateEnv(params *HermetoGenerateEnvParams) error {
	logLevel := logger.Logger.GetLevel().String()

	args := []string{
		"--log-level",
		logLevel,
		"generate-env",
		params.OutputDir,
		"--for-output-dir",
		params.ForOutputDir,
		"--output",
		params.Output,
	}

	log.Debugf("Executing %s", shellJoin("hermeto", args...))
	_, _, _, err := hc.Executor.Execute(Cmd{Name: "hermeto", Args: args, LogOutput: true})
	return err
}

type HermetoInjectFilesParams struct {
	OutputDir    string
	ForOutputDir string
}

// Run the Hermeto inject-files command.
func (hc *HermetoCli) InjectFiles(params *HermetoInjectFilesParams) error {
	logLevel := logger.Logger.GetLevel().String()

	args := []string{
		"--log-level",
		logLevel,
		"inject-files",
		params.OutputDir,
		"--for-output-dir",
		params.ForOutputDir,
	}

	log.Debugf("Executing %s", shellJoin("hermeto", args...))
	_, _, _, err := hc.Executor.Execute(Cmd{Name: "hermeto", Args: args, LogOutput: true})
	return err
}
