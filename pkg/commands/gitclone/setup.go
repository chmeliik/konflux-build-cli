package gitclone

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const (
	envGitConfigGlobal = "GIT_CONFIG_GLOBAL"
	envGitSSHCommand   = "GIT_SSH_COMMAND"
	envGitSSLNoVerify  = "GIT_SSL_NO_VERIFY"
)

// cleanCheckoutDir removes all contents from the checkout directory while preserving
// the directory itself. We iterate over entries rather than using os.RemoveAll on the
// directory because the checkout directory may be a mount point (e.g., a Kubernetes
// volume) that should not be removed.
func (c *GitClone) cleanCheckoutDir() error {
	checkoutDir := c.getCheckoutDir()

	// Use Lstat (not Stat) so we detect symlinks instead of following them.
	info, err := os.Lstat(checkoutDir)
	if os.IsNotExist(err) {
		// Directory doesn't exist, nothing to clean
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to stat checkout directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("checkout directory is a symlink, refusing to clean: %s", checkoutDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("checkout path exists but is not a directory: %s", checkoutDir)
	}

	l.Logger.Debugf("Cleaning existing checkout directory: %s", checkoutDir)

	entries, err := os.ReadDir(checkoutDir)
	if err != nil {
		return fmt.Errorf("failed to read checkout directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(checkoutDir, entry.Name())
		if err := os.RemoveAll(entryPath); err != nil {
			return fmt.Errorf("failed to remove %s: %w", entryPath, err)
		}
	}

	return nil
}

// setupGitConfig configures git settings for SSL verification.
func (c *GitClone) setupGitConfig() {
	if !c.Params.SSLVerify {
		l.Logger.Debug("Disabling SSL verification (GIT_SSL_NO_VERIFY=true)")
		c.CliWrappers.GitCli.SetEnv(envGitSSLNoVerify, "true")
	}
}

// setupBasicAuth sets up git credentials from a basic-auth workspace.
// Supports two formats:
// 1. .git-credentials and .gitconfig files (copied directly)
// 2. username and password files (kubernetes.io/basic-auth secret format)
func (c *GitClone) setupBasicAuth() error {
	if c.Params.BasicAuthDirectory == "" {
		return nil
	}

	authDir := c.Params.BasicAuthDirectory

	if _, err := os.Stat(authDir); err != nil {
		if os.IsNotExist(err) {
			l.Logger.Debugf("Basic auth directory not found: %s", authDir)
			return nil
		}
		return fmt.Errorf("failed to access basic auth directory: %w", err)
	}

	gitCredentialsPath := filepath.Join(authDir, ".git-credentials")
	gitConfigPath := filepath.Join(authDir, ".gitconfig")
	usernamePath := filepath.Join(authDir, "username")
	passwordPath := filepath.Join(authDir, "password")

	destCredentials := filepath.Join(c.internalDir, ".git-credentials")
	destConfig := filepath.Join(c.internalDir, ".gitconfig")

	// Format 1: .git-credentials and .gitconfig files
	if fileExists(gitCredentialsPath) && fileExists(gitConfigPath) {
		l.Logger.Debug("Setting up basic auth from .git-credentials and .gitconfig")

		if err := copyFile(gitCredentialsPath, destCredentials, 0400); err != nil {
			return fmt.Errorf("failed to copy .git-credentials: %w", err)
		}

		configContent, err := readFileWithLimit(gitConfigPath, maxAuthFileSize)
		if err != nil {
			return fmt.Errorf("failed to read .gitconfig: %w", err)
		}
		rewritten := rewriteGitConfigCredentialHelper(string(configContent), destCredentials)
		if err := os.WriteFile(destConfig, []byte(rewritten), 0400); err != nil {
			return fmt.Errorf("failed to write .gitconfig: %w", err)
		}

		c.CliWrappers.GitCli.SetEnv(envGitConfigGlobal, destConfig)

		l.Logger.Debug("Basic auth credentials configured")
		return nil
	}

	// Format 2: kubernetes.io/basic-auth secret (username and password files)
	if fileExists(usernamePath) && fileExists(passwordPath) {
		l.Logger.Debug("Setting up basic auth from username/password files")

		username, err := readFileWithLimit(usernamePath, maxAuthFileSize)
		if err != nil {
			return fmt.Errorf("failed to read username file: %w", err)
		}

		password, err := readFileWithLimit(passwordPath, maxAuthFileSize)
		if err != nil {
			return fmt.Errorf("failed to read password file: %w", err)
		}

		parsedURL, err := url.Parse(c.Params.URL)
		if err != nil {
			return fmt.Errorf("failed to parse repository URL: %w", err)
		}
		if parsedURL.Scheme == "" || parsedURL.Host == "" {
			return fmt.Errorf("basic-auth requires an HTTP(S) URL with scheme and host, got: %s", sanitizeURL(c.Params.URL))
		}
		hostname := parsedURL.Host

		credentialsContent := fmt.Sprintf("%s://%s:%s@%s\n",
			parsedURL.Scheme,
			url.QueryEscape(strings.TrimSpace(string(username))),
			url.QueryEscape(strings.TrimSpace(string(password))),
			hostname)

		if err := os.WriteFile(destCredentials, []byte(credentialsContent), 0400); err != nil {
			return fmt.Errorf("failed to write .git-credentials: %w", err)
		}

		gitConfigContent := fmt.Sprintf("[credential \"%s://%s\"]\n  helper = store --file=%s\n", parsedURL.Scheme, hostname, destCredentials)
		if err := os.WriteFile(destConfig, []byte(gitConfigContent), 0400); err != nil {
			return fmt.Errorf("failed to write .gitconfig: %w", err)
		}

		c.CliWrappers.GitCli.SetEnv(envGitConfigGlobal, destConfig)

		l.Logger.Debugf("Basic auth credentials configured for %s", hostname)
		return nil
	}

	return fmt.Errorf("unknown basic-auth workspace format: expected .git-credentials/.gitconfig or username/password files")
}

// rewriteGitConfigCredentialHelper rewrites "helper = store" lines in a git config
// to include an explicit --file flag pointing to the given credentials path.
func rewriteGitConfigCredentialHelper(configContent, credentialsPath string) string {
	lines := strings.Split(configContent, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "helper = store" {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = fmt.Sprintf("%shelper = store --file=%s", indent, credentialsPath)
		}
	}
	return strings.Join(lines, "\n")
}

// setupSSH sets up SSH keys from an ssh-directory workspace.
// SSH files are copied to c.internalDir/.ssh/ and GIT_SSH_COMMAND is configured
// with explicit flags so that git uses the custom SSH config without modifying $HOME.
func (c *GitClone) setupSSH() error {
	if c.Params.SSHDirectory == "" {
		return nil
	}

	sshDir := c.Params.SSHDirectory

	if _, err := os.Stat(sshDir); err != nil {
		if os.IsNotExist(err) {
			l.Logger.Debugf("SSH directory not found: %s", sshDir)
			return nil
		}
		return fmt.Errorf("failed to access SSH directory: %w", err)
	}

	l.Logger.Debugf("Setting up SSH keys from %s", sshDir)

	destSSHDir := filepath.Join(c.internalDir, ".ssh")

	if err := os.MkdirAll(destSSHDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return fmt.Errorf("failed to read SSH directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		srcPath := filepath.Join(sshDir, entry.Name())
		destPath := filepath.Join(destSSHDir, entry.Name())

		if err := copyFile(srcPath, destPath, 0400); err != nil {
			return fmt.Errorf("failed to copy SSH file %s: %w", entry.Name(), err)
		}
	}

	sshCmd := "ssh"

	configPath := filepath.Join(destSSHDir, "config")
	if fileExists(configPath) {
		sshCmd += fmt.Sprintf(` -F "%s"`, configPath)
	} else {
		sshCmd += " -F /dev/null"
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "id_") && !strings.HasSuffix(name, ".pub") && !entry.IsDir() {
			sshCmd += fmt.Sprintf(` -i "%s"`, filepath.Join(destSSHDir, name))
		}
	}

	knownHostsPath := filepath.Join(destSSHDir, "known_hosts")
	if fileExists(knownHostsPath) {
		sshCmd += fmt.Sprintf(` -o UserKnownHostsFile="%s"`, knownHostsPath)
	}

	c.CliWrappers.GitCli.SetEnv(envGitSSHCommand, sshCmd)

	l.Logger.Debugf("SSH keys configured (GIT_SSH_COMMAND=%s)", sshCmd)
	return nil
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil && !info.IsDir()
}

// maxAuthFileSize is the maximum allowed size for auth-related files
// (.gitconfig, .git-credentials, SSH keys, username, password).
const maxAuthFileSize = 1 << 20 // 1MB

// readFileWithLimit reads a file, rejecting files larger than maxSize.
// Uses file-descriptor-based stat and limited read to avoid TOCTOU races.
func readFileWithLimit(path string, maxSize int64) (data []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSize {
		return nil, fmt.Errorf("authentication file exceeds maximum allowed size (%d bytes)", maxSize)
	}

	// Use LimitReader to enforce the size cap during read, even if the file
	// was extended between Stat and Read (belt-and-suspenders).
	data, err = io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("authentication file exceeds maximum allowed size (%d bytes)", maxSize)
	}
	return data, nil
}

// copyFile copies a file from src to dest with the specified permissions.
func copyFile(src, dest string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}

	data, err := readFileWithLimit(src, maxAuthFileSize)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, perm)
}
