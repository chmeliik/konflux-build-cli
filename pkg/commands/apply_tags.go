package commands

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
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

	c.imageName = c.normalizeImageName(c.Params.ImageUrl)
	if err := c.validateParams(); err != nil {
		return err
	}

	c.imageByDigest = c.imageName + "@" + c.Params.Digest

	var tagsFromLabel []string
	if c.Params.LabelWithTags != "" {
		var err error
		tagsFromLabel, err = c.retrieveTagsFromImageLabel(c.Params.LabelWithTags)
		if err != nil {
			l.Logger.Errorf("failed to retrieve tags from '%s' label value: %s", c.Params.LabelWithTags, err.Error())
			return err
		}
		for _, tag := range tagsFromLabel {
			if !c.isTagValid(tag) {
				return fmt.Errorf("tag from label '%s' is invalid", tag)
			}
		}

		if len(tagsFromLabel) > 0 {
			l.Logger.Infof("Additional tags from '%s' image label: %s", c.Params.LabelWithTags, strings.Join(tagsFromLabel, ", "))
		} else {
			l.Logger.Warnf("No tags given in '%s' image label", c.Params.LabelWithTags)
		}
	} else {
		l.Logger.Debug("Label with additional tags is not set")
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

func (c *ApplyTags) retrieveTagsFromImageLabel(labelName string) ([]string, error) {
	inspectArgs := &cliWrappers.SkopeoInspectArgs{
		ImageRef:   c.imageByDigest,
		Format:     fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName),
		RetryTimes: 3,
		NoTags:     true,
	}
	tagsLabelValue, err := c.CliWrappers.SkopeoCli.Inspect(inspectArgs)
	if err != nil {
		return nil, err
	}
	tagsLabelValue = strings.TrimSpace(tagsLabelValue)
	l.Logger.Debugf("Tags label value: %s", tagsLabelValue)

	if tagsLabelValue == "" {
		return nil, nil
	}

	tagSeparatorRegex := regexp.MustCompile(`[\s,]+`)
	tags := tagSeparatorRegex.Split(tagsLabelValue, -1)
	return tags, nil
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

// normalizeImageName returns full image name from given image reference
// by removing tag or digest or both, if any.
func (c *ApplyTags) normalizeImageName(imageURL string) string {
	digestRegex := regexp.MustCompile(`@sha256:[a-fA-F0-9]{64}$`)
	imageWithoutDigest := digestRegex.ReplaceAllString(imageURL, "")

	tagRegex := regexp.MustCompile(`:[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$`)
	image := tagRegex.ReplaceAllString(imageWithoutDigest, "")

	return image
}

func (c *ApplyTags) validateParams() error {
	// Validate imageName instead of Params.ImageUrl to avoid calling normalizeImageName second time.
	if !c.isImageNameValid(c.imageName) {
		return fmt.Errorf("image '%s' is invalid", c.imageName)
	}

	if !c.isDigestValid(c.Params.Digest) {
		return fmt.Errorf("image digest '%s' is invalid", c.Params.Digest)
	}

	for _, tag := range c.Params.NewTags {
		if !c.isTagValid(tag) {
			return fmt.Errorf("tag '%s' is invalid", tag)
		}
	}

	if c.Params.LabelWithTags != "" && !c.isImageLabelNameValid(c.Params.LabelWithTags) {
		return fmt.Errorf("image label name '%s' is invalid", c.Params.LabelWithTags)
	}

	return nil
}

// isImageNameValid validates image name without tag and digest.
// Image name can contain lowercase letters and digits plus separators: dash, period, underscore, slash.
// Image name cannot start or end with a separator.
// Image name max length is 128 characters.
// It's not allowed to have double separator and triple underscore.
func (c *ApplyTags) isImageNameValid(imageName string) bool {
	if imageName == "" {
		return false
	}
	if len(imageName) > 128 {
		return false
	}
	if strings.Contains(imageName, "___") ||
		strings.Contains(imageName, "//") ||
		strings.Contains(imageName, "..") ||
		strings.Contains(imageName, "--") ||
		strings.Contains(imageName, "_.") ||
		strings.Contains(imageName, "._") ||
		strings.Contains(imageName, "-.") ||
		strings.Contains(imageName, ".-") ||
		strings.Contains(imageName, "-_") ||
		strings.Contains(imageName, "_-") {
		return false
	}

	imagePartPattern := "^[a-z0-9](?:[a-z0-9_.-]*[a-z0-9])?$"
	imagePartRegex := regexp.MustCompile(imagePartPattern)

	parts := strings.Split(imageName, "/")
	if len(parts) == 1 {
		return imagePartRegex.MatchString(parts[0])
	}
	// Multiple path parts.
	// Handle the first part differently, since it might have a port.
	addressAndPortRegex := regexp.MustCompile(`^([a-z0-9](?:[a-z0-9_.-]*[a-z0-9])?)(?::(\d+))?$`)
	match := addressAndPortRegex.FindStringSubmatch(parts[0])
	if len(match) == 0 {
		return false
	}
	if len(match) > 1 {
		registryAddress := match[1]
		if !imagePartRegex.MatchString(registryAddress) {
			// It should never happen if both regex are correct.
			// Still keeping the check to catch regex editing mistakes.
			return false
		}
	}
	if len(match) > 2 && match[2] != "" {
		portString := match[2]
		if port, err := strconv.Atoi(portString); err == nil {
			if port < 0 || port > 65535 {
				return false
			}
		} else {
			return false
		}
	}
	// Validate the rest of the path parts the usual way.
	for _, part := range parts[1:] {
		if !imagePartRegex.MatchString(part) {
			return false
		}
	}
	return true
}

func (c *ApplyTags) isDigestValid(digest string) bool {
	digestPattern := `^sha256:[a-f0-9]{64}$`
	digestRegex := regexp.MustCompile(digestPattern)
	return digestRegex.MatchString(digest)
}

// isTagValid validates image tag.
// Image tag can contain letters and digits plus underscore, period, dash.
// Image tag cannot start with period or dash.
// Image tag max length is 128 characters.
func (c *ApplyTags) isTagValid(tag string) bool {
	tagPattern := "^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$"
	tagRegex := regexp.MustCompile(tagPattern)
	return tagRegex.MatchString(tag)
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
