package gitclone

import (
	"fmt"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

// Results holds commit information gathered after a successful clone.
type Results struct {
	Commit          string `json:"commit"`
	ShortCommit     string `json:"shortCommit"`
	URL             string `json:"url"`
	CommitTimestamp string `json:"commitTimestamp"`
	MergedSha       string `json:"mergedSha,omitempty"`
	ChainsGitURL    string `json:"CHAINS-GIT_URL"`
	ChainsGitCommit string `json:"CHAINS-GIT_COMMIT"`
}

func (c *GitClone) gatherCommitInfo() error {
	// Get full SHA
	sha, err := c.CliWrappers.GitCli.RevParse("HEAD", false, 0)
	if err != nil {
		return fmt.Errorf("failed to get commit SHA: %w", err)
	}
	c.Results.Commit = sha

	// Get short SHA
	shortSha, err := c.CliWrappers.GitCli.RevParse("HEAD", true, c.Params.ShortCommitLength)
	if err != nil {
		return fmt.Errorf("failed to get short commit SHA: %w", err)
	}
	c.Results.ShortCommit = shortSha

	// Get commit timestamp
	timestamp, err := c.CliWrappers.GitCli.Log("%ct", 1)
	if err != nil {
		return fmt.Errorf("failed to get commit timestamp: %w", err)
	}
	c.Results.CommitTimestamp = timestamp

	c.Results.URL = c.Params.URL

	// CHAINS results are duplicates for Tekton Chains provenance
	c.Results.ChainsGitURL = c.Params.URL
	c.Results.ChainsGitCommit = sha

	l.Logger.Debugf("Commit: %s", c.Results.Commit)
	l.Logger.Debugf("Short commit: %s", c.Results.ShortCommit)
	l.Logger.Debugf("Commit timestamp: %s", c.Results.CommitTimestamp)

	return nil
}

func (c *GitClone) outputResults() error {
	resultJson, err := c.ResultsWriter.CreateResultJson(c.Results)
	if err != nil {
		return fmt.Errorf("failed to create results json: %w", err)
	}
	fmt.Println(resultJson)
	return nil
}
