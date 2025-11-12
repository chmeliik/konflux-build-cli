package cliwrappers

import (
	"errors"
	"strconv"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

type SkopeoCliInterface interface {
	Copy(args *SkopeoCopyArgs) error
	Inspect(args *SkopeoInspectArgs) (string, error)
}

var _ SkopeoCliInterface = &SkopeoCli{}

type SkopeoCli struct {
	Executor CliExecutorInterface
}

func NewSkopeoCli(executor CliExecutorInterface) (*SkopeoCli, error) {
	skopeoCliAvailable, err := CheckCliToolAvailable("skopeo")
	if err != nil {
		return nil, err
	}
	if !skopeoCliAvailable {
		return nil, errors.New("skopeo CLI is not available")
	}

	return &SkopeoCli{
		Executor: executor,
	}, nil
}

type SkopeoCopyArgMultiArch string

const (
	SkopeoCopyArgMultiArchSystem    SkopeoCopyArgMultiArch = "system"
	SkopeoCopyArgMultiArchAll       SkopeoCopyArgMultiArch = "all"
	SkopeoCopyArgMultiArchIndexOnly SkopeoCopyArgMultiArch = "index-only"
)

type SkopeoCopyArgs struct {
	SourceImage      string
	DestinationImage string
	MultiArch        SkopeoCopyArgMultiArch
	RetryTimes       int
	ExtraArgs        []string
}

func (s *SkopeoCli) Copy(args *SkopeoCopyArgs) error {
	if args.SourceImage == "" {
		return errors.New("source image is empty, image to copy from must be set")
	}
	if args.DestinationImage == "" {
		return errors.New("destination image is empty, image to copy to must be set")
	}

	scopeoArgs := []string{"copy"}

	if args.MultiArch != "" {
		scopeoArgs = append(scopeoArgs, "--multi-arch", string(args.MultiArch))
	}
	if args.RetryTimes != 0 {
		scopeoArgs = append(scopeoArgs, "--retry-times", strconv.Itoa(args.RetryTimes))
	}

	if len(args.ExtraArgs) != 0 {
		scopeoArgs = append(scopeoArgs, args.ExtraArgs...)
	}

	dockerPrefix := "docker://"
	scopeoArgs = append(scopeoArgs, dockerPrefix+args.SourceImage, dockerPrefix+args.DestinationImage)

	l.Logger.Debugf("Running command:\nskopeo %s", strings.Join(scopeoArgs, " "))
	stdout, stderr, _, err := s.Executor.Execute("skopeo", scopeoArgs...)
	if err != nil {
		l.Logger.Errorf("skopeo copy failed: %s", err.Error())
		l.Logger.Infof("[stdout]:\n%s", stdout)
		l.Logger.Infof("[stderr]:\n%s", stderr)
		return err
	}

	l.Logger.Debug("[stdout]:\n" + stdout)
	l.Logger.Debug("[stderr]:\n" + stderr)

	return nil
}

type SkopeoInspectArgs struct {
	ImageRef   string
	RetryTimes int
	Raw        bool
	NoTags     bool
	Format     string
	ExtraArgs  []string
}

func (s *SkopeoCli) Inspect(args *SkopeoInspectArgs) (string, error) {
	if args.ImageRef == "" {
		return "", errors.New("no image to inspect")
	}

	scopeoArgs := []string{"inspect"}

	if args.RetryTimes != 0 {
		scopeoArgs = append(scopeoArgs, "--retry-times", strconv.Itoa(args.RetryTimes))
	}
	if args.Raw {
		scopeoArgs = append(scopeoArgs, "--raw")
	}
	if args.NoTags {
		scopeoArgs = append(scopeoArgs, "--no-tags")
	}
	if args.Format != "" {
		scopeoArgs = append(scopeoArgs, "--format", args.Format)
	}

	if len(args.ExtraArgs) != 0 {
		scopeoArgs = append(scopeoArgs, args.ExtraArgs...)
	}

	dockerPrefix := "docker://"
	scopeoArgs = append(scopeoArgs, dockerPrefix+args.ImageRef)

	l.Logger.Debugf("Running command:\nskopeo %s", strings.Join(scopeoArgs, " "))
	stdout, stderr, _, err := s.Executor.Execute("skopeo", scopeoArgs...)
	if err != nil {
		l.Logger.Errorf("skopeo inspect failed: %s", err.Error())
		l.Logger.Infof("[stdout]:\n%s", stdout)
		l.Logger.Infof("[stderr]:\n%s", stderr)
		return "", err
	}

	l.Logger.Debug("[stdout]:\n" + stdout)
	l.Logger.Debug("[stderr]:\n" + stderr)

	return stdout, nil
}
