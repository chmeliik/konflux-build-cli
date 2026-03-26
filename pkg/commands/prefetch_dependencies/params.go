package prefetch_dependencies

import (
	"reflect"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
)

var ParamsConfig = map[string]common.Parameter{
	"input": {
		Name:         "input",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_INPUT",
		DefaultValue: "",
		Usage:        "input data specifying package managers and various configuration",
		Required:     false,
	},
	"source-dir": {
		Name:         "source-dir",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_SOURCE_DIR",
		DefaultValue: ".",
		Usage:        "directory with the source code",
		Required:     false,
	},
	"output-dir": {
		Name:         "output-dir",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_OUTPUT_DIR",
		DefaultValue: "./prefetch-output",
		Usage:        "directory where prefetched dependencies and SBOM will be stored",
		Required:     false,
	},
	"config-file": {
		Name:         "config-file",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_CONFIG_FILE",
		DefaultValue: "",
		Usage:        "path to YAML configuration file for Hermeto",
		Required:     false,
	},
	"sbom-format": {
		Name:         "sbom-format",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_SBOM_FORMAT",
		DefaultValue: "spdx",
		Usage:        "SBOM format to generate (spdx or cyclonedx)",
		Required:     false,
	},
	"mode": {
		Name:         "mode",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_MODE",
		DefaultValue: "strict",
		Usage:        "how to handle input requirements: strict (fail) or permissive (warn)",
		Required:     false,
	},
	"output-dir-mount-point": {
		Name:         "output-dir-mount-point",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_OUTPUT_DIR_MOUNT_POINT",
		DefaultValue: "/tmp",
		Usage:        "directory where output directory will be mounted in a container for hermetic build",
		Required:     false,
	},
	"env-files": {
		Name:         "env-files",
		TypeKind:     reflect.Slice,
		EnvVarName:   "KBC_PD_ENV_FILES",
		DefaultValue: "./prefetch.env",
		Usage:        "paths to files where environment variables for hermetic build will be written, format is inferred from file suffix",
		Required:     false,
	},
	"rhsm-org": {
		Name:         "rhsm-org",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_RHSM_ORG",
		DefaultValue: "",
		Usage:        "path to file containing Red Hat Subscription Manager organization ID",
		Required:     false,
	},
	"rhsm-activation-key": {
		Name:         "rhsm-activation-key",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_RHSM_ACTIVATION_KEY",
		DefaultValue: "",
		Usage:        "path to file containing Red Hat Subscription Manager activation key",
		Required:     false,
	},
	"git-auth-directory": {
		Name:         "git-auth-directory",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_PD_GIT_AUTH_DIRECTORY",
		DefaultValue: "",
		Usage:        "directory with git auth credentials (.git-credentials, .gitconfig or username/password)",
		Required:     false,
	},
	"enable-package-registry-proxy": { // Pipeline-level registry proxy switch.
		Name:         "enable-package-registry-proxy",
		EnvVarName:   "KBC_PD_ENABLE_PACKAGE_REGISTRY_PROXY",
		TypeKind:     reflect.Bool,
		Usage:        "Pipeline-level enable package registry proxy. Defaults to true.",
		DefaultValue: "true", // A pipeline will use a proxy unless explicitly told otherwise.
		Required:     false,
	},
}

type Params struct {
	Input                      string   `paramName:"input"`
	SourceDir                  string   `paramName:"source-dir"`
	OutputDir                  string   `paramName:"output-dir"`
	ConfigFile                 string   `paramName:"config-file"`
	SBOMFormat                 string   `paramName:"sbom-format"`
	Mode                       string   `paramName:"mode"`
	OutputDirMountPoint        string   `paramName:"output-dir-mount-point"`
	EnvFiles                   []string `paramName:"env-files"`
	RHSMOrg                    string   `paramName:"rhsm-org"`
	RHSMActivationKey          string   `paramName:"rhsm-activation-key"`
	GitAuthDirectory           string   `paramName:"git-auth-directory"`
	EnablePackageRegistryProxy bool     `paramName:"enable-package-registry-proxy"`
}
