package cliwrappers

import (
	"fmt"
	"slices"
)

// A command that wraps another command, e.g. 'unshare --user -- <command that runs in user namespace>'
type WrapperCmd struct {
	cmd []string
}

func NewWrapperCmd(name string, args ...string) WrapperCmd {
	cmd := append([]string{name}, args...)
	return WrapperCmd{cmd}
}

func (w WrapperCmd) CheckExists() error {
	exists, err := CheckCliToolAvailable(w.cmd[0])
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("executable not found in PATH: %s", w.cmd[0])
	}
	return nil
}

func (w WrapperCmd) WithArgs(args ...string) WrapperCmd {
	return WrapperCmd{slices.Concat(w.cmd, args)}
}

// Wrap a command (given as name + args, same as exec.Command()) with this WrapperCmd.
// Return the name of the wrapper executable and the wrapped args.
func (w WrapperCmd) Wrap(name string, args []string) (string, []string) {
	wrapped := JoinWrappers(w, NewWrapperCmd(name, args...))
	return wrapped.cmd[0], wrapped.cmd[1:]
}

// Join multiple wrappers to create a new wrapper.
// Separates the individual commands with the -- separator.
func JoinWrappers(wrappers ...WrapperCmd) WrapperCmd {
	var w WrapperCmd
	for _, w2 := range wrappers {
		if len(w.cmd) == 0 {
			w.cmd = slices.Clone(w2.cmd)
		} else if len(w2.cmd) == 0 {
			continue
		} else {
			w.cmd = append(w.cmd, "--")
			w.cmd = append(w.cmd, w2.cmd...)
		}
	}
	return w
}
