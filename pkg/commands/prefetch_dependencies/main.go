package prefetch_dependencies

import (
	"encoding/json"
	"fmt"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	cfg "github.com/konflux-ci/konflux-build-cli/pkg/config"
	"github.com/konflux-ci/konflux-build-cli/pkg/logger"

	"github.com/spf13/cobra"
)

var log = logger.Logger.WithField("logger", "PrefetchDependencies")

type PrefetchDependencies struct {
	Config     *Params
	HermetoCli cliwrappers.HermetoCliInterface
}

func getPackageProxyConfiguration() ([]string, error) {
	hermetoEnv := []string{}
	konfluxConfig, err := cfg.GetKonfluxConfig()
	if err != nil {
		return hermetoEnv, err
	}
	packageProxyConfig := konfluxConfig.HermetoProxy
	if packageProxyConfig == nil {
		return hermetoEnv, nil
	}

	if !packageProxyConfig.PackageRegistryProxyAllowed {
		log.Info("Not using package registry proxy because allow-package-registry-proxy " +
			"is not set to `true` on the cluster level")
		return hermetoEnv, err
	}
	// Note that empty URLs must be sanitized here, or it would result in validation
	// error in Hermeto.
	if packageProxyConfig.NpmProxy != "" {
		envEntry := fmt.Sprintf("HERMETO_NPM__PROXY_URL=%s", packageProxyConfig.NpmProxy)
		hermetoEnv = append(hermetoEnv, envEntry)
	}
	if packageProxyConfig.YarnProxy != "" {
		envEntry := fmt.Sprintf("HERMETO_YARN__PROXY_URL=%s", packageProxyConfig.YarnProxy)
		hermetoEnv = append(hermetoEnv, envEntry)
	}

	return hermetoEnv, err
}

func New(cmd *cobra.Command) (*PrefetchDependencies, error) {
	var err error
	local_config := Params{}
	if err = common.ParseParameters(cmd, ParamsConfig, &local_config); err != nil {
		return nil, err
	}

	hermetoEnv := []string{}
	if local_config.EnablePackageRegistryProxy {
		hermetoEnv, err = getPackageProxyConfiguration()
		if err != nil {
			log.Warnf("Failed to extract Hermeto environment settings from ConfigMap: %+v", err)
		}
	} else {
		log.Info("Not using package registry proxy because enable-package-registry-proxy is not set to `true` " +
			"on the pipeline level")
	}

	executor := cliwrappers.NewCliExecutor()
	hermetoCli, err := cliwrappers.NewHermetoCli(executor, hermetoEnv)
	if err != nil {
		return nil, err
	}

	prefetchDependencies := PrefetchDependencies{Config: &local_config, HermetoCli: hermetoCli}
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
		registerRHSM := pd.Config.RHSMOrg != "" && pd.Config.RHSMActivationKey != ""
		if registerRHSM {
			if err := registerSubscriptionManager(pd.Config.RHSMOrg, pd.Config.RHSMActivationKey); err != nil {
				return fmt.Errorf("failed to register with subscription-manager: %w", err)
			}
			defer unregisterSubscriptionManager()
		}

		modifiedInput, err := injectRPMInput(decodedJSONInput, registerRHSM)
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

	for _, envFile := range pd.Config.EnvFiles {
		generateEnvParams := cliwrappers.HermetoGenerateEnvParams{
			OutputDir:    pd.Config.OutputDir,
			ForOutputDir: pd.Config.OutputDirMountPoint,
			Output:       envFile,
		}
		if err := pd.HermetoCli.GenerateEnv(&generateEnvParams); err != nil {
			return fmt.Errorf("hermeto generate-env command failed: %w", err)
		}
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
