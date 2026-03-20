package common_test

import (
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
)

func Test_ImageRefUntils_GetImageName(t *testing.T) {
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
		{
			name:  "should return empty string if image reference is invalid",
			image: "registry.io:1234/user/imAge:tag",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := common.GetImageName(tc.image)
			if got != tc.want {
				t.Errorf("For %s expected %s, but got: %s", tc.image, got, tc.want)
			}
		})
	}
}

func Test_ImageRefUntils_IsImageNameValid(t *testing.T) {
	validImages := []string{
		"image",
		"i",
		"im",
		"i-m",
		"i.m",
		"i_m",
		"i__m",
		"ima--ge",
		"ima---ge",
		"namespace/image",
		"doMAIN/path/image",
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
		"namespace/verylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverymax",
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
		// The original lib doesn't care about invalid port number...
		// "registry.io:65536/image",
		// "registry.io:12345678901234567890123456789012345678901234567890123456789012345678901234567890/image",
		"registry.io:port/image",
		"registry.io:/image",
		"namespace/verylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylongimagenameverylong",
	}
	for _, image := range validImages {
		t.Run("valid image", func(t *testing.T) {
			if !common.IsImageNameValid(image) {
				t.Errorf("%s expected to be valid", image)
			}
		})
	}
	for _, image := range invalidImages {
		t.Run("invalid image", func(t *testing.T) {
			if common.IsImageNameValid(image) {
				t.Errorf("%s expected to be invalid", image)
			}
		})
	}
}

func Test_ImageRefUntils_IsImageDigestValid(t *testing.T) {
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
	for _, digest := range validDigests {
		t.Run("valid digest", func(t *testing.T) {
			if !common.IsImageDigestValid(digest) {
				t.Errorf("%s expected to be valid", digest)
			}
		})
	}
	for _, digest := range invalidDigests {
		t.Run("invalid digest", func(t *testing.T) {
			if common.IsImageDigestValid(digest) {
				t.Errorf("%s expected to be invalid", digest)
			}
		})
	}
}

func Test_ImageRefUntils_IsImageTagValid(t *testing.T) {
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
	for _, tag := range validTags {
		t.Run("valid tag", func(t *testing.T) {
			if !common.IsImageTagValid(tag) {
				t.Errorf("%s expected to be valid", tag)
			}
		})
	}
	for _, tag := range invalidTags {
		t.Run("invalid tag", func(t *testing.T) {
			if common.IsImageTagValid(tag) {
				t.Errorf("%s expected to be invalid", tag)
			}
		})
	}
}

func Test_ImageRefUntils_GetImageDigest(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{
			name:  "image with digest should return digest",
			image: "registry.io/namespace/image@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
		},
		{
			name:  "image with tag and digest should return digest",
			image: "registry.io/namespace/image:tag@sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
			want:  "sha256:586ab46b9d6d906b2df3dad12751e807bd0f0632d5a2ab3991bdac78bdccd59a",
		},
		{
			name:  "image with only tag should return empty string",
			image: "registry.io/namespace/image:tag",
			want:  "",
		},
		{
			name:  "image without tag or digest should return empty string",
			image: "registry.io/namespace/image",
			want:  "",
		},
		{
			name:  "invalid image should return empty string",
			image: "not a valid reference",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := common.GetImageDigest(tc.image)
			if got != tc.want {
				t.Errorf("For %s expected %s, but got: %s", tc.image, tc.want, got)
			}
		})
	}
}

func Test_ValidateImageHasTagOrDigest(t *testing.T) {
	tests := []struct {
		name      string
		imageRef  string
		wantError bool
	}{
		{
			name:      "image with tag should pass",
			imageRef:  "registry.io/repo:tag",
			wantError: false,
		},
		{
			name:      "image with digest should pass",
			imageRef:  "registry.io/repo@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			wantError: false,
		},
		{
			name:      "image with both tag and digest should pass",
			imageRef:  "registry.io/repo:tag@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			wantError: false,
		},
		{
			name:      "image without tag or digest should fail",
			imageRef:  "registry.io/repo",
			wantError: true,
		},
		{
			name:      "simple image without tag or digest should fail",
			imageRef:  "myimage",
			wantError: true,
		},
		{
			name:      "namespaced image without tag or digest should fail",
			imageRef:  "namespace/myimage",
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := common.ValidateImageHasTagOrDigest(tc.imageRef)
			if tc.wantError && err == nil {
				t.Errorf("Expected error for %s, but got nil", tc.imageRef)
			}
			if !tc.wantError && err != nil {
				t.Errorf("Expected no error for %s, but got: %v", tc.imageRef, err)
			}
		})
	}
}
