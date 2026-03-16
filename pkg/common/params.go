package common

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/spf13/cobra"
)

type Parameter struct {
	Name         string
	TypeKind     reflect.Kind
	ShortName    string
	EnvVarName   string
	DefaultValue string // makes no sense if Required is true
	Usage        string
	Required     bool
}

// RegisterParameters configures Cobra CLI parameters based on given Parameters data.
func RegisterParameters(cmd *cobra.Command, paramsConfig map[string]Parameter) {
	getMessageInvalidParameterDefaultValue := func(p Parameter) string {
		return fmt.Sprintf("parameter '%s' has invalid default value '%v'", p.Name, p.DefaultValue)
	}
	var err error

	for pName, p := range paramsConfig {
		if pName != p.Name {
			panic(fmt.Sprintf("parameter name '%s' and tag '%s' must be equal", p.Name, pName))
		}

		switch p.TypeKind {

		case reflect.String:
			if p.ShortName != "" {
				cmd.Flags().StringP(p.Name, p.ShortName, p.DefaultValue, p.Usage)
			} else {
				cmd.Flags().String(p.Name, p.DefaultValue, p.Usage)
			}

		case reflect.Int:
			var defaultValue int
			if p.DefaultValue != "" {
				defaultValue, err = strconv.Atoi(p.DefaultValue)
				if err != nil {
					panic(getMessageInvalidParameterDefaultValue(p))
				}
			}

			if p.ShortName != "" {
				cmd.Flags().IntP(p.Name, p.ShortName, defaultValue, p.Usage)
			} else {
				cmd.Flags().Int(p.Name, defaultValue, p.Usage)
			}

		case reflect.Bool:
			var defaultValue bool
			if p.DefaultValue != "" {
				defaultValue, err = strconv.ParseBool(p.DefaultValue)
				if err != nil {
					panic(getMessageInvalidParameterDefaultValue(p))
				}
			}

			if p.ShortName != "" {
				cmd.Flags().BoolP(p.Name, p.ShortName, defaultValue, p.Usage)
			} else {
				cmd.Flags().Bool(p.Name, defaultValue, p.Usage)
			}

		case reflect.Array, reflect.Slice:
			recordArrayParamForCommand(cmd, "--"+p.Name)
			// Imply string array
			var defaultValue []string = nil
			if p.DefaultValue != "" {
				defaultValue = strings.Fields(p.DefaultValue)
			}
			if p.ShortName != "" {
				cmd.Flags().StringArrayP(p.Name, p.ShortName, defaultValue, p.Usage)
				recordArrayParamForCommand(cmd, "-"+p.ShortName)
			} else {
				cmd.Flags().StringArray(p.Name, defaultValue, p.Usage)
			}

		default:
			panic("RegisterParameters: unknown parameter type")
		}
	}
}

// ParseParameters populates parameters structure with provided values based on paramters configuration
func ParseParameters(cmd *cobra.Command, paramsConfig map[string]Parameter, params interface{}) error {
	getMessageRequiredParameterMissing := func(p Parameter) string {
		return fmt.Sprintf("required parameter '%s' is not set", p.Name)
	}

	paramsStruct := reflect.ValueOf(params).Elem()
	paramsStructType := paramsStruct.Type()

	// Iterate over parameters in the top loop to avoid missing a required parameter
	for tag, paramData := range paramsConfig {
		fieldFound := false
		for i := 0; i < paramsStruct.NumField(); i++ {
			field := paramsStructType.Field(i)
			fieldTag := field.Tag.Get("paramName")
			if fieldTag == "" {
				// Skip if no paramName tag
				continue
			}
			if fieldTag == tag {
				fieldValue := paramsStruct.Field(i)
				if fieldValue.CanSet() {
					paramProvided := cmd.Flags().Changed(paramData.Name)

					switch fieldValue.Kind() {

					case reflect.String:
						if paramProvided {
							val, err := cmd.Flags().GetString(paramData.Name)
							if err != nil {
								return err
							}
							fieldValue.SetString(val)
							break
						}
						if paramData.EnvVarName != "" {
							val := os.Getenv(paramData.EnvVarName)
							if val != "" {
								fieldValue.SetString(val)
								break
							}
						}
						// The cli parameter was not provided nor env var set
						if paramData.Required {
							return errors.New(getMessageRequiredParameterMissing(paramData))
						}
						// Fall back to default value
						fieldValue.SetString(paramData.DefaultValue)

					case reflect.Int:
						if paramProvided {
							val, err := cmd.Flags().GetInt(paramData.Name)
							if err != nil {
								return err
							}
							fieldValue.SetInt(int64(val))
							break
						}
						if paramData.EnvVarName != "" {
							valStr := os.Getenv(paramData.EnvVarName)
							if valStr != "" {
								val, err := strconv.ParseInt(valStr, 10, 64)
								if err != nil {
									return err
								}
								fieldValue.SetInt(val)
								break
							}
						}
						// The cli parameter was not provided nor env var set
						if paramData.Required {
							return errors.New(getMessageRequiredParameterMissing(paramData))
						}
						// Fall back to default value
						val, err := cmd.Flags().GetInt(paramData.Name)
						if err != nil {
							return err
						}
						fieldValue.SetInt(int64(val))

					case reflect.Bool:
						if paramProvided {
							val, err := cmd.Flags().GetBool(paramData.Name)
							if err != nil {
								return err
							}
							fieldValue.SetBool(val)
							break
						}
						if paramData.EnvVarName != "" {
							valStr := os.Getenv(paramData.EnvVarName)
							if valStr != "" {
								val, err := strconv.ParseBool(valStr)
								if err != nil {
									return err
								}
								fieldValue.SetBool(val)
								break
							}
						}
						// The cli parameter was not provided nor env var set
						if paramData.Required {
							return errors.New(getMessageRequiredParameterMissing(paramData))
						}
						// Fall back to default value
						val, err := cmd.Flags().GetBool(paramData.Name)
						if err != nil {
							return err
						}
						fieldValue.SetBool(val)

					case reflect.Array, reflect.Slice:
						// Imply string array
						if paramProvided {
							val, err := cmd.Flags().GetStringArray(paramData.Name)
							if err != nil {
								return err
							}
							fieldValue.Set(reflect.ValueOf(val))
							break
						}
						if paramData.EnvVarName != "" {
							valStr := os.Getenv(paramData.EnvVarName)
							if valStr != "" {
								val := strings.Fields(valStr)
								fieldValue.Set(reflect.ValueOf(val))
								break
							}
						}
						// The cli parameter was not provided nor env var set
						if paramData.Required {
							return errors.New(getMessageRequiredParameterMissing(paramData))
						}
						// Fall back to default value
						val, err := cmd.Flags().GetStringArray(paramData.Name)
						if err != nil {
							return err
						}
						fieldValue.Set(reflect.ValueOf(val))

					default:
						panic(fmt.Sprintf("not supported parameter type '%v' for '%s' parameter", fieldValue.Kind(), paramData.Name))
					}

					fieldFound = true
					break
				} else {
					panic(fmt.Sprintf("cannot set value for '%s' field", field.Name))
				}
			}
		}
		if !fieldFound {
			panic(fmt.Sprintf("field with tag '%s' not found in '%s' struct", tag, paramsStructType.Name()))
		}
	}
	return nil
}

// LogParameters takes a params struct populated by ParseParameters and logs parameter values.
// Also needs the paramsConfig map to find parameter info.
//
// Logs the user-facing parameter names (from the `paramName` struct tag).
//
// Always logs required params.
// For optional params, decides whether to log as follows:
// - booleans: log if the value is different than the default
// - slices, arrays: log if not empty
// - anything else: log if the value is not the "zero value" for its type
func LogParameters(paramsConfig map[string]Parameter, params any) {
	paramsStruct := reflect.ValueOf(params).Elem()
	paramsStructType := paramsStruct.Type()

	for i := 0; i < paramsStruct.NumField(); i++ {
		field := paramsStructType.Field(i)
		tag := field.Tag.Get("paramName")
		if tag == "" {
			continue
		}

		paramData, ok := paramsConfig[tag]
		if !ok {
			continue
		}

		fieldValue := paramsStruct.Field(i)

		if !shouldLog(fieldValue, paramData) {
			continue
		}

		l.Logger.Infof("[param] %s: %v", paramData.Name, fieldValue.Interface())
	}
}

func shouldLog(fieldValue reflect.Value, paramData Parameter) bool {
	if paramData.Required {
		return true
	}

	switch fieldValue.Kind() {
	case reflect.Bool:
		defaultValue := paramData.DefaultValue
		if defaultValue == "" {
			return fieldValue.Bool()
		}
		if defBool, err := strconv.ParseBool(defaultValue); err == nil {
			return fieldValue.Bool() != defBool
		}
		// shouldn't be possible to get here, RegisterParameters would have panicked by now
		return true
	case reflect.Array, reflect.Slice:
		return fieldValue.Len() > 0
	default:
		return !fieldValue.IsZero()
	}
}
