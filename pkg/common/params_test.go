package common

import (
	"os"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestRegisterParameters(t *testing.T) {

	t.Run("should register string parameter", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"testParam": {
				Name:         "testParam",
				TypeKind:     reflect.String,
				DefaultValue: "default",
				Usage:        "test usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("testParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal(""))
		g.Expect(flag.DefValue).To(Equal("default"))
		g.Expect(flag.Usage).To(Equal("test usage"))
	})

	t.Run("should register string parameter with short from", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"testParam": {
				Name:         "testParam",
				ShortName:    "t",
				TypeKind:     reflect.String,
				DefaultValue: "default",
				Usage:        "test usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("testParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal("t"))
		g.Expect(flag.DefValue).To(Equal("default"))
		g.Expect(flag.Usage).To(Equal("test usage"))
	})

	t.Run("should register int parameter", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"intParam": {
				Name:         "intParam",
				TypeKind:     reflect.Int,
				DefaultValue: "1234",
				Usage:        "int usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("intParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal(""))
		g.Expect(flag.DefValue).To(Equal("1234"))
		g.Expect(flag.Usage).To(Equal("int usage"))
	})

	t.Run("should register int parameter with short from", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"intParam": {
				Name:         "intParam",
				ShortName:    "i",
				TypeKind:     reflect.Int,
				DefaultValue: "1234",
				Usage:        "int usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("intParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal("i"))
		g.Expect(flag.DefValue).To(Equal("1234"))
		g.Expect(flag.Usage).To(Equal("int usage"))
	})

	t.Run("should register bool parameter", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"boolParam": {
				Name:         "boolParam",
				TypeKind:     reflect.Bool,
				DefaultValue: "true",
				Usage:        "bool usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("boolParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal(""))
		g.Expect(flag.DefValue).To(Equal("true"))
		g.Expect(flag.Usage).To(Equal("bool usage"))
	})

	t.Run("should register bool parameter with short from", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"boolParam": {
				Name:         "boolParam",
				ShortName:    "b",
				TypeKind:     reflect.Bool,
				DefaultValue: "true",
				Usage:        "bool usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("boolParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal("b"))
		g.Expect(flag.DefValue).To(Equal("true"))
		g.Expect(flag.Usage).To(Equal("bool usage"))
	})

	t.Run("should register array parameter", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"arrayParam": {
				Name:         "arrayParam",
				TypeKind:     reflect.Array,
				DefaultValue: "a b c",
				Usage:        "array usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("arrayParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal(""))
		g.Expect(flag.DefValue).To(Equal("[a,b,c]"))
		g.Expect(flag.Usage).To(Equal("array usage"))
	})

	t.Run("should register array parameter with short from", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"arrayParam": {
				Name:         "arrayParam",
				ShortName:    "a",
				TypeKind:     reflect.Array,
				DefaultValue: "a b c",
				Usage:        "array usage",
			},
		}

		RegisterParameters(cmd, paramsConfig)

		flag := cmd.Flags().Lookup("arrayParam")
		g.Expect(flag).ToNot(BeNil())
		g.Expect(flag.Shorthand).To(Equal("a"))
		g.Expect(flag.DefValue).To(Equal("[a,b,c]"))
		g.Expect(flag.Usage).To(Equal("array usage"))
	})

	t.Run("should panic on invalid int default value", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"intParam": {
				Name:         "intParam",
				TypeKind:     reflect.Int,
				DefaultValue: "invalid",
				Usage:        "int usage",
			},
		}

		g.Expect(func() {
			RegisterParameters(cmd, paramsConfig)
		}).To(Panic())
	})

	t.Run("should panic on invalid bool default value", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"boolParam": {
				Name:         "boolParam",
				TypeKind:     reflect.Bool,
				DefaultValue: "invalid",
				Usage:        "bool usage",
			},
		}

		g.Expect(func() {
			RegisterParameters(cmd, paramsConfig)
		}).To(Panic())
	})

	t.Run("should panic on parameter name mismatch", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"testParam": {
				Name:     "differentName",
				TypeKind: reflect.String,
				Usage:    "test usage",
			},
		}

		g.Expect(func() {
			RegisterParameters(cmd, paramsConfig)
		}).To(Panic())
	})

	t.Run("should panic on unknown parameter type", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		paramsConfig := map[string]Parameter{
			"testParam": {
				Name:     "testParam",
				TypeKind: reflect.Float64,
				Usage:    "test usage",
			},
		}

		g.Expect(func() {
			RegisterParameters(cmd, paramsConfig)
		}).To(Panic())
	})
}

func TestParseParameters(t *testing.T) {

	type TestParams struct {
		StringParam string   `paramName:"stringParam"`
		IntParam    int      `paramName:"intParam"`
		BoolParam   bool     `paramName:"boolParam"`
		ArrayParam  []string `paramName:"arrayParam"`
	}

	t.Run("should parse string parameter from command line", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().String("stringParam", "default", "usage")
		cmd.Flags().Set("stringParam", "test-value")

		paramsConfig := map[string]Parameter{
			"stringParam": {
				Name:     "stringParam",
				TypeKind: reflect.String,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.StringParam).To(Equal("test-value"))
	})

	t.Run("should parse short string parameter from command line", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().StringP("stringParam", "s", "default", "usage")
		cmd.Flags().Set("stringParam", "test-value")

		paramsConfig := map[string]Parameter{
			"stringParam": {
				Name:      "stringParam",
				ShortName: "s",
				TypeKind:  reflect.String,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.StringParam).To(Equal("test-value"))
	})

	t.Run("should parse string parameter from environment variable", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().String("stringParam", "default", "usage")

		os.Setenv("TEST_ENV_VAR", "env-value")
		defer os.Unsetenv("TEST_ENV_VAR")

		paramsConfig := map[string]Parameter{
			"stringParam": {
				Name:       "stringParam",
				TypeKind:   reflect.String,
				EnvVarName: "TEST_ENV_VAR",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.StringParam).To(Equal("env-value"))
	})

	t.Run("should use default value when string parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().String("stringParam", "default-value", "usage")

		paramsConfig := map[string]Parameter{
			"stringParam": {
				Name:         "stringParam",
				TypeKind:     reflect.String,
				DefaultValue: "default-value",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.StringParam).To(Equal("default-value"))
	})

	t.Run("should return error for required string parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().String("stringParam", "", "usage")

		paramsConfig := map[string]Parameter{
			"stringParam": {
				Name:     "stringParam",
				TypeKind: reflect.String,
				Required: true,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("required parameter 'stringParam' is not set"))
	})

	t.Run("should parse int parameter", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Int("intParam", 0, "usage")
		cmd.Flags().Set("intParam", "123")

		paramsConfig := map[string]Parameter{
			"intParam": {
				Name:     "intParam",
				TypeKind: reflect.Int,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.IntParam).To(Equal(123))
	})

	t.Run("should parse int parameter from environment variable", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Int("intParam", 0, "usage")

		os.Setenv("INT_ENV_VAR", "456")
		defer os.Unsetenv("INT_ENV_VAR")

		paramsConfig := map[string]Parameter{
			"intParam": {
				Name:       "intParam",
				TypeKind:   reflect.Int,
				EnvVarName: "INT_ENV_VAR",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.IntParam).To(Equal(456))
	})

	t.Run("should use default value when int parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Int("intParam", 1234, "usage")

		paramsConfig := map[string]Parameter{
			"intParam": {
				Name:         "intParam",
				TypeKind:     reflect.Int,
				DefaultValue: "1234",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.IntParam).To(Equal(1234))
	})

	t.Run("should return error for required int parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Int("intParam", 1234, "usage")

		paramsConfig := map[string]Parameter{
			"intParam": {
				Name:     "intParam",
				TypeKind: reflect.Int,
				Required: true,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("required parameter 'intParam' is not set"))
	})

	t.Run("should parse bool parameter", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Bool("boolParam", false, "usage")
		cmd.Flags().Set("boolParam", "true")

		paramsConfig := map[string]Parameter{
			"boolParam": {
				Name:     "boolParam",
				TypeKind: reflect.Bool,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.BoolParam).To(BeTrue())
	})

	t.Run("should parse bool parameter from environment variable", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Bool("boolParam", false, "usage")

		os.Setenv("BOOL_ENV_VAR", "true")
		defer os.Unsetenv("BOOL_ENV_VAR")

		paramsConfig := map[string]Parameter{
			"boolParam": {
				Name:       "boolParam",
				TypeKind:   reflect.Bool,
				EnvVarName: "BOOL_ENV_VAR",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.BoolParam).To(Equal(true))
	})

	t.Run("should use default value when bool parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Bool("boolParam", true, "usage")

		paramsConfig := map[string]Parameter{
			"boolParam": {
				Name:         "boolParam",
				TypeKind:     reflect.Bool,
				DefaultValue: "true",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.BoolParam).To(Equal(true))
	})

	t.Run("should return error for required bool parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().Bool("boolParam", true, "usage")

		paramsConfig := map[string]Parameter{
			"boolParam": {
				Name:     "boolParam",
				TypeKind: reflect.Bool,
				Required: true,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("required parameter 'boolParam' is not set"))
	})

	t.Run("should parse array parameter", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().StringArray("arrayParam", nil, "usage")
		cmd.Flags().Set("arrayParam", "item1")
		cmd.Flags().Set("arrayParam", "item2")

		paramsConfig := map[string]Parameter{
			"arrayParam": {
				Name:     "arrayParam",
				TypeKind: reflect.Array,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.ArrayParam).To(Equal([]string{"item1", "item2"}))
	})

	t.Run("should parse array parameter from environment variable", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().StringArray("arrayParam", nil, "usage")

		os.Setenv("ARRAY_ENV_VAR", "item1 item2 item3")
		defer os.Unsetenv("ARRAY_ENV_VAR")

		paramsConfig := map[string]Parameter{
			"arrayParam": {
				Name:       "arrayParam",
				TypeKind:   reflect.Array,
				EnvVarName: "ARRAY_ENV_VAR",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.ArrayParam).To(Equal([]string{"item1", "item2", "item3"}))
	})

	t.Run("should use default value when array parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().StringArray("arrayParam", []string{"a", "b"}, "usage")

		paramsConfig := map[string]Parameter{
			"arrayParam": {
				Name:         "arrayParam",
				TypeKind:     reflect.Array,
				DefaultValue: "a,b",
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(params.ArrayParam).To(Equal([]string{"a", "b"}))
	})

	t.Run("should return error for required array parameter not provided", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{}
		cmd.Flags().StringArray("arrayParam", []string{"a", "b"}, "usage")

		paramsConfig := map[string]Parameter{
			"arrayParam": {
				Name:     "arrayParam",
				TypeKind: reflect.Array,
				Required: true,
			},
		}

		params := &TestParams{}
		err := ParseParameters(cmd, paramsConfig, params)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("required parameter 'arrayParam' is not set"))
	})

	t.Run("should panic on field without paramName tag", func(t *testing.T) {
		g := NewWithT(t)

		type BadParams struct {
			StringParam string
		}

		cmd := &cobra.Command{}
		cmd.Flags().String("stringParam", "", "usage")

		paramsConfig := map[string]Parameter{
			"stringParam": {
				Name:     "stringParam",
				TypeKind: reflect.String,
			},
		}

		params := &BadParams{}

		g.Expect(func() {
			ParseParameters(cmd, paramsConfig, params)
		}).To(Panic())
	})

	t.Run("should panic on unsupported parameter type", func(t *testing.T) {
		g := NewWithT(t)

		type BadParams struct {
			FloatParam float64 `paramName:"floatParam"`
		}

		cmd := &cobra.Command{}
		cmd.Flags().Float64("floatParam", 0.0, "usage")

		paramsConfig := map[string]Parameter{
			"floatParam": {
				Name:     "floatParam",
				TypeKind: reflect.Float64,
			},
		}

		params := &BadParams{}

		g.Expect(func() {
			ParseParameters(cmd, paramsConfig, params)
		}).To(Panic())
	})
}
