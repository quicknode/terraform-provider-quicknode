package provider

import (
	"regexp"
	"testing"
)

func TestUserAgentFollowsTheQuicknodeClientFormat(t *testing.T) {
	got := userAgent("0.3.0", "1.9.8")
	want := regexp.MustCompile(`^quicknode-terraform/0\.3\.0 \((linux|macos|windows)-(x86_64|aarch64|x86); terraform-1\.9\.8\)$`)
	if !want.MatchString(got) {
		t.Fatalf("userAgent = %q, want a match for %s", got, want)
	}
}

func TestPlatformNameKeepsUnmappedNames(t *testing.T) {
	if got := platformName(userAgentArch, "riscv64"); got != "riscv64" {
		t.Fatalf("platformName = %q, want riscv64", got)
	}
}
