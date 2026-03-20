package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var BuildImageIndexParamsConfig = map[string]common.Parameter{
	"image": {
		Name:       "image",
		ShortName:  "i",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_IMAGE",
		TypeKind:   reflect.String,
		Usage:      "The target image and tag where the image will be pushed to.",
		Required:   true,
	},
	"images": {
		Name:       "images",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_IMAGES",
		TypeKind:   reflect.Slice,
		Usage:      "List of Image Manifests to be referenced by the Image Index.",
		Required:   true,
	},
	"tls-verify": {
		Name:         "tls-verify",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_IMAGE_INDEX_TLS_VERIFY",
		TypeKind:     reflect.Bool,
		DefaultValue: "true",
		Usage:        "Verify the TLS on the registry endpoint (for push/pull to a non-TLS registry).",
	},
	"buildah-format": {
		Name:         "buildah-format",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_IMAGE_INDEX_BUILDAH_FORMAT",
		TypeKind:     reflect.String,
		DefaultValue: "oci",
		Usage:        "The format for the resulting image's mediaType. Valid values are oci (default) or docker.",
	},
	"always-build-index": {
		Name:         "always-build-index",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_IMAGE_INDEX_ALWAYS_BUILD_INDEX",
		TypeKind:     reflect.Bool,
		DefaultValue: "true",
		Usage:        "Force creation of image index even with a single image.",
	},
	"additional-tags": {
		Name:       "additional-tags",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_ADDITIONAL_TAGS",
		TypeKind:   reflect.Slice,
		Usage:      "Additional tags to push the image index to (e.g., taskrun name, commit sha).",
	},
	"output-manifest-path": {
		Name:       "output-manifest-path",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_OUTPUT_MANIFEST_PATH",
		TypeKind:   reflect.String,
		Usage:      "Path where the manifest JSON will be written for SBOM generation.",
	},
	"result-path-image-digest": {
		Name:       "result-path-image-digest",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_RESULT_PATH_IMAGE_DIGEST",
		TypeKind:   reflect.String,
		Usage:      "Write the image digest into this file.",
	},
	"result-path-image-url": {
		Name:       "result-path-image-url",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_RESULT_PATH_IMAGE_URL",
		TypeKind:   reflect.String,
		Usage:      "Write the image URL into this file.",
	},
	"result-path-image-ref": {
		Name:       "result-path-image-ref",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_RESULT_PATH_IMAGE_REF",
		TypeKind:   reflect.String,
		Usage:      "Write the image reference (with digest) into this file.",
	},
	"result-path-images": {
		Name:       "result-path-images",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_INDEX_RESULT_PATH_IMAGES",
		TypeKind:   reflect.String,
		Usage:      "Write the comma-separated list of platform images into this file.",
	},
}

type BuildImageIndexParams struct {
	Image                 string   `paramName:"image"`
	Images                []string `paramName:"images"`
	TLSVerify             bool     `paramName:"tls-verify"`
	BuildahFormat         string   `paramName:"buildah-format"`
	AlwaysBuildIndex      bool     `paramName:"always-build-index"`
	AdditionalTags        []string `paramName:"additional-tags"`
	OutputManifestPath    string   `paramName:"output-manifest-path"`
	ResultPathImageDigest string   `paramName:"result-path-image-digest"`
	ResultPathImageURL    string   `paramName:"result-path-image-url"`
	ResultPathImageRef    string   `paramName:"result-path-image-ref"`
	ResultPathImages      string   `paramName:"result-path-images"`
}

type BuildImageIndexResults struct {
	// Digest of the image just built (e.g., "sha256:abc123...")
	ImageDigest string `json:"image_digest"`
	// Image repository and tag where the built image was pushed (e.g., "quay.io/org/repo:tag")
	ImageURL string `json:"image_url"`
	// Image reference of the built image containing both the repository and the digest (e.g., "quay.io/org/repo@sha256:abc123...")
	ImageRef string `json:"image_ref"`
	// Comma-separated list of all referenced image manifests with digests (e.g., "repo@sha256:aaa,repo@sha256:bbb")
	Images string `json:"images"`
}

type BuildImageIndexCliWrappers struct {
	BuildahCli cliwrappers.BuildahCliInterface
}

type BuildImageIndex struct {
	Params        *BuildImageIndexParams
	CliWrappers   BuildImageIndexCliWrappers
	Results       BuildImageIndexResults
	ResultsWriter common.ResultsWriterInterface

	imageName   string
	imageDigest string
	images      []string
}

func NewBuildImageIndex(cmd *cobra.Command) (*BuildImageIndex, error) {
	params := &BuildImageIndexParams{}
	if err := common.ParseParameters(cmd, BuildImageIndexParamsConfig, params); err != nil {
		return nil, err
	}

	buildImageIndex := &BuildImageIndex{
		Params:        params,
		ResultsWriter: common.NewResultsWriter(),
	}

	if err := buildImageIndex.initCliWrappers(); err != nil {
		return nil, err
	}

	return buildImageIndex, nil
}

func (c *BuildImageIndex) initCliWrappers() error {
	executor := cliwrappers.NewCliExecutor()

	buildahCli, err := cliwrappers.NewBuildahCli(executor)
	if err != nil {
		return err
	}
	c.CliWrappers.BuildahCli = buildahCli

	return nil
}

func (c *BuildImageIndex) Run() error {
	common.LogParameters(BuildImageIndexParamsConfig, c.Params)

	if err := c.validateParams(); err != nil {
		return err
	}

	c.imageName = common.GetImageName(c.Params.Image)

	if err := c.buildManifestIndex(); err != nil {
		return fmt.Errorf("failed to build image index: %w", err)
	}

	c.Results.ImageDigest = c.imageDigest
	c.Results.ImageURL = c.Params.Image
	c.Results.ImageRef = c.imageName + "@" + c.imageDigest
	c.Results.Images = strings.Join(c.images, ",")

	if resultsJson, err := c.ResultsWriter.CreateResultJson(c.Results); err == nil {
		fmt.Print(resultsJson)
	} else {
		return fmt.Errorf("failed to create results JSON: %w", err)
	}

	// Write individual results to files if paths are provided
	if c.Params.ResultPathImageDigest != "" {
		if err := c.ResultsWriter.WriteResultString(c.Results.ImageDigest, c.Params.ResultPathImageDigest); err != nil {
			return fmt.Errorf("failed to write image digest result: %w", err)
		}
	}

	if c.Params.ResultPathImageURL != "" {
		if err := c.ResultsWriter.WriteResultString(c.Results.ImageURL, c.Params.ResultPathImageURL); err != nil {
			return fmt.Errorf("failed to write image URL result: %w", err)
		}
	}

	if c.Params.ResultPathImageRef != "" {
		if err := c.ResultsWriter.WriteResultString(c.Results.ImageRef, c.Params.ResultPathImageRef); err != nil {
			return fmt.Errorf("failed to write image ref result: %w", err)
		}
	}

	if c.Params.ResultPathImages != "" {
		if err := c.ResultsWriter.WriteResultString(c.Results.Images, c.Params.ResultPathImages); err != nil {
			return fmt.Errorf("failed to write images result: %w", err)
		}
	}

	return nil
}

func (c *BuildImageIndex) buildManifestIndex() error {
	l.Logger.Infof("Creating manifest list: %s", c.Params.Image)
	err := c.CliWrappers.BuildahCli.ManifestCreate(&cliwrappers.BuildahManifestCreateArgs{
		ManifestName: c.Params.Image,
	})
	if err != nil {
		return err
	}

	for _, imageRef := range c.Params.Images {
		// Special case: single image with always-build-index=false
		if !c.Params.AlwaysBuildIndex && len(c.Params.Images) == 1 {
			l.Logger.Info("Skipping image index generation. Returning results for single image.")
			c.images = []string{imageRef}
			c.imageDigest = common.GetImageDigest(imageRef)
			return nil
		}

		l.Logger.Infof("Adding image to manifest: %s", imageRef)
		err = c.CliWrappers.BuildahCli.ManifestAdd(&cliwrappers.BuildahManifestAddArgs{
			ManifestName: c.Params.Image,
			ImageRef:     imageRef,
			All:          true,
		})
		if err != nil {
			return fmt.Errorf("failed to add image %s: %w", imageRef, err)
		}
	}

	manifestJson, err := c.CliWrappers.BuildahCli.ManifestInspect(&cliwrappers.BuildahManifestInspectArgs{
		ManifestName: c.Params.Image,
	})
	if err != nil {
		return err
	}

	l.Logger.Info("Validating format consistency")
	if err := c.validateFormatConsistency(manifestJson); err != nil {
		return err
	}

	l.Logger.Infof("Pushing image index to registry: %s", c.Params.Image)

	digest, err := c.CliWrappers.BuildahCli.ManifestPush(&cliwrappers.BuildahManifestPushArgs{
		ManifestName: c.Params.Image,
		Destination:  "docker://" + c.Params.Image,
		Format:       c.Params.BuildahFormat,
		TLSVerify:    c.Params.TLSVerify,
	})
	if err != nil {
		return fmt.Errorf("failed to push manifest: %w", err)
	}

	c.imageDigest = digest
	l.Logger.Infof("Manifest pushed successfully with digest: %s", digest)

	if len(c.Params.AdditionalTags) > 0 {
		for _, tag := range c.Params.AdditionalTags {
			additionalImage := c.imageName + ":" + tag
			l.Logger.Infof("Pushing manifest to additional tag: %s", additionalImage)

			_, err := c.CliWrappers.BuildahCli.ManifestPush(&cliwrappers.BuildahManifestPushArgs{
				ManifestName: c.Params.Image,
				Destination:  "docker://" + additionalImage,
				Format:       c.Params.BuildahFormat,
				TLSVerify:    c.Params.TLSVerify,
			})
			if err != nil {
				return fmt.Errorf("failed to push manifest to additional tag %s: %w", additionalImage, err)
			}
			l.Logger.Infof("Manifest pushed successfully to %s", additionalImage)
		}
	}

	platformImages, err := c.extractPlatformImages(manifestJson)
	if err != nil {
		return fmt.Errorf("failed to extract platform images: %w", err)
	}
	c.images = platformImages

	if c.Params.OutputManifestPath != "" {
		if err := os.WriteFile(c.Params.OutputManifestPath, []byte(manifestJson), 0644); err != nil {
			return fmt.Errorf("failed to write manifest file: %w", err)
		}
		l.Logger.Infof("Manifest data saved to %s", c.Params.OutputManifestPath)
	}

	return nil
}

func (c *BuildImageIndex) validateParams() error {
	imageName := common.GetImageName(c.Params.Image)
	if !common.IsImageNameValid(imageName) {
		return fmt.Errorf("image name '%s' is invalid", c.Params.Image)
	}

	if err := common.ValidateImageHasTagOrDigest(c.Params.Image); err != nil {
		return fmt.Errorf("invalid image parameter: %w", err)
	}

	if len(c.Params.Images) == 0 {
		return fmt.Errorf("at least one image must be provided via --images")
	}

	// Validate each image reference and check for duplicates
	seenImages := make(map[string]bool)
	for _, img := range c.Params.Images {
		imgName := common.GetImageName(img)
		if !common.IsImageNameValid(imgName) {
			return fmt.Errorf("invalid image reference: %s", img)
		}

		if err := common.ValidateImageHasTagOrDigest(img); err != nil {
			return fmt.Errorf("invalid image parameter: %w", err)
		}

		// Check for duplicates
		if seenImages[img] {
			return fmt.Errorf("duplicate image reference: %s", img)
		}
		seenImages[img] = true
	}

	for _, tag := range c.Params.AdditionalTags {
		if !common.IsImageTagValid(tag) {
			return fmt.Errorf("invalid additional tag: %s", tag)
		}
	}

	validFormats := map[string]bool{"oci": true, "docker": true}
	if !validFormats[c.Params.BuildahFormat] {
		return fmt.Errorf("format must be 'oci' or 'docker', got '%s'", c.Params.BuildahFormat)
	}

	return nil
}

func (c *BuildImageIndex) validateFormatConsistency(manifestJson string) error {
	var manifest struct {
		Manifests []struct {
			MediaType string `json:"mediaType"`
		} `json:"manifests"`
	}

	if err := json.Unmarshal([]byte(manifestJson), &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	// Determine incompatible format string based on target format
	incompatibleString := "vnd.oci.image.manifest"
	incompatibleName := "oci"
	if c.Params.BuildahFormat == "oci" {
		incompatibleString = "vnd.docker.distribution.manifest"
		incompatibleName = "docker"
	}

	// Check if any manifest has incompatible format
	for _, m := range manifest.Manifests {
		if strings.Contains(m.MediaType, incompatibleString) {
			return fmt.Errorf(
				"platform image contains %s format, but index will be %s. "+
					"This will cause digest changes and break SBOM accessibility. "+
					"Ensure all platform images are built with buildah format: %s",
				incompatibleName, c.Params.BuildahFormat, c.Params.BuildahFormat)
		}
	}

	return nil
}

// extractPlatformImages extracts platform image references from the manifest list JSON.
// Returns a list of image references in the format: <index-repository>@<platform-manifest-digest>
//
// Note: The OCI/Docker manifest list spec does not preserve the original repository names
// of the platform images that were added to the index. Therefore, all returned image references
// use the index repository name (c.imageName), not the original platform image repository names.
//
// For example, if platform images were pushed as:
//   - quay.io/myapp-platform1@sha256:aaa...
//   - quay.io/myapp-platform2@sha256:bbb...
//
// The returned references will be:
//   - quay.io/myapp@sha256:aaa...
//   - quay.io/myapp@sha256:bbb...
func (c *BuildImageIndex) extractPlatformImages(manifestJson string) ([]string, error) {
	l.Logger.Infof("DEBUG: Full manifest JSON:\n%s", manifestJson)
	var manifest struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}

	if err := json.Unmarshal([]byte(manifestJson), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	var platformImages []string
	for _, m := range manifest.Manifests {
		platformImages = append(platformImages, c.imageName+"@"+m.Digest)
	}

	return platformImages, nil
}
