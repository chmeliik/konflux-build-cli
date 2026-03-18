package common

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestProxyEnvVars(t *testing.T) {
	g := NewWithT(t)

	t.Run("should return all proxy env vars for both params", func(t *testing.T) {
		g := NewWithT(t)

		env := ProxyEnvVars("http://proxy:8080", "localhost,10.0.0.0/8")

		g.Expect(env).To(Equal([]string{
			"HTTP_PROXY=http://proxy:8080",
			"http_proxy=http://proxy:8080",
			"HTTPS_PROXY=http://proxy:8080",
			"https_proxy=http://proxy:8080",
			"NO_PROXY=localhost,10.0.0.0/8",
			"no_proxy=localhost,10.0.0.0/8",
		}))
	})

	t.Run("should return only http proxy vars when no_proxy is empty", func(t *testing.T) {
		g := NewWithT(t)

		env := ProxyEnvVars("http://proxy:8080", "")

		g.Expect(env).To(Equal([]string{
			"HTTP_PROXY=http://proxy:8080",
			"http_proxy=http://proxy:8080",
			"HTTPS_PROXY=http://proxy:8080",
			"https_proxy=http://proxy:8080",
		}))
	})

	t.Run("should return only no_proxy vars when http_proxy is empty", func(t *testing.T) {
		g := NewWithT(t)

		env := ProxyEnvVars("", "localhost")

		g.Expect(env).To(Equal([]string{
			"NO_PROXY=localhost",
			"no_proxy=localhost",
		}))
	})

	t.Run("should return nil when both params are empty", func(t *testing.T) {
		env := ProxyEnvVars("", "")

		g.Expect(env).To(BeNil())
	})
}
