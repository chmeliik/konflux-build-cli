package gitclone

import (
	"reflect"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
)

var ParamsConfig = map[string]common.Parameter{
	"url": {
		Name:       "url",
		ShortName:  "u",
		EnvVarName: "KBC_GIT_CLONE_URL",
		TypeKind:   reflect.String,
		Usage:      "Repository URL to clone from.",
		Required:   true,
	},
	"revision": {
		Name:         "revision",
		EnvVarName:   "KBC_GIT_CLONE_REVISION",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Revision to checkout (branch, tag, sha, ref).",
	},
	"refspec": {
		Name:         "refspec",
		EnvVarName:   "KBC_GIT_CLONE_REFSPEC",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Refspec to fetch before checking out revision.",
	},
	"submodules": {
		Name:         "submodules",
		EnvVarName:   "KBC_GIT_CLONE_SUBMODULES",
		TypeKind:     reflect.Bool,
		DefaultValue: "true",
		Usage:        "Initialize and fetch git submodules.",
	},
	"submodule-paths": {
		Name:         "submodule-paths",
		EnvVarName:   "KBC_GIT_CLONE_SUBMODULE_PATHS",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "CSV list of specific submodule paths to initialize and fetch. Only submodules in the specified directories and their subdirectories will be fetched. Empty string fetches all submodules. Parameter 'submodules' must be set to 'true' to make this parameter applicable.",
	},
	"depth": {
		Name:         "depth",
		EnvVarName:   "KBC_GIT_CLONE_DEPTH",
		TypeKind:     reflect.Int,
		DefaultValue: "1",
		Usage:        "Perform a shallow clone, fetching only the most recent N commits. Set to 0 to fetch the full commit history.",
	},
	"short-commit-length": {
		Name:         "short-commit-length",
		EnvVarName:   "KBC_GIT_CLONE_SHORT_COMMIT_LENGTH",
		TypeKind:     reflect.Int,
		DefaultValue: "7",
		Usage:        "Length of short commit SHA.",
	},
	"ssl-verify": {
		Name:         "ssl-verify",
		EnvVarName:   "KBC_GIT_CLONE_SSL_VERIFY",
		TypeKind:     reflect.Bool,
		DefaultValue: "true",
		Usage:        "Verify SSL certificates when cloning. Disabling is not advised unless you trust the git remote.",
	},
	"subdirectory": {
		Name:         "subdirectory",
		EnvVarName:   "KBC_GIT_CLONE_SUBDIRECTORY",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Subdirectory inside the output directory to clone the repo into.",
	},
	"sparse-checkout-directories": {
		Name:         "sparse-checkout-directories",
		EnvVarName:   "KBC_GIT_CLONE_SPARSE_CHECKOUT_DIRECTORIES",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "CSV list of directories to check out when performing a sparse checkout.",
	},
	"delete-existing": {
		Name:         "delete-existing",
		EnvVarName:   "KBC_GIT_CLONE_DELETE_EXISTING",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "Clean out the contents of the destination directory if it already exists before cloning.",
	},
	"enable-symlink-check": {
		Name:         "enable-symlink-check",
		EnvVarName:   "KBC_GIT_CLONE_ENABLE_SYMLINK_CHECK",
		TypeKind:     reflect.Bool,
		DefaultValue: "true",
		Usage:        "Check symlinks in the repo. If they're pointing outside of the repo, the build will fail.",
	},
	"fetch-tags": {
		Name:         "fetch-tags",
		EnvVarName:   "KBC_GIT_CLONE_FETCH_TAGS",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "Fetch all tags for the repo.",
	},
	"merge-target-branch": {
		Name:         "merge-target-branch",
		EnvVarName:   "KBC_GIT_CLONE_MERGE_TARGET_BRANCH",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "Merge the target branch into the checked-out revision.",
	},
	"target-branch": {
		Name:         "target-branch",
		EnvVarName:   "KBC_GIT_CLONE_TARGET_BRANCH",
		TypeKind:     reflect.String,
		DefaultValue: "main",
		Usage:        "The target branch to merge into the revision when --merge-target-branch is set.",
	},
	"merge-source-repo-url": {
		Name:         "merge-source-repo-url",
		EnvVarName:   "KBC_GIT_CLONE_MERGE_SOURCE_REPO_URL",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "URL of the repository to fetch the target branch from when merge-target-branch is true. If empty, uses the same repository (origin). This allows merging a branch from a different repository.",
	},
	"merge-source-depth": {
		Name:         "merge-source-depth",
		EnvVarName:   "KBC_GIT_CLONE_MERGE_SOURCE_DEPTH",
		TypeKind:     reflect.Int,
		DefaultValue: "0",
		Usage:        "Perform a shallow fetch of the target branch, fetching only the most recent N commits. If 0, fetches the full history of the target branch.",
	},
	"merge-commit-author-name": {
		Name:         "merge-commit-author-name",
		EnvVarName:   "KBC_GIT_CLONE_MERGE_COMMIT_AUTHOR_NAME",
		TypeKind:     reflect.String,
		DefaultValue: "Konflux CI Git Clone",
		Usage:        "Author name used for merge commits when --merge-target-branch is set.",
	},
	"merge-commit-author-email": {
		Name:         "merge-commit-author-email",
		EnvVarName:   "KBC_GIT_CLONE_MERGE_COMMIT_AUTHOR_EMAIL",
		TypeKind:     reflect.String,
		DefaultValue: "git-clone@konflux-ci.dev",
		Usage:        "Author email used for merge commits when --merge-target-branch is set.",
	},
	"output-dir": {
		Name:         "output-dir",
		ShortName:    "o",
		EnvVarName:   "KBC_GIT_CLONE_OUTPUT_DIR",
		TypeKind:     reflect.String,
		DefaultValue: ".",
		Usage:        "Output directory where the repository will be cloned (the subdirectory parameter will be appended to this).",
	},
	"retry-max-attempts": {
		Name:         "retry-max-attempts",
		EnvVarName:   "KBC_GIT_CLONE_RETRY_MAX_ATTEMPTS",
		TypeKind:     reflect.Int,
		DefaultValue: "10",
		Usage:        "Maximum number of retry attempts for git network operations.",
	},
	"basic-auth-directory": {
		Name:         "basic-auth-directory",
		EnvVarName:   "KBC_GIT_CLONE_BASIC_AUTH_DIRECTORY",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Path to directory containing basic auth credentials (.git-credentials and .gitconfig, or username and password files).",
	},
	"ssh-directory": {
		Name:         "ssh-directory",
		EnvVarName:   "KBC_GIT_CLONE_SSH_DIRECTORY",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Path to directory containing SSH keys to use for git operations.",
	},
}

type Params struct {
	URL                       string `paramName:"url"`
	Revision                  string `paramName:"revision"`
	Refspec                   string `paramName:"refspec"`
	Submodules                bool   `paramName:"submodules"`
	SubmodulePaths            string `paramName:"submodule-paths"`
	Depth                     int    `paramName:"depth"`
	ShortCommitLength         int    `paramName:"short-commit-length"`
	SSLVerify                 bool   `paramName:"ssl-verify"`
	Subdirectory              string `paramName:"subdirectory"`
	SparseCheckoutDirectories string `paramName:"sparse-checkout-directories"`
	DeleteExisting            bool   `paramName:"delete-existing"`
	EnableSymlinkCheck        bool   `paramName:"enable-symlink-check"`
	FetchTags                 bool   `paramName:"fetch-tags"`
	MergeTargetBranch         bool   `paramName:"merge-target-branch"`
	TargetBranch              string `paramName:"target-branch"`
	MergeSourceRepoURL        string `paramName:"merge-source-repo-url"`
	MergeSourceDepth          int    `paramName:"merge-source-depth"`
	MergeCommitAuthorName     string `paramName:"merge-commit-author-name"`
	MergeCommitAuthorEmail    string `paramName:"merge-commit-author-email"`
	OutputDir                 string `paramName:"output-dir"`
	RetryMaxAttempts          int    `paramName:"retry-max-attempts"`
	BasicAuthDirectory        string `paramName:"basic-auth-directory"`
	SSHDirectory              string `paramName:"ssh-directory"`
}
