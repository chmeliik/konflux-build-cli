package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"

	"github.com/containerd/platforms"
	"github.com/keilerkonzept/dockerfile-json/pkg/buildargs"
	"github.com/keilerkonzept/dockerfile-json/pkg/dockerfile"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
)

var BuildParamsConfig = map[string]common.Parameter{
	"containerfile": {
		Name:         "containerfile",
		ShortName:    "f",
		EnvVarName:   "KBC_BUILD_CONTAINERFILE",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Path to Containerfile. If not specified, uses Containerfile/Dockerfile from the context directory.",
	},
	"context": {
		Name:         "context",
		ShortName:    "c",
		EnvVarName:   "KBC_BUILD_CONTEXT",
		TypeKind:     reflect.String,
		DefaultValue: ".",
		Usage:        "Build context directory.",
	},
	"output-ref": {
		Name:       "output-ref",
		ShortName:  "t",
		EnvVarName: "KBC_BUILD_OUTPUT_REF",
		TypeKind:   reflect.String,
		Usage:      `The reference of the output image - [registry/namespace/]name[:tag]. Required.`,
		Required:   true,
	},
	"push": {
		Name:         "push",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_PUSH",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "Push the built image to the registry.",
	},
	"secret-dirs": {
		Name:       "secret-dirs",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_SECRET_DIRS",
		TypeKind:   reflect.Slice,
		Usage:      "Directories containing secret files to make available during build.",
	},
	"workdir-mount": {
		Name:         "workdir-mount",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_WORKDIR_MOUNT",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "Mount the context directory (which is also the workdir) into the build with '--volume $PWD:$WORKDIR_MOUNT'.",
	},
	"build-args": {
		Name:       "build-args",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_BUILD_ARGS",
		TypeKind:   reflect.Slice,
		Usage:      "Arguments to pass to the build using buildah's --build-arg option.",
	},
	"build-args-file": {
		Name:       "build-args-file",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_BUILD_ARGS_FILE",
		TypeKind:   reflect.String,
		Usage:      "Path to a file with build arguments, see https://www.mankier.com/1/buildah-build#--build-arg-file",
	},
	"envs": {
		Name:       "envs",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_ENVS",
		TypeKind:   reflect.Slice,
		Usage:      "Environment variables to pass to the build using buildah's --env option.",
	},
	"labels": {
		Name:       "labels",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_LABELS",
		TypeKind:   reflect.Slice,
		Usage:      "Labels to apply to the image using buildah's --label option.",
	},
	"annotations": {
		Name:       "annotations",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_ANNOTATIONS",
		TypeKind:   reflect.Slice,
		Usage:      "Annotations to apply to the image using buildah's --annotation option.",
	},
	"annotations-file": {
		Name:       "annotations-file",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_ANNOTATIONS_FILE",
		TypeKind:   reflect.String,
		Usage:      "Path to a file with annotations, same file format as --build-args-file.",
	},
	"image-source": {
		Name:       "image-source",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_SOURCE",
		TypeKind:   reflect.String,
		Usage:      "Set the org.opencontainers.image.source annotation (and label) to this value.",
	},
	"image-revision": {
		Name:       "image-revision",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_IMAGE_REVISION",
		TypeKind:   reflect.String,
		Usage:      "Set the org.opencontainers.image.revision annotation (and label) to this value.",
	},
	"legacy-build-timestamp": {
		Name:       "legacy-build-timestamp",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_LEGACY_BUILD_TIMESTAMP",
		TypeKind:   reflect.String,
		Usage:      "Timestamp for the org.opencontainers.image.created annotation (and label). If not provided, uses the current time.\nThis does NOT behave like buildah's --timestamp option, it only sets the annotation and label.\nConflicts with --source-date-epoch.",
	},
	"source-date-epoch": {
		Name:      "source-date-epoch",
		ShortName: "",
		// Note: intentionally omits the KBC_BUILD_ prefix. SOURCE_DATE_EPOCH is a standard variable.
		EnvVarName: "SOURCE_DATE_EPOCH",
		TypeKind:   reflect.String,
		Usage:      "See https://www.mankier.com/1/buildah-build#--source-date-epoch.\nThe timestamp will also be used for the org.opencontainers.image.created annotation and label.\nConflicts with --legacy-build-timestamp.",
	},
	"rewrite-timestamp": {
		Name:       "rewrite-timestamp",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_REWRITE_TIMESTAMP",
		TypeKind:   reflect.Bool,
		Usage:      "See https://www.mankier.com/1/buildah-build#--rewrite-timestamp. Has no effect if --source-date-epoch is not set.",
	},
	"quay-image-expires-after": {
		Name:       "quay-image-expires-after",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_QUAY_IMAGE_EXPIRES_AFTER",
		TypeKind:   reflect.String,
		Usage:      "Time after which the image expires on quay.io (e.g. 1h, 2d, 3w). Adds the quay.expires-after label.",
	},
	"add-legacy-labels": {
		Name:         "add-legacy-labels",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_ADD_LEGACY_LABELS",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "In addition to OCI annotations and labels, also set projectatomic labels (https://github.com/projectatomic/ContainerApplicationGenericLabels).",
	},
	"containerfile-json-output": {
		Name:       "containerfile-json-output",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_CONTAINERFILE_JSON_OUTPUT",
		TypeKind:   reflect.String,
		Usage:      "Write the parsed Containerfile JSON representation to this path.",
	},
	"skip-injections": {
		Name:         "skip-injections",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_SKIP_INJECTIONS",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "Do not inject anything into /usr/share/buildinfo/.",
	},
	"include-legacy-buildinfo-path": {
		Name:         "include-legacy-buildinfo-path",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_INCLUDE_LEGACY_BUILDINFO_PATH",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "When injecting files to /usr/share/buildinfo/, also inject them to /root/buildinfo/.",
	},
	"inherit-labels": {
		Name:         "inherit-labels",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_INHERIT_LABELS",
		TypeKind:     reflect.Bool,
		DefaultValue: "true",
		Usage:        "Inherit labels from the base image or base stages.",
	},
	"target": {
		Name:       "target",
		ShortName:  "",
		EnvVarName: "KBC_BUILD_TARGET",
		TypeKind:   reflect.String,
		Usage:      "Target stage in the Containerfile to build. By default, the target stage is the last stage.",
	},
	"skip-unused-stages": {
		Name:         "skip-unused-stages",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_SKIP_UNUSED_STAGES",
		TypeKind:     reflect.Bool,
		DefaultValue: "true",
		Usage:        "Skip stages in multi-stage builds which don't affect the target stage.",
	},
	"hermetic": {
		Name:         "hermetic",
		ShortName:    "",
		EnvVarName:   "KBC_BUILD_HERMETIC",
		TypeKind:     reflect.Bool,
		DefaultValue: "false",
		Usage:        "Prevent network access while building the containerfile.",
	},
}

type BuildParams struct {
	Containerfile              string   `paramName:"containerfile"`
	Context                    string   `paramName:"context"`
	OutputRef                  string   `paramName:"output-ref"`
	Push                       bool     `paramName:"push"`
	SecretDirs                 []string `paramName:"secret-dirs"`
	WorkdirMount               string   `paramName:"workdir-mount"`
	BuildArgs                  []string `paramName:"build-args"`
	BuildArgsFile              string   `paramName:"build-args-file"`
	Envs                       []string `paramName:"envs"`
	Labels                     []string `paramName:"labels"`
	Annotations                []string `paramName:"annotations"`
	AnnotationsFile            string   `paramName:"annotations-file"`
	ImageSource                string   `paramName:"image-source"`
	ImageRevision              string   `paramName:"image-revision"`
	LegacyBuildTimestamp       string   `paramName:"legacy-build-timestamp"`
	SourceDateEpoch            string   `paramName:"source-date-epoch"`
	RewriteTimestamp           bool     `paramName:"rewrite-timestamp"`
	QuayImageExpiresAfter      string   `paramName:"quay-image-expires-after"`
	AddLegacyLabels            bool     `paramName:"add-legacy-labels"`
	ContainerfileJsonOutput    string   `paramName:"containerfile-json-output"`
	SkipInjections             bool     `paramName:"skip-injections"`
	InheritLabels              bool     `paramName:"inherit-labels"`
	IncludeLegacyBuildinfoPath bool     `paramName:"include-legacy-buildinfo-path"`
	Target                     string   `paramName:"target"`
	SkipUnusedStages           bool     `paramName:"skip-unused-stages"`
	Hermetic                   bool     `paramName:"hermetic"`
	ExtraArgs                  []string // Additional arguments to pass to buildah build
}

type BuildCliWrappers struct {
	BuildahCli          cliWrappers.BuildahCliInterface
	BuildahUnshare      cliWrappers.WrapperCmd
	Unshare             cliWrappers.WrapperCmd
	SelfInUserNamespace cliWrappers.WrapperCmd
}

type BuildResults struct {
	ImageUrl string `json:"image_url"`
	Digest   string `json:"digest,omitempty"`
}

type Build struct {
	Params        *BuildParams
	CliWrappers   BuildCliWrappers
	Results       BuildResults
	ResultsWriter common.ResultsWriterInterface

	containerfilePath string

	// pre-computed buildah arguments
	buildahSecrets        []cliWrappers.BuildahSecret
	mergedLabels          []string
	mergedAnnotations     []string
	buildinfoBuildContext *cliWrappers.BuildahBuildContext

	// temporary workdir and related paths
	tempWorkdir           string
	containerfileCopyPath string
}

func NewBuild(cmd *cobra.Command, extraArgs []string) (*Build, error) {
	build := &Build{}

	params := &BuildParams{}
	if err := common.ParseParameters(cmd, BuildParamsConfig, params); err != nil {
		return nil, err
	}
	// Store any extra arguments passed after -- separator
	params.ExtraArgs = extraArgs
	build.Params = params

	if err := build.initCliWrappers(); err != nil {
		return nil, err
	}

	build.ResultsWriter = common.NewResultsWriter()

	return build, nil
}

func (c *Build) cleanup() {
	if c.tempWorkdir != "" {
		if err := os.RemoveAll(c.tempWorkdir); err != nil {
			l.Logger.Warnf("Failed to clean up temporary workdir %s: %s", c.tempWorkdir, err)
		}
	}
}

func (c *Build) initCliWrappers() error {
	executor := cliWrappers.NewCliExecutor()

	buildahCli, err := cliWrappers.NewBuildahCli(executor)
	if err != nil {
		return err
	}
	c.CliWrappers.BuildahCli = buildahCli

	c.CliWrappers.BuildahUnshare = cliWrappers.NewWrapperCmd("buildah", "unshare")

	c.CliWrappers.Unshare = cliWrappers.NewWrapperCmd("unshare")

	selfPath, err := os.Executable()
	if err != nil {
		return err
	}
	c.CliWrappers.SelfInUserNamespace = cliWrappers.NewWrapperCmd(selfPath, "internal", "in-user-namespace")

	return nil
}

func (c *Build) ensureTempWorkdirExists() error {
	if c.tempWorkdir == "" {
		tempWorkdir, err := os.MkdirTemp("", "kbc-image-build-")
		if err != nil {
			return fmt.Errorf("creating temporary workdir: %w", err)
		}
		c.tempWorkdir = tempWorkdir
	}

	return nil
}

func (c *Build) copyToTempWorkdir(filePath string) (copyPath string, err error) {
	if err := c.ensureTempWorkdirExists(); err != nil {
		return "", err
	}

	infile, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer infile.Close()

	outfile, err := os.CreateTemp(c.tempWorkdir, filepath.Base(filePath)+"-*")
	if err != nil {
		return "", err
	}
	// Failing to close the outfile could mean that it's not fully written,
	// so handle the Close() errors rather than using defer.

	_, err = io.Copy(outfile, infile)
	if err != nil {
		// we already have an error and want to return that one
		_ = outfile.Close()
		return "", err
	}

	err = outfile.Close()
	return outfile.Name(), err
}

// Run executes the command logic.
func (c *Build) Run() error {
	common.LogParameters(BuildParamsConfig, c.Params)
	if len(c.Params.ExtraArgs) > 0 {
		l.Logger.Infof("[extra args]: %v", c.Params.ExtraArgs)
	}

	defer c.cleanup()

	if err := c.validateParams(); err != nil {
		return err
	}

	if err := c.detectContainerfile(); err != nil {
		return err
	}

	containerfile, err := c.parseContainerfile()
	if err != nil {
		return err
	}

	if err := c.processLabelsAndAnnotations(); err != nil {
		return err
	}

	if err := c.setSecretArgs(); err != nil {
		return err
	}

	if !c.Params.SkipInjections {
		if c.Params.Target != "" {
			l.Logger.Warnf("Injecting buildinfo is not supported with --target. Skipping.")
		} else if err := c.injectBuildinfo(containerfile, c.mergedLabels); err != nil {
			return fmt.Errorf("injecting buildinfo metadata: %w", err)
		}
	}

	if err := c.prePullBaseImages(containerfile); err != nil {
		return err
	}

	if err := c.buildImage(); err != nil {
		return err
	}

	c.Results.ImageUrl = c.Params.OutputRef

	if c.Params.Push {
		digest, err := c.pushImage()
		if err != nil {
			return err
		}
		c.Results.Digest = digest
	}

	if c.Params.ContainerfileJsonOutput != "" {
		if err := c.writeContainerfileJson(containerfile, c.Params.ContainerfileJsonOutput); err != nil {
			return err
		}
	}

	if resultJson, err := c.ResultsWriter.CreateResultJson(c.Results); err == nil {
		fmt.Print(resultJson)
	} else {
		l.Logger.Errorf("failed to create results json: %s", err.Error())
		return err
	}

	return nil
}

func (c *Build) validateParams() error {
	if !common.IsImageNameValid(common.GetImageName(c.Params.OutputRef)) {
		return fmt.Errorf("output-ref '%s' is invalid", c.Params.OutputRef)
	}

	if stat, err := os.Stat(c.Params.Context); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("context directory '%s' does not exist", c.Params.Context)
		}
		return fmt.Errorf("failed to stat context directory: %w", err)
	} else if !stat.IsDir() {
		return fmt.Errorf("context path '%s' is not a directory", c.Params.Context)
	}

	if c.Params.LegacyBuildTimestamp != "" && c.Params.SourceDateEpoch != "" {
		return fmt.Errorf("legacy-build-timestamp and source-date-epoch are mutually exclusive")
	}

	if c.Params.RewriteTimestamp && c.Params.SourceDateEpoch == "" {
		// Not an error, just a warning (buildah also doesn't error for this combination of flags)
		l.Logger.Warn("RewriteTimestamp is enabled but SourceDateEpoch was not provided. Timestamps will not be re-written.")
	}

	return nil
}

func (c *Build) detectContainerfile() error {
	if c.Params.Containerfile != "" {
		// Try the filepath as-is first
		if stat, err := os.Stat(c.Params.Containerfile); err == nil && !stat.IsDir() {
			c.containerfilePath = c.Params.Containerfile
			l.Logger.Infof("Using containerfile: %s", c.containerfilePath)
			return nil
		}

		// Fallback: try relative to context directory
		fallbackPath := filepath.Join(c.Params.Context, c.Params.Containerfile)
		if stat, err := os.Stat(fallbackPath); err == nil && !stat.IsDir() {
			c.containerfilePath = fallbackPath
			l.Logger.Infof("Using containerfile: %s", c.containerfilePath)
			return nil
		}

		return fmt.Errorf("containerfile '%s' not found", c.Params.Containerfile)
	}

	// Auto-detection: look only in context directory (same as buildah)
	candidates := []string{"Containerfile", "Dockerfile"}
	for _, candidate := range candidates {
		candidatePath := filepath.Join(c.Params.Context, candidate)
		if stat, err := os.Stat(candidatePath); err == nil && !stat.IsDir() {
			c.containerfilePath = candidatePath
			l.Logger.Infof("Auto-detected containerfile: %s", c.containerfilePath)
			return nil
		}
	}

	return fmt.Errorf("no Containerfile or Dockerfile found in context directory '%s'", c.Params.Context)
}

func (c *Build) setSecretArgs() error {
	secretDirs, err := parseSecretDirs(c.Params.SecretDirs)
	if err != nil {
		return fmt.Errorf("parsing --secret-dirs: %w", err)
	}
	buildahSecrets, err := c.processSecretDirs(secretDirs)
	if err != nil {
		return fmt.Errorf("processing --secret-dirs: %w", err)
	}
	c.buildahSecrets = buildahSecrets
	return nil
}

type secretDir struct {
	src      string
	name     string
	optional bool
}

func parseSecretDirs(secretDirArgs []string) ([]secretDir, error) {
	var secretDirs []secretDir

	for _, arg := range secretDirArgs {
		secretDir := secretDir{}
		keyValues := strings.Split(arg, ",")

		for _, kv := range keyValues {
			key, value, hasSep := strings.Cut(kv, "=")
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)

			if !hasSep {
				value = key
				key = "src"
			}

			switch key {
			case "src":
				secretDir.src = value
			case "name":
				secretDir.name = value
			case "optional":
				switch value {
				case "true":
					secretDir.optional = true
				case "false":
					secretDir.optional = false
				default:
					return nil, fmt.Errorf("invalid argument: optional=%s (expected true|false)", value)
				}
			default:
				return nil, fmt.Errorf("invalid attribute: %s", key)
			}
		}

		secretDirs = append(secretDirs, secretDir)
	}

	return secretDirs, nil
}

// processSecretDirs processes secret directories and returns buildah --secret arguments.
func (c *Build) processSecretDirs(secretDirs []secretDir) ([]cliWrappers.BuildahSecret, error) {
	var buildahSecrets []cliWrappers.BuildahSecret
	usedIDs := make(map[string]bool)

	for _, secretDir := range secretDirs {
		idPrefix := secretDir.name
		if idPrefix == "" {
			idPrefix = filepath.Base(secretDir.src)
		}

		entries, err := os.ReadDir(secretDir.src)
		if err != nil {
			if os.IsNotExist(err) && secretDir.optional {
				l.Logger.Debugf("secret directory %s doesn't exist but is marked optional, skipping", secretDir.src)
				continue
			}
			return nil, fmt.Errorf("failed to read secret directory %s: %w", secretDir.src, err)
		}

		for _, entry := range entries {
			isFile, err := isRegular(entry, secretDir.src)
			if err != nil {
				return nil, err
			}
			if !isFile {
				continue
			}

			filename := entry.Name()
			fullID := filepath.Join(idPrefix, filename)

			// Check for ID conflicts
			if usedIDs[fullID] {
				return nil, fmt.Errorf("duplicate secret ID '%s': ensure unique basename/filename combinations", fullID)
			}
			usedIDs[fullID] = true

			secretPath := filepath.Join(secretDir.src, filename)
			buildahSecrets = append(
				buildahSecrets, cliWrappers.BuildahSecret{Src: secretPath, Id: fullID},
			)

			l.Logger.Infof("Adding secret %s to the build, available with 'RUN --mount=type=secret,id=%s'", fullID, fullID)
		}
	}

	return buildahSecrets, nil
}

func isRegular(entry os.DirEntry, dir string) (bool, error) {
	t := entry.Type()
	if t.IsRegular() {
		return true, nil
	}
	if t == os.ModeSymlink {
		path := filepath.Join(dir, entry.Name())
		stat, err := os.Stat(path)
		if err != nil {
			return false, fmt.Errorf("failed to stat %s: %w", path, err)
		}
		return stat.Mode().IsRegular(), nil
	}
	return false, nil
}

func (c *Build) parseContainerfile() (*dockerfile.Dockerfile, error) {
	l.Logger.Debugf("Parsing Containerfile: %s", c.containerfilePath)

	containerfile, err := dockerfile.Parse(c.containerfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", c.containerfilePath, err)
	}

	envs := processKeyValueEnvs(c.Params.Envs)

	argExp, err := c.createBuildArgExpander()
	if err != nil {
		return nil, fmt.Errorf("failed to process build args: %w", err)
	}

	containerfile.InjectEnv(envs)
	containerfile.Expand(argExp)
	return containerfile, nil
}

func (c *Build) createBuildArgExpander() (dockerfile.SingleWordExpander, error) {
	// Define built-in ARG variables
	// See https://docs.docker.com/build/building/variables/#multi-platform-build-arguments
	platform := platforms.Normalize(platforms.DefaultSpec())
	args := map[string]string{
		// We current don't explicitly expose the --platform flag, so the TARGET* values always
		// match the BUILD* values. If we add --platform handling, we would want to respect it here.
		"TARGETPLATFORM": platforms.Format(platform),
		"TARGETOS":       platform.OS,
		"TARGETARCH":     platform.Architecture,
		"TARGETVARIANT":  platform.Variant,
		"BUILDPLATFORM":  platforms.Format(platform),
		"BUILDOS":        platform.OS,
		"BUILDARCH":      platform.Architecture,
		"BUILDVARIANT":   platform.Variant,
	}

	// Load from --build-args-file, can override built-in args
	if c.Params.BuildArgsFile != "" {
		fileArgs, err := buildargs.ParseBuildArgFile(c.Params.BuildArgsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read build args file: %w", err)
		}
		maps.Copy(args, fileArgs)
	}

	// CLI --build-args take precedence over everything else
	cliArgs := processKeyValueEnvs(c.Params.BuildArgs)
	maps.Copy(args, cliArgs)

	// Return the kind of "expander" function expected by the dockerfile-json API
	// (takes the name of a build arg, returns the value or error for undefined build args)
	argExp := func(word string) (string, error) {
		if value, ok := args[word]; ok {
			return value, nil
		}
		return "", fmt.Errorf("not defined: $%s", word)
	}
	return argExp, nil
}

// Parse an array of key[=value] args. If '=' is missing, look up the value in
// environment variables. This is how buildah handles --build-arg and --env values.
func processKeyValueEnvs(args []string) map[string]string {
	values := make(map[string]string)
	for _, arg := range args {
		key, value, hasValue := strings.Cut(arg, "=")
		if hasValue {
			values[key] = value
		} else if valueFromEnv, ok := os.LookupEnv(key); ok {
			values[key] = valueFromEnv
		}
	}
	return values
}

// Prepends default labels and annotations to the user-provided values.
// User-provided values override defaults via buildah's "last value wins" behavior.
//
// The default annotations are primarily based on the OCI annotation spec:
// https://specs.opencontainers.org/image-spec/annotations/
//
// Note that we also add them as labels, not just annotations. When distributing images
// in the docker format rather than the OCI format, annotations disappear, so the information
// is at least preserved as labels.
//
// In addition to the OCI annotations (and labels), if AddLegacyLabels is enabled,
// adds labels based on https://github.com/projectatomic/ContainerApplicationGenericLabels.
func (c *Build) processLabelsAndAnnotations() error {
	var defaultLabels []string
	var defaultAnnotations []string

	buildTimeStr, err := c.getBuildTimeRFC3339()
	if err != nil {
		return fmt.Errorf("determining build timestamp: %w", err)
	}
	ociCreated := "org.opencontainers.image.created=" + buildTimeStr
	defaultAnnotations = append(defaultAnnotations, ociCreated)
	defaultLabels = append(defaultLabels, ociCreated)

	if c.Params.ImageSource != "" {
		ociSource := "org.opencontainers.image.source=" + c.Params.ImageSource

		defaultAnnotations = append(defaultAnnotations, ociSource)
		defaultLabels = append(defaultLabels, ociSource)
	}

	if c.Params.ImageRevision != "" {
		ociRevision := "org.opencontainers.image.revision=" + c.Params.ImageRevision

		defaultAnnotations = append(defaultAnnotations, ociRevision)
		defaultLabels = append(defaultLabels, ociRevision)
	}

	if c.Params.QuayImageExpiresAfter != "" {
		defaultLabels = append(defaultLabels, "quay.expires-after="+c.Params.QuayImageExpiresAfter)
	}

	if c.Params.AddLegacyLabels {
		defaultLabels = append(defaultLabels, "build-date="+buildTimeStr)

		arch := goArchToArchitectureLabel(runtime.GOARCH)
		defaultLabels = append(defaultLabels, "architecture="+arch)

		if c.Params.ImageSource != "" {
			defaultLabels = append(defaultLabels, "vcs-url="+c.Params.ImageSource)
		}

		if c.Params.ImageRevision != "" {
			defaultLabels = append(defaultLabels, "vcs-ref="+c.Params.ImageRevision)
		}

		if c.Params.ImageSource != "" || c.Params.ImageRevision != "" {
			// We don't know if it's git, but this label serves no purpose other than
			// to appease the default Red Hat label policy, so it doesn't really matter.
			// https://github.com/release-engineering/rhtap-ec-policy/blob/25b163398303105a539998f1a276f176bf3384b2/data/rule_data.yml#L103
			defaultLabels = append(defaultLabels, "vcs-type=git")
		}
	}

	mergedLabels := append(defaultLabels, c.Params.Labels...)

	mergedAnnotations := defaultAnnotations
	if c.Params.AnnotationsFile != "" {
		fileAnnotations, err := parseAnnotationsFile(c.Params.AnnotationsFile)
		if err != nil {
			return fmt.Errorf("parsing annotations file: %w", err)
		}
		// --annotations-file takes precedence over defaults
		mergedAnnotations = append(mergedAnnotations, fileAnnotations...)
	}
	// --annotations take precedence over --annotations-file
	mergedAnnotations = append(mergedAnnotations, c.Params.Annotations...)

	c.mergedLabels = mergedLabels
	c.mergedAnnotations = mergedAnnotations
	return nil
}

func (c *Build) getBuildTimeRFC3339() (string, error) {
	var buildTime time.Time
	if c.Params.SourceDateEpoch != "" {
		timestamp, err := strconv.ParseInt(c.Params.SourceDateEpoch, 10, 64)
		if err != nil {
			return "", fmt.Errorf("parsing source-date-epoch: %w", err)
		}
		buildTime = time.Unix(timestamp, 0).UTC()
	} else if c.Params.LegacyBuildTimestamp != "" {
		timestamp, err := strconv.ParseInt(c.Params.LegacyBuildTimestamp, 10, 64)
		if err != nil {
			return "", fmt.Errorf("parsing legacy-build-timestamp: %w", err)
		}
		buildTime = time.Unix(timestamp, 0).UTC()
	} else {
		buildTime = time.Now().UTC()
	}
	return buildTime.Format(time.RFC3339), nil
}

func parseAnnotationsFile(filePath string) ([]string, error) {
	annotations, err := buildargs.ParseBuildArgFile(filePath)
	if err != nil {
		return nil, err
	}
	annotationStrings := []string{}
	for k, v := range annotations {
		annotationStrings = append(annotationStrings, k+"="+v)
	}
	slices.Sort(annotationStrings)
	return annotationStrings, nil
}

// Convert Go's GOARCH value to the value used for the 'architecture' label.
//
// Historically, the 'architecture' label used the RPM architecture names rather
// than the GOARCH names. Keep that the same.
func goArchToArchitectureLabel(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return goarch
	}
}

// Injects metadata into the image at /usr/share/buildinfo.
//
// Injected files:
// - labels.json: contains the labels of the resulting image, needed for the Clair scanning tool
func (c *Build) injectBuildinfo(df *dockerfile.Dockerfile, userLabels []string) error {
	// Create buildinfo directory in the temporary workdir and add it as a --build-context
	buildinfoDir, err := c.createBuildinfoDir()
	if err != nil {
		return fmt.Errorf("creating buildinfo dir: %w", err)
	}
	c.buildinfoBuildContext = &cliWrappers.BuildahBuildContext{Name: ".konflux-buildinfo", Location: buildinfoDir}

	// Create labels.json in buildinfo dir
	labels, err := c.determineFinalLabels(df, userLabels)
	if err != nil {
		return fmt.Errorf("determining labels for labels.json: %w", err)
	}
	if err := writeBuildinfoJSON(buildinfoDir, labels, "labels.json"); err != nil {
		return fmt.Errorf("writing labels.json to buildinfo dir: %w", err)
	}

	// Copy containerfile and modify the copy
	containerfileCopy, err := c.copyToTempWorkdir(c.containerfilePath)
	if err != nil {
		return fmt.Errorf("creating containerfile copy: %w", err)
	}
	l.Logger.Debugf("Copied containerfile to %s", containerfileCopy)
	c.containerfileCopyPath = containerfileCopy

	appendLines := []string{"COPY --from=.konflux-buildinfo . /usr/share/buildinfo/"}
	if c.Params.IncludeLegacyBuildinfoPath {
		appendLines = append(appendLines, "COPY --from=.konflux-buildinfo . /root/buildinfo/")
	}
	for _, line := range appendLines {
		l.Logger.Debugf("Appending to containerfile: %s", line)
	}
	// prepend a newline in case the input containerfile doesn't end with one
	appendContent := "\n" + strings.Join(appendLines, "\n") + "\n"

	if err := appendToFile(containerfileCopy, appendContent); err != nil {
		return fmt.Errorf("writing to containerfile copy: %w", err)
	}

	return nil
}

func (c *Build) createBuildinfoDir() (string, error) {
	if err := c.ensureTempWorkdirExists(); err != nil {
		return "", err
	}

	buildinfoDir := filepath.Join(c.tempWorkdir, "buildinfo")
	if err := os.Mkdir(buildinfoDir, 0755); err != nil {
		return "", err
	}

	return buildinfoDir, nil
}

func writeBuildinfoJSON(buildinfoDir string, value any, filename string) error {
	// Note: json.MarshalIndent sorts map keys, so the output is deterministic.
	// This is crucial for reproducibility.
	jsonContent, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling to JSON: %w", err)
	}

	filePath := filepath.Join(buildinfoDir, filename)
	// For backwards compatibility, buildinfo files should be readable to all
	if err := os.WriteFile(filePath, append(jsonContent, '\n'), 0644); err != nil {
		return err
	}

	return nil
}

func appendToFile(filePath, content string) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		// we already have an error and want to return that one
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (c *Build) determineFinalLabels(df *dockerfile.Dockerfile, userLabels []string) (map[string]string, error) {
	labels := make(map[string]string)

	var baseImage string
	var containerfileLabels map[string]string
	if c.Params.InheritLabels {
		baseImage, containerfileLabels = processUntilBaseStage(df)
	} else if df != nil && len(df.Stages) > 0 {
		// Label inheritance disabled, don't get base image labels
		// or labels from any stage except the target stage
		containerfileLabels = getStageLabels(df.Stages[len(df.Stages)-1])
	}

	// Base image labels
	if baseImage != "" {
		if isPullableImage(baseImage) {
			baseImageLabels, err := c.getImageLabels(baseImage)
			if err != nil {
				return nil, fmt.Errorf("getting base image labels: %w", err)
			}
			maps.Copy(labels, baseImageLabels)
		} else {
			l.Logger.Warnf("Injecting labels.json: ignoring base image labels due to unsupported transport: %s", baseImage)
		}
	} // else base image is FROM scratch => no labels

	// Containerfile labels
	maps.Copy(labels, containerfileLabels)

	// User-provided labels
	for _, label := range userLabels {
		key, value, _ := strings.Cut(label, "=")
		labels[key] = value
	}

	// Automatic buildah version label (highest precedence)
	// Only injected if --source-date-epoch (or --timestamp, which we do not expose) are not used.
	// See https://www.mankier.com/1/buildah-build#--identity-label.
	if c.Params.SourceDateEpoch == "" {
		versionInfo, err := c.CliWrappers.BuildahCli.Version()
		if err != nil {
			return nil, fmt.Errorf("getting buildah version: %w", err)
		}
		labels["io.buildah.version"] = versionInfo.Version
	}

	return labels, nil
}

// Resolves the base stage of the final stage by following FROM references
// through intermediate stages. Collects LABELs from each stage in the chain.
// Returns the base image for the base stage and the collected labels.
func processUntilBaseStage(df *dockerfile.Dockerfile) (string, map[string]string) {
	if df == nil || len(df.Stages) == 0 {
		return "", nil
	}

	stage := df.Stages[len(df.Stages)-1]
	stageChain := []*dockerfile.Stage{}

	for stage != nil {
		stageChain = append(stageChain, stage)
		if stage.From.Stage != nil {
			stage = df.Stages[stage.From.Stage.Index]
		} else {
			stage = nil
		}
	}

	// We need to process labels starting from the base stage through to the final stage
	slices.Reverse(stageChain)

	var baseImage string
	baseStage := stageChain[0]
	if !baseStage.From.Scratch && baseStage.From.Image != nil {
		baseImage = *baseStage.From.Image
	}

	labels := make(map[string]string)
	for _, stage := range stageChain {
		maps.Copy(labels, getStageLabels(stage))
	}

	return baseImage, labels
}

func getStageLabels(stage *dockerfile.Stage) map[string]string {
	if stage == nil {
		return nil
	}

	labels := make(map[string]string)
	for _, cmd := range stage.Commands {
		if labelCmd, ok := cmd.Command.(*instructions.LabelCommand); ok {
			for _, kv := range labelCmd.Labels {
				labels[kv.Key] = kv.Value
			}
		}
	}
	return labels
}

// Determine if we should try to pull the image.
//
// A base image in the Containerfile can use any of the container transports [1].
// We only care about container images from a registry, i.e. those that do not specify
// a transport or use the docker:// transport. The containers-storage: transport is also
// trivially supportable (the image is already present in the local storage).
//
// Most of the others are unsupportable (they can reference a local path, which can get
// created dynamically during the build) or just aren't a real use case.
//
// [1]: https://man.archlinux.org/man/containers-transports.5.en
func isPullableImage(imageRef string) bool {
	if imageRef == "" {
		return false
	}

	transport, _ := splitTransport(imageRef)
	switch transport {
	case "", "docker://", "containers-storage:":
		return true
	default:
		return false
	}
}

// Split an image ref that includes a transport into (transport, image ref).
//
// Example:
// - "docker://registry.io/image:tag" -> ("docker://", "registry.io/image:tag")
// - "registry.io/image:tag" -> ("", "registry.io/image:tag")
func splitTransport(imageRef string) (string, string) {
	transports := []string{
		"docker://",
		"containers-storage:",
		"dir:",
		"docker-archive:",
		"docker-daemon:",
		"oci:",
		"oci-archive:",
		"sif:",
	}
	for _, transport := range transports {
		if imageRef, ok := strings.CutPrefix(imageRef, transport); ok {
			return transport, imageRef
		}
	}
	return "", imageRef
}

func (c *Build) getImageLabels(imageRef string) (map[string]string, error) {
	l.Logger.Debugf("Pulling image %s to read labels...", imageRef)
	err := c.CliWrappers.BuildahCli.Pull(&cliWrappers.BuildahPullArgs{Image: imageRef})
	if err != nil {
		return nil, fmt.Errorf("pulling image %s: %w", imageRef, err)
	}

	// buildah inspect doesn't support the <transport>: prefix, strip it
	_, inspectableRef := splitTransport(imageRef)
	info, err := c.CliWrappers.BuildahCli.InspectImage(inspectableRef)
	if err != nil {
		return nil, fmt.Errorf("inspecting image %s: %w", inspectableRef, err)
	}

	return info.OCIv1.Config.Labels, nil
}

// Pull all images referenced by the target stage and its dependencies.
// Primarily needed for hermetic builds where network access is disabled,
// but also useful to ensure image pulls use our retry logic instead of relying on buildah.
func (c *Build) prePullBaseImages(df *dockerfile.Dockerfile) error {
	if df == nil || len(df.Stages) == 0 {
		return nil
	}

	var targetStage int
	if c.Params.Target != "" {
		if stages, ok := findMatchingStages(df.Stages, c.Params.Target); ok {
			// buildah's --target matches the first stage with a matching name
			targetStage = stages[0]
		} else {
			return fmt.Errorf("target stage %q not found", c.Params.Target)
		}
	} else {
		targetStage = len(df.Stages) - 1
	}

	for _, image := range c.collectBaseImages(df, targetStage) {
		if !isPullableImage(image) {
			l.Logger.Warnf("Skipping pre-pull of %s: unsupported transport", image)
			continue
		}
		l.Logger.Debugf("Pre-pulling base image: %s", image)
		if err := c.CliWrappers.BuildahCli.Pull(&cliWrappers.BuildahPullArgs{Image: image}); err != nil {
			return fmt.Errorf("pre-pulling image %s: %w", image, err)
		}
	}

	return nil
}

// Collect all images needed to build the target stage.
//
// For all the "stages of interest":
//   - For all the instructions that support a 'from' reference:
//     (those being 'FROM <ref>', 'COPY --from=<ref>', 'RUN --mount=from=<ref>'):
//     -- If <ref> is an image, collect this image
//     -- If <ref> is an earlier stage in the containerfile, also collect images for that stage
//
// With skip-unused-stages=true (the default), there is one stage of interest - the target stage.
// With skip-unused-stages=false, it's all the stages up to and including the target stage.
func (c *Build) collectBaseImages(df *dockerfile.Dockerfile, targetStage int) []string {
	baseImageSet := make(map[string]struct{})

	stagesToProcess := []int{}
	stagesSeen := make(map[int]struct{})

	enqueue := func(stageIndexes ...int) {
		for _, stageIdx := range stageIndexes {
			if _, seen := stagesSeen[stageIdx]; !seen {
				stagesSeen[stageIdx] = struct{}{}
				stagesToProcess = append(stagesToProcess, stageIdx)
			}
		}
	}
	if c.Params.SkipUnusedStages {
		enqueue(targetStage)
	} else {
		// skip-unused-stages=false => buildah builds all stages up to and including the target stage
		// The graph search algorithm is unnecessary in this case, but let's keep things simple
		for i := range targetStage + 1 {
			enqueue(i)
		}
	}

	for len(stagesToProcess) > 0 {
		stageIdx := stagesToProcess[0]
		stage := df.Stages[stageIdx]
		stagesToProcess = stagesToProcess[1:]

		if stage.From.Image != nil {
			baseImageSet[*stage.From.Image] = struct{}{}
		} else if stage.From.Stage != nil {
			enqueue(stage.From.Stage.Index)
		}

		// 'From' refs can only reference earlier stages. If they reference a later stage
		// (or the same stage in which they appear), buildah treats them as external images.
		// Note: for FROM instructions, dockerfile-json handles this on its own.
		precedingStages := df.Stages[:stageIdx]

		for _, ref := range getFromRefsInCommands(stage) {
			if stages, ok := findMatchingStages(precedingStages, ref); ok {
				// ref matches one or more stages
				// buildah builds all matching stages, we have to pre-pull all the images
				enqueue(stages...)
			} else {
				// ref is an image
				// (the third option is that ref is a --build-context,
				//  but we don't expose any way to add build contexts)
				baseImageSet[ref] = struct{}{}
			}
		}
	}

	return slices.Sorted(maps.Keys(baseImageSet))
}

// Given a list of containerfile stages and a string ref, determine if the ref matches any stage(s).
// If yes, return ({indexes of matching stages}, true).
//
// First, look for matching named stages ('FROM ... AS name').
// If no such stage exists, the ref may be an integer index of a stage - parse it and verify bounds.
// If ref doesn't match any named stage and isn't a valid integer index, return (nil, false).
func findMatchingStages(stages []*dockerfile.Stage, ref string) ([]int, bool) {
	var matchingStages []int
	for i, stage := range stages {
		if stage.Name != nil && *stage.Name == ref {
			matchingStages = append(matchingStages, i)
		}
	}
	if len(matchingStages) > 0 {
		return matchingStages, true
	}

	if i, err := strconv.Atoi(ref); err == nil && 0 <= i && i < len(stages) {
		return []int{i}, true
	}

	return nil, false
}

// Returns all 'from' references from a stage's commands (COPY --from and RUN --mount=from).
func getFromRefsInCommands(stage *dockerfile.Stage) []string {
	var refs []string
	for _, cmd := range stage.Commands {
		if copyCmd, ok := cmd.Command.(*instructions.CopyCommand); ok && copyCmd.From != "" {
			refs = append(refs, copyCmd.From)
		}
		for _, mount := range cmd.Mounts {
			if mount.From != "" {
				refs = append(refs, mount.From)
			}
		}
	}
	return refs
}

func (c *Build) buildImage() error {
	l.Logger.Info("Building container image...")

	originalCwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(c.Params.Context); err != nil {
		return fmt.Errorf("couldn't cd to context directory: %w", err)
	}
	defer os.Chdir(originalCwd)

	containerfilePath := c.containerfilePath
	if c.containerfileCopyPath != "" {
		containerfilePath = c.containerfileCopyPath
	}

	buildArgs := &cliWrappers.BuildahBuildArgs{
		Containerfile:    containerfilePath,
		ContextDir:       c.Params.Context,
		OutputRef:        c.Params.OutputRef,
		Secrets:          c.buildahSecrets,
		BuildArgs:        c.Params.BuildArgs,
		BuildArgsFile:    c.Params.BuildArgsFile,
		Envs:             c.Params.Envs,
		Labels:           c.mergedLabels,
		Annotations:      c.mergedAnnotations,
		SourceDateEpoch:  c.Params.SourceDateEpoch,
		RewriteTimestamp: c.Params.RewriteTimestamp,
		ExtraArgs:        c.Params.ExtraArgs,
		InheritLabels:    &c.Params.InheritLabels,
		Target:           c.Params.Target,
		SkipUnusedStages: &c.Params.SkipUnusedStages,
		Wrapper:          c.chooseBuildahWrappers(),
	}
	if c.Params.WorkdirMount != "" {
		buildArgs.Volumes = []cliWrappers.BuildahVolume{
			{HostDir: c.Params.Context, ContainerDir: c.Params.WorkdirMount, Options: "z"},
		}
	}
	if c.buildinfoBuildContext != nil {
		buildArgs.BuildContexts = []cliWrappers.BuildahBuildContext{*c.buildinfoBuildContext}
	}

	if err := buildArgs.MakePathsAbsolute(originalCwd); err != nil {
		return err
	}

	if err := c.CliWrappers.BuildahCli.Build(buildArgs); err != nil {
		return err
	}

	l.Logger.Info("Build completed successfully")
	return nil
}

// Choose how to wrap the 'buildah build' command.
//
// Rather than executing buildah directly, we wrap it in commands that manipulate user namespaces.
// These are the main reasons:
//
//  1. Hermetic builds
//
//     We want the build to be executed entirely without network access, including ADD instructions.
//     Buildah has a --network=none flag, but it only affects RUN instructions, not ADD.
//     And it doesn't work with BUILDAH_ISOLATION=chroot, which is the typical setup in Konflux.
//     Instead, we create a network namespace manually using 'unshare --net'.
//
//  2. Running as root with BUILDAH_ISOLATION=chroot
//
//     When running as root, chroot isolation skips creating a user namespace,
//     so the root inside the container build is the actual root from the host.
//     Creating a user namespace manually slightly improves security.
func (c *Build) chooseBuildahWrappers() *cliWrappers.WrapperCmd {
	var wrapper cliWrappers.WrapperCmd

	if os.Getuid() == 0 {
		// 'buildah unshare' doesn't work as root, use regular unshare.
		// --map-root-user: Need to stay root, by default unshare would map to a non-root UID.
		// --map-auto: Map subordinate UIDs and GIDs based on /etc/subuid and /etc/subgid.
		//             By default, the namespace would only have 1 UID available.
		//             Buildah needs more UIDs available to manipulate container filesystems.
		// --mount: Create a new mount namespace.
		//          Without this, buildah would fail to mount /var/lib/containers/storage/overlay.
		wrapper = c.CliWrappers.Unshare.WithArgs("--map-root-user", "--map-auto", "--mount")
	} else {
		// Buildah doesn't work under regular unshare as non-root, use 'buildah unshare'.
		// It does mostly the same things as the raw unshare that we use for root,
		// but also some buildah-specific magic that makes it work rootless. E.g. this:
		// https://github.com/containers/storage/blob/83cf57466529353aced8f1803f2302698e0b5cb7/pkg/unshare/unshare_linux.go#L462-L465

		// Unlike the root case, 'buildah unshare' doesn't provide any meaningful security benefits;
		// buildah always creates a userns for non-root users.
		// But becoming root in this outer namespace is necessary for 'unshare --net' to work.
		wrapper = c.CliWrappers.BuildahUnshare
	}

	if c.Params.Hermetic {
		wrapper = cliWrappers.JoinWrappers(
			wrapper,
			// Create an isolated network namespace
			c.CliWrappers.Unshare.WithArgs("--net"),
			// But bring up the loopback interface inside this namespace.
			// Mainly needed for Bazel builds, Bazel runs a server on localhost.
			c.CliWrappers.SelfInUserNamespace.WithArgs("--loopback-up"))
	}

	return &wrapper
}

func (c *Build) pushImage() (string, error) {
	l.Logger.Infof("Pushing image to registry: %s", c.Params.OutputRef)

	pushArgs := &cliWrappers.BuildahPushArgs{
		Image: c.Params.OutputRef,
	}

	digest, err := c.CliWrappers.BuildahCli.Push(pushArgs)
	if err != nil {
		return "", err
	}

	l.Logger.Info("Push completed successfully")
	l.Logger.Infof("Image digest: %s", digest)

	return digest, nil
}

func (c *Build) writeContainerfileJson(containerfile *dockerfile.Dockerfile, outputPath string) error {
	l.Logger.Infof("Writing parsed Containerfile to: %s", outputPath)

	jsonData, err := json.MarshalIndent(containerfile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Containerfile to JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write Containerfile JSON: %w", err)
	}

	l.Logger.Info("Containerfile JSON written successfully")
	return nil
}
