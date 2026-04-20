package gitclone

import (
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/spf13/cobra"
)

type CliWrappers struct {
	GitCli cliwrappers.GitCliInterface
}

type GitClone struct {
	Params        *Params
	CliWrappers   CliWrappers
	Results       Results
	ResultsWriter common.ResultsWriterInterface
	internalDir   string
}

func New(cmd *cobra.Command) (*GitClone, error) {
	gitClone := &GitClone{}

	params := &Params{}
	if err := common.ParseParameters(cmd, ParamsConfig, params); err != nil {
		return nil, err
	}
	gitClone.Params = params

	gitClone.ResultsWriter = common.NewResultsWriter()

	return gitClone, nil
}

func (c *GitClone) initCliWrappers() error {
	executor := cliwrappers.NewCliExecutor()

	gitCli, err := cliwrappers.NewGitCli(executor, c.getCheckoutDir())
	if err != nil {
		return err
	}
	c.CliWrappers.GitCli = gitCli

	return nil
}

func (c *GitClone) Run() error {
	c.logParams()

	if err := c.validateParams(); err != nil {
		return err
	}

	if c.CliWrappers.GitCli == nil {
		if err := c.initCliWrappers(); err != nil {
			return err
		}
	}

	// internalDir is a temporary directory for storing credentials and config files
	// (e.g., .git-credentials, .gitconfig, SSH keys) without modifying the user's home directory.
	internalDir, err := os.MkdirTemp("", "git-clone-internal-*")
	if err != nil {
		return fmt.Errorf("failed to create internal directory: %w", err)
	}
	c.internalDir = internalDir

	defer func() {
		_ = os.RemoveAll(c.internalDir)
	}()

	c.setupGitConfig()

	// Setup authentication
	if err := c.setupBasicAuth(); err != nil {
		return err
	}

	if err := c.setupSSH(); err != nil {
		return err
	}

	// Verify the checkout directory path doesn't escape OutputDir via symlinks
	// before any destructive operations (clean/clone).
	if err := c.verifyCheckoutDirContainment(); err != nil {
		return fmt.Errorf("path containment check: %w", err)
	}

	// Clean checkout directory if requested
	if c.Params.DeleteExisting {
		if err := c.cleanCheckoutDir(); err != nil {
			return err
		}
	}

	if err := c.performClone(); err != nil {
		return err
	}

	if c.Params.MergeTargetBranch {
		if err := c.mergeTargetBranch(); err != nil {
			return err
		}
	}

	if c.Params.EnableSymlinkCheck {
		if err := common.CheckSymlinks(c.getCheckoutDir()); err != nil {
			return fmt.Errorf("symlink check: %w", err)
		}
	}

	if err := c.gatherCommitInfo(); err != nil {
		return err
	}

	if c.Params.FetchTags {
		if _, err := c.CliWrappers.GitCli.FetchTags(); err != nil {
			return err
		}
	}

	return c.outputResults()
}

func (c *GitClone) logParams() {
	// Log URL params separately with sanitization to avoid leaking credentials
	l.Logger.Infof("[param] url: %s", sanitizeURL(c.Params.URL))
	if c.Params.MergeSourceRepoURL != "" {
		l.Logger.Infof("[param] merge-source-repo-url: %s", sanitizeURL(c.Params.MergeSourceRepoURL))
	}
	common.LogParameters(ParamsConfig, c.Params, "url", "merge-source-repo-url")
}

// sanitizeURL removes credentials from a URL for safe logging.
func sanitizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if parsed.User != nil {
		parsed.User = url.User("***")
	}
	return parsed.String()
}

// normalizeGitURL strips trailing slashes and ".git" suffix for URL comparison.
func normalizeGitURL(rawURL string) string {
	rawURL = strings.TrimSuffix(rawURL, "/")
	rawURL = strings.TrimSuffix(rawURL, ".git")
	return rawURL
}

func (c *GitClone) validateParams() error {
	if c.Params.URL == "" {
		return fmt.Errorf("url parameter is required")
	}
	if c.Params.Depth < 0 {
		return fmt.Errorf("depth must be >= 0 (0 means full history)")
	}
	if c.Params.MergeSourceDepth < 0 {
		return fmt.Errorf("merge-source-depth must be >= 0 (0 means full history)")
	}
	if c.Params.RetryMaxAttempts < 1 {
		return fmt.Errorf("retry-max-attempts must be >= 1, got %d", c.Params.RetryMaxAttempts)
	}
	if c.Params.RetryMaxAttempts > 100 {
		return fmt.Errorf("retry-max-attempts must be <= 100, got %d", c.Params.RetryMaxAttempts)
	}
	if c.Params.MergeTargetBranch && c.Params.TargetBranch == "" {
		return fmt.Errorf("target-branch is required when merge-target-branch is true")
	}

	// Validate subdirectory for path traversal
	if c.Params.Subdirectory != "" {
		if filepath.IsAbs(c.Params.Subdirectory) {
			return fmt.Errorf("subdirectory must be a relative path, got absolute path: %s", c.Params.Subdirectory)
		}
		if strings.Contains(c.Params.Subdirectory, "..") {
			return fmt.Errorf("subdirectory must not contain path traversal (..): %s", c.Params.Subdirectory)
		}
		baseDir := c.Params.OutputDir
		if baseDir == "" {
			baseDir = "."
		}
		absOutput, err := filepath.Abs(baseDir)
		if err != nil {
			return fmt.Errorf("failed to resolve output dir: %w", err)
		}
		absCheckout, err := filepath.Abs(filepath.Join(baseDir, c.Params.Subdirectory))
		if err != nil {
			return fmt.Errorf("failed to resolve checkout dir: %w", err)
		}
		if absCheckout != absOutput && !strings.HasPrefix(absCheckout, absOutput+string(filepath.Separator)) {
			return fmt.Errorf("subdirectory must not escape output directory")
		}
	}

	return nil
}

func (c *GitClone) getCheckoutDir() string {
	return filepath.Join(c.Params.OutputDir, c.Params.Subdirectory)
}

// verifyCheckoutDirContainment ensures the checkout directory, after resolving
// all symlinks, remains within the output directory. This prevents symlink-based
// path traversal where a pre-existing symlink under OutputDir redirects
// operations (deletion, git init) outside the workspace.
func (c *GitClone) verifyCheckoutDirContainment() error {
	if c.Params.Subdirectory == "" {
		return nil // checkout dir is the output dir itself
	}

	baseDir := c.Params.OutputDir
	if baseDir == "" {
		baseDir = "."
	}

	resolvedOutput, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}
	if evaluated, evalErr := filepath.EvalSymlinks(resolvedOutput); evalErr == nil {
		resolvedOutput = evaluated
	}

	// Walk each component of Subdirectory under OutputDir. If any existing
	// component is a symlink, reject it.
	parts := strings.Split(filepath.Clean(c.Params.Subdirectory), string(filepath.Separator))
	current := baseDir
	for _, part := range parts {
		current = filepath.Join(current, part)
		linfo, lErr := os.Lstat(current)
		if os.IsNotExist(lErr) {
			break // remaining components don't exist yet
		}
		if lErr != nil {
			return fmt.Errorf("failed to stat path component %s: %w", current, lErr)
		}
		if linfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("checkout path component is a symlink: %s (symlinks in the checkout path are not allowed)", current)
		}
	}

	// If the full checkout path already exists, do a resolved-path containment
	// check as belt-and-suspenders.
	checkoutDir := c.getCheckoutDir()
	if _, statErr := os.Lstat(checkoutDir); statErr == nil {
		resolvedCheckout, evalErr := filepath.EvalSymlinks(checkoutDir)
		if evalErr != nil {
			return fmt.Errorf("failed to resolve checkout directory: %w", evalErr)
		}
		resolvedCheckout, err = filepath.Abs(resolvedCheckout)
		if err != nil {
			return fmt.Errorf("failed to get absolute checkout path: %w", err)
		}
		if resolvedCheckout != resolvedOutput && !strings.HasPrefix(resolvedCheckout, resolvedOutput+string(filepath.Separator)) {
			return fmt.Errorf("checkout directory %s resolves to %s which is outside output directory %s", checkoutDir, resolvedCheckout, resolvedOutput)
		}
	}

	return nil
}

// performClone initializes a git repo, fetches the requested revision, and checks it out.
func (c *GitClone) performClone() error {
	checkoutDir := c.getCheckoutDir()

	// Ensure checkout directory exists
	if err := os.MkdirAll(checkoutDir, 0755); err != nil {
		return fmt.Errorf("failed to create checkout directory: %w", err)
	}

	// Re-verify containment after creating the directory, in case a path
	// component was replaced between the earlier check and MkdirAll.
	if err := c.verifyCheckoutDirContainment(); err != nil {
		return fmt.Errorf("path containment check after mkdir: %w", err)
	}

	l.Logger.Debug("Initializing git repository")
	if err := c.CliWrappers.GitCli.Init(); err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}

	// Configure sparse checkout if directories are specified
	if c.Params.SparseCheckoutDirectories != "" {
		directories, err := parseCSV(c.Params.SparseCheckoutDirectories)
		if err != nil {
			return fmt.Errorf("failed to parse sparse-checkout-directories: %w", err)
		}
		if err := c.CliWrappers.GitCli.SetSparseCheckout(directories); err != nil {
			return err
		}
	}

	l.Logger.Debugf("Adding remote origin: %s", sanitizeURL(c.Params.URL))
	if _, err := c.CliWrappers.GitCli.RemoteAdd("origin", c.Params.URL); err != nil {
		return fmt.Errorf("git remote add failed: %w", err)
	}

	if err := c.fetchRevision(); err != nil {
		return err
	}

	// If both refspec and revision are set, the refspec is fetched first,
	// then the specific revision is checked out. Otherwise, check out FETCH_HEAD.
	checkoutRef := "FETCH_HEAD"
	if c.Params.Refspec != "" && c.Params.Revision != "" {
		checkoutRef = c.Params.Revision
	}

	l.Logger.Debugf("Checking out %s", checkoutRef)
	if err := c.CliWrappers.GitCli.Checkout(checkoutRef); err != nil {
		return fmt.Errorf("git checkout failed: %w", err)
	}

	if c.Params.Submodules {
		l.Logger.Debug("Updating submodules")
		paths, err := parseCSV(c.Params.SubmodulePaths)
		if err != nil {
			return fmt.Errorf("failed to parse submodule-paths: %w", err)
		}
		if err := c.CliWrappers.GitCli.SubmoduleUpdate(true, c.Params.Depth, paths); err != nil {
			return fmt.Errorf("git submodule update failed: %w", err)
		}
	}

	return nil
}

// fetchRevision fetches refs from the remote based on refspec and revision parameters.
// If a refspec is provided, it is fetched directly. Otherwise, the revision is used as the refspec.
func (c *GitClone) fetchRevision() error {
	refspec := c.Params.Refspec
	if refspec == "" && c.Params.Revision != "" {
		refspec = c.Params.Revision
	}

	l.Logger.Debugf("Fetching from origin (depth=%d, refspec=%s)", c.Params.Depth, refspec)

	maxAttempts := c.Params.RetryMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	err := c.CliWrappers.GitCli.FetchWithRefspec(cliwrappers.GitFetchOptions{
		Remote:      "origin",
		Refspec:     refspec,
		Depth:       c.Params.Depth,
		Submodules:  c.Params.Submodules,
		MaxAttempts: maxAttempts,
	})
	if err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}
	return nil
}

// parseCSV parses a comma-separated string into a slice of trimmed values.
func parseCSV(input string) ([]string, error) {
	if input == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(input))
	reader.TrimLeadingSpace = true
	return reader.Read()
}

func (c *GitClone) mergeTargetBranch() error {
	if c.Params.Depth == 1 {
		l.Logger.Warning("Shallow clone with depth=1 may cause merge conflicts due to insufficient commit history.")
	}

	if c.Params.MergeSourceDepth == 1 {
		l.Logger.Warning("Shallow fetch with merge-source-depth=1 may cause merge conflicts due to insufficient commit history.")
	}

	mergeRemote := "origin"
	if c.Params.MergeSourceRepoURL != "" {
		normalizedOrigin := normalizeGitURL(c.Params.URL)
		normalizedMerge := normalizeGitURL(c.Params.MergeSourceRepoURL)

		if normalizedOrigin == normalizedMerge {
			l.Logger.Debug("Merge source URL is the same as origin. Using existing 'origin' remote.")
		} else {
			l.Logger.Debugf("Merging from different repository: '%s'", c.Params.MergeSourceRepoURL)
			mergeRemote = "merge-source"
			if _, err := c.CliWrappers.GitCli.RemoteAdd(mergeRemote, c.Params.MergeSourceRepoURL); err != nil {
				return err
			}
		}
	}

	maxAttempts := c.Params.RetryMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	err := c.CliWrappers.GitCli.FetchWithRefspec(cliwrappers.GitFetchOptions{
		Remote:      mergeRemote,
		Refspec:     c.Params.TargetBranch,
		Depth:       c.Params.MergeSourceDepth,
		Submodules:  false,
		MaxAttempts: maxAttempts,
	})
	if err != nil {
		return fmt.Errorf("failed to fetch target branch: %w", err)
	}

	err = c.CliWrappers.GitCli.ConfigLocal("user.email", c.Params.MergeCommitAuthorEmail)
	if err != nil {
		return fmt.Errorf("failed to configure merge commit author email: %w", err)
	}
	err = c.CliWrappers.GitCli.ConfigLocal("user.name", c.Params.MergeCommitAuthorName)
	if err != nil {
		return fmt.Errorf("failed to configure merge commit author name: %w", err)
	}

	// Get the current HEAD SHA before merging to use in the commit message
	currentSha, err := c.CliWrappers.GitCli.RevParse("HEAD", false, 0)
	if err != nil {
		return fmt.Errorf("failed to get pre-merge HEAD SHA: %w", err)
	}

	mergeRef := fmt.Sprintf("%s/%s", mergeRemote, c.Params.TargetBranch)
	message := fmt.Sprintf("Merge branch '%s' from %s into %s", c.Params.TargetBranch, mergeRemote, currentSha)
	merge, err := c.CliWrappers.GitCli.Merge(mergeRef, message)
	if err != nil {
		return fmt.Errorf("failed to merge target branch: %w", err)
	}
	l.Logger.Debugf("Merge: %s", merge)

	c.Results.MergedSha, err = c.CliWrappers.GitCli.RevParse("HEAD", false, 0)
	if err != nil {
		return err
	}

	return nil
}
