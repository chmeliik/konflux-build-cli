package commands

import (
	"reflect"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/konflux-ci/konflux-build-cli/pkg/config"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/spf13/cobra"
)

var CacheProxyParamsConfig = map[string]common.Parameter{
	"enable": {
		Name:       "enable",
		ShortName:  "e",
		EnvVarName: "ENABLE",
		TypeKind:   reflect.String,
		Usage:      "Whether to enable cache proxy or not. Required.",
		Required:   true,
	},
	"default-http-proxy": {
		Name:         "default-http-proxy",
		ShortName:    "p",
		EnvVarName:   "DEFAULT_HTTP_PROXY",
		TypeKind:     reflect.String,
		Usage:        "change the default http proxy value. Optional.",
		Required:     false,
		DefaultValue: "",
	},
	"default-no-proxy": {
		Name:         "default-no-proxy",
		ShortName:    "n",
		EnvVarName:   "DEFAULT_NO_PROXY",
		TypeKind:     reflect.String,
		Usage:        "change the default no proxy value. Optional.",
		Required:     false,
		DefaultValue: "",
	},
	"http-proxy-result-path": {
		Name:         "http-proxy-result-path",
		ShortName:    "r",
		EnvVarName:   "HTTP_PROXY_RESULTS_PATH",
		TypeKind:     reflect.String,
		Usage:        "set the result path for http proxy. Optional.",
		Required:     false,
		DefaultValue: "http-proxy",
	},
	"no-proxy-result-path": {
		Name:         "no-proxy-result-path",
		ShortName:    "s",
		EnvVarName:   "NO_PROXY_RESULTS_PATH",
		TypeKind:     reflect.String,
		Usage:        "set the result path for no proxy. Optional.",
		Required:     false,
		DefaultValue: "no-proxy",
	},
}

type CacheProxyParams struct {
	Enable              string `paramName:"enable"`
	DefaultHttpProxy    string `paramName:"default-http-proxy"`
	DefaultNoProxy      string `paramName:"default-no-proxy"`
	HttpProxyResultPath string `paramName:"http-proxy-result-path"`
	NoProxyResultPath   string `paramName:"no-proxy-result-path"`
}

type CacheProxyConfigs struct {
	ConfigReader config.ConfigReader
}

type CacheProxyResults struct {
	HttpProxy string `json:"http-proxy"`
	NoProxy   string `json:"no-proxy"`
}

type CacheProxy struct {
	Configs       CacheProxyConfigs
	Params        *CacheProxyParams
	Results       CacheProxyResults
	ResultsWriter common.ResultsWriterInterface
}

func NewCacheProxy(cmd *cobra.Command) (*CacheProxy, error) {
	var err error
	cacheProxy := &CacheProxy{}

	params := &CacheProxyParams{}
	if err = common.ParseParameters(cmd, CacheProxyParamsConfig, params); err != nil {
		return nil, err
	}
	cacheProxy.Params = params

	// Initialize Config Reader
	newConfigReader, err := config.NewConfigReader()
	if err != nil {
		return nil, err
	}
	cacheProxy.Configs.ConfigReader = newConfigReader

	cacheProxy.ResultsWriter = common.NewResultsWriter()

	return cacheProxy, nil
}

// Run executes the command logic.
func (c *CacheProxy) Run() error {
	var allowCache string
	var httpProxy, noProxy string

	common.LogParameters(CacheProxyParamsConfig, c.Params)

	l.Logger.Debug("Reading config data")
	cacheProxyConfig, err := c.Configs.ConfigReader.ReadConfigData()
	if err != nil {
		l.Logger.Warnf("Error while reading config data: %s", err.Error())
		// failed to read config data, use defaults
		httpProxy = c.Params.DefaultHttpProxy
		noProxy = c.Params.DefaultNoProxy
		allowCache = "true"
	} else {
		l.Logger.Debugf("cache proxy config: %v", cacheProxyConfig)
		allowCache = cacheProxyConfig.AllowCacheProxy
		if allowCache == "true" {
			httpProxy = cacheProxyConfig.HttpProxy
			noProxy = cacheProxyConfig.NoProxy
		} else { // allow-cache-proxy is false (or any other value != true)
			httpProxy = ""
			noProxy = ""
		}
	}

	// Apply ENABLE_CACHE_PROXY check from the param ONLY if backend allows it, for example, k8s cluster
	if allowCache == "true" && c.Params.Enable == "true" {
		l.Logger.Info("Cache proxy enabled in both backend and param")
	} else {
		l.Logger.Info("Cache proxy is disabled in param or in backend")
		httpProxy = ""
		noProxy = ""
	}

	c.Results.HttpProxy = httpProxy
	c.Results.NoProxy = noProxy

	c.logResults()

	err = c.ResultsWriter.WriteResultString(c.Results.HttpProxy, c.Params.HttpProxyResultPath)
	if err != nil {
		l.Logger.Errorf("failed to write result for http-proxy with error: %s", err.Error())
	}
	err = c.ResultsWriter.WriteResultString(c.Results.NoProxy, c.Params.NoProxyResultPath)
	if err != nil {
		l.Logger.Errorf("failed to write result for no-proxy with error: %s", err.Error())
	}

	return nil
}

func (c *CacheProxy) logResults() {
	l.Logger.Infof("[result] HTTP PROXY: %s", c.Results.HttpProxy)
	l.Logger.Infof("[result] NO PROXY: %s", c.Results.NoProxy)
}
