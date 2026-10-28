package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentPlist(t *testing.T) {
	t.Setenv("TAAS_CONFIG", "/tmp/a&b/config.json")
	t.Setenv("TAAS_INDEX", "/tmp/index.json")
	plist := agentPlist("/Users/me/bin/taa<sant", "/tmp/log")
	text := string(plist)
	for _, want := range []string{
		"<string>com.taasant.drive</string>",
		"<string>/Users/me/bin/taa&lt;sant</string>",
		"<string>background</string>",
		"<string>/tmp/a&amp;b/config.json</string>",
		"<string>/tmp/index.json</string>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the plist has no %s", want)
		}
	}
	if strings.Count(text, "<key>EnvironmentVariables</key>") != 1 {
		t.Error("the environment must be one dict")
	}

	if runtime.GOOS == "darwin" {
		file := filepath.Join(t.TempDir(), "agent.plist")
		os.WriteFile(file, plist, 0o600)
		if out, err := exec.Command("plutil", "-lint", file).CombinedOutput(); err != nil {
			t.Errorf("plutil: %s", out)
		}
	}
}

func TestAgentPlistWithoutEnvironment(t *testing.T) {
	t.Setenv("TAAS_CONFIG", "")
	t.Setenv("TAAS_INDEX", "")
	if strings.Contains(string(agentPlist("/bin/taasant", "/tmp/log")), "EnvironmentVariables") {
		t.Error("an empty environment must be left out")
	}
}
