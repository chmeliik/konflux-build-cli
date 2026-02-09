package cliwrappers

import (
	"fmt"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var orasLog = l.Logger.WithField("logger", "OrasCli")

type OrasCliInterface interface {
	Push(args *OrasPushArgs) (string, string, error)
}

var _ OrasCliInterface = &OrasCli{}

type OrasCli struct {
	Executor CliExecutorInterface
}

func NewOrasCli(executor CliExecutorInterface) (*OrasCli, error) {
	cliAvailable, err := CheckCliToolAvailable("oras")
	if err != nil {
		return nil, err
	}
	if !cliAvailable {
		return nil, fmt.Errorf("oras CLI is not available")
	}

	return &OrasCli{
		Executor: executor,
	}, nil
}

type OrasPushArgs struct {
	DestinationImage string
	FileName         string
	ArtifactType     string
	RegistryConfig   string
	Format           string
	Template         string
}

// Push a file from local to the registry. Return the stdout and stderr output from oras command.
func (b *OrasCli) Push(args *OrasPushArgs) (string, string, error) {
	if args.DestinationImage == "" {
		return "", "", fmt.Errorf("destination image arg is empty")
	}
	if args.FileName == "" {
		return "", "", fmt.Errorf("file name arg is empty")
	}

	orasArgs := []string{"push"}
	if args.ArtifactType != "" {
		orasArgs = append(orasArgs, "--artifact-type", args.ArtifactType)
	}
	if args.RegistryConfig != "" {
		orasArgs = append(orasArgs, "--registry-config", args.RegistryConfig)
	}
	if args.Format != "" {
		orasArgs = append(orasArgs, "--format", args.Format)
	}
	if args.Template != "" {
		orasArgs = append(orasArgs, "--template", args.Template)
	}
	orasArgs = append(orasArgs, args.DestinationImage, args.FileName)

	orasLog.Debugf("Running command:\noras %s", strings.Join(orasArgs, " "))

	stdout, stderr, _, err := b.Executor.ExecuteWithOutput("oras", orasArgs...)
	if err != nil {
		orasLog.Errorf("oras push failed: %s", err.Error())
		return "", "", err
	}

	orasLog.Debug("Push completed successfully")

	return stdout, stderr, nil
}
