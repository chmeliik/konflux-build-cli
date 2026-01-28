package cliwrappers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const (
	UnsupportedOCIConfigMediaType = "application/vnd.unknown.config.v1+json"
)

var skopeoLog = l.Logger.WithField("logger", "ScopeoCli")

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

	skopeoLog.Debugf("Running command:\nskopeo %s", strings.Join(scopeoArgs, " "))

	retryer := NewRetryer(func() (string, string, int, error) {
		return s.Executor.Execute("skopeo", scopeoArgs...)
	}).WithImageRegistryPreset().StopIfOutputContains("unauthorized")

	stdout, stderr, _, err := retryer.Run()
	if err != nil {
		skopeoLog.Errorf("skopeo copy failed: %s", err.Error())
		skopeoLog.Infof("[stdout]:\n%s", stdout)
		skopeoLog.Infof("[stderr]:\n%s", stderr)
		return err
	}

	skopeoLog.Debug("[stdout]:\n" + stdout)
	skopeoLog.Debug("[stderr]:\n" + stderr)

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

	skopeoLog.Debugf("Running command:\nskopeo %s", strings.Join(scopeoArgs, " "))

	retryer := NewRetryer(func() (string, string, int, error) {
		return s.Executor.Execute("skopeo", scopeoArgs...)
	}).WithImageRegistryPreset().
		StopIfOutputContains("unauthorized").
		// Stop on unsupported config media type
		StopIfOutputContains(UnsupportedOCIConfigMediaType)

	stdout, stderr, _, err := retryer.Run()
	if err != nil {
		skopeoLog.Errorf("skopeo inspect failed: %s", err.Error())
		skopeoLog.Infof("[stdout]:\n%s", stdout)
		skopeoLog.Infof("[stderr]:\n%s", stderr)
		return "", fmt.Errorf("%w: %s", err, stderr)
	}

	skopeoLog.Debug("[stdout]:\n" + stdout)
	skopeoLog.Debug("[stderr]:\n" + stderr)

	return stdout, nil
}
