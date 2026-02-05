package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const (
	dockerfileArtifactTagSuffix = ".dockerfile"
	dockerfileArtifactType      = "application/vnd.konflux.dockerfile"
	dockerfileContext           = "."
	dockerfileFilePath          = "./Dockerfile"

	// Max length of a tag - length of sha256 digest: 128 - 71
	// Refer to https://github.com/opencontainers/distribution-spec/blob/main/spec.md#pulling-manifests
	tagSuffixRegex = "^[a-zA-Z0-9._-]{1,57}$"
)

var PushDockerfileParamsConfig = map[string]common.Parameter{
	"image-url": {
		Name:       "image-url",
		ShortName:  "i",
		EnvVarName: "KBC_PUSH_DOCKERFILE_IMAGE_URL",
		TypeKind:   reflect.String,
		Usage:      "Binary image URL. Dockerfile is pushed to the image repository where this binary image is.",
		Required:   true,
	},
	"image-digest": {
		Name:       "image-digest",
		ShortName:  "d",
		EnvVarName: "KBC_PUSH_DOCKERFILE_IMAGE_DIGEST",
		TypeKind:   reflect.String,
		Usage:      "Binary image digest, which is used to construct the tag of Dockerfile image.",
		Required:   true,
	},
	"dockerfile": {
		Name:         "dockerfile",
		ShortName:    "f",
		EnvVarName:   "KBC_PUSH_DOCKERFILE_DOCKERFILE_PATH",
		TypeKind:     reflect.String,
		DefaultValue: dockerfileFilePath,
		Usage:        fmt.Sprintf("Path to Dockerfile relative to source repository root. Defaults to '%s'.", dockerfileFilePath),
		Required:     false,
	},
	"context": {
		Name:         "context",
		ShortName:    "c",
		EnvVarName:   "KBC_PUSH_DOCKERFILE_CONTEXT",
		TypeKind:     reflect.String,
		DefaultValue: dockerfileContext,
		Usage:        fmt.Sprintf("Build context used to search Dockerfile. Defaults to '%s'.", dockerfileContext),
		Required:     false,
	},
	"tag-suffix": {
		Name:         "tag-suffix",
		ShortName:    "t",
		EnvVarName:   "KBC_PUSH_DOCKERFILE_TAG_SUFFIX",
		TypeKind:     reflect.String,
		DefaultValue: dockerfileArtifactTagSuffix,
		Usage:        "Suffix to construct artifact image tag. Defaults to '.dockerfile'.",
		Required:     false,
	},
	"artifact-type": {
		Name:         "artifact-type",
		ShortName:    "a",
		EnvVarName:   "KBC_PUSH_DOCKERFILE_ARTIFACT_TYPE",
		TypeKind:     reflect.String,
		DefaultValue: dockerfileArtifactType,
		Usage:        fmt.Sprintf("Artifact type of the dockerfile artifact image. Defaults to '%s'.", dockerfileArtifactType),
		Required:     false,
	},
	"source": {
		Name:       "source",
		ShortName:  "s",
		EnvVarName: "KBC_PUSH_DOCKERFILE_SOURCE",
		TypeKind:   reflect.String,
		Usage:      "Directory containing the source code. It is a relative path to the root of current working directory.",
		Required:   true,
	},
	"image-ref-result-file": {
		Name:       "image-ref-result-file",
		ShortName:  "r",
		EnvVarName: "KBC_PUSH_DOCKERFILE_RESULT_IMAGE_REF",
		TypeKind:   reflect.String,
		Usage:      "Write digested image reference of the pushed Dockerfile image into this file.",
		Required:   false,
	},
}

type PushDockerfileParams struct {
	ImageUrl           string `paramName:"image-url"`
	ImageDigest        string `paramName:"image-digest"`
	Dockerfile         string `paramName:"dockerfile"`
	Context            string `paramName:"context"`
	TagSuffix          string `paramName:"tag-suffix"`
	ArtifactType       string `paramName:"artifact-type"`
	Source             string `paramName:"source"`
	ImageRefResultFile string `paramName:"image-ref-result-file"`
}

type PushDockerfileResults struct {
	ImageRef string `json:"image_ref"`
}

type PushDockerfile struct {
	Params        *PushDockerfileParams
	OrasClient    common.OrasClientInterface
	Results       PushDockerfileResults
	ResultsWriter common.ResultsWriterInterface

	imageName string
}

func NewPushDockerfile(cmd *cobra.Command) (*PushDockerfile, error) {
	params := &PushDockerfileParams{}
	if err := common.ParseParameters(cmd, PushDockerfileParamsConfig, params); err != nil {
		return nil, err
	}
	pushDockerfile := &PushDockerfile{
		Params:        params,
		OrasClient:    common.NewOrasClient(),
		ResultsWriter: common.NewResultsWriter(),
	}
	return pushDockerfile, nil
}

func (c *PushDockerfile) Run() error {
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

	dockerfilePath, err := common.SearchDockerfile(common.DockerfileSearchOpts{
		SourceDir:  c.Params.Source,
		ContextDir: c.Params.Context,
		Dockerfile: c.Params.Dockerfile,
	})
	if err != nil {
		return fmt.Errorf("Error on searching Dockerfile: %w", err)
	}

	if dockerfilePath == "" {
		l.Logger.Debugf("Dockerfile '%s' is not found from source '%s' and context '%s'. Abort push.",
			c.Params.Dockerfile, c.Params.Source, c.Params.Context)
		return nil
	}

	l.Logger.Debugf("Got Dockerfile: %s", dockerfilePath)

	l.Logger.Debugf("Select registry authentication for %s\n", imageUrl)
	registryAuth, err := common.SelectRegistryAuthFromDefaultAuthFile(imageUrl)
	if err != nil {
		return fmt.Errorf("Cannot select registry authentication for image %s: %w", imageUrl, err)
	}

	username, password, err := common.ExtractCredentials(registryAuth.Token)
	if err != nil {
		return fmt.Errorf("Error on extracting authentication credential: %w", err)
	}

	tag := c.generateDockerfileImageTag()

	absDockerfilePath, err := filepath.Abs(dockerfilePath)
	if err != nil {
		return fmt.Errorf("Error on getting absolute path of %s: %w", dockerfilePath, err)
	}
	remoteRepo := common.NewRepository(c.imageName, username, password)
	digest, err := c.OrasClient.Push(remoteRepo, tag, absDockerfilePath, c.Params.ArtifactType)
	if err != nil {
		return fmt.Errorf("Failed to push Dockerfile %s: %w", dockerfilePath, err)
	}

	l.Logger.Debugf("Dockerfile '%s' is pushed to registry with tag: %s\n", dockerfilePath, tag)

	artifactImageRef := fmt.Sprintf("%s@%s", c.imageName, digest)

	c.Results.ImageRef = artifactImageRef
	if resultsJson, err := c.ResultsWriter.CreateResultJson(c.Results); err != nil {
		return fmt.Errorf("Error on creating results JSON: %w", err)
	} else {
		fmt.Print(resultsJson)
	}

	if c.Params.ImageRefResultFile != "" {
		err = c.ResultsWriter.WriteResultString(artifactImageRef, c.Params.ImageRefResultFile)
		if err != nil {
			return fmt.Errorf("Error on writing result image digest: %w", err)
		}
	}

	return nil
}

func (c *PushDockerfile) generateDockerfileImageTag() string {
	digest := strings.Replace(c.Params.ImageDigest, ":", "-", 1)
	return digest + c.Params.TagSuffix
}

func (c *PushDockerfile) validateParams() error {
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

func (c *PushDockerfile) logParams() {
	l.Logger.Infof("[param] Image URL: %s", c.Params.ImageUrl)
	l.Logger.Infof("[param] Image digest: %s", c.Params.ImageDigest)
	l.Logger.Infof("[param] Tag suffix: %s", c.Params.TagSuffix)
	l.Logger.Infof("[param] Dockerfile: %s", c.Params.Dockerfile)
	l.Logger.Infof("[param] Context: %s", c.Params.Context)
	l.Logger.Infof("[param] Artifact type: %s", c.Params.ArtifactType)
	l.Logger.Infof("[param] Source directory: %s", c.Params.Source)
	l.Logger.Infof("[param] Image Reference result file: %s", c.Params.ImageRefResultFile)
}
