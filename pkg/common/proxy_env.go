package common

// Given an http_proxy URL and a no_proxy string,
// return all the environment variables needed to ensure the proxy is configured.
//
// That means HTTP_PROXY, HTTPS_PROXY, NO_PROXY and their lowercase variants,
// because some tools use lowercase while others use uppercase.
//
// If either of the params is an empty string, does not include the associated variables.
func ProxyEnvVars(httpProxy string, noProxy string) []string {
	var env []string
	if httpProxy != "" {
		env = append(env,
			"HTTP_PROXY="+httpProxy,
			"http_proxy="+httpProxy,
			"HTTPS_PROXY="+httpProxy,
			"https_proxy="+httpProxy,
		)
	}
	if noProxy != "" {
		env = append(env, "NO_PROXY="+noProxy, "no_proxy="+noProxy)
	}
	return env
}
