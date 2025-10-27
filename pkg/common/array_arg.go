package common

import (
	"strings"

	"github.com/spf13/cobra"
)

var arrayParamsInCommands = map[*cobra.Command][]string{}

// recordArrayParamForCommand saves the command and the array param for future processing.
func recordArrayParamForCommand(cmd *cobra.Command, paramName string) {
	params := arrayParamsInCommands[cmd]
	params = append(params, paramName)
	arrayParamsInCommands[cmd] = params
}

// buildArrayParamsData creates a map of all subcommands with array parameters to expand.
// We cannot build this map at params registration time, because not all commands were initialized,
// which makes the command path unknown.
func buildArrayParamsData() map[string][]string {
	arrayParams := map[string][]string{}
	for cmd, params := range arrayParamsInCommands {
		commandPath := cmd.CommandPath()
		// Remove the root command from the command path
		firstSpaceIndex := strings.Index(commandPath, " ")
		if firstSpaceIndex > 0 {
			commandPath = commandPath[firstSpaceIndex+1:]
		}
		arrayParams[commandPath] = params
	}
	return arrayParams
}

// expandArrayParameters is a workaround for missing pflag ability to parse parameters array separated by spaces.
// We need to process parameters like:
// cli --array v1 v2 v3 --some-arg
// but pflag supports only
// cli --array v1 --array v2 --array v3 --some-arg
// or comma separated values like:
// cli --array v1,v2,v3 --some-arg
// This function expands array parameters, so "--array v1 v2 v3" becomes "--array v1 --array v2 --array v3"
func ExpandArrayParameters(argv []string) []string {
	out := make([]string, 0, len(argv))

	// Determine the command which is run.
	commandPathArray := []string{}
	for _, arg := range argv {
		if strings.HasPrefix(arg, "-") {
			break
		}
		commandPathArray = append(commandPathArray, arg)
	}
	commandPath := strings.Join(commandPathArray, " ")

	arrayParams := buildArrayParamsData()

	multiFlags := map[string]bool{}
	for _, arrayParam := range arrayParams[commandPath] {
		multiFlags[arrayParam] = true
	}

	for i := 0; i < len(argv); i++ {
		arg := argv[i]

		// Stop processing after "--" sentinel, in case we have positional arguments.
		if arg == "--" {
			out = append(out, argv[i:]...)
			break
		}

		// Handle parameters with = for example: --array=v1 v2
		if strings.HasPrefix(arg, "-") && strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			flag := parts[0]
			if multiFlags[flag] {
				out = append(out, flag, parts[1])
				i++
				for i < len(argv) && argv[i] != "--" && !strings.HasPrefix(argv[i], "-") {
					out = append(out, flag, argv[i])
					i++
				}
				// Now i points to the next parameter, if any.
				// Step back, because the for loop will increment i
				i--
				continue
			}
			// Just regular arg=value parameter
			out = append(out, arg)
			continue
		}

		// If this arg is an array, duplicate the arg before each array element.
		if multiFlags[arg] {
			j := i + 1
			for j < len(argv) && argv[j] != "--" && !strings.HasPrefix(argv[j], "-") {
				out = append(out, arg, argv[j])
				j++
			}
			// Now j points to the next parameter, if any.
			i = j
			// Step back, because the for loop will increment i
			i--
			continue
		}

		out = append(out, arg)
	}
	return out
}
