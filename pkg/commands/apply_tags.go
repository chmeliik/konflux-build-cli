package commands

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var ApplyTagsParamsConfig = map[string]common.Parameter{
	"image-url": {
		Name:       "image-url",
		ShortName:  "i",
		EnvVarName: "KBC_APPLY_TAGS_IMAGE_URL",
		TypeKind:   reflect.String,
		Usage:      "Image name to add tags to. Tag and digest are ignored. Required.",
		Required:   true,
	},
	"digest": {
		Name:       "digest",
		ShortName:  "d",
		EnvVarName: "KBC_APPLY_TAGS_IMAGE_DIGEST",
		TypeKind:   reflect.String,
		Usage:      "Image digest to add tags to. Required.",
		Required:   true,
	},
	"tags": {
		Name:         "tags",
		ShortName:    "t",
		EnvVarName:   "KBC_APPLY_TAGS",
		TypeKind:     reflect.Array,
		DefaultValue: "",
		Usage:        "Tags to add to the given image",
	},
	"tags-from-image-label": {
		Name:         "tags-from-image-label",
		ShortName:    "l",
		EnvVarName:   "KBC_APPLY_TAGS_FROM_IMAGE_LABEL",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Image label name to add tags from. Tags are comma or whitespace separated in the label value.",
	},
}

type ApplyTagsParams struct {
	ImageUrl      string   `paramName:"image-url"`
	Digest        string   `paramName:"digest"`
	NewTags       []string `paramName:"tags"`
	LabelWithTags string   `paramName:"tags-from-image-label"`
}

type ApplyTagsCliWrappers struct {
	SkopeoCli cliWrappers.SkopeoCliInterface
}

type ApplyTagsResults struct {
	Tags []string `json:"tags"`
}

type ApplyTags struct {
	Params        *ApplyTagsParams
	CliWrappers   ApplyTagsCliWrappers
	Results       ApplyTagsResults
	ResultsWriter common.ResultsWriterInterface

	imageName     string
	imageByDigest string
}

func NewApplyTags(cmd *cobra.Command) (*ApplyTags, error) {
	applyTags := &ApplyTags{}

	params := &ApplyTagsParams{}
	if err := common.ParseParameters(cmd, ApplyTagsParamsConfig, params); err != nil {
		return nil, err
	}
	applyTags.Params = params

	if err := applyTags.initCliWrappers(); err != nil {
		return nil, err
	}

	applyTags.ResultsWriter = common.NewResultsWriter()

	return applyTags, nil
}

func (c *ApplyTags) initCliWrappers() error {
	executor := cliWrappers.NewCliExecutor()

	skopeoCli, err := cliWrappers.NewSkopeoCli(executor)
	if err != nil {
		return err
	}
	c.CliWrappers.SkopeoCli = skopeoCli
	return nil
}

// Run executes the command logic.
func (c *ApplyTags) Run() error {
	common.LogParameters(ApplyTagsParamsConfig, c.Params)

	c.imageName = common.GetImageName(c.Params.ImageUrl)
	if err := c.validateParams(); err != nil {
		return err
	}

	c.imageByDigest = c.imageName + "@" + c.Params.Digest

	tagsFromLabel, err := c.retrieveTagsFromImageLabel(c.Params.LabelWithTags)
	if err != nil {
		return err
	}

	tags := append(c.Params.NewTags, tagsFromLabel...)
	l.Logger.Debugf("Tags to create: %s", strings.Join(tags, ", "))

	if err := c.applyTags(tags); err != nil {
		return err
	}

	c.Results.Tags = tags

	if resultJson, err := c.ResultsWriter.CreateResultJson(c.Results); err == nil {
		fmt.Print(resultJson)
	} else {
		l.Logger.Errorf("failed to create results json: %s", err.Error())
		return err
	}

	return nil
}

// retrieveTagsFromImageLabel fetches list of tags from the given image label.
// In fact, two skopeo invocations are needed (and this is optimal way):
//  1. Read the raw reference data (light request) to see if we have image manifest or image index.
//     In case of an image index, get image manifest for any architecture (we need only labels).
//  2. Perform actual inspect request on the image manifest.
func (c *ApplyTags) retrieveTagsFromImageLabel(labelName string) ([]string, error) {
	type imageManifest struct {
		MediaType string `json:"mediaType,omitempty"`
		Digest    string `json:"digest,omitempty"`
	}
	type imageIndexManifest struct {
		MediaType string          `json:"mediaType,omitempty"`
		Manifests []imageManifest `json:"manifests,omitempty"`
	}

	if labelName == "" {
		l.Logger.Debug("Label with additional tags is not set")
		return nil, nil
	}

	// Do the raw inspect of the image to get image manifest digest for the inspection.
	rawInspectArgs := &cliWrappers.SkopeoInspectArgs{
		ImageRef:   c.imageByDigest,
		Raw:        true,
		RetryTimes: 3,
	}
	rawManifest, err := c.CliWrappers.SkopeoCli.Inspect(rawInspectArgs)
	if err != nil {
		l.Logger.Errorf("failed to inspect %s image manifest, cause: %s", c.imageByDigest, err.Error())
		return nil, err
	}
	imageIndex := &imageIndexManifest{}
	if err := json.Unmarshal([]byte(rawManifest), imageIndex); err != nil {
		l.Logger.Errorf("failed to unmarshall image manifest for %s, cause: %s", c.imageByDigest, err.Error())
		return nil, err
	}

	// Image reference to inspect labels onto.
	targetImageReference := ""

	if strings.Contains(imageIndex.MediaType, ".index.") || strings.Contains(imageIndex.MediaType, ".manifest.list.") {
		// Provided by user reference is image index, e.g. "application/vnd.oci.image.index.v1+json"
		// Pick image with arbitrary architecture for the target reference.
		digest := ""
		for _, manifest := range imageIndex.Manifests {
			if strings.Contains(manifest.MediaType, ".manifest.") {
				digest = manifest.Digest
				break
			}
		}
		if digest == "" {
			// The index doesn't contain an image manifest, print warning and proceed.
			l.Logger.Warnf("image index %s does not contain an image manifest", c.imageByDigest)
			return nil, nil
		}
		targetImageReference = c.imageName + "@" + digest
	} else if strings.Contains(imageIndex.MediaType, ".manifest.") {
		// Provided by user reference is image manifest, e.g. "application/vnd.docker.distribution.manifest.v2+json"
		targetImageReference = c.imageByDigest
	} else {
		// Not supported OCI image type, print warning and proceed.
		l.Logger.Warnf("unsupported OCI image type: %s in %s", imageIndex.MediaType, c.imageByDigest)
		return nil, nil
	}

	// Perform inspect on the target image manifest
	inspectArgs := &cliWrappers.SkopeoInspectArgs{
		ImageRef:   targetImageReference,
		Format:     fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName),
		RetryTimes: 3,
		NoTags:     true,
	}
	tagsLabelValue, err := c.CliWrappers.SkopeoCli.Inspect(inspectArgs)
	if err != nil {
		if strings.Contains(err.Error(), cliWrappers.UnsupportedOCIConfigMediaType) {
			// Skip the label with tags for unsupported config media type.
			// Print warning message and continue.
			l.Logger.Warnf("unsupported config media type '%s' of input image. Skipping reading %s image label",
				cliWrappers.UnsupportedOCIConfigMediaType, c.Params.LabelWithTags)
			return nil, nil
		}
		l.Logger.Errorf("failed to retrieve tags from '%s' label value: %s", c.Params.LabelWithTags, err.Error())
		return nil, err
	}
	tagsLabelValue = strings.TrimSpace(tagsLabelValue)
	l.Logger.Debugf("Tags label value: %s", tagsLabelValue)

	if tagsLabelValue == "" {
		l.Logger.Warnf("No tags given in '%s' image label", c.Params.LabelWithTags)
		return nil, nil
	}

	tagSeparatorRegex := regexp.MustCompile(`[\s,]+`)
	tagsFromLabel := tagSeparatorRegex.Split(tagsLabelValue, -1)

	// Successfully obtained tags from the image label
	// Validate the obtained tags
	for _, tag := range tagsFromLabel {
		if !common.IsImageTagValid(tag) {
			return nil, fmt.Errorf("tag from label '%s' is invalid", tag)
		}
	}

	if len(tagsFromLabel) > 0 {
		l.Logger.Infof("Additional tags from '%s' image label: %s", c.Params.LabelWithTags, strings.Join(tagsFromLabel, ", "))
	}

	return tagsFromLabel, nil
}

func (c *ApplyTags) applyTags(tags []string) error {
	args := &cliWrappers.SkopeoCopyArgs{
		SourceImage: c.imageByDigest,
		MultiArch:   cliWrappers.SkopeoCopyArgMultiArchIndexOnly,
		RetryTimes:  3,
	}

	for _, tag := range tags {
		l.Logger.Debugf("Creating tag: %s", tag)

		args.DestinationImage = c.imageName + ":" + tag
		if err := c.CliWrappers.SkopeoCli.Copy(args); err != nil {
			l.Logger.Errorf("failed to push '%s' tag: %s", tag, err.Error())
			return err
		}

		l.Logger.Debugf("Tag '%s' pushed", tag)
	}

	return nil
}

func (c *ApplyTags) validateParams() error {
	// Validate imageName instead of Params.ImageUrl to avoid calling normalizeImageName second time.
	if !common.IsImageNameValid(c.imageName) {
		return fmt.Errorf("image '%s' is invalid", c.imageName)
	}

	if !common.IsImageDigestValid(c.Params.Digest) {
		return fmt.Errorf("image digest '%s' is invalid", c.Params.Digest)
	}

	for _, tag := range c.Params.NewTags {
		if !common.IsImageTagValid(tag) {
			return fmt.Errorf("tag '%s' is invalid", tag)
		}
	}

	if c.Params.LabelWithTags != "" && !c.isImageLabelNameValid(c.Params.LabelWithTags) {
		return fmt.Errorf("image label name '%s' is invalid", c.Params.LabelWithTags)
	}

	return nil
}

// isImageLabelNameValid checks if label key for docker image is valid.
// Image label name can contain lowercase letters and digits plus underscore, period, dash and slash.
// Image label should start and end with a letter.
// Double separator is not allowed.
// Image label max length is 256 characters.
func (c *ApplyTags) isImageLabelNameValid(imageLabelName string) bool {
	if len(imageLabelName) == 0 || len(imageLabelName) > 256 {
		return false
	}
	doubleSeparatorPattern := `[/._-]{2}`
	doubleSeparatorRegex := regexp.MustCompile(doubleSeparatorPattern)
	if doubleSeparatorRegex.MatchString(imageLabelName) {
		return false
	}
	imageLabelNamePattern := `^[a-z](?:[a-z0-9/._-]*[a-z])$`
	imageLabelNameRegex := regexp.MustCompile(imageLabelNamePattern)
	return imageLabelNameRegex.MatchString(imageLabelName)
}
