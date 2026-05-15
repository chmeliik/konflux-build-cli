package integration_tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

const ApplyTagsImage = "quay.io/konflux-ci/task-runner:1.7.0@sha256:1c0582a85dae0949f3ec94aca95695c5d7698b371f1b76c5a78822a6249073dc"

const KonfluxAdditionalTagsLabelName = "konflux.additional-tags"

type ApplyTagsParams struct {
	ImageRepoUrl string
	ImageDigest  string
	Tags         []string
}

func RunApplyTags(applyTagsParams ApplyTagsParams, imageRegistry ImageRegistry) error {
	var err error

	container := NewBuildCliRunnerContainer("apply-tags", ApplyTagsImage)
	defer container.DeleteIfExists()

	err = container.StartWithRegistryIntegration(imageRegistry)
	if err != nil {
		return err
	}

	// Construct the apply-tags arguments
	args := []string{"image", "apply-tags"}
	args = append(args, "--image-url", applyTagsParams.ImageRepoUrl)
	args = append(args, "--digest", applyTagsParams.ImageDigest)
	if len(applyTagsParams.Tags) > 0 {
		args = append(args, "--tags")
		args = append(args, applyTagsParams.Tags...)
	}
	args = append(args, "--tags-from-image-label", KonfluxAdditionalTagsLabelName)

	err = container.ExecuteBuildCli(args...)
	if err != nil {
		return err
	}

	return nil
}

func TestApplyTags(t *testing.T) {
	SetupGomega(t)
	var err error

	// Setup registry
	imageRegistry := NewImageRegistry()
	err = imageRegistry.Prepare()
	Expect(err).ToNot(HaveOccurred())
	err = imageRegistry.Start()
	Expect(err).ToNot(HaveOccurred())
	defer imageRegistry.Stop()

	// Create input data
	imageRepoUrl := imageRegistry.GetTestNamespace() + "test-image"
	newTagsFromLabel := []string{"label-tag-1", "label-tag-2"}
	newTag := time.Now().Format("2006-01-02_15-04-05")
	newTagsFromArg := []string{newTag, "test"}

	// Create base image for the test
	err = CreateTestImage(TestImageConfig{
		ImageRef: imageRepoUrl,
		Labels: map[string]string{
			KonfluxAdditionalTagsLabelName: strings.Join(newTagsFromLabel, " "),
			QuayExpiresAfterLabelName:      "1h",
		},
		RandomDataSize: 10 * 1024,
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(imageRepoUrl)
	imageDigest, err := PushImage(imageRepoUrl)
	Expect(err).ToNot(HaveOccurred())

	// Run the command
	applyTagsParams := ApplyTagsParams{
		ImageRepoUrl: imageRepoUrl,
		ImageDigest:  imageDigest,
		Tags:         newTagsFromArg,
	}
	err = RunApplyTags(applyTagsParams, imageRegistry)
	Expect(err).ToNot(HaveOccurred())

	// Check the result
	for _, tag := range append(newTagsFromArg, newTagsFromLabel...) {
		tagExists, err := imageRegistry.CheckTagExistence(imageRepoUrl, tag)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to check for %s tag existence", tag))
		Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s:%s to exist", imageRepoUrl, tag))
	}
}

func TestApplyTagsWithImageIndex(t *testing.T) {
	SetupGomega(t)
	var err error

	// Setup registry
	imageRegistry := NewImageRegistry()
	err = imageRegistry.Prepare()
	Expect(err).ToNot(HaveOccurred())
	err = imageRegistry.Start()
	Expect(err).ToNot(HaveOccurred())
	defer imageRegistry.Stop()

	// Create input data
	imageRepoUrl := imageRegistry.GetTestNamespace() + "test-image-index"
	newTag := time.Now().Format("2006-01-02_15-04-05")
	newTagsFromLabel := []string{"arch-label-tag-" + newTag}
	newTagsFromArg := []string{newTag}

	// Do not create linux/amd64 to test foreign arch scenario
	arches := []string{"linux/s390x", "linux/ppc64le"}
	images := make([]string, len(arches))
	imagesDigestInIndex := make([]string, len(arches))
	for i, arch := range arches {
		imageRef := fmt.Sprintf("%s:%s", imageRepoUrl, strings.ReplaceAll(arch, "/", "-"))
		err := CreateTestImage(TestImageConfig{
			ImageRef: imageRef,
			Platform: arch,
			Labels: map[string]string{
				KonfluxAdditionalTagsLabelName: strings.Join(newTagsFromLabel, " "),
				QuayExpiresAfterLabelName:      "1h",
			},
			RandomDataSize: 10 * 1024,
		})
		Expect(err).ToNot(HaveOccurred())
		defer DeleteLocalImage(imageRef)
		digest, err := PushImage(imageRef)
		Expect(err).ToNot(HaveOccurred())

		images[i] = imageRef
		imagesDigestInIndex[i] = digest
	}

	indexRef := imageRepoUrl + ":index"
	indexDigest, err := CreateAndPushImageIndex(indexRef, images)
	Expect(err).ToNot(HaveOccurred())

	// Run the command
	applyTagsParams := ApplyTagsParams{
		ImageRepoUrl: imageRepoUrl,
		ImageDigest:  indexDigest,
		Tags:         newTagsFromArg,
	}
	err = RunApplyTags(applyTagsParams, imageRegistry)
	Expect(err).ToNot(HaveOccurred())

	// Check the result
	for _, tag := range append(newTagsFromArg, newTagsFromLabel...) {
		tagExists, err := imageRegistry.CheckTagExistence(imageRepoUrl, tag)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to check for %s tag existence", tag))
		Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s:%s to exist", imageRepoUrl, tag))

		// We need to be sure that the tag is applied to the image index, not a specific image.
		// Because podman doesn't allow getting image index digest, check if the tag refers to the image index.
		imageIndexInfo, err := imageRegistry.GetImageIndexInfo(imageRepoUrl, tag)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get image index %s:%s", imageRepoUrl, tag))
		Expect(imageIndexInfo.MediaType).To(BeElementOf([]string{
			"application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json"}),
			"Created reference is not an image index")
		Expect(imageIndexInfo.Manifests).To(HaveLen(len(arches)))
		obtainedDigests := make([]string, 0, len(arches))
		for _, manifestInfo := range imageIndexInfo.Manifests {
			Expect(manifestInfo.MediaType).To(BeElementOf([]string{
				"application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json"}))
			obtainedDigests = append(obtainedDigests, manifestInfo.Digest)
		}
		// Check that all image manifests included in the image index match.
		Expect(obtainedDigests).To(ConsistOf(imagesDigestInIndex))
	}
}
