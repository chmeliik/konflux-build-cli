package integration_tests_framework

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const (
	ResultsPathInContainer = "/tmp/"
	kbcPathInContainer     = "/usr/bin/" + KonfluxBuildCli
)

type ContainerStatus int

const (
	ContainerStatus_NotStarted ContainerStatus = iota
	ContainerStatus_Running
	ContainerStatus_Deleted
)

type TestRunnerContainer struct {
	ReplaceEntrypoint bool

	name       string
	image      string
	workdir    string
	user       string
	privileged bool
	env        map[string]string
	volumes    map[string]hostMount // container dir => (host dir, options)
	ports      map[string]string
	networks   []string
	results    map[string]string

	executor cliWrappers.CliExecutorInterface

	containerStatus ContainerStatus
}

type hostMount struct {
	path    string
	options string
}

// ContainerOption is a functional-style option for configuring TestRunnerContainer.
type ContainerOption func(*TestRunnerContainer)

func NewTestRunnerContainer(name, image string, opts ...ContainerOption) *TestRunnerContainer {
	container := &TestRunnerContainer{
		ReplaceEntrypoint: true,

		executor: cliWrappers.NewCliExecutor(),
		name:     name,
		image:    image,
		env:      make(map[string]string),
		volumes:  make(map[string]hostMount),
		ports:    make(map[string]string),
		results:  make(map[string]string),
	}

	for _, opt := range opts {
		opt(container)
	}

	return container
}

// NewBuildCliRunnerContainer creates NewTestRunnerContainer
// with additional settings for running the Build CLI.
func NewBuildCliRunnerContainer(name, image string, opts ...ContainerOption) *TestRunnerContainer {
	container := NewTestRunnerContainer(name, image, opts...)

	container.AddVolumeWithOptions(GetCliBinPath(), kbcPathInContainer, "z")
	container.AddNetwork("host")
	if Debug {
		container.AddPort("2345", "2345")
	}
	container.AddEnv("KBC_LOG_LEVEL", "debug")
	// On macOS, containers run in a Linux VM; overlay storage driver
	// doesn't work reliably with host volume mounts through the VM
	if runtime.GOOS == "darwin" {
		container.AddEnv("STORAGE_DRIVER", "vfs")
	}

	return container
}

func (c *TestRunnerContainer) ensureContainerNotStarted() {
	if c.containerStatus != ContainerStatus_NotStarted {
		panic("the operation can be done only before the container is started")
	}
}

func (c *TestRunnerContainer) ensureContainerRunning() {
	if c.containerStatus != ContainerStatus_Running {
		panic("the operation can be done only on running container")
	}
}

func (c *TestRunnerContainer) SetWorkdir(workdir string) {
	c.ensureContainerNotStarted()
	c.workdir = workdir
}

func WithWorkdir(workdir string) ContainerOption {
	return func(c *TestRunnerContainer) {
		c.SetWorkdir(workdir)
	}
}

func (c *TestRunnerContainer) SetUser(user string) {
	c.ensureContainerNotStarted()
	c.user = user
}

func WithUser(user string) ContainerOption {
	return func(c *TestRunnerContainer) {
		c.SetUser(user)
	}
}

func (c *TestRunnerContainer) AddEnv(key, value string) {
	c.ensureContainerNotStarted()
	c.env[key] = value
}

func WithEnv(key, value string) ContainerOption {
	return func(c *TestRunnerContainer) {
		c.AddEnv(key, value)
	}
}

func (c *TestRunnerContainer) AddVolume(hostPath, containerPath string) {
	c.ensureContainerNotStarted()
	c.volumes[containerPath] = hostMount{path: hostPath}
}

func WithVolume(hostPath, containerPath string) ContainerOption {
	return func(c *TestRunnerContainer) {
		c.AddVolume(hostPath, containerPath)
	}
}

func (c *TestRunnerContainer) AddVolumeWithOptions(hostPath, containerPath, mountOptions string) {
	c.ensureContainerNotStarted()
	c.volumes[containerPath] = hostMount{path: hostPath, options: mountOptions}
}

func WithVolumeWithOptions(hostPath, containerPath, mountOptions string) ContainerOption {
	return func(c *TestRunnerContainer) {
		c.AddVolumeWithOptions(hostPath, containerPath, mountOptions)
	}
}

func (c *TestRunnerContainer) AddPort(hostPort, containerPort string) {
	c.ensureContainerNotStarted()
	c.ports[hostPort] = containerPort
}

func WithPort(hostPort, containerPort string) ContainerOption {
	return func(c *TestRunnerContainer) {
		c.AddPort(hostPort, containerPort)
	}
}

func (c *TestRunnerContainer) AddNetwork(networkName string) {
	c.ensureContainerNotStarted()
	c.networks = append(c.networks, networkName)
}

func WithNetwork(networkName string) ContainerOption {
	return func(c *TestRunnerContainer) {
		c.AddNetwork(networkName)
	}
}

// ContainerExists checks for container with the same name.
func (c *TestRunnerContainer) ContainerExists(isRunning bool) (bool, error) {
	args := []string{"ps", "-q"}
	if !isRunning {
		args = append(args, "-a")
	}
	args = append(args, "-f", "name="+c.name)

	stdout, stderr, _, err := c.executor.Execute(cliWrappers.Command(containerTool, args...))
	if err != nil {
		l.Logger.Infof("[stdout]:\n%s\n", stdout)
		l.Logger.Infof("[stderr]:\n%s\n", stderr)
		return false, err
	}
	return len(stdout) > 0, nil
}

func (c *TestRunnerContainer) ensureDoesNotExist() error {
	existRunning, err := c.ContainerExists(true)
	if err != nil {
		return err
	}
	if existRunning {
		return c.Delete()
	}

	existStopped, err := c.ContainerExists(false)
	if err != nil {
		return err
	}
	if existStopped {
		return c.Delete()
	}
	return nil
}

func (c *TestRunnerContainer) Start() error {
	if err := c.ensureDoesNotExist(); err != nil {
		return err
	}

	args := []string{"run", "--detach", "--name", c.name}
	for name, value := range c.env {
		args = append(args, "-e", name+"="+value)
	}
	for containerPath, mount := range c.volumes {
		volumeArg := mount.path + ":" + containerPath
		if mount.options != "" {
			volumeArg += ":" + mount.options
		}
		args = append(args, "-v", volumeArg)
	}
	for hostPort, containerPort := range c.ports {
		args = append(args, "-p", hostPort+":"+containerPort)
	}
	for _, network := range c.networks {
		args = append(args, "--network", network)
	}
	if c.workdir != "" {
		args = append(args, "--workdir", c.workdir)
	}
	if c.user != "" {
		args = append(args, "--user", c.user)
	}
	if c.privileged {
		args = append(args, "--privileged")
	}

	if c.ReplaceEntrypoint {
		args = append(args, "--entrypoint", "sleep", c.image, "infinity")
	} else {
		args = append(args, c.image)
	}

	stdout, stderr, _, err := c.executor.Execute(cliWrappers.Command(containerTool, args...))
	if err != nil {
		l.Logger.Infof("[stdout]:\n%s\n", stdout)
		l.Logger.Infof("[stderr]:\n%s\n", stderr)
	}
	c.containerStatus = ContainerStatus_Running
	return err
}

// Start the container while injecting the certificates and credentials required to access
// the image registry.
//
// Note that the method may fail to start the container but may also fail *after*
// starting the container. Use the DeleteIfExists() method for cleanup to handle either case.
func (c *TestRunnerContainer) StartWithRegistryIntegration(imageRegistry ImageRegistry) error {
	if imageRegistry.IsLocal() {
		c.AddVolumeWithOptions(imageRegistry.GetCaCertPath(), "/etc/pki/tls/certs/ca-custom-bundle.crt", "z")
	}
	err := c.Start()
	if err != nil {
		return err
	}

	login, password := imageRegistry.GetCredentials()
	return c.InjectDockerAuth(imageRegistry.GetRegistryDomain(), login, password)
}

func (c *TestRunnerContainer) Delete() error {
	args := []string{"rm", "-f", c.name}
	if containerTool != "docker" {
		// By default, podman sends SIGTERM first and, if the container doesn't stop within
		// 10 seconds, sends SIGKILL. Docker always sends SIGKILL right away and doesn't have
		// a --time flag. Do the same thing for podman, for consistency and to speed up tests.
		args = append(args, "--time=0")
	}
	stdout, stderr, _, err := c.executor.Execute(cliWrappers.Command(containerTool, args...))
	if err == nil {
		c.containerStatus = ContainerStatus_Deleted
	} else {
		l.Logger.Infof("[stdout]:\n%s\n", stdout)
		l.Logger.Infof("[stderr]:\n%s\n", stderr)
	}
	return err
}

func (c *TestRunnerContainer) DeleteIfExists() error {
	if c.containerStatus != ContainerStatus_Running {
		return nil
	}
	return c.Delete()
}

func (c *TestRunnerContainer) CopyFileIntoContainer(hostPath, containerPath string) error {
	c.ensureContainerRunning()
	stdout, stderr, _, err := c.executor.Execute(cliWrappers.Command(containerTool, "cp", hostPath, c.name+":"+containerPath))
	if err != nil {
		l.Logger.Infof("[stdout]:\n%s\n", stdout)
		l.Logger.Infof("[stderr]:\n%s\n", stderr)
	}
	return err
}

// GetFileContent reads file inside the container.
func (c *TestRunnerContainer) GetFileContent(path string) (string, error) {
	c.ensureContainerRunning()
	stdout, stderr, _, err := c.executor.Execute(cliWrappers.Command(containerTool, "exec", c.name, "cat", path))
	if err != nil {
		l.Logger.Infof("[stdout]:\n%s\n", stdout)
		l.Logger.Infof("[stderr]:\n%s\n", stderr)
		if strings.Contains(stderr, "No such file or directory") {
			return "", fmt.Errorf("no such file or directory: '%s'", path)
		}
		return "", err
	}
	return stdout, nil
}

func (c *TestRunnerContainer) ExecuteBuildCli(args ...string) error {
	if Debug {
		_, _, err := c.debugBuildCli(args...)
		return err
	}
	return c.ExecuteCommand(kbcPathInContainer, args...)
}

func (c *TestRunnerContainer) ExecuteBuildCliWithOutput(args ...string) (string, string, error) {
	if Debug {
		return c.debugBuildCli(args...)
	}
	return c.ExecuteCommandWithOutput(kbcPathInContainer, args...)
}

// ExecuteCommandWithOutput executes a command in the container and returns
// stdout, stderr, and error.
func (c *TestRunnerContainer) ExecuteCommandWithOutput(command string, args ...string) (string, string, error) {
	c.ensureContainerRunning()
	execArgs := []string{"exec", c.name}
	execArgs = append(execArgs, command)
	execArgs = append(execArgs, args...)

	stdout, stderr, _, err := c.executor.Execute(cliWrappers.Cmd{Name: containerTool, Args: execArgs, LogOutput: true})
	if err != nil {
		l.Logger.Infof("[stdout]:\n%s\n", stdout)
		l.Logger.Infof("[stderr]:\n%s\n", stderr)
	}
	return stdout, stderr, err
}

func (c *TestRunnerContainer) ExecuteCommand(command string, args ...string) error {
	_, _, err := c.ExecuteCommandWithOutput(command, args...)
	return err
}

func (c *TestRunnerContainer) debugBuildCli(cliArgs ...string) (string, string, error) {
	c.ensureContainerRunning()

	dlvPath, err := getDlvPath()
	if err != nil {
		return "", "", err
	}
	err = c.CopyFileIntoContainer(dlvPath, "/usr/bin/")
	if err != nil {
		return "", "", err
	}

	execArgs := []string{"exec", c.name}
	execArgs = append(execArgs, "dlv", "--listen=0.0.0.0:2345", "--headless=true", "--log=true", "--api-version=2", "exec", kbcPathInContainer)
	if len(cliArgs) > 0 {
		execArgs = append(execArgs, "--")
		execArgs = append(execArgs, cliArgs...)
	}

	stdout, stderr, _, err := c.executor.Execute(cliWrappers.Cmd{Name: containerTool, Args: execArgs, LogOutput: true})
	if err != nil {
		l.Logger.Infof("[stdout]:\n%s\n", stdout)
		l.Logger.Infof("[stderr]:\n%s\n", stderr)
	}
	return stdout, stderr, err
}

func getDlvPath() (string, error) {
	goPath, isSet := os.LookupEnv("GOPATH")
	if !isSet {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		goPath = path.Join(homeDir, "go")
	}
	dlvPath := path.Join(goPath, "bin", "dlv")
	if !FileExists(dlvPath) {
		return "", fmt.Errorf("dlv is not found")
	}
	return dlvPath, nil
}

// GetTaskResultValue returns result file content from container.
func (c *TestRunnerContainer) GetTaskResultValue(resultFilePath string) (string, error) {
	resultValue, err := c.GetFileContent(resultFilePath)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
			return "", fmt.Errorf("result file '%s' is not created", resultFilePath)
		}
		return "", err
	}
	return resultValue, nil
}

func (c *TestRunnerContainer) GetHomeDir() (string, error) {
	execCmd := []string{"exec", c.name, "bash", "-c", "echo -n $HOME"}
	homeDir, _, _, err := c.executor.Execute(cliWrappers.Command(containerTool, execCmd...))
	if err != nil {
		return "", err
	}
	return homeDir, nil
}

func (c *TestRunnerContainer) InjectDockerAuth(registry, login, password string) error {
	c.ensureContainerRunning()

	authContent, err := GenerateDockerAuthContent(registry, login, password)
	if err != nil {
		return err
	}

	filePath, err := SaveToTempFile(authContent)
	if err != nil {
		return err
	}
	defer func() { os.Remove(filePath) }()

	homeDir, err := c.GetHomeDir()
	if err != nil {
		return err
	}

	dockerDir := filepath.Join(homeDir, ".docker")
	if err := c.ExecuteCommand("mkdir", "-p", dockerDir); err != nil {
		return err
	}
	if err := c.CopyFileIntoContainer(filePath, path.Join(dockerDir, "config.json")); err != nil {
		return err
	}

	return nil
}
