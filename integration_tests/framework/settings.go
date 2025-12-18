package integration_tests_framework

// Edit the following variables according to your test needs.

// If true the CLI in test container for all integration tests will be run in debug mode.
// Note, that the CLI will wait a debugger to connect before starting execution.
var Debug = false

// If true, integration tests that require image registry to run
// will set up a local Zot registry in a separate container.
var LocalRegistry = true
