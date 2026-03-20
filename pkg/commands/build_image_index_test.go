package commands

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_BuildImageIndex_validateParams(t *testing.T) {
	g := NewWithT(t)

	const validDigest1 = "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	const validDigest2 = "sha256:fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"

	tests := []struct {
		name         string
		params       BuildImageIndexParams
		errExpected  bool
		errSubstring string
	}{
		{
			name: "should allow valid parameters",
			params: BuildImageIndexParams{
				Image:            "quay.io/org/myapp:latest",
				Images:           []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat:           "oci",
				AlwaysBuildIndex: true,
			},
			errExpected: false,
		},
		{
			name: "should allow multiple images",
			params: BuildImageIndexParams{
				Image: "quay.io/org/myapp:latest",
				Images: []string{
					"quay.io/org/myapp@" + validDigest1,
					"quay.io/org/myapp@" + validDigest2,
				},
				BuildahFormat:           "oci",
				AlwaysBuildIndex: true,
			},
			errExpected: false,
		},
		{
			name: "should allow docker format",
			params: BuildImageIndexParams{
				Image:            "quay.io/org/myapp:latest",
				Images:           []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat:           "docker",
				AlwaysBuildIndex: true,
			},
			errExpected: false,
		},
		{
			name: "should fail on invalid image name",
			params: BuildImageIndexParams{
				Image:  "Invalid Image Name",
				Images: []string{"quay.io/org/myapp@sha256:abc123"},
				BuildahFormat: "oci",
			},
			errExpected:  true,
			errSubstring: "image name.*is invalid",
		},
		{
			name: "should fail on empty images list",
			params: BuildImageIndexParams{
				Image:  "quay.io/org/myapp:latest",
				Images: []string{},
				BuildahFormat: "oci",
			},
			errExpected:  true,
			errSubstring: "at least one image must be provided",
		},
		{
			name: "should fail on invalid format",
			params: BuildImageIndexParams{
				Image:  "quay.io/org/myapp:latest",
				Images: []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat: "invalid",
			},
			errExpected:  true,
			errSubstring: "format must be 'oci' or 'docker'",
		},
		{
			name: "should fail on invalid image reference in images list",
			params: BuildImageIndexParams{
				Image:  "quay.io/org/myapp:latest",
				Images: []string{"Invalid Image Ref"},
				BuildahFormat: "oci",
			},
			errExpected:  true,
			errSubstring: "invalid image reference",
		},
		{
			name: "should allow single image with always-build-index false",
			params: BuildImageIndexParams{
				Image:            "quay.io/org/myapp:latest",
				Images:           []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat:           "oci",
				AlwaysBuildIndex: false,
			},
			errExpected: false,
		},
		{
			name: "should succeed with single image and always-build-index true",
			params: BuildImageIndexParams{
				Image:            "quay.io/org/myapp:latest",
				Images:           []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat:           "oci",
				AlwaysBuildIndex: true,
			},
			errExpected: false,
		},
		{
			name: "should fail when image has no tag or digest",
			params: BuildImageIndexParams{
				Image:         "quay.io/org/myapp",
				Images:        []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat: "oci",
			},
			errExpected:  true,
			errSubstring: "must have a tag or digest",
		},
		{
			name: "should succeed when image has digest",
			params: BuildImageIndexParams{
				Image:         "quay.io/org/myapp@" + validDigest1,
				Images:        []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat: "oci",
			},
			errExpected: false,
		},
		{
			name: "should succeed when image has both tag and digest",
			params: BuildImageIndexParams{
				Image:         "quay.io/org/myapp:latest@" + validDigest1,
				Images:        []string{"quay.io/org/myapp@" + validDigest1},
				BuildahFormat: "oci",
			},
			errExpected: false,
		},
		{
			name: "should fail on duplicate images",
			params: BuildImageIndexParams{
				Image: "quay.io/org/myapp:latest",
				Images: []string{
					"quay.io/org/myapp@" + validDigest1,
					"quay.io/org/myapp@" + validDigest2,
					"quay.io/org/myapp@" + validDigest1,
				},
				BuildahFormat: "oci",
			},
			errExpected:  true,
			errSubstring: "duplicate image reference",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &BuildImageIndex{Params: &tc.params}

			err := c.validateParams()

			if tc.errExpected {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(MatchRegexp(tc.errSubstring))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_BuildImageIndex_validateFormatConsistency(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name         string
		format       string
		manifestJson string
		errExpected  bool
		errSubstring string
	}{
		{
			name:   "should allow OCI format with OCI manifests",
			format: "oci",
			manifestJson: `{
				"manifests": [
					{"mediaType": "application/vnd.oci.image.manifest.v1+json"},
					{"mediaType": "application/vnd.oci.image.manifest.v1+json"}
				]
			}`,
			errExpected: false,
		},
		{
			name:   "should allow docker format with docker manifests",
			format: "docker",
			manifestJson: `{
				"manifests": [
					{"mediaType": "application/vnd.docker.distribution.manifest.v2+json"},
					{"mediaType": "application/vnd.docker.distribution.manifest.v2+json"}
				]
			}`,
			errExpected: false,
		},
		{
			name:   "should fail on docker manifests when format is oci",
			format: "oci",
			manifestJson: `{
				"manifests": [
					{"mediaType": "application/vnd.docker.distribution.manifest.v2+json"}
				]
			}`,
			errExpected:  true,
			errSubstring: "platform image contains docker format, but index will be oci",
		},
		{
			name:   "should fail on oci manifests when format is docker",
			format: "docker",
			manifestJson: `{
				"manifests": [
					{"mediaType": "application/vnd.oci.image.manifest.v1+json"}
				]
			}`,
			errExpected:  true,
			errSubstring: "platform image contains oci format, but index will be docker",
		},
		{
			name:   "should fail on mixed formats with oci target",
			format: "oci",
			manifestJson: `{
				"manifests": [
					{"mediaType": "application/vnd.oci.image.manifest.v1+json"},
					{"mediaType": "application/vnd.docker.distribution.manifest.v2+json"}
				]
			}`,
			errExpected:  true,
			errSubstring: "platform image contains docker format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &BuildImageIndex{
				Params: &BuildImageIndexParams{
					BuildahFormat: tc.format,
				},
			}

			err := c.validateFormatConsistency(tc.manifestJson)

			if tc.errExpected {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.errSubstring))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_BuildImageIndex_extractPlatformImages(t *testing.T) {
	g := NewWithT(t)

	const digest1 = "sha256:aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa111aaa1"
	const digest2 = "sha256:bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb222bbb2"

	tests := []struct {
		name          string
		imageName     string
		manifestJson  string
		expected      []string
		errExpected   bool
		errSubstring  string
	}{
		{
			name:      "should extract multiple platform images",
			imageName: "quay.io/org/myapp",
			manifestJson: `{
				"manifests": [
					{"digest": "` + digest1 + `"},
					{"digest": "` + digest2 + `"}
				]
			}`,
			expected: []string{
				"quay.io/org/myapp@" + digest1,
				"quay.io/org/myapp@" + digest2,
			},
			errExpected: false,
		},
		{
			name:      "should extract single platform image",
			imageName: "quay.io/org/myapp",
			manifestJson: `{
				"manifests": [
					{"digest": "` + digest1 + `"}
				]
			}`,
			expected: []string{
				"quay.io/org/myapp@" + digest1,
			},
			errExpected: false,
		},
		{
			name:      "should handle different repository",
			imageName: "docker.io/library/nginx",
			manifestJson: `{
				"manifests": [
					{"digest": "` + digest1 + `"}
				]
			}`,
			expected: []string{
				"docker.io/library/nginx@" + digest1,
			},
			errExpected: false,
		},
		{
			name:         "should error on invalid JSON",
			imageName:    "quay.io/org/myapp",
			manifestJson: `{invalid json`,
			errExpected:  true,
			errSubstring: "failed to parse manifest JSON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &BuildImageIndex{
				imageName: tc.imageName,
			}

			platformImages, err := c.extractPlatformImages(tc.manifestJson)

			if tc.errExpected {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.errSubstring))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(platformImages).To(Equal(tc.expected))
			}
		})
	}
}
