package commands

import (
	"errors"
	"fmt"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func Test_normalizeImageName(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{
			name:  "should not change simple image",
			image: "image-name",
			want:  "image-name",
		},
		{
			name:  "should not change namespaced image",
			image: "namespace/image-name",
			want:  "namespace/image-name",
		},
		{
			name:  "should not change image with registry",
			image: "registry.io/image-name",
			want:  "registry.io/image-name",
		},
		{
			name:  "should not change image with registry and namespace",
			image: "registry.io/namespace/image-name",
			want:  "registry.io/namespace/image-name",
		},
		{
			name:  "should not change image with registry and port",
			image: "registry.io:1234/image-name",
			want:  "registry.io:1234/image-name",
		},
		{
			name:  "should not change image with registry and port and namespace",
			image: "registry.io:1234/namespace/image-name",
			want:  "registry.io:1234/namespace/image-name",
		},
		{
			name:  "should delete digest in simple image",
			image: "image@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "image",
		},
		{
			name:  "should delete digest in namespaced image",
			image: "namespace/image@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "namespace/image",
		},
		{
			name:  "should delete digest for image with registry and namespace",
			image: "registry.io/user/image@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "registry.io/user/image",
		},
		{
			name:  "should delete digest for image with registry and port and namespace",
			image: "registry.io:1234/user/image@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "registry.io:1234/user/image",
		},
		{
			name:  "should delete tag in simple image",
			image: "image:tag",
			want:  "image",
		},
		{
			name:  "should delete tag in namespaced image",
			image: "namespace/image:tag",
			want:  "namespace/image",
		},
		{
			name:  "should delete tag for image with registry and namespace",
			image: "registry.io/user/image:tag",
			want:  "registry.io/user/image",
		},
		{
			name:  "should delete tag for image with registry and port and namespace",
			image: "registry.io:1234/user/image:tag",
			want:  "registry.io:1234/user/image",
		},
		{
			name:  "should delete numeric tag for image with registry and port and namespace",
			image: "registry.io:1234/user/image:1234",
			want:  "registry.io:1234/user/image",
		},
		{
			name:  "should delete tag with separators for image with registry and port and namespace",
			image: "registry.io:1234/user/image:_t-a.g",
			want:  "registry.io:1234/user/image",
		},
		{
			name:  "should delete tag and digest in simple image",
			image: "image:tag@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "image",
		},
		{
			name:  "should delete tag and digest in namespaced image",
			image: "namespace/image:tag@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "namespace/image",
		},
		{
			name:  "should delete tag and digest for image with registry and namespace",
			image: "registry.io/user/image:tag@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "registry.io/user/image",
		},
		{
			name:  "should delete tag and digest for image with registry and port and namespace",
			image: "registry.io:1234/user/image:tag@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "registry.io:1234/user/image",
		},
		{
			name:  "should delete numeric tag and digest for image with registry and port and namespace",
			image: "registry.io:1234/user/image:1234@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "registry.io:1234/user/image",
		},
		{
			name:  "should delete tag with separators and digest for image with registry and port and namespace",
			image: "registry.io:1234/user/image:t-a.g_1234@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "registry.io:1234/user/image",
		},
	}
	c := &ApplyTags{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := c.normalizeImageName(tc.image)
			if got != tc.want {
				t.Errorf("For %s expected %s, but got: %s", tc.image, got, tc.want)
			}
		})
	}
}

func Test_isImageBaseValid(t *testing.T) {
	validImages := []string{
		"image",
		"i",
		"im",
		"i-m",
		"i.m",
		"i_m",
		"i__m",
		"namespace/image",
		"registry.io/user/image",
		"registry.io/user/namespace/image",
		"registry.io:1234/image",
		"registry.io:1234/user/image",
		"registry.io:1234/user1234/image1234",
		"registry.io:1234/us12er/ima34ge",
		"re-gis-try.io/us-er/ima-ge",
		"re.gis.try.io/us.er/ima.ge",
		"re_gis_try.io/us_er/ima_ge",
		"registry.io/us__er/i_ma__ge",
		"registry.io:1234/us_er/name-space/ima.ge",
		"registry.io:1/image",
		"registry.io:65535/image",
		"n/i",
		"r/n/i",
		"r/o/n/i",
		"r:1/i",
		"r:1/n/i",
		"namespace/verylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenam",
	}
	invalidImages := []string{
		"",
		"Image",
		"imAge",
		"image_",
		"image.",
		"image-",
		"image/",
		"_image",
		".image",
		"-image",
		"/image",
		"ima___ge",
		"ima..ge",
		"ima--ge",
		"i_.m",
		"i._m",
		"i-_m",
		"i_-m",
		"i-.m",
		"i.-m",
		"i_-.m",
		"i-_.m",
		"i-._m",
		"namespace//image",
		"nameSpace/path/image",
		"namespace/Path/image",
		"namespace/path/imAge",
		"registry.io/./image",
		"registry.io/_/image",
		"registry.io/-/image",
		"registry.io/user//namespace/image",
		"registry.io/user///namespace/image",
		"registry.io/user/name..space/image",
		"registry.io/us___er/namespace/image",
		"registry.io/user/namespace/ima..ge",
		"registry.io/user/.namespace/image",
		"registry.io/user/_namespace/image",
		"registry.io/user/-namespace/image",
		"registry.io/user/namespace./image",
		"registry.io/user/namespace_/image",
		"registry.io/user/namespace-/image",
		"registry.io/user/nameSpace/image",
		"registry.io:1234",
		"registry.io:-1234/image",
		"registry.io:65536/image",
		"registry.io:12345678901234567890123456789012345678901234567890123456789012345678901234567890/image",
		"registry.io:port/image",
		"registry.io:/image",
		"namespace/verylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagename",
	}
	c := &ApplyTags{}
	for _, image := range validImages {
		t.Run("valid image", func(t *testing.T) {
			if !c.isImageNameValid(image) {
				t.Errorf("%s expected to be valid", image)
			}
		})
	}
	for _, image := range invalidImages {
		t.Run("invalid image", func(t *testing.T) {
			if c.isImageNameValid(image) {
				t.Errorf("%s expected to be invalid", image)
			}
		})
	}
}

func Test_isDigestValid(t *testing.T) {
	validDigests := []string{
		"sha256:5f2332b1661b2d0967f2652dfe906ef4893438d298290cd090a1358653af1d55",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}
	invalidDigests := []string{
		"",
		"5f2332b1661b2d0967f2652dfe906ef4893438d298290cd090a1358653af1d55",
		"sha255:5f2332b1661b2d0967f2652dfe906ef4893438d298290cd090a1358653af1d55",
		"sha2565f2332b1661b2d0967f2652dfe906ef4893438d298290cd090a1358653af1d55",
		"sha256:5f2332b1661b2d0967f2652dfe906eg4893438d298290cd090a1358653af1d55",
		"sha256:5f2332b1661b2d0967f2652dfe906ef4893438d298290cd090a1358653af1d5",
		"sha256:5f2332b1661b2d0967f2652dfe906ef4893438d298290cd090a1358653af1d55e",
	}
	c := &ApplyTags{}
	for _, digest := range validDigests {
		t.Run("valid digest", func(t *testing.T) {
			if !c.isDigestValid(digest) {
				t.Errorf("%s expected to be valid", digest)
			}
		})
	}
	for _, digest := range invalidDigests {
		t.Run("invalid digest", func(t *testing.T) {
			if c.isDigestValid(digest) {
				t.Errorf("%s expected to be invalid", digest)
			}
		})
	}
}

func Test_isTagValid(t *testing.T) {
	validTags := []string{
		"tag",
		"Tag",
		"TaG",
		"tag12",
		"12tag",
		"t",
		"1",
		"_tag",
		"tag_",
		"tag.",
		"tag-",
		"t.-_ag",
		"t___ag",
		"t.-ag",
		"t-.ag",
		"t_-ag",
		"t-_ag",
		"t._ag",
		"t_.ag",
		"_.-",
		"veryverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagveryloooongtag",
	}
	invalidTags := []string{
		"",
		".tag",
		"-tag",
		"ta:g",
		"t ag",
		"verylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtagverylongtag",
	}
	c := &ApplyTags{}
	for _, tag := range validTags {
		t.Run("valid tag", func(t *testing.T) {
			if !c.isTagValid(tag) {
				t.Errorf("%s expected to be valid", tag)
			}
		})
	}
	for _, tag := range invalidTags {
		t.Run("invalid tag", func(t *testing.T) {
			if c.isTagValid(tag) {
				t.Errorf("%s expected to be invalid", tag)
			}
		})
	}
}

func Test_isImageLabelNameValid(t *testing.T) {
	validImageLabelName := []string{
		"labelname",
		"label/name",
		"label-name",
		"label.name",
		"label_name",
		"label12345name",
		"la-be.l_na/me",
		"com.example.some-label",
		"com.example.io/some-label",
		"verylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelname",
	}
	invalidImageLabelName := []string{
		"",
		"labelName",
		".labelname",
		"-labelname",
		"_labelname",
		"/labelname",
		"1labelname",
		"labelname.",
		"labelname-",
		"labelname_",
		"labelname/",
		"labelname1",
		"label..name",
		"label--name",
		"label__name",
		"label//name",
		"label.-name",
		"label._name",
		"label.-name",
		"label./name",
		"label-.name",
		"label-_name",
		"label-/name",
		"label_.name",
		"label_-name",
		"label_/name",
		"label/.name",
		"label/-name",
		"label/_name",
		"veryverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelnameverylonglabelname",
	}
	c := &ApplyTags{}
	for _, digest := range validImageLabelName {
		t.Run("valid image label name", func(t *testing.T) {
			if !c.isImageLabelNameValid(digest) {
				t.Errorf("%s expected to be valid", digest)
			}
		})
	}
	for _, digest := range invalidImageLabelName {
		t.Run("invalid image label name", func(t *testing.T) {
			if c.isImageLabelNameValid(digest) {
				t.Errorf("%s expected to be invalid", digest)
			}
		})
	}
}

func Test_validateParams(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		name         string
		params       ApplyTagsParams
		errExpected  bool
		errSubstring string
	}{
		{
			name: "should allow valid parameters",
			params: ApplyTagsParams{
				ImageUrl:      "image-registry.net/org/user/image",
				Digest:        "sha256:312515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
				NewTags:       []string{"tag1", "tag2"},
				LabelWithTags: "konflux.additional-tags",
			},
			errExpected: false,
		},
		{
			name: "should allow valid parameters when label is not given",
			params: ApplyTagsParams{
				ImageUrl:      "quay.io/org/image-name",
				Digest:        "sha256:312515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
				NewTags:       []string{"tag1", "tag2"},
				LabelWithTags: "",
			},
			errExpected: false,
		},
		{
			name: "should allow empty tags and missing label",
			params: ApplyTagsParams{
				ImageUrl:      "host:8000/namespace/image",
				Digest:        "sha256:312515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
				NewTags:       []string{},
				LabelWithTags: "",
			},
			errExpected: false,
		},
		{
			name: "should allow tag in image name",
			params: ApplyTagsParams{
				ImageUrl: "image-registry.net/org/user/image:tag",
				Digest:   "sha256:312515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
			},
			errExpected: false,
		},
		{
			name: "should fail on invalid image name",
			params: ApplyTagsParams{
				ImageUrl:      "quay.io/org/imAge",
				Digest:        "sha256:312515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
				NewTags:       []string{"tag1", "tag2"},
				LabelWithTags: "konflux.additional-tags",
			},
			errExpected:  true,
			errSubstring: "image",
		},
		{
			name: "should fail on invalid digets",
			params: ApplyTagsParams{
				ImageUrl:      "quay.io/org/image",
				Digest:        "sha256:31z515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
				NewTags:       []string{"tag1", "tag2"},
				LabelWithTags: "konflux.additional-tags",
			},
			errExpected:  true,
			errSubstring: "image digest",
		},
		{
			name: "should fail on invalid tag",
			params: ApplyTagsParams{
				ImageUrl:      "quay.io/org/image",
				Digest:        "sha256:312515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
				NewTags:       []string{"tag1", "-tag2"},
				LabelWithTags: "konflux.additional-tags",
			},
			errExpected:  true,
			errSubstring: "tag",
		},
		{
			name: "should fail on invalid label name",
			params: ApplyTagsParams{
				ImageUrl:      "quay.io/org/image",
				Digest:        "sha256:312515df62b06ed562904777a627032c93cbef945df527bcc332fe333cc0f94c",
				NewTags:       []string{"tag1", "tag2"},
				LabelWithTags: "konflux.Additional-tags",
			},
			errExpected:  true,
			errSubstring: "image label name",
		},
	}
	c := &ApplyTags{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c.Params = &tc.params
			c.imageName = c.normalizeImageName(c.Params.ImageUrl)

			err := c.validateParams()

			if tc.errExpected {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("is invalid"))
				g.Expect(err.Error()).To(ContainSubstring(tc.errSubstring))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_retrieveTagsFromImageLabel(t *testing.T) {
	g := NewWithT(t)

	const labelName = "more-tags/label"
	const imageRef = "image@sha256:abcdef12345"

	mockSkopeoCli := &mockSkopeoCli{}
	c := &ApplyTags{
		CliWrappers:   ApplyTagsCliWrappers{SkopeoCli: mockSkopeoCli},
		imageByDigest: imageRef,
	}

	t.Run("should retrieve single tag from label value", func(t *testing.T) {
		isScopeoInspectCalled := false
		mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(imageRef))
			g.Expect(args.Format).To(Equal(fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName)))
			return "tag", nil
		}

		tags, err := c.retrieveTagsFromImageLabel(labelName)
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(Equal([]string{"tag"}))
	})

	t.Run("should retrieve tags from label value if they are space separated", func(t *testing.T) {
		isScopeoInspectCalled := false
		mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(imageRef))
			g.Expect(args.Format).To(Equal(fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName)))
			return "tag1 tag2", nil
		}

		tags, err := c.retrieveTagsFromImageLabel(labelName)
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(Equal([]string{"tag1", "tag2"}))
	})

	t.Run("should retrieve tags from label value if they are comma separated", func(t *testing.T) {
		isScopeoInspectCalled := false
		mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(imageRef))
			g.Expect(args.Format).To(Equal(fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName)))
			return "tag1, tag2", nil
		}

		tags, err := c.retrieveTagsFromImageLabel(labelName)
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(Equal([]string{"tag1", "tag2"}))
	})

	t.Run("should retrieve tags from label value if many whitespaces used", func(t *testing.T) {
		isScopeoInspectCalled := false
		mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(imageRef))
			g.Expect(args.Format).To(Equal(fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName)))
			return " \ntag1 \n\n   tag2\n", nil
		}

		tags, err := c.retrieveTagsFromImageLabel(labelName)
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(Equal([]string{"tag1", "tag2"}))
	})

	t.Run("should not fail if label value is empty", func(t *testing.T) {
		isScopeoInspectCalled := false
		mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(imageRef))
			g.Expect(args.Format).To(Equal(fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName)))
			return "", nil
		}

		tags, err := c.retrieveTagsFromImageLabel(labelName)
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(BeNil())
	})

	t.Run("should not fail if label value is newline", func(t *testing.T) {
		isScopeoInspectCalled := false
		mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(imageRef))
			g.Expect(args.Format).To(Equal(fmt.Sprintf(`{{ index .Labels "%s" }}`, labelName)))
			return "\n", nil
		}

		tags, err := c.retrieveTagsFromImageLabel(labelName)
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(BeNil())
	})

	t.Run("should fail if scopeo failed to inspect image", func(t *testing.T) {
		isScopeoInspectCalled := false
		mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			return "", errors.New("failed to inspect image")
		}

		_, err := c.retrieveTagsFromImageLabel(labelName)
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(err).To(HaveOccurred())
	})
}

func Test_applyTags(t *testing.T) {
	g := NewWithT(t)

	const imageRef = "my-image@sha256:abcdef12345"
	const imageName = "my-image"

	mockSkopeoCli := &mockSkopeoCli{}
	c := &ApplyTags{
		CliWrappers:   ApplyTagsCliWrappers{SkopeoCli: mockSkopeoCli},
		imageByDigest: imageRef,
		imageName:     imageName,
	}

	t.Run("should create tag", func(t *testing.T) {
		const tagName = "my-tag"
		isScopeoCopyCalled := false
		mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			isScopeoCopyCalled = true
			g.Expect(args.SourceImage).To(Equal(imageRef))
			g.Expect(args.DestinationImage).To(Equal(imageName + ":" + tagName))
			return nil
		}

		err := c.applyTags([]string{tagName})
		g.Expect(isScopeoCopyCalled).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should create tags", func(t *testing.T) {
		tags := []string{"tag1", "tag2", "tag3"}
		scopeoCopyCalledTimes := 0
		mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			g.Expect(args.SourceImage).To(Equal(imageRef))
			g.Expect(args.DestinationImage).To(Equal(imageName + ":" + tags[scopeoCopyCalledTimes]))
			scopeoCopyCalledTimes++
			return nil
		}

		err := c.applyTags(tags)
		g.Expect(scopeoCopyCalledTimes).To(Equal(len(tags)))
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should error if creating tag failed", func(t *testing.T) {
		tags := []string{"tag1", "tag2", "tag3", "tag4"}
		scopeoCopyCalledTimes := 0
		mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			scopeoCopyCalledTimes++
			if scopeoCopyCalledTimes == 3 {
				return errors.New("failed to create tag")
			}
			return nil
		}

		err := c.applyTags(tags)
		g.Expect(err).To(HaveOccurred())
		g.Expect(scopeoCopyCalledTimes).To(Equal(3))
	})

	t.Run("should not error if no tags given", func(t *testing.T) {
		isScopeoCopyCalled := false
		mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			isScopeoCopyCalled = true
			return nil
		}

		err := c.applyTags([]string{})
		g.Expect(isScopeoCopyCalled).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func Test_Run(t *testing.T) {
	g := NewWithT(t)

	var _mockSkopeoCli *mockSkopeoCli
	var _mockResultsWriter *mockResultsWriter
	var c *ApplyTags
	beforeEach := func() {
		_mockSkopeoCli = &mockSkopeoCli{}
		_mockResultsWriter = &mockResultsWriter{}
		c = &ApplyTags{
			CliWrappers: ApplyTagsCliWrappers{SkopeoCli: _mockSkopeoCli},
			Params: &ApplyTagsParams{
				ImageUrl:      "quay.io/my-organization/namespace/image",
				Digest:        "sha256:806a5df5f70987524b87da868672ba1cec327b4d35eed01f71f2765177b7754c",
				NewTags:       []string{},
				LabelWithTags: "",
			},
			ResultsWriter: _mockResultsWriter,
		}
	}

	t.Run("should successfully run apply-tags with no tags", func(t *testing.T) {
		beforeEach()
		c.Params.NewTags = []string{}
		c.Params.LabelWithTags = ""

		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			return "", nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should successfully run apply-tags with tags parameter", func(t *testing.T) {
		beforeEach()
		tags := []string{"tag1", "tag2"}
		c.Params.NewTags = tags
		c.Params.LabelWithTags = ""

		scopeoCopyCalledTimes := 0
		_mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			g.Expect(args.DestinationImage).To(HaveSuffix(tags[scopeoCopyCalledTimes]))
			scopeoCopyCalledTimes++
			return nil
		}
		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			applyTagsResults, ok := result.(ApplyTagsResults)
			g.Expect(ok).To(BeTrue())
			g.Expect(applyTagsResults.Tags).To(Equal([]string{"tag1", "tag2"}))
			return "", nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(scopeoCopyCalledTimes).To(Equal(len(tags)))
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should successfully run apply-tags with tags from label only", func(t *testing.T) {
		beforeEach()
		const labelWithTagsValue = "l1tag l2tag"
		const labelWithTagsName = "konflux.additional-tags"
		c.Params.NewTags = nil
		c.Params.LabelWithTags = labelWithTagsName

		isScopeoInspectCalled := false
		_mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(c.Params.ImageUrl + "@" + c.Params.Digest))
			g.Expect(args.Format).To(ContainSubstring(labelWithTagsName))
			return labelWithTagsValue, nil
		}
		scopeoCopyCalledTimes := 0
		_mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			g.Expect(args.DestinationImage).To(HaveSuffix("tag"))
			scopeoCopyCalledTimes++
			return nil
		}
		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			applyTagsResults, ok := result.(ApplyTagsResults)
			g.Expect(ok).To(BeTrue())
			g.Expect(applyTagsResults.Tags).To(Equal([]string{"l1tag", "l2tag"}))
			return "", nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(scopeoCopyCalledTimes).To(Equal(2))
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should successfully run apply-tags with tags from param and label", func(t *testing.T) {
		beforeEach()
		tags := []string{"param-1-tag", "param-2-tag"}
		const labelWithTagsValue = "label-1-tag,label-2-tag"
		const labelWithTagsName = "konflux.additional-tags"
		c.Params.NewTags = tags
		c.Params.LabelWithTags = labelWithTagsName

		isScopeoInspectCalled := false
		_mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(c.Params.ImageUrl + "@" + c.Params.Digest))
			g.Expect(args.Format).To(ContainSubstring(labelWithTagsName))
			return labelWithTagsValue, nil
		}
		scopeoCopyCalledTimes := 0
		_mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			g.Expect(args.DestinationImage).To(HaveSuffix("tag"))
			scopeoCopyCalledTimes++
			return nil
		}
		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			applyTagsResults, ok := result.(ApplyTagsResults)
			g.Expect(ok).To(BeTrue())
			g.Expect(applyTagsResults.Tags).To(Equal([]string{"param-1-tag", "param-2-tag", "label-1-tag", "label-2-tag"}))
			return "", nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(scopeoCopyCalledTimes).To(Equal(4))
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should successfully run apply-tags with tags from param when label is set but empty", func(t *testing.T) {
		beforeEach()
		tags := []string{"param-1-tag", "param-2-tag"}
		const labelWithTagsName = "konflux.additional-tags"
		c.Params.NewTags = tags
		c.Params.LabelWithTags = labelWithTagsName

		isScopeoInspectCalled := false
		_mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			g.Expect(args.ImageRef).To(Equal(c.Params.ImageUrl + "@" + c.Params.Digest))
			g.Expect(args.Format).To(ContainSubstring(labelWithTagsName))
			return "", nil
		}
		scopeoCopyCalledTimes := 0
		_mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			g.Expect(args.DestinationImage).To(HaveSuffix("tag"))
			scopeoCopyCalledTimes++
			return nil
		}
		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			applyTagsResults, ok := result.(ApplyTagsResults)
			g.Expect(ok).To(BeTrue())
			g.Expect(applyTagsResults.Tags).To(Equal([]string{"param-1-tag", "param-2-tag"}))
			return "", nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isScopeoInspectCalled).To(BeTrue())
		g.Expect(scopeoCopyCalledTimes).To(Equal(2))
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should error if creation of a tag failed", func(t *testing.T) {
		beforeEach()
		tags := []string{"tag1", "tag2", "tag3", "tag4"}
		c.Params.NewTags = tags

		scopeoCopyCalledTimes := 0
		_mockSkopeoCli.CopyFunc = func(args *cliwrappers.SkopeoCopyArgs) error {
			if scopeoCopyCalledTimes == 2 {
				return errors.New("scopeo copy failed")
			}
			scopeoCopyCalledTimes++
			return nil
		}

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(scopeoCopyCalledTimes).To(BeNumerically(">", 0))
	})

	t.Run("should error if inspecting image fails", func(t *testing.T) {
		beforeEach()
		c.Params.LabelWithTags = "some-label"

		isScopeoInspectCalled := false
		_mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			return "", errors.New("failed to inspect image")
		}

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(isScopeoInspectCalled).To(BeTrue())
	})

	t.Run("should error if a tag from label is invalid", func(t *testing.T) {
		beforeEach()
		tags := []string{"param-1-tag", "param-2-tag"}
		const labelWithTagsValue = "label-1-tag -label-2-tag label-3-tag"
		const labelWithTagsName = "konflux.additional-tags"
		c.Params.NewTags = tags
		c.Params.LabelWithTags = labelWithTagsName

		isScopeoInspectCalled := false
		_mockSkopeoCli.InspectFunc = func(args *cliwrappers.SkopeoInspectArgs) (string, error) {
			isScopeoInspectCalled = true
			return labelWithTagsValue, nil
		}

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(isScopeoInspectCalled).To(BeTrue())
	})

	t.Run("should error if a tag from parameter is invalid", func(t *testing.T) {
		beforeEach()
		tags := []string{"tag1", "tag@2"}
		c.Params.NewTags = tags

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should error if a image parameter is invalid", func(t *testing.T) {
		beforeEach()
		c.Params.ImageUrl = "image//url"

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should error if a digest parameter is invalid", func(t *testing.T) {
		beforeEach()
		c.Params.Digest = "sha256:abcde1234"

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should error if a image label parameter is invalid", func(t *testing.T) {
		beforeEach()
		c.Params.LabelWithTags = "Label"

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should error if creation of result failed", func(t *testing.T) {
		beforeEach()
		c.Params.NewTags = []string{"tag"}

		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			return "", errors.New("failed to create json from result")
		}
		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})
}

func Test_NewApplyTags(t *testing.T) {
	g := NewWithT(t)

	t.Run("should create ApplyTags instance", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("image-url", "", "image")
		cmd.Flags().String("digest", "", "digest")
		cmd.Flags().StringArray("tags", nil, "tags")
		parseErr := cmd.Flags().Parse([]string{
			"--image-url", "image",
			"--digest", "sha256:abcdef1234",
			"--tags", "tag",
		})
		g.Expect(parseErr).ToNot(HaveOccurred())

		applyTags, err := NewApplyTags(cmd)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(applyTags.Params).ToNot(BeNil())
		g.Expect(applyTags.CliWrappers.SkopeoCli).ToNot(BeNil())
		g.Expect(applyTags.ResultsWriter).ToNot(BeNil())
	})
}
