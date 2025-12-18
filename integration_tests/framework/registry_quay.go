package integration_tests_framework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/sirupsen/logrus"
)

const (
	QuayExpiresAfterLabelName = "quay.expires-after"

	quayDockerConfigDir = "/tmp/kbc-docker-config"
)

var _ ImageRegistry = &QuayRegistry{}

type QuayRegistry struct {
	namespace string
	login     string
	password  string

	logger *logrus.Entry
}

func NewQuayRegistry() ImageRegistry {
	return &QuayRegistry{}
}

func (q *QuayRegistry) Prepare() error {
	q.logger = l.Logger.WithField("logger", "quay-registry")

	if login, err := q.getEnvVar("QUAY_ROBOT_NAME"); err != nil {
		return err
	} else {
		q.login = login
	}
	if password, err := q.getEnvVar("QUAY_ROBOT_TOKEN"); err != nil {
		return err
	} else {
		q.password = password
	}
	if namespace, err := q.getEnvVar("QUAY_NAMESPACE"); err != nil {
		return err
	} else {
		q.namespace = namespace
	}

	if err := q.configureDockerAuth(); err != nil {
		return err
	}

	return nil
}

func (q *QuayRegistry) getEnvVar(envVarName string) (string, error) {
	value := os.Getenv(envVarName)
	if value == "" {
		return "", fmt.Errorf("%s env var is not set", envVarName)
	}
	return value, nil
}

func (q *QuayRegistry) configureDockerAuth() error {
	if err := EnsureDirectory(quayDockerConfigDir); err != nil {
		return err
	}

	dockerConfigJsonPath := path.Join(quayDockerConfigDir, "config.json")
	if !FileExists(dockerConfigJsonPath) {
		// Generate docker config json
		dockerConfigJson, err := GenerateDockerAuthContent(q.GetRegistryDomain(), q.login, q.password)
		if err != nil {
			q.logger.Errorf("failed to generate dockerconfigjson: %s", err.Error())
			return err
		}
		if err := os.WriteFile(dockerConfigJsonPath, dockerConfigJson, 0644); err != nil {
			q.logger.Errorf("failed to save dockerconfigjson: %s", err.Error())
			return err
		}
	}

	os.Setenv("DOCKER_CONFIG", quayDockerConfigDir)
	return nil
}

func (q *QuayRegistry) Start() error {
	return nil
}

func (q *QuayRegistry) Stop() error {
	os.RemoveAll(quayDockerConfigDir)
	return nil
}

func (q *QuayRegistry) IsLocal() bool {
	return false
}

func (q *QuayRegistry) GetRegistryDomain() string {
	return "quay.io"
}

func (q *QuayRegistry) GetTestNamespace() string {
	return q.GetRegistryDomain() + "/" + q.namespace + "/"
}

func (q *QuayRegistry) GetCredentials() (string, string) {
	return q.login, q.password
}

func (q *QuayRegistry) GetDockerConfigJsonContent() []byte {
	content, err := GenerateDockerAuthContent(q.GetRegistryDomain(), q.login, q.password)
	if err != nil {
		panic(fmt.Sprintf("failed to create docker config json data: %s", err.Error()))
	}
	return content
}

func (q *QuayRegistry) GetCaCertPath() string {
	return ""
}

// CheckTagExistance quaries Quay API to check the tag existance.
// Args example: quay.io/namespace/repo, tag
func (q *QuayRegistry) CheckTagExistance(repo string, tag string) (bool, error) {
	repoParts := strings.Split(repo, "/")
	if len(repoParts) != 3 {
		return false, fmt.Errorf("invalid image format, expected quay.io/namespace/repo")
	}
	namespace := repoParts[1]
	repository := repoParts[2]

	url := fmt.Sprintf("https://quay.io/api/v1/repository/%s/%s/tag/?specificTag=%s", namespace, repository, tag)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	username, password := q.GetCredentials()
	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("API request failed with status code %d", resp.StatusCode)
	}

	// {
	//   "tags": [
	//     {
	//       "name": "tag-name",
	//       "reversion": false,
	//       "start_ts": 1756740181,
	//       "manifest_digest": "sha256:33735bd63cf84d7e388d9f6d297d348c523c044410f553bd878c6d7829612735",
	//       "is_manifest_list": false,
	//       "size": 3623807,
	//       "last_modified": "Mon, 01 Sep 2025 15:23:01 -0000"
	//     }
	//   ]
	// }
	type Tag struct {
		Name string `json:"name"`
	}
	type Response struct {
		Tags []Tag `json:"tags"`
	}
	var result Response
	err = json.Unmarshal(body, &result)
	if err != nil {
		return false, err
	}

	for _, t := range result.Tags {
		if t.Name == tag {
			return true, nil
		}
	}
	return false, nil
}
