package commands

import (
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
	c.logParams()

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

func (c *ApplyTags) logParams() {
	l.Logger.Infof("[param] Image URL: %s", c.Params.ImageUrl)
	l.Logger.Infof("[param] Image digest: %s", c.Params.Digest)
	if len(c.Params.NewTags) > 0 {
		l.Logger.Infof("[param] Tags: %s", strings.Join(c.Params.NewTags, ", "))
	}
	if c.Params.LabelWithTags != "" {
		l.Logger.Infof("[param] image label: %s", c.Params.LabelWithTags)
	}
}

// retrieveTagsFromImageLabel fetches list of tags from the given image label and interprets the results.
func (c *ApplyTags) retrieveTagsFromImageLabel(labelName string) ([]string, error) {
	if labelName == "" {
		l.Logger.Debug("Label with additional tags is not set")
		return nil, nil
	}

	inspectArgs := &cliWrappers.SkopeoInspectArgs{
		ImageRef:   c.imageByDigest,
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
