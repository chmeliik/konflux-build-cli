module github.com/konflux-ci/konflux-build-cli

go 1.23.3

toolchain go1.24.6

require (
	github.com/containerd/platforms v1.0.0-rc.2
	github.com/containers/image/v5 v5.36.2
	github.com/keilerkonzept/dockerfile-json v1.2.2
	github.com/onsi/gomega v1.38.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.9.1
)

require (
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/typeurl/v2 v2.2.3 // indirect
	github.com/containers/storage v1.59.1 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/moby/buildkit v0.19.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/onsi/ginkgo/v2 v2.25.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/tonistiigi/go-csvvalue v0.0.0-20240710180619-ddb21b71c0b4 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/keilerkonzept/dockerfile-json => github.com/konflux-ci/dockerfile-json v0.0.0-20260211115307-8b6cecfd575e
