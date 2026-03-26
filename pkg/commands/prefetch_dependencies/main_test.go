package prefetch_dependencies

import (
	"context"
	"fmt"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/config"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclient "k8s.io/client-go/kubernetes/fake"
)

func Test_getHermetoEnvFromConfigMap(t *testing.T) {
	g := NewWithT(t)

	testHttpProxy := "test.caching:3323"
	testNamespace := "test_namespace"
	testConfigMapName := "test_name"

	t.Run("can successfully read a config map", func(t *testing.T) {
		fakeClient := fakeclient.NewClientset()
		fake_proxy_url := "https://www.example.com"
		fakeK8sConfigMapReader := &config.K8sConfigMapReader{Name: testConfigMapName, Namespace: testNamespace, Clientset: fakeClient}
		clusterConfigCMWithAllowCacheTrue := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testConfigMapName,
				Namespace: testNamespace,
			},
			Data: map[string]string{
				"allow-cache-proxy":              "true",
				"http-proxy":                     testHttpProxy,
				"no-proxy":                       "",
				"package-registry-proxy-npm-url": fake_proxy_url,
				"allow-package-registry-proxy":   "true",
			},
		}
		ctx := context.Background()
		fakeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx, clusterConfigCMWithAllowCacheTrue, metav1.CreateOptions{})
		fakeConfigReaderFactory := func() (config.ConfigReader, error) {
			return fakeK8sConfigMapReader, nil
		}
		expectedEnv := fmt.Sprintf("HERMETO_NPM__PROXY_URL=%s", fake_proxy_url)

		parsed_config_map, err := getPackageProxyConfiguration(fakeConfigReaderFactory)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parsed_config_map).ToNot(BeNil())
		g.Expect(parsed_config_map[0]).To(Equal(expectedEnv))
	})

	t.Run("returns empty env when there is an error creating config map reader", func(t *testing.T) {
		fakeConfigReaderFactory := func() (config.ConfigReader, error) {
			return nil, fmt.Errorf("Fake error")
		}

		parsed_config_map, err := getPackageProxyConfiguration(fakeConfigReaderFactory)

		g.Expect(err).To(HaveOccurred())
		g.Expect(parsed_config_map).To(BeEmpty())
	})

	t.Run("returns empty env when config map is empty", func(t *testing.T) {
		fakeClient := fakeclient.NewClientset()
		fakeK8sConfigMapReader := &config.K8sConfigMapReader{Name: testConfigMapName, Namespace: testNamespace, Clientset: fakeClient}
		clusterConfigCMWithAllowCacheTrue := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testConfigMapName,
				Namespace: testNamespace,
			},
			Data: map[string]string{},
		}
		ctx := context.Background()
		fakeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx, clusterConfigCMWithAllowCacheTrue, metav1.CreateOptions{})
		fakeConfigReaderFactory := func() (config.ConfigReader, error) {
			return fakeK8sConfigMapReader, nil
		}

		parsed_config_map, err := getPackageProxyConfiguration(fakeConfigReaderFactory)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parsed_config_map).To(BeEmpty())
	})

	t.Run("returns empty env when no Hermeto fields are defined", func(t *testing.T) {
		fakeClient := fakeclient.NewClientset()
		fake_proxy_url := ""
		fakeK8sConfigMapReader := &config.K8sConfigMapReader{Name: testConfigMapName, Namespace: testNamespace, Clientset: fakeClient}
		clusterConfigCMWithAllowCacheTrue := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testConfigMapName,
				Namespace: testNamespace,
			},
			Data: map[string]string{
				"allow-cache-proxy": "true",
				"http-proxy":        testHttpProxy,
				"no-proxy":          "",
				"hermeto-npm-proxy": fake_proxy_url,
			},
		}
		ctx := context.Background()
		fakeClient.CoreV1().ConfigMaps(testNamespace).Create(ctx, clusterConfigCMWithAllowCacheTrue, metav1.CreateOptions{})
		fakeConfigReaderFactory := func() (config.ConfigReader, error) {
			return fakeK8sConfigMapReader, nil
		}

		parsed_config_map, err := getPackageProxyConfiguration(fakeConfigReaderFactory)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(parsed_config_map).ToNot(BeNil())
		g.Expect(parsed_config_map).To(BeEmpty())
	})
}
