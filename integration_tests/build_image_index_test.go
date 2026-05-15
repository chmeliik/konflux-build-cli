package integration_tests

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
)

const BuildImageIndexImage = "quay.io/konflux-ci/task-runner:1.7.0"

type BuildImageIndexParams struct {
	Image         string
	Images        []string
	BuildahFormat string
	// Use bool pointers to allow tests to omit these flags and rely on CLI defaults (true), instead of being set
	// to false (bool default)
	TLSVerify             *bool
	AlwaysBuildIndex      *bool
	AdditionalTags        []string
	ResultPathImageDigest string
	ResultPathImageURL    string
	ResultPathImageRef    string
	ResultPathImages      string
}

type BuildImageIndexResults struct {
	ImageDigest string `json:"image_digest"`
	ImageURL    string `json:"image_url"`
	ImageRef    string `json:"image_ref"`
	Images      string `json:"images"`
}

type RunBuildImageIndexOutput struct {
	Results *BuildImageIndexResults
	Stderr  string
}

func RunBuildImageIndex(params BuildImageIndexParams, imageRegistry ImageRegistry, cleanupContainer bool) (*RunBuildImageIndexOutput, *TestRunnerContainer, error) {
	var err error

	container := NewBuildCliRunnerContainer("build-image-index", BuildImageIndexImage)
	if cleanupContainer {
		defer container.DeleteIfExists()
	}

	err = container.StartWithRegistryIntegration(imageRegistry)
	if err != nil {
		return nil, nil, err
	}

	// Construct the build-image-index arguments
	args := []string{"image", "build-image-index"}
	args = append(args, "--image", params.Image)
	if params.TLSVerify != nil {
		args = append(args, fmt.Sprintf("--tls-verify=%t", *params.TLSVerify))
	}
	args = append(args, "--buildah-format", params.BuildahFormat)
	if params.AlwaysBuildIndex != nil {
		args = append(args, fmt.Sprintf("--always-build-index=%t", *params.AlwaysBuildIndex))
	}

	if len(params.Images) > 0 {
		args = append(args, "--images")
		args = append(args, params.Images...)
	}

	if len(params.AdditionalTags) > 0 {
		args = append(args, "--additional-tags")
		args = append(args, params.AdditionalTags...)
	}

	if params.ResultPathImageDigest != "" {
		args = append(args, "--result-path-image-digest", params.ResultPathImageDigest)
	}
	if params.ResultPathImageURL != "" {
		args = append(args, "--result-path-image-url", params.ResultPathImageURL)
	}
	if params.ResultPathImageRef != "" {
		args = append(args, "--result-path-image-ref", params.ResultPathImageRef)
	}
	if params.ResultPathImages != "" {
		args = append(args, "--result-path-images", params.ResultPathImages)
	}

	stdout, stderr, err := container.ExecuteCommandWithOutput(KonfluxBuildCli, args...)
	if err != nil {
		return nil, container, fmt.Errorf("%w (stderr: %s)", err, stderr)
	}

	// Parse the JSON output from stdout
	var results BuildImageIndexResults
	if err := json.Unmarshal([]byte(stdout), &results); err != nil {
		return nil, container, fmt.Errorf("failed to parse results JSON (stderr: %s): %w", stderr, err)
	}

	return &RunBuildImageIndexOutput{Results: &results, Stderr: stderr}, container, nil
}

func TestBuildImageIndex_MultipleImages(t *testing.T) {
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
	baseImageRepo := imageRegistry.GetTestNamespace() + "test-image-index"
	tag := GenerateUniqueTag(t)
	indexImage := baseImageRepo + ":" + tag

	// Create and push two platform images (simulating amd64 and arm64)
	image1Ref := baseImageRepo + "-platform1:" + tag
	image2Ref := baseImageRepo + "-platform2:" + tag

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image1Ref,
		RandomDataSize: 1024,
		Labels: map[string]string{
			"platform": "amd64",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image1Ref)

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image2Ref,
		RandomDataSize: 2048,
		Labels: map[string]string{
			"platform": "arm64",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image2Ref)

	digest1, err := PushImage(image1Ref)
	Expect(err).ToNot(HaveOccurred())

	digest2, err := PushImage(image2Ref)
	Expect(err).ToNot(HaveOccurred())

	// Build the image references with digests
	imageRepo1 := common.GetImageName(image1Ref)
	imageRepo2 := common.GetImageName(image2Ref)
	image1WithDigest := imageRepo1 + "@" + digest1
	image2WithDigest := imageRepo2 + "@" + digest2

	// Run the command
	params := BuildImageIndexParams{
		Image:            indexImage,
		Images:           []string{image1WithDigest, image2WithDigest},
		TLSVerify:        boolptr(true),
		BuildahFormat:    "oci",
		AlwaysBuildIndex: boolptr(true),
		AdditionalTags:   []string{"test-tag-1"},
	}

	output, _, err := RunBuildImageIndex(params, imageRegistry, true)
	Expect(err).ToNot(HaveOccurred())
	results := output.Results

	// Verify results
	Expect(results.ImageURL).To(Equal(indexImage))
	Expect(results.ImageDigest).ToNot(BeEmpty())
	Expect(results.ImageDigest).To(HavePrefix("sha256:"))
	Expect(results.ImageRef).To(Equal(baseImageRepo + "@" + results.ImageDigest))

	// Images should contain both platform image digests (order may vary)
	Expect(results.Images).To(Or(
		Equal(baseImageRepo+"@"+digest1+","+baseImageRepo+"@"+digest2),
		Equal(baseImageRepo+"@"+digest2+","+baseImageRepo+"@"+digest1),
	))

	// Verify the index was pushed to registry
	tagExists, err := imageRegistry.CheckTagExistence(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred())
	Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s to exist", indexImage))

	// Verify additional tag was created
	tagExists, err = imageRegistry.CheckTagExistence(baseImageRepo, "test-tag-1")
	Expect(err).ToNot(HaveOccurred())
	Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s:test-tag-1 to exist", baseImageRepo))

	// Verify the manifest is actually an index (multi-arch)
	imageIndexInfo, err := imageRegistry.GetImageIndexInfo(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get image index %s:%s", baseImageRepo, tag))
	Expect(imageIndexInfo.MediaType).To(Equal("application/vnd.oci.image.index.v1+json"),
		"Created reference is not an OCI image index")
	Expect(imageIndexInfo.Manifests).To(HaveLen(2))

	// Verify platform manifests are OCI format and extract digests
	obtainedDigests := make([]string, 0, 2)
	for _, manifestInfo := range imageIndexInfo.Manifests {
		Expect(manifestInfo.MediaType).To(Equal("application/vnd.oci.image.manifest.v1+json"))
		obtainedDigests = append(obtainedDigests, manifestInfo.Digest)
	}

	// Check that platform image digests are included in the index
	Expect(obtainedDigests).To(ConsistOf(digest1, digest2))

	// Verify the digest matches the actual manifest digest
	actualDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(imageIndexInfo.RawManifest))
	Expect(results.ImageDigest).To(Equal(actualDigest))
}

func TestBuildImageIndex_DockerFormat(t *testing.T) {
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
	baseImageRepo := imageRegistry.GetTestNamespace() + "test-docker-format"
	tag := GenerateUniqueTag(t)
	indexImage := baseImageRepo + ":" + tag

	// Create and push two platform images
	image1Ref := baseImageRepo + "-platform1:" + tag
	image2Ref := baseImageRepo + "-platform2:" + tag

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image1Ref,
		RandomDataSize: 1024,
		Labels: map[string]string{
			"platform": "amd64",
		},
		BuildahFormat: "docker",
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image1Ref)

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image2Ref,
		RandomDataSize: 2048,
		Labels: map[string]string{
			"platform": "arm64",
		},
		BuildahFormat: "docker",
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image2Ref)

	digest1, err := PushImage(image1Ref)
	Expect(err).ToNot(HaveOccurred())

	digest2, err := PushImage(image2Ref)
	Expect(err).ToNot(HaveOccurred())

	// Build the image references with digests
	imageRepo1 := common.GetImageName(image1Ref)
	imageRepo2 := common.GetImageName(image2Ref)
	image1WithDigest := imageRepo1 + "@" + digest1
	image2WithDigest := imageRepo2 + "@" + digest2

	// Run the command with docker format
	params := BuildImageIndexParams{
		Image:         indexImage,
		Images:        []string{image1WithDigest, image2WithDigest},
		BuildahFormat: "docker",
	}

	output, _, err := RunBuildImageIndex(params, imageRegistry, true)
	Expect(err).ToNot(HaveOccurred())
	results := output.Results

	// Verify results
	Expect(results.ImageURL).To(Equal(indexImage))
	Expect(results.ImageDigest).ToNot(BeEmpty())
	Expect(results.ImageDigest).To(HavePrefix("sha256:"))
	Expect(results.ImageRef).To(Equal(baseImageRepo + "@" + results.ImageDigest))

	// Images should contain both platform image digests (order may vary)
	Expect(results.Images).To(Or(
		Equal(baseImageRepo+"@"+digest1+","+baseImageRepo+"@"+digest2),
		Equal(baseImageRepo+"@"+digest2+","+baseImageRepo+"@"+digest1),
	))

	// Verify the index was pushed to registry
	tagExists, err := imageRegistry.CheckTagExistence(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred())
	Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s to exist", indexImage))

	// Verify the manifest is actually a docker manifest list (not OCI)
	imageIndexInfo, err := imageRegistry.GetImageIndexInfo(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get image index %s:%s", baseImageRepo, tag))
	Expect(imageIndexInfo.MediaType).To(Equal("application/vnd.docker.distribution.manifest.list.v2+json"),
		"Created reference is not a docker manifest list")
	Expect(imageIndexInfo.Manifests).To(HaveLen(2))

	// Verify platform manifests are also docker format
	obtainedDigests := make([]string, 0, 2)
	for _, manifestInfo := range imageIndexInfo.Manifests {
		Expect(manifestInfo.MediaType).To(Equal("application/vnd.docker.distribution.manifest.v2+json"))
		obtainedDigests = append(obtainedDigests, manifestInfo.Digest)
	}

	// Check that platform image digests are included in the index
	Expect(obtainedDigests).To(ConsistOf(digest1, digest2))

	// Verify the digest matches the actual manifest digest
	actualDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(imageIndexInfo.RawManifest))
	Expect(results.ImageDigest).To(Equal(actualDigest))
}

func TestBuildImageIndex_SingleImageSkipIndex(t *testing.T) {
	SetupGomega(t)

	// No registry needed - we're skipping index build, just returning input image info
	targetImage := "quay.io/test/myapp:latest"
	inputImageURL := "quay.io/test/myapp:latest-x86_64"
	digest := "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	inputImage := inputImageURL + "@" + digest

	container := NewBuildCliRunnerContainer("build-image-index", BuildImageIndexImage)
	defer container.DeleteIfExists()

	err := container.Start()
	Expect(err).ToNot(HaveOccurred())

	// Run the command with always-build-index=false
	args := []string{"image", "build-image-index"}
	args = append(args, "--image", targetImage)
	args = append(args, "--images", inputImage)
	args = append(args, "--buildah-format", "oci")
	args = append(args, "--always-build-index=false")

	stdout, stderr, err := container.ExecuteCommandWithOutput(KonfluxBuildCli, args...)
	Expect(err).ToNot(HaveOccurred())

	// Parse the JSON output
	var results BuildImageIndexResults
	err = json.Unmarshal([]byte(stdout), &results)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to parse results JSON (stderr: %s)", stderr))

	// Verify results - should just return info about the single image
	Expect(results.ImageURL).To(Equal(inputImageURL))
	Expect(results.ImageDigest).To(Equal(digest))
	Expect(results.ImageRef).To(Equal("quay.io/test/myapp@" + digest))
	Expect(results.Images).To(Equal("quay.io/test/myapp@" + digest))
}

func TestBuildImageIndex_SingleImageAlwaysBuildIndex(t *testing.T) {
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
	baseImageRepo := imageRegistry.GetTestNamespace() + "test-single-always"
	tag := GenerateUniqueTag(t)
	indexImage := baseImageRepo + ":" + tag

	// Create and push a single image
	sourceImageRef := baseImageRepo + "-source:" + tag
	err = CreateTestImage(TestImageConfig{
		ImageRef:       sourceImageRef,
		RandomDataSize: 1024,
		Labels: map[string]string{
			"platform": "amd64",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(sourceImageRef)

	digest, err := PushImage(sourceImageRef)
	Expect(err).ToNot(HaveOccurred())

	// Build the image reference with digest
	imageRepo := common.GetImageName(sourceImageRef)
	imageWithDigest := imageRepo + "@" + digest

	// Run the command with always-build-index=true
	params := BuildImageIndexParams{
		Image:            indexImage,
		Images:           []string{imageWithDigest},
		BuildahFormat:    "oci",
		AlwaysBuildIndex: boolptr(true),
	}

	output, _, err := RunBuildImageIndex(params, imageRegistry, true)
	Expect(err).ToNot(HaveOccurred())
	results := output.Results

	// Verify results
	Expect(results.ImageURL).To(Equal(indexImage))
	Expect(results.ImageDigest).ToNot(BeEmpty())
	Expect(results.ImageDigest).To(HavePrefix("sha256:"))
	Expect(results.ImageRef).To(Equal(baseImageRepo + "@" + results.ImageDigest))
	Expect(results.Images).To(Equal(baseImageRepo + "@" + digest))

	// Verify the index was pushed to registry
	tagExists, err := imageRegistry.CheckTagExistence(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred())
	Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s to exist", indexImage))

	// Verify the manifest is actually an index (even with single image)
	imageIndexInfo, err := imageRegistry.GetImageIndexInfo(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get image index %s:%s", baseImageRepo, tag))
	Expect(imageIndexInfo.MediaType).To(Equal("application/vnd.oci.image.index.v1+json"),
		"Created reference is not an OCI image index")
	Expect(imageIndexInfo.Manifests).To(HaveLen(1))

	// Verify platform manifest is OCI format
	Expect(imageIndexInfo.Manifests[0].MediaType).To(Equal("application/vnd.oci.image.manifest.v1+json"))
	Expect(imageIndexInfo.Manifests[0].Digest).To(Equal(digest))

	// Verify the digest matches the actual manifest digest
	actualDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(imageIndexInfo.RawManifest))
	Expect(results.ImageDigest).To(Equal(actualDigest))
}

func TestBuildImageIndex_ResultPaths(t *testing.T) {
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
	baseImageRepo := imageRegistry.GetTestNamespace() + "test-result-paths"
	tag := GenerateUniqueTag(t)
	indexImage := baseImageRepo + ":" + tag

	// Create and push two platform images
	image1Ref := baseImageRepo + "-platform1:" + tag
	image2Ref := baseImageRepo + "-platform2:" + tag

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image1Ref,
		RandomDataSize: 1024,
		Labels: map[string]string{
			"platform": "amd64",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image1Ref)

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image2Ref,
		RandomDataSize: 2048,
		Labels: map[string]string{
			"platform": "arm64",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image2Ref)

	digest1, err := PushImage(image1Ref)
	Expect(err).ToNot(HaveOccurred())

	digest2, err := PushImage(image2Ref)
	Expect(err).ToNot(HaveOccurred())

	// Build the image references with digests
	imageRepo1 := common.GetImageName(image1Ref)
	imageRepo2 := common.GetImageName(image2Ref)
	image1WithDigest := imageRepo1 + "@" + digest1
	image2WithDigest := imageRepo2 + "@" + digest2

	// Use paths in container's /tmp directory
	resultPathDigest := "/tmp/test-result-digest"
	resultPathURL := "/tmp/test-result-url"
	resultPathRef := "/tmp/test-result-ref"
	resultPathImages := "/tmp/test-result-images"

	// Run the command with result-path parameters
	params := BuildImageIndexParams{
		Image:                 indexImage,
		Images:                []string{image1WithDigest, image2WithDigest},
		BuildahFormat:         "oci",
		ResultPathImageDigest: resultPathDigest,
		ResultPathImageURL:    resultPathURL,
		ResultPathImageRef:    resultPathRef,
		ResultPathImages:      resultPathImages,
	}

	// Don't cleanup container automatically - we need to read result files first
	output, container, err := RunBuildImageIndex(params, imageRegistry, false)
	Expect(err).ToNot(HaveOccurred())
	results := output.Results
	defer container.DeleteIfExists()

	// Verify result files were created and contain correct content
	digestContent, err := container.GetFileContent(resultPathDigest)
	Expect(err).ToNot(HaveOccurred())
	Expect(digestContent).To(Equal(results.ImageDigest))

	urlContent, err := container.GetFileContent(resultPathURL)
	Expect(err).ToNot(HaveOccurred())
	Expect(urlContent).To(Equal(results.ImageURL))

	refContent, err := container.GetFileContent(resultPathRef)
	Expect(err).ToNot(HaveOccurred())
	Expect(refContent).To(Equal(results.ImageRef))

	imagesContent, err := container.GetFileContent(resultPathImages)
	Expect(err).ToNot(HaveOccurred())
	Expect(imagesContent).To(Equal(results.Images))
}

func TestBuildImageIndex_FormatMismatch(t *testing.T) {
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
	baseImageRepo := imageRegistry.GetTestNamespace() + "test-format-mismatch"
	tag := GenerateUniqueTag(t)
	indexImage := baseImageRepo + ":" + tag

	// Create and push platform images with docker format
	image1Ref := baseImageRepo + "-platform1:" + tag
	image2Ref := baseImageRepo + "-platform2:" + tag

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image1Ref,
		RandomDataSize: 1024,
		Labels: map[string]string{
			"platform": "amd64",
		},
		BuildahFormat: "docker",
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image1Ref)

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image2Ref,
		RandomDataSize: 2048,
		Labels: map[string]string{
			"platform": "arm64",
		},
		BuildahFormat: "docker",
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image2Ref)

	digest1, err := PushImage(image1Ref)
	Expect(err).ToNot(HaveOccurred())

	digest2, err := PushImage(image2Ref)
	Expect(err).ToNot(HaveOccurred())

	// Build the image references with digests
	imageRepo1 := common.GetImageName(image1Ref)
	imageRepo2 := common.GetImageName(image2Ref)
	image1WithDigest := imageRepo1 + "@" + digest1
	image2WithDigest := imageRepo2 + "@" + digest2

	// Try to build an OCI index from docker format images (should fail)
	params := BuildImageIndexParams{
		Image:         indexImage,
		Images:        []string{image1WithDigest, image2WithDigest},
		BuildahFormat: "oci",
	}

	_, _, err = RunBuildImageIndex(params, imageRegistry, true)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("platform image contains docker format, but index will be oci"))
}

func TestBuildImageIndex_ImagesWithTagAndDigest(t *testing.T) {
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
	baseImageRepo := imageRegistry.GetTestNamespace() + "test-tag-and-digest"
	tag := GenerateUniqueTag(t)
	indexImage := baseImageRepo + ":" + tag

	// Create and push two platform images
	image1Ref := baseImageRepo + "-platform1:" + tag
	image2Ref := baseImageRepo + "-platform2:" + tag

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image1Ref,
		RandomDataSize: 1024,
		Labels: map[string]string{
			"platform": "amd64",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image1Ref)

	err = CreateTestImage(TestImageConfig{
		ImageRef:       image2Ref,
		RandomDataSize: 2048,
		Labels: map[string]string{
			"platform": "arm64",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	defer DeleteLocalImage(image2Ref)

	digest1, err := PushImage(image1Ref)
	Expect(err).ToNot(HaveOccurred())

	digest2, err := PushImage(image2Ref)
	Expect(err).ToNot(HaveOccurred())

	// Use tag+digest format (repo:tag@sha256:digest) to test
	// that the references are normalized to repo@digest, as
	// buildah does not support the tag+digest format unless the image
	// is available locally
	image1WithTagAndDigest := image1Ref + "@" + digest1
	image2WithTagAndDigest := image2Ref + "@" + digest2

	params := BuildImageIndexParams{
		Image:            indexImage,
		Images:           []string{image1WithTagAndDigest, image2WithTagAndDigest},
		TLSVerify:        boolptr(true),
		BuildahFormat:    "oci",
		AlwaysBuildIndex: boolptr(true),
	}

	output, _, err := RunBuildImageIndex(params, imageRegistry, true)
	Expect(err).ToNot(HaveOccurred())
	results := output.Results

	// Verify that buildah manifest add was called with normalized references
	// (docker://repo@digest, tag stripped)
	imageRepo1 := common.GetImageName(image1Ref)
	imageRepo2 := common.GetImageName(image2Ref)
	Expect(output.Stderr).To(ContainSubstring(
		"buildah manifest add " + indexImage + " docker://" + imageRepo1 + "@" + digest1))
	Expect(output.Stderr).To(ContainSubstring(
		"buildah manifest add " + indexImage + " docker://" + imageRepo2 + "@" + digest2))

	// Verify results
	Expect(results.ImageURL).To(Equal(indexImage))
	Expect(results.ImageDigest).ToNot(BeEmpty())
	Expect(results.ImageDigest).To(HavePrefix("sha256:"))
	Expect(results.ImageRef).To(Equal(baseImageRepo + "@" + results.ImageDigest))

	// Images should contain both platform image digests (order may vary)
	Expect(results.Images).To(Or(
		Equal(baseImageRepo+"@"+digest1+","+baseImageRepo+"@"+digest2),
		Equal(baseImageRepo+"@"+digest2+","+baseImageRepo+"@"+digest1),
	))

	// Verify the index was pushed to registry
	tagExists, err := imageRegistry.CheckTagExistence(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred())
	Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s to exist", indexImage))

	// Verify the manifest is actually an index (multi-arch)
	imageIndexInfo, err := imageRegistry.GetImageIndexInfo(baseImageRepo, tag)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get image index %s:%s", baseImageRepo, tag))
	Expect(imageIndexInfo.MediaType).To(Equal("application/vnd.oci.image.index.v1+json"),
		"Created reference is not an OCI image index")
	Expect(imageIndexInfo.Manifests).To(HaveLen(2))

	// Verify platform manifests are OCI format and extract digests
	obtainedDigests := make([]string, 0, 2)
	for _, manifestInfo := range imageIndexInfo.Manifests {
		Expect(manifestInfo.MediaType).To(Equal("application/vnd.oci.image.manifest.v1+json"))
		obtainedDigests = append(obtainedDigests, manifestInfo.Digest)
	}

	// Check that platform image digests are included in the index
	Expect(obtainedDigests).To(ConsistOf(digest1, digest2))

	// Verify the digest matches the actual manifest digest
	actualDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(imageIndexInfo.RawManifest))
	Expect(results.ImageDigest).To(Equal(actualDigest))
}
