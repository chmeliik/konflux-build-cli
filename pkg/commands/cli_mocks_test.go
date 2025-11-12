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
