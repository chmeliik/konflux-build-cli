package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"

	"github.com/keilerkonzept/dockerfile-json/pkg/dockerfile"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var BuildParamsConfig = map[string]common.Parameter{
	"containerfile": {
		Name:         "containerfile",
		ShortName:    "f",
		EnvVarName:   "KBC_BUILD_CONTAINERFILE",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Path to Containerfile. If not specified, uses Containerfile/Dockerfile from the context directory.",
	},
	"context": {
		Name:         "context",
		ShortName:    "c",
		EnvVarName:   "KBC_BUILD_CONTEXT",
		TypeKind:     reflect.String,
		DefaultValue: ".",
		Usage:        "Build context directory.",
	},
	"output-ref": {
		Name:       "output-ref",
		ShortName:  "t",
		EnvVarName: "KBC_BUILD_OUTPUT_REF",
		TypeKind:   reflect.String,
		Usage:      `The reference of the output image - [registry/namespace/]name[:tag]. Required.`,
		Required:   true,
	},
	"push": {
		Name:         "push",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_PUSH",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "Push the built image to the registry.",
	},
	"secret-dirs": {
		Name:       "secret-dirs",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_SECRET_DIRS",
		TypeKind:   reflect.Slice,
		Usage:      "Directories containing secret files to make available during build.",
	},
	"workdir-mount": {
		Name:         "workdir-mount",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_WORKDIR_MOUNT",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Mount the context directory (which is also the workdir) into the build with '--volume $PWD:$WORKDIR_MOUNT'.",
	},
	"build-args": {
		Name:       "build-args",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_BUILD_ARGS",
		TypeKind:   reflect.Slice,
		Usage:      "Arguments to pass to the build using buildah's --build-arg option.",
	},
	"build-args-file": {
		Name:       "build-args-file",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_BUILD_ARGS_FILE",
		TypeKind:   reflect.String,
		Usage:      "Path to a file with build arguments, see https://www.mankier.com/1/buildah-build#--build-arg-file",
	},
	"dockerfile-json-output": {
		Name:       "dockerfile-json-output",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_DOCKERFILE_JSON_OUTPUT",
		TypeKind:   reflect.String,
		Usage:      "Path to write the parsed Dockerfile JSON representation.",
	},
}

type BuildParams struct {
	Containerfile         string   `paramName:"containerfile"`
	Context               string   `paramName:"context"`
	OutputRef             string   `paramName:"output-ref"`
	Push                  bool     `paramName:"push"`
	SecretDirs            []string `paramName:"secret-dirs"`
	WorkdirMount          string   `paramName:"workdir-mount"`
	BuildArgs             []string `paramName:"build-args"`
	BuildArgsFile         string   `paramName:"build-args-file"`
	DockerfileJsonOutput  string   `paramName:"dockerfile-json-output"`
	ExtraArgs             []string // Additional arguments to pass to buildah build
}

type BuildCliWrappers struct {
	BuildahCli cliWrappers.BuildahCliInterface
}

type BuildResults struct {
	ImageUrl string `json:"image_url"`
	Digest   string `json:"digest,omitempty"`
}

type Build struct {
	Params        *BuildParams
	CliWrappers   BuildCliWrappers
	Results       BuildResults
	ResultsWriter common.ResultsWriterInterface

	containerfilePath string
	buildahSecrets    []cliWrappers.BuildahSecret
}

func NewBuild(cmd *cobra.Command, extraArgs []string) (*Build, error) {
	build := &Build{}

	params := &BuildParams{}
	if err := common.ParseParameters(cmd, BuildParamsConfig, params); err != nil {
		return nil, err
	}
	// Store any extra arguments passed after -- separator
	params.ExtraArgs = extraArgs
	build.Params = params

	if err := build.initCliWrappers(); err != nil {
		return nil, err
	}

	build.ResultsWriter = common.NewResultsWriter()

	return build, nil
}

func (c *Build) initCliWrappers() error {
	executor := cliWrappers.NewCliExecutor()

	buildahCli, err := cliWrappers.NewBuildahCli(executor)
	if err != nil {
		return err
	}
	c.CliWrappers.BuildahCli = buildahCli
	return nil
}

// Run executes the command logic.
func (c *Build) Run() error {
	c.logParams()

	if err := c.validateParams(); err != nil {
		return err
	}

	if err := c.detectContainerfile(); err != nil {
		return err
	}

	if err := c.setSecretArgs(); err != nil {
		return err
	}

	if c.Params.DockerfileJsonOutput != "" {
		if err := c.parseAndWriteDockerfile(); err != nil {
			return err
		}
	}

	if err := c.buildImage(); err != nil {
		return err
	}

	c.Results.ImageUrl = c.Params.OutputRef

	if c.Params.Push {
		digest, err := c.pushImage()
		if err != nil {
			return err
		}
		c.Results.Digest = digest
	}

	if resultJson, err := c.ResultsWriter.CreateResultJson(c.Results); err == nil {
		fmt.Print(resultJson)
	} else {
		l.Logger.Errorf("failed to create results json: %s", err.Error())
		return err
	}

	return nil
}

func (c *Build) logParams() {
	if c.Params.Containerfile != "" {
		l.Logger.Infof("[param] Containerfile: %s", c.Params.Containerfile)
	}
	l.Logger.Infof("[param] Context: %s", c.Params.Context)
	l.Logger.Infof("[param] OutputRef: %s", c.Params.OutputRef)
	l.Logger.Infof("[param] Push: %t", c.Params.Push)
	if len(c.Params.SecretDirs) > 0 {
		l.Logger.Infof("[param] SecretDirs: %v", c.Params.SecretDirs)
	}
	if c.Params.WorkdirMount != "" {
		l.Logger.Infof("[param] WorkdirMount: %s", c.Params.WorkdirMount)
	}
	if len(c.Params.BuildArgs) > 0 {
		l.Logger.Infof("[param] BuildArgs: %v", c.Params.BuildArgs)
	}
	if c.Params.BuildArgsFile != "" {
		l.Logger.Infof("[param] BuildArgsFile: %s", c.Params.BuildArgsFile)
	}
	if c.Params.DockerfileJsonOutput != "" {
		l.Logger.Infof("[param] DockerfileJsonOutput: %s", c.Params.DockerfileJsonOutput)
	}
	if len(c.Params.ExtraArgs) > 0 {
		l.Logger.Infof("[param] ExtraArgs: %v", c.Params.ExtraArgs)
	}
}

func (c *Build) validateParams() error {
	if !common.IsImageNameValid(common.GetImageName(c.Params.OutputRef)) {
		return fmt.Errorf("output-ref '%s' is invalid", c.Params.OutputRef)
	}

	if stat, err := os.Stat(c.Params.Context); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("context directory '%s' does not exist", c.Params.Context)
		}
		return fmt.Errorf("failed to stat context directory: %w", err)
	} else if !stat.IsDir() {
		return fmt.Errorf("context path '%s' is not a directory", c.Params.Context)
	}

	return nil
}

func (c *Build) detectContainerfile() error {
	if c.Params.Containerfile != "" {
		// Try the filepath as-is first
		if stat, err := os.Stat(c.Params.Containerfile); err == nil && !stat.IsDir() {
			c.containerfilePath = c.Params.Containerfile
			l.Logger.Infof("Using containerfile: %s", c.containerfilePath)
			return nil
		}

		// Fallback: try relative to context directory
		fallbackPath := filepath.Join(c.Params.Context, c.Params.Containerfile)
		if stat, err := os.Stat(fallbackPath); err == nil && !stat.IsDir() {
			c.containerfilePath = fallbackPath
			l.Logger.Infof("Using containerfile: %s", c.containerfilePath)
			return nil
		}

		return fmt.Errorf("containerfile '%s' not found", c.Params.Containerfile)
	}

	// Auto-detection: look only in context directory (same as buildah)
	candidates := []string{"Containerfile", "Dockerfile"}
	for _, candidate := range candidates {
		candidatePath := filepath.Join(c.Params.Context, candidate)
		if stat, err := os.Stat(candidatePath); err == nil && !stat.IsDir() {
			c.containerfilePath = candidatePath
			l.Logger.Infof("Auto-detected containerfile: %s", c.containerfilePath)
			return nil
		}
	}

	return fmt.Errorf("no Containerfile or Dockerfile found in context directory '%s'", c.Params.Context)
}

func (c *Build) setSecretArgs() error {
	secretDirs, err := parseSecretDirs(c.Params.SecretDirs)
	if err != nil {
		return fmt.Errorf("parsing --secret-dirs: %w", err)
	}
	buildahSecrets, err := c.processSecretDirs(secretDirs)
	if err != nil {
		return fmt.Errorf("processing --secret-dirs: %w", err)
	}
	c.buildahSecrets = buildahSecrets
	return nil
}

type secretDir struct {
	src      string
	name     string
	optional bool
}

func parseSecretDirs(secretDirArgs []string) ([]secretDir, error) {
	var secretDirs []secretDir

	for _, arg := range secretDirArgs {
		secretDir := secretDir{}
		keyValues := strings.Split(arg, ",")

		for _, kv := range keyValues {
			key, value, hasSep := strings.Cut(kv, "=")
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)

			if !hasSep {
				value = key
				key = "src"
			}

			switch key {
			case "src":
				secretDir.src = value
			case "name":
				secretDir.name = value
			case "optional":
				switch value {
				case "true":
					secretDir.optional = true
				case "false":
					secretDir.optional = false
				default:
					return nil, fmt.Errorf("invalid argument: optional=%s (expected true|false)", value)
				}
			default:
				return nil, fmt.Errorf("invalid attribute: %s", key)
			}
		}

		secretDirs = append(secretDirs, secretDir)
	}

	return secretDirs, nil
}

// processSecretDirs processes secret directories and returns buildah --secret arguments.
func (c *Build) processSecretDirs(secretDirs []secretDir) ([]cliWrappers.BuildahSecret, error) {
	var buildahSecrets []cliWrappers.BuildahSecret
	usedIDs := make(map[string]bool)

	for _, secretDir := range secretDirs {
		idPrefix := secretDir.name
		if idPrefix == "" {
			idPrefix = filepath.Base(secretDir.src)
		}

		entries, err := os.ReadDir(secretDir.src)
		if err != nil {
			if os.IsNotExist(err) && secretDir.optional {
				l.Logger.Debugf("secret directory %s doesn't exist but is marked optional, skipping", secretDir.src)
				continue
			}
			return nil, fmt.Errorf("failed to read secret directory %s: %w", secretDir.src, err)
		}

		for _, entry := range entries {
			isFile, err := isRegular(entry, secretDir.src)
			if err != nil {
				return nil, err
			}
			if !isFile {
				continue
			}

			filename := entry.Name()
			fullID := filepath.Join(idPrefix, filename)

			// Check for ID conflicts
			if usedIDs[fullID] {
				return nil, fmt.Errorf("duplicate secret ID '%s': ensure unique basename/filename combinations", fullID)
			}
			usedIDs[fullID] = true

			secretPath := filepath.Join(secretDir.src, filename)
			buildahSecrets = append(
				buildahSecrets, cliWrappers.BuildahSecret{Src: secretPath, Id: fullID},
			)

			l.Logger.Infof("Adding secret %s to the build, available with 'RUN --mount=type=secret,id=%s'", fullID, fullID)
		}
	}

	return buildahSecrets, nil
}

func isRegular(entry os.DirEntry, dir string) (bool, error) {
	t := entry.Type()
	if t.IsRegular() {
		return true, nil
	}
	if t == os.ModeSymlink {
		path := filepath.Join(dir, entry.Name())
		stat, err := os.Stat(path)
		if err != nil {
			return false, fmt.Errorf("failed to stat %s: %w", path, err)
		}
		return stat.Mode().IsRegular(), nil
	}
	return false, nil
}

func readBuildArgsFile(buildArgFilePath string) (map[string]string, error) {
	buildArgFile, err := os.ReadFile(buildArgFilePath)
	if err != nil {
		return nil, err
	}

	args := make(map[string]string)

	for i, line := range strings.Split(string(buildArgFile), "\n") {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("%s:%d: expected arg=value format", buildArgFilePath, i+1)
		}

		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		if key == "" {
			return nil, fmt.Errorf("%s:%d: empty key", buildArgFilePath, i+1)
		}

		args[key] = value
	}

	return args, nil
}

func (c *Build) createBuildArgExpander() (dockerfile.SingleWordExpander, error) {
	args := make(map[string]string)

	// Add built-in Docker platform ARGs
	// These are typically set by the build system
	args["BUILDPLATFORM"] = ""
	args["BUILDOS"] = ""
	args["BUILDARCH"] = ""
	args["BUILDVARIANT"] = ""
	args["TARGETPLATFORM"] = ""
	args["TARGETOS"] = ""
	args["TARGETARCH"] = ""
	args["TARGETVARIANT"] = ""

	// Load from --build-args-file
	if c.Params.BuildArgsFile != "" {
		fileArgs, err := readBuildArgsFile(c.Params.BuildArgsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read build args file: %w", err)
		}
		for key, value := range fileArgs {
			args[key] = value
		}
	}

	// CLI --build-args take precedence
	for _, arg := range c.Params.BuildArgs {
		// Parse KEY=VALUE or just KEY (empty value)
		key, value, found := strings.Cut(arg, "=")
		if !found {
			// Just KEY without =, set to empty string
			args[key] = ""
		} else {
			args[key] = value
		}
	}

	// Return expander function
	return func(word string) (string, error) {
		if value, ok := args[word]; ok {
			return value, nil
		}
		// Return empty string for unknown variables (best-effort expansion)
		return "", nil
	}, nil
}

func createEnvExpander() dockerfile.SingleWordExpander {
	return func(word string) (string, error) {
		// Never resolve environment variables for now
		return "", nil
	}
}

func (c *Build) buildImage() error {
	l.Logger.Info("Building container image...")

	originalCwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(c.Params.Context); err != nil {
		return fmt.Errorf("couldn't cd to context directory: %w", err)
	}
	defer os.Chdir(originalCwd)

	buildArgs := &cliWrappers.BuildahBuildArgs{
		Containerfile: c.containerfilePath,
		ContextDir:    c.Params.Context,
		OutputRef:     c.Params.OutputRef,
		Secrets:       c.buildahSecrets,
		BuildArgs:     c.Params.BuildArgs,
		BuildArgsFile: c.Params.BuildArgsFile,
		ExtraArgs:     c.Params.ExtraArgs,
	}
	if c.Params.WorkdirMount != "" {
		buildArgs.Volumes = []cliWrappers.BuildahVolume{
			{HostDir: c.Params.Context, ContainerDir: c.Params.WorkdirMount, Options: "z"},
		}
	}

	if err := buildArgs.MakePathsAbsolute(originalCwd); err != nil {
		return err
	}

	if err := c.CliWrappers.BuildahCli.Build(buildArgs); err != nil {
		return err
	}

	l.Logger.Info("Build completed successfully")
	return nil
}

func (c *Build) pushImage() (string, error) {
	l.Logger.Infof("Pushing image to registry: %s", c.Params.OutputRef)

	pushArgs := &cliWrappers.BuildahPushArgs{
		Image: c.Params.OutputRef,
	}

	digest, err := c.CliWrappers.BuildahCli.Push(pushArgs)
	if err != nil {
		return "", err
	}

	l.Logger.Info("Push completed successfully")
	l.Logger.Infof("Image digest: %s", digest)

	return digest, nil
}

func (c *Build) parseAndWriteDockerfile() error {
	l.Logger.Infof("Parsing Dockerfile: %s", c.containerfilePath)

	// Parse the Dockerfile
	df, err := dockerfile.Parse(c.containerfilePath)
	if err != nil {
		return fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	// Create build arg expander
	argExp, err := c.createBuildArgExpander()
	if err != nil {
		return fmt.Errorf("failed to create build arg expander: %w", err)
	}

	// Create env expander (placeholder that never resolves)
	envExp := createEnvExpander()

	// Expand variables in the parsed Dockerfile
	df.Expand(argExp, envExp)

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(df, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Dockerfile to JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(c.Params.DockerfileJsonOutput, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile JSON to %s: %w", c.Params.DockerfileJsonOutput, err)
	}

	l.Logger.Infof("Wrote parsed Dockerfile to: %s", c.Params.DockerfileJsonOutput)
	return nil
}
