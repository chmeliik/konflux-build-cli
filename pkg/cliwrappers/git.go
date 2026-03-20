package cliwrappers

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var gitLog = l.Logger.WithField("logger", "GitCli")

// GitCliInterface defines the methods for interacting with git via the CLI.
type GitCliInterface interface {
	// SetEnv adds an environment variable that will be passed to all git commands via Cmd.Env.
	SetEnv(key, value string)
	// Init initializes a new git repository. Runs: git init
	Init() error
	// ConfigLocal sets a local git config value. Runs: git config --local <key> <value>
	ConfigLocal(key, value string) error
	// RevParse resolves a git ref to its SHA. Runs: git rev-parse [--short[=N]] <ref>
	RevParse(ref string, short bool, length int) (string, error)
	// RemoteAdd adds a new remote. Runs: git remote add <name> <url>
	RemoteAdd(name, url string) (string, error)
	// FetchWithRefspec fetches a refspec from a remote with retry. Runs: git fetch [options] <remote> [<refspec>]
	FetchWithRefspec(opts GitFetchOptions) error
	// Checkout checks out a ref. Runs: git checkout <ref>
	Checkout(ref string) error
	// Commit creates a commit with the given message. Runs: git commit -m <message>
	Commit(message string) (string, error)
	// Merge merges a ref with a commit message. Runs: git merge -m <message> --no-ff --allow-unrelated-histories <ref>
	Merge(ref, message string) (string, error)
	// SetSparseCheckout configures sparse checkout directories. Runs: git sparse-checkout set <dirs...>
	SetSparseCheckout(directories []string) error
	// SubmoduleUpdate initializes and updates submodules. Runs: git submodule update --recursive [--init] --force [--depth=N] [-- paths...]
	SubmoduleUpdate(init bool, depth int, paths []string) error
	// FetchTags fetches all tags from the remote. Runs: git fetch --tags
	FetchTags() ([]string, error)
	// Log returns formatted git log output. Runs: git log [--pretty=<format>] [-N]
	Log(format string, count int) (string, error)
}

// GitFetchOptions contains the options for FetchWithRefspec.
type GitFetchOptions struct {
	Remote      string
	Refspec     string
	Depth       int
	Submodules  bool
	MaxAttempts int
}

var _ GitCliInterface = &GitCli{}

// GitCli provides methods for executing git commands via a CLI executor.
type GitCli struct {
	Executor CliExecutorInterface
	Workdir  string
	ExtraEnv []string
}

var minGitVersion = [3]int{2, 25, 0}
var gitVersionRegex = regexp.MustCompile(`git version (\d+)\.(\d+)\.(\d+)`)

// NewGitCli creates a new GitCli instance after verifying git is available and meets the minimum version.
func NewGitCli(executor CliExecutorInterface, workdir string) (*GitCli, error) {
	gitCliAvailable, err := CheckCliToolAvailable("git")
	if err != nil {
		return nil, err
	}
	if !gitCliAvailable {
		return nil, errors.New("git CLI is not available")
	}

	stdout, _, _, err := executor.Execute(Command("git", "--version"))
	if err != nil {
		return nil, fmt.Errorf("failed to get git version: %w", err)
	}
	version, err := parseGitVersion(stdout)
	if err != nil {
		return nil, err
	}
	if !isVersionAtLeast(version, minGitVersion) {
		return nil, fmt.Errorf("git version %d.%d.%d is below minimum required %d.%d.%d",
			version[0], version[1], version[2],
			minGitVersion[0], minGitVersion[1], minGitVersion[2])
	}

	return &GitCli{
		Executor: executor,
		Workdir:  workdir,
	}, nil
}

func parseGitVersion(output string) ([3]int, error) {
	m := gitVersionRegex.FindStringSubmatch(output)
	if m == nil {
		return [3]int{}, fmt.Errorf("failed to parse git version from output: %q", output)
	}
	var version [3]int
	for i := 0; i < 3; i++ {
		v, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, fmt.Errorf("failed to parse git version component %q: %w", m[i+1], err)
		}
		version[i] = v
	}
	return version, nil
}

func isVersionAtLeast(version, minimum [3]int) bool {
	return slices.Compare(version[:], minimum[:]) >= 0
}

func (g *GitCli) SetEnv(key, value string) {
	g.ExtraEnv = append(g.ExtraEnv, key+"="+value)
}

func (g *GitCli) buildCmd(args []string) Cmd {
	cmd := Cmd{Name: "git", Args: args, Dir: g.Workdir}
	if len(g.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), g.ExtraEnv...)
	}
	return cmd
}

// run executes a git command in the working directory, logs it, and returns
// the trimmed stdout. Returns an error if the command fails or exits non-zero.
func (g *GitCli) run(args ...string) (string, error) {
	gitLog.Debugf("[command]: git %s (in %s)", strings.Join(args, " "), g.Workdir)
	stdout, stderr, exitCode, err := g.Executor.Execute(g.buildCmd(args))
	if err != nil || exitCode != 0 {
		gitLog.Debugf("git %s stderr: %s", args[0], stderr)
		return "", fmt.Errorf("git %s failed with exit code %d: %w", args[0], exitCode, err)
	}
	return strings.TrimSpace(stdout), nil
}

// --- Repository operations ---

// Init initializes a new git repository in the working directory.
// Runs: git init
func (g *GitCli) Init() error {
	_, err := g.run("init")
	return err
}

// SetSparseCheckout configures sparse checkout for the given directories.
// Runs: git config --local core.sparseCheckout true && git sparse-checkout set <directories...>
func (g *GitCli) SetSparseCheckout(directories []string) error {
	gitLog.Debugf("Configuring sparse checkout: %v", directories)
	if len(directories) == 0 {
		return fmt.Errorf("directories parameter empty")
	}

	if err := g.ConfigLocal("core.sparseCheckout", "true"); err != nil {
		return fmt.Errorf("failed to enable sparse checkout: %w", err)
	}

	args := append([]string{"sparse-checkout", "set", "--"}, directories...)
	_, err := g.run(args...)
	return err
}

// ConfigLocal sets a git config value locally in the repository.
// Runs: git config --local <key> <value>
func (g *GitCli) ConfigLocal(key, value string) error {
	if key == "" {
		return errors.New("config key must not be empty")
	}
	_, err := g.run("config", "--local", key, value)
	return err
}

// Commit creates a commit with the specified message.
// Runs: git commit -m <message>
func (g *GitCli) Commit(message string) (string, error) {
	return g.run("commit", "-m", message)
}

// Merge merges the specified ref into the current branch with the given commit message.
// Uses --no-ff to always create a merge commit. Returns the merge output.
// If the merge is already up-to-date, no commit is created and no error is returned.
// Runs: git merge -m <message> --no-ff --allow-unrelated-histories <ref>
func (g *GitCli) Merge(ref, message string) (string, error) {
	if ref == "" {
		return "", errors.New("ref must not be empty")
	}
	return g.run("merge", "-m", message, "--no-ff", "--allow-unrelated-histories", ref)
}

// --- Remote operations ---

// RemoteAdd adds a new remote with the given name and URL.
// Runs: git remote add <name> <url>
func (g *GitCli) RemoteAdd(name, url string) (string, error) {
	if name == "" {
		return "", errors.New("remote name must not be empty")
	}
	if url == "" {
		return "", errors.New("remote url must not be empty")
	}
	return g.run("remote", "add", name, url)
}

// FetchTags fetches all tags from the remote and returns the list of tags.
// Runs: git fetch --tags && git tag -l
func (g *GitCli) FetchTags() ([]string, error) {
	if _, err := g.run("fetch", "--tags"); err != nil {
		return nil, err
	}

	stdout, err := g.run("tag", "-l")
	if err != nil {
		return nil, err
	}

	tags := []string{}
	for _, tag := range strings.Split(stdout, "\n") {
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// FetchWithRefspec fetches a specific refspec from a remote with optional depth and retry.
// Runs: git fetch [--recurse-submodules=yes] [--depth=N] <remote> --update-head-ok --force [<refspec>]
func (g *GitCli) FetchWithRefspec(opts GitFetchOptions) error {
	if opts.Remote == "" {
		return errors.New("remote must not be empty")
	}
	gitArgs := []string{"fetch"}

	if opts.Submodules {
		gitArgs = append(gitArgs, "--recurse-submodules=yes")
	}

	if opts.Depth > 0 {
		gitArgs = append(gitArgs, fmt.Sprintf("--depth=%d", opts.Depth))
	}

	gitArgs = append(gitArgs, opts.Remote, "--update-head-ok", "--force")

	if opts.Refspec != "" {
		gitArgs = append(gitArgs, opts.Refspec)
	}

	retryer := NewRetryer(func() (string, string, int, error) {
		return g.Executor.Execute(g.buildCmd(gitArgs))
	})
	if opts.MaxAttempts > 0 {
		retryer = retryer.WithMaxAttempts(opts.MaxAttempts)
	}
	retryer = retryer.
		StopOnExitCode(128).
		StopIfOutputContains("Authentication failed").
		StopIfOutputContains("could not read Username").
		StopIfOutputContains("fatal: repository").
		StopIfOutputContains("Permission denied").
		StopIfOutputContains("Could not resolve hostname")

	_, stderr, exitCode, err := retryer.Run()
	if err != nil || exitCode != 0 {
		gitLog.Debugf("git fetch stderr: %s", stderr)
		return fmt.Errorf("git fetch failed with exit code %d: %w", exitCode, err)
	}
	return nil
}

// Checkout checks out the specified ref (branch, tag, or commit SHA).
// Runs: git checkout <ref>
func (g *GitCli) Checkout(ref string) error {
	if ref == "" {
		return errors.New("ref must not be empty")
	}
	_, err := g.run("checkout", ref)
	return err
}

// SubmoduleUpdate initializes and/or updates submodules recursively.
// Runs: git submodule update --recursive [--init] [--force] [--depth=N] [-- paths...]
func (g *GitCli) SubmoduleUpdate(init bool, depth int, paths []string) error {
	gitArgs := []string{"submodule", "update", "--recursive"}

	if init {
		gitArgs = append(gitArgs, "--init")
	}

	gitArgs = append(gitArgs, "--force")

	if depth > 0 {
		gitArgs = append(gitArgs, fmt.Sprintf("--depth=%d", depth))
	}

	if len(paths) > 0 {
		gitArgs = append(gitArgs, "--")
		gitArgs = append(gitArgs, paths...)
	}

	_, err := g.run(gitArgs...)
	return err
}

// --- Info operations ---

// RevParse resolves a git ref to its SHA. If short is true, returns a shortened SHA.
// Runs: git rev-parse [--short[=N]] <ref>
func (g *GitCli) RevParse(ref string, short bool, length int) (string, error) {
	if ref == "" {
		return "", errors.New("ref must not be empty")
	}
	gitArgs := []string{"rev-parse"}

	if short {
		if length > 0 {
			gitArgs = append(gitArgs, fmt.Sprintf("--short=%d", length))
		} else {
			gitArgs = append(gitArgs, "--short")
		}
	}
	gitArgs = append(gitArgs, ref)

	return g.run(gitArgs...)
}

// Log runs git log with the specified format and count, returning the output.
// Runs: git log [-N] [--pretty=<format>]
func (g *GitCli) Log(format string, count int) (string, error) {
	gitArgs := []string{"log"}

	if count > 0 {
		gitArgs = append(gitArgs, fmt.Sprintf("-%d", count))
	}
	if format != "" {
		gitArgs = append(gitArgs, fmt.Sprintf("--pretty=%s", format))
	}

	return g.run(gitArgs...)
}
