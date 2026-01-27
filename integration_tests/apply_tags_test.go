package integration_tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

const ApplyTagsImage = "quay.io/konflux-ci/task-runner:1.1.1"

const KonfluxAdditionalTagsLabelName = "konflux.additional-tags"

type ApplyTagsParams struct {
	ImageRepoUrl string
	ImageDigest  string
	Tags         []string
}

func RunApplyTags(applyTagsParams ApplyTagsParams, imageRegistry ImageRegistry) error {
	var err error

	container := NewBuildCliRunnerContainer("apply-tags", ApplyTagsImage)

	if imageRegistry.IsLocal() {
		container.AddVolumeWithOptions(imageRegistry.GetCaCertPath(), "/etc/pki/tls/certs/ca-custom-bundle.crt", "z")
	}

	err = container.Start()
	if err != nil {
		return err
	}
	defer container.Delete()

	login, password := imageRegistry.GetCredentials()
	err = container.InjectDockerAuth(imageRegistry.GetRegistryDomain(), login, password)
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
		tagExists, err := imageRegistry.CheckTagExistance(imageRepoUrl, tag)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to check for %s tag existance", tag))
		Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s:%s to exist", imageRepoUrl, tag))
	}
}
