# Building and testing the CLI

## How to build

```bash
go build -o konflux-build-cli main.go
```
or statically:
```bash
CGO_ENABLED=0 go build -o konflux-build-cli main.go
```
or in debug mode:
```bash
go build -gcflags "all=-N -l" -o konflux-build-cli main.go
```

## How to run / debug a command on host

Build the CLI and setup the command environment.

Parameters can be passed via CLI arguments or environment variables, CLI arguments take precedence.

```bash
./konflux-build-cli my-command --image-url quay.io/namespace/image:tag --digest sha256:abcde1234 --tags tag1 tag2 --result-sha=/tmp/my-command-result-sha
```

Alternatively, it's possible to provide data via environment variables:

```bash
# my-command-env.sh
export KBC_MYCOMMAND_IMAGE_URL=quay.io/namespace/image:tag
export KBC_MYCOMMAND_DIGEST=sha256:abcde1234
export KBC_MYCOMMAND_TAGS='tag1 tag2'
export KBC_MYCOMMAND_SOME_FLAG=true

export KBC_MYCOMMAND_RESULTS_DIR="/tmp/my-command-results"
mkdir -p "$RESULTS_DIR"
export KBC_MYCOMMAND_RESULT_SHA="${RESULTS_DIR}/RESULT_SHA"
```
```bash
. my-command-env.sh
./konflux-build-cli my-command
```

or mix approaches:

```bash
export KBC_MYCOMMAND_RESULT_FILE_SHA=/tmp/my-command-result-sha
./konflux-build-cli my-command --image-url quay.io/namespace/image:tag --digest sha256:abcde1234 --tags tag1 tag2
```

## How to run unit tests

To run all unit tests:
```bash
go test ./pkg/...
```

To run or debug a specific test or run all tests in a single file, it's most convenient to use UI of your IDE.
To run specific test from terminal execute:
```bash
go test -run ^TestMyCommand_SuccessScenario$ ./pkg/...
```
or for all tests in single package:
```bash
go test ./pkg/commands
```
