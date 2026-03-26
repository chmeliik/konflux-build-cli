package cliwrappers_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

func setupHermetoCli() (*cliwrappers.HermetoCli, *mockExecutor) {
	executor := &mockExecutor{}
	hermetoCli := &cliwrappers.HermetoCli{Executor: executor}
	return hermetoCli, executor
}

func TestHermetoCliVersionOutput(t *testing.T) {
	g := NewWithT(t)

	hermetoCli, executor := setupHermetoCli()
	var capturedArgs []string
	var capturedStdout string

	executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
		g.Expect(cmd.Name).To(Equal("hermeto"))
		capturedArgs = cmd.Args
		capturedStdout = "hermeto 0.1.0"
		// mock stdout, stderr, exit code and error
		return capturedStdout, "", 0, nil
	}

	err := hermetoCli.Version()
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(capturedArgs).To(Equal([]string{"--version"}))
	g.Expect(capturedStdout).To(Equal("hermeto 0.1.0"))
}

func TestHermetoCliFetchDepsArgs(t *testing.T) {
	g := NewWithT(t)

	hermetoCli, executor := setupHermetoCli()
	var capturedArgs []string

	executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
		g.Expect(cmd.Name).To(Equal("hermeto"))
		capturedArgs = cmd.Args
		// mock stdout, stderr, exit code and error
		return "", "", 0, nil
	}

	params := &cliwrappers.HermetoFetchDepsParams{
		Input:      "gomod",
		SourceDir:  "/source",
		OutputDir:  "/output",
		ConfigFile: "/config.yaml",
		SBOMFormat: "spdx",
		Mode:       "strict",
	}

	err := hermetoCli.FetchDeps(params)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(capturedArgs).To(HaveLen(14))
	g.Expect(capturedArgs[0]).To(Equal("--log-level"))
	g.Expect(capturedArgs[1]).ToNot(BeEmpty()) // log level value
	g.Expect(capturedArgs[2]).To(Equal("--mode"))
	g.Expect(capturedArgs[3]).To(Equal("strict"))
	g.Expect(capturedArgs[4]).To(Equal("--config-file"))
	g.Expect(capturedArgs[5]).To(Equal("/config.yaml"))
	g.Expect(capturedArgs[6]).To(Equal("fetch-deps"))
	g.Expect(capturedArgs[7]).To(Equal("gomod"))
	g.Expect(capturedArgs[8]).To(Equal("--sbom-output-type"))
	g.Expect(capturedArgs[9]).To(Equal("spdx"))
	g.Expect(capturedArgs[10]).To(Equal("--source"))
	g.Expect(capturedArgs[11]).To(Equal("/source"))
	g.Expect(capturedArgs[12]).To(Equal("--output"))
	g.Expect(capturedArgs[13]).To(Equal("/output"))
}

func TestHermetoCliGenerateEnvArgs(t *testing.T) {
	g := NewWithT(t)

	hermetoCli, executor := setupHermetoCli()
	var capturedArgs []string

	executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
		g.Expect(cmd.Name).To(Equal("hermeto"))
		capturedArgs = cmd.Args
		// mock stdout, stderr, exit code and error
		return "", "", 0, nil
	}

	params := &cliwrappers.HermetoGenerateEnvParams{
		OutputDir:    "/output",
		ForOutputDir: "/tmp",
		Output:       "/prefetch.env",
	}

	err := hermetoCli.GenerateEnv(params)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(capturedArgs).To(HaveLen(8))
	g.Expect(capturedArgs[0]).To(Equal("--log-level"))
	g.Expect(capturedArgs[1]).ToNot(BeEmpty()) // log level value
	g.Expect(capturedArgs[2]).To(Equal("generate-env"))
	g.Expect(capturedArgs[3]).To(Equal("/output"))
	g.Expect(capturedArgs[4]).To(Equal("--for-output-dir"))
	g.Expect(capturedArgs[5]).To(Equal("/tmp"))
	g.Expect(capturedArgs[6]).To(Equal("--output"))
	g.Expect(capturedArgs[7]).To(Equal("/prefetch.env"))
}

func TestHermetoCliInjectFilesArgs(t *testing.T) {
	g := NewWithT(t)

	hermetoCli, executor := setupHermetoCli()
	var capturedArgs []string

	executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
		g.Expect(cmd.Name).To(Equal("hermeto"))
		capturedArgs = cmd.Args
		// mock stdout, stderr, exit code and error
		return "", "", 0, nil
	}

	params := &cliwrappers.HermetoInjectFilesParams{
		OutputDir:    "/output",
		ForOutputDir: "/tmp",
	}

	err := hermetoCli.InjectFiles(params)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(capturedArgs).To(HaveLen(6))
	g.Expect(capturedArgs[0]).To(Equal("--log-level"))
	g.Expect(capturedArgs[1]).ToNot(BeEmpty()) // log level value
	g.Expect(capturedArgs[2]).To(Equal("inject-files"))
	g.Expect(capturedArgs[3]).To(Equal("/output"))
	g.Expect(capturedArgs[4]).To(Equal("--for-output-dir"))
	g.Expect(capturedArgs[5]).To(Equal("/tmp"))
}
