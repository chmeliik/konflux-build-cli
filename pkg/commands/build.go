package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"

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
}

type BuildParams struct {
	Containerfile string `paramName:"containerfile"`
	Context       string `paramName:"context"`
	OutputRef     string `paramName:"output-ref"`
	Push          bool   `paramName:"push"`
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
}

func NewBuild(cmd *cobra.Command) (*Build, error) {
	build := &Build{}

	params := &BuildParams{}
	if err := common.ParseParameters(cmd, BuildParamsConfig, params); err != nil {
		return nil, err
	}
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

func (c *Build) buildImage() error {
	l.Logger.Info("Building container image...")

	buildArgs := &cliWrappers.BuildahBuildArgs{
		Containerfile: c.containerfilePath,
		ContextDir:    c.Params.Context,
		OutputRef:     c.Params.OutputRef,
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
