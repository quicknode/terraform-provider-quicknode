package provider

import (
	"fmt"
	"runtime"
)

var userAgentOS = map[string]string{
	"darwin":  "macos",
	"linux":   "linux",
	"windows": "windows",
}

var userAgentArch = map[string]string{
	"amd64": "x86_64",
	"arm64": "aarch64",
	"386":   "x86",
}

// userAgent identifies the provider to Quicknode, in the same
// `client/version (os-arch; runtime-version)` form the other Quicknode tools
// send. The platform uses Rust's OS and architecture names to match them.
func userAgent(providerVersion, terraformVersion string) string {
	return fmt.Sprintf("quicknode-terraform/%s (%s-%s; terraform-%s)",
		providerVersion, platformName(userAgentOS, runtime.GOOS), platformName(userAgentArch, runtime.GOARCH), terraformVersion)
}

func platformName(names map[string]string, goName string) string {
	if name, ok := names[goName]; ok {
		return name
	}
	return goName
}
