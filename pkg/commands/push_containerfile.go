package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const (
	containerfileArtifactTagSuffix = ".containerfile"
	containerfileArtifactType      = "application/vnd.konflux.containerfile"
	containerfileContext           = "."

	// Max length of a tag - length of sha256 digest: 128 - 71
	// Refer to https://github.com/opencontainers/distribution-spec/blob/main/spec.md#pulling-manifests
	tagSuffixRegex = "^[a-zA-Z0-9._-]{1,57}$"
)

var PushContainerfileParamsConfig = map[string]common.Parameter{
	"image-url": {
		Name:       "image-url",
		ShortName:  "i",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_IMAGE_URL",
		TypeKind:   reflect.String,
		Usage:      "Binary image URL. Containerfile is pushed to the image repository where this binary image is.",
		Required:   true,
	},
	"image-digest": {
		Name:       "image-digest",
		ShortName:  "d",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_IMAGE_DIGEST",
		TypeKind:   reflect.String,
		Usage:      "Digest of the built binary image represented by argument --image-url. It is used to construct the tag of Containerfile image.",
		Required:   true,
	},
	"containerfile": {
		Name:       "containerfile",
		ShortName:  "f",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_CONTAINERFILE",
		TypeKind:   reflect.String,
		Usage:      "Path to Containerfile relative to source repository root. If not specified, Containerfile is searched from context then the source directory. Fallback to search Dockerfile if no Containerfile is found.",
		Required:   false,
	},
	"context": {
		Name:         "context",
		ShortName:    "c",
		EnvVarName:   "KBC_PUSH_CONTAINERFILE_CONTEXT",
		TypeKind:     reflect.String,
		DefaultValue: containerfileContext,
		Usage:        "Build context used to search Containerfile in.",
		Required:     false,
	},
	"tag-suffix": {
		Name:         "tag-suffix",
		ShortName:    "t",
		EnvVarName:   "KBC_PUSH_CONTAINERFILE_TAG_SUFFIX",
		TypeKind:     reflect.String,
		DefaultValue: containerfileArtifactTagSuffix,
		Usage:        "Suffix to construct artifact image tag",
		Required:     false,
	},
	"artifact-type": {
		Name:         "artifact-type",
		ShortName:    "a",
		EnvVarName:   "KBC_PUSH_CONTAINERFILE_ARTIFACT_TYPE",
		TypeKind:     reflect.String,
		DefaultValue: containerfileArtifactType,
		Usage:        "Artifact type of the Containerfile artifact image.",
		Required:     false,
	},
	"source": {
		Name:       "source",
		ShortName:  "s",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_SOURCE",
		TypeKind:   reflect.String,
		Usage:      "Path to a directory containing the source code.",
		Required:   true,
	},
	"result-path-image-ref": {
		Name:       "result-path-image-ref",
		ShortName:  "r",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_RESULT_PATH_IMAGE_REF",
		TypeKind:   reflect.String,
		Usage:      "Write digested image reference of the pushed Containerfile image into this file.",
		Required:   false,
	},
}

type PushContainerfileParams struct {
	ImageUrl           string `paramName:"image-url"`
	ImageDigest        string `paramName:"image-digest"`
	Containerfile      string `paramName:"containerfile"`
	Context            string `paramName:"context"`
	TagSuffix          string `paramName:"tag-suffix"`
	ArtifactType       string `paramName:"artifact-type"`
	Source             string `paramName:"source"`
	ResultPathImageRef string `paramName:"result-path-image-ref"`
}

type PushContainerfileResults struct {
	ImageRef string `json:"image_ref"`
}

type PushContainerfileCliWrappers struct {
	OrasCli cliwrappers.OrasCliInterface
}

type PushContainerfile struct {
	Params        *PushContainerfileParams
	CliWrappers   PushContainerfileCliWrappers
	Results       PushContainerfileResults
	ResultsWriter common.ResultsWriterInterface

	imageName string
}

func NewPushContainerfile(cmd *cobra.Command) (*PushContainerfile, error) {
	params := &PushContainerfileParams{}
	if err := common.ParseParameters(cmd, PushContainerfileParamsConfig, params); err != nil {
		return nil, err
	}
	pushContainerfile := &PushContainerfile{
		Params:        params,
		ResultsWriter: common.NewResultsWriter(),
	}
	if err := pushContainerfile.initCliWrappers(); err != nil {
		return nil, err
	}
	return pushContainerfile, nil
}

func (c *PushContainerfile) initCliWrappers() error {
	executor := cliwrappers.NewCliExecutor()
	orasCli, err := cliwrappers.NewOrasCli(executor)
	if err != nil {
		return err
	}
	c.CliWrappers.OrasCli = orasCli
	return nil
}

func (c *PushContainerfile) Run() error {
	c.logParams()

	imageUrl := c.Params.ImageUrl
	c.imageName = common.GetImageName(imageUrl)

	if err := c.validateParams(); err != nil {
		return err
	}

	curDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Error getting current directory: %w", err)
	}
	l.Logger.Debugf("Using current directory: %s\n", curDir)

	containerfilePath, err := common.SearchDockerfile(common.DockerfileSearchOpts{
		SourceDir:  c.Params.Source,
		ContextDir: c.Params.Context,
		Dockerfile: c.Params.Containerfile,
	})
	if err != nil {
		return fmt.Errorf("Error on searching Container: %w", err)
	}

	if containerfilePath == "" {
		l.Logger.Infof("Containerfile '%s' is not found from source '%s' and context '%s'. Abort push.",
			c.Params.Containerfile, c.Params.Source, c.Params.Context)
		return nil
	}

	l.Logger.Debugf("Got Containerfile: %s", containerfilePath)

	l.Logger.Debugf("Select registry authentication for %s", imageUrl)
	registryAuth, err := common.SelectRegistryAuthFromDefaultAuthFile(imageUrl)
	if err != nil {
		return fmt.Errorf("Cannot select registry authentication for image %s: %w", imageUrl, err)
	}

	registryConfigFile, err := os.CreateTemp("", "oras-push-registry-config-*")
	if err != nil {
		return fmt.Errorf("Error on creating temporary file for registry config: %w", err)
	}
	_, err = registryConfigFile.WriteString(fmt.Sprintf(`{"auths":{"%s":{"auth":"%s"}}}`, registryAuth.Registry, registryAuth.Token))
	if err != nil {
		return fmt.Errorf("Error on writing registry config file: %w", err)
	}
	if err = registryConfigFile.Close(); err != nil {
		return fmt.Errorf("Error on closing registry config file after write: %w", err)
	}
	defer os.Remove(registryConfigFile.Name())

	tag := c.generateContainerfileImageTag()

	absContainerfilePath, err := filepath.Abs(containerfilePath)
	if err != nil {
		return fmt.Errorf("Error on getting absolute path of %s: %w", containerfilePath, err)
	}

	os.Chdir(filepath.Dir(absContainerfilePath))
	defer os.Chdir(curDir)

	stdout, _, err := c.CliWrappers.OrasCli.Push(&cliwrappers.OrasPushArgs{
		ArtifactType:     c.Params.ArtifactType,
		RegistryConfig:   registryConfigFile.Name(),
		Format:           "go-template",
		Template:         "{{.reference}}",
		DestinationImage: fmt.Sprintf("%s:%s", c.imageName, tag),
		FileName:         filepath.Base(absContainerfilePath),
	})
	if err != nil {
		return fmt.Errorf("Error on pushing Containerfile %s: %w", containerfilePath, err)
	}

	l.Logger.Infof("Containerfile '%s' is pushed to registry with tag: %s", containerfilePath, tag)

	artifactImageRef := strings.TrimSpace(stdout)

	c.Results.ImageRef = artifactImageRef
	if resultsJson, err := c.ResultsWriter.CreateResultJson(c.Results); err != nil {
		return fmt.Errorf("Error on creating results JSON: %w", err)
	} else {
		fmt.Print(resultsJson)
	}

	if c.Params.ResultPathImageRef != "" {
		err = c.ResultsWriter.WriteResultString(artifactImageRef, c.Params.ResultPathImageRef)
		if err != nil {
			return fmt.Errorf("Error on writing result image digest: %w", err)
		}
	}

	return nil
}

func (c *PushContainerfile) generateContainerfileImageTag() string {
	digest := strings.Replace(c.Params.ImageDigest, ":", "-", 1)
	return digest + c.Params.TagSuffix
}

func (c *PushContainerfile) validateParams() error {
	if !common.IsImageNameValid(c.imageName) {
		return fmt.Errorf("image name '%s' is invalid", c.imageName)
	}

	if !common.IsImageDigestValid(c.Params.ImageDigest) {
		return fmt.Errorf("image digest '%s' is invalid", c.Params.ImageDigest)
	}

	tagSuffix := c.Params.TagSuffix
	if !regexp.MustCompile(tagSuffixRegex).MatchString(tagSuffix) {
		return fmt.Errorf("Tag suffix includes invalid characters or exceeds the max length of 57 characters.")
	}

	return nil
}

func (c *PushContainerfile) logParams() {
	l.Logger.Infof("[param] Image URL: %s", c.Params.ImageUrl)
	l.Logger.Infof("[param] Image digest: %s", c.Params.ImageDigest)
	l.Logger.Infof("[param] Tag suffix: %s", c.Params.TagSuffix)
	l.Logger.Infof("[param] Containerfile: %s", c.Params.Containerfile)
	l.Logger.Infof("[param] Context: %s", c.Params.Context)
	l.Logger.Infof("[param] Artifact type: %s", c.Params.ArtifactType)
	l.Logger.Infof("[param] Source directory: %s", c.Params.Source)
	if c.Params.ResultPathImageRef != "" {
		l.Logger.Infof("[param] Image Reference result file: %s", c.Params.ResultPathImageRef)
	}
}
