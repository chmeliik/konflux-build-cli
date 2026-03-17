package prefetch_dependencies

import (
	"encoding/json"
	"fmt"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/konflux-ci/konflux-build-cli/pkg/logger"

	"github.com/spf13/cobra"
)

var log = logger.Logger.WithField("logger", "PrefetchDependencies")

type PrefetchDependencies struct {
	Config     *Params
	HermetoCli cliwrappers.HermetoCliInterface
}

func New(cmd *cobra.Command) (*PrefetchDependencies, error) {
	config := Params{}
	if err := common.ParseParameters(cmd, ParamsConfig, &config); err != nil {
		return nil, err
	}

	executor := cliwrappers.NewCliExecutor()
	hermetoCli, err := cliwrappers.NewHermetoCli(executor)
	if err != nil {
		return nil, err
	}

	prefetchDependencies := PrefetchDependencies{Config: &config, HermetoCli: hermetoCli}
	return &prefetchDependencies, nil
}

func (pd *PrefetchDependencies) Run() error {
	common.LogParameters(ParamsConfig, pd.Config)

	if err := pd.HermetoCli.Version(); err != nil {
		return fmt.Errorf("hermeto --version command failed: %w", err)
	}

	if pd.Config.Input == "" {
		log.Warn("No input provided; skipping prefetch-dependencies")
		return nil
	}

	if err := dropGoProxyFrom(pd.Config.ConfigFile); err != nil {
		return fmt.Errorf("failed to drop Go proxy from config file: %w", err)
	}

	if err := setupGitBasicAuth(pd.Config.GitAuthDirectory, pd.Config.SourceDir); err != nil {
		return fmt.Errorf("failed to setup Git authentication: %w", err)
	}

	decodedJSONInput := parseInput(pd.Config.Input)
	if containsRPM(decodedJSONInput) {
		defer unregisterSubscriptionManager()

		modifiedInput, err := injectRPMInput(decodedJSONInput, pd.Config.RHSMOrg, pd.Config.RHSMActivationKey)
		if err != nil {
			return fmt.Errorf("failed to inject RPM input: %w", err)
		}
		decodedJSONInput = modifiedInput
	}

	encodedJSONInput, err := json.Marshal(decodedJSONInput)
	if err != nil {
		return err
	}

	log.Debugf("Using modified input for Hermeto:\n%s", string(encodedJSONInput))

	fetchDepsParams := cliwrappers.HermetoFetchDepsParams{
		SourceDir:  pd.Config.SourceDir,
		OutputDir:  pd.Config.OutputDir,
		Input:      string(encodedJSONInput),
		ConfigFile: pd.Config.ConfigFile,
		SBOMFormat: pd.Config.SBOMFormat,
		Mode:       pd.Config.Mode,
	}
	if err := pd.HermetoCli.FetchDeps(&fetchDepsParams); err != nil {
		return fmt.Errorf("hermeto fetch-deps command failed: %w", err)
	}

	generateEnvParams := cliwrappers.HermetoGenerateEnvParams{
		OutputDir:    pd.Config.OutputDir,
		ForOutputDir: pd.Config.OutputDirMountPoint,
		Format:       "env",
		Output:       pd.Config.EnvFile,
	}
	if err := pd.HermetoCli.GenerateEnv(&generateEnvParams); err != nil {
		return fmt.Errorf("hermeto generate-env command failed: %w", err)
	}

	injectFilesParams := cliwrappers.HermetoInjectFilesParams{
		OutputDir:    pd.Config.OutputDir,
		ForOutputDir: pd.Config.OutputDirMountPoint,
	}
	if err := pd.HermetoCli.InjectFiles(&injectFilesParams); err != nil {
		return fmt.Errorf("hermeto inject-files command failed: %w", err)
	}

	if err := renameRepoFiles(pd.Config.OutputDir); err != nil {
		return fmt.Errorf("failed to rename hermeto.repo files: %w", err)
	}

	return nil
}
