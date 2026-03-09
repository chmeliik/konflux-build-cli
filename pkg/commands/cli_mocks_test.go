package commands

import (
	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

var _ cliwrappers.SkopeoCliInterface = &mockSkopeoCli{}

type mockSkopeoCli struct {
	CopyFunc    func(args *cliwrappers.SkopeoCopyArgs) error
	InspectFunc func(args *cliwrappers.SkopeoInspectArgs) (string, error)
}

func (m *mockSkopeoCli) Copy(args *cliwrappers.SkopeoCopyArgs) error {
	if m.CopyFunc != nil {
		return m.CopyFunc(args)
	}
	return nil
}

func (m *mockSkopeoCli) Inspect(args *cliwrappers.SkopeoInspectArgs) (string, error) {
	if m.InspectFunc != nil {
		return m.InspectFunc(args)
	}
	return "", nil
}

var _ cliwrappers.BuildahCliInterface = &mockBuildahCli{}

type mockBuildahCli struct {
	BuildFunc        func(args *cliwrappers.BuildahBuildArgs) error
	PushFunc         func(args *cliwrappers.BuildahPushArgs) (string, error)
	PullFunc         func(args *cliwrappers.BuildahPullArgs) error
	InspectFunc      func(args *cliwrappers.BuildahInspectArgs) (string, error)
	InspectImageFunc func(name string) (cliwrappers.BuildahImageInfo, error)
	VersionFunc      func() (cliwrappers.BuildahVersionInfo, error)
}

func (m *mockBuildahCli) Build(args *cliwrappers.BuildahBuildArgs) error {
	if m.BuildFunc != nil {
		return m.BuildFunc(args)
	}
	return nil
}

func (m *mockBuildahCli) Push(args *cliwrappers.BuildahPushArgs) (string, error) {
	if m.PushFunc != nil {
		return m.PushFunc(args)
	}
	return "", nil
}

func (m *mockBuildahCli) Pull(args *cliwrappers.BuildahPullArgs) error {
	if m.PullFunc != nil {
		return m.PullFunc(args)
	}
	return nil
}

func (m *mockBuildahCli) Inspect(args *cliwrappers.BuildahInspectArgs) (string, error) {
	if m.InspectFunc != nil {
		return m.InspectFunc(args)
	}
	return "", nil
}

func (m *mockBuildahCli) InspectImage(name string) (cliwrappers.BuildahImageInfo, error) {
	if m.InspectImageFunc != nil {
		return m.InspectImageFunc(name)
	}
	return cliwrappers.BuildahImageInfo{}, nil
}

func (m *mockBuildahCli) Version() (cliwrappers.BuildahVersionInfo, error) {
	if m.VersionFunc != nil {
		return m.VersionFunc()
	}
	return cliwrappers.BuildahVersionInfo{}, nil
}

var _ cliwrappers.OrasCliInterface = &mockOrasCli{}

type mockOrasCli struct {
	Executor cliwrappers.CliExecutorInterface
	PushFunc func(args *cliwrappers.OrasPushArgs) (string, string, error)
}

func (m *mockOrasCli) Push(args *cliwrappers.OrasPushArgs) (string, string, error) {
	if m.PushFunc != nil {
		return m.PushFunc(args)
	}
	return "", "", nil
}
