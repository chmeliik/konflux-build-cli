---
name: Update dockerfile-json dependency
description: Use this skill to update or change versuions of dockerfile-json dependency in go.mod
---

This skill helps to update `dockerfile-json` dependency.

Original `github.com/keilerkonzept/dockerfile-json` in `go.mod` is replaced by its fork `github.com/konflux-ci/dockerfile-json`.
To update `dockerfile-json` dependency to the newest version use the following command:
```sh
go mod edit -replace github.com/keilerkonzept/dockerfile-json=github.com/konflux-ci/dockerfile-json@dev
```
After updating the version, always run `go mod tidy` to ensure update went smooth.
