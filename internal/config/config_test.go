package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a goforge.yaml into a fresh temp dir and returns
// the dir, ready for Load.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadMissingFileYieldsDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, ""))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != DefaultPort {
		t.Errorf("port = %d, want default %d", cfg.Server.Port, DefaultPort)
	}
	if cfg.Project.Name != "" {
		t.Errorf("name = %q, want empty", cfg.Project.Name)
	}
}

func TestLoadFullFile(t *testing.T) {
	cfg, err := Load(writeConfig(t, "project:\n  name: user-service\nserver:\n  port: 9090\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Project.Name != "user-service" {
		t.Errorf("name = %q, want user-service", cfg.Project.Name)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoadPartialFileAppliesDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, "project:\n  name: demo\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != DefaultPort {
		t.Errorf("port = %d, want default %d", cfg.Server.Port, DefaultPort)
	}
}

func TestLoadBrokenYAML(t *testing.T) {
	_, err := Load(writeConfig(t, "project:\n  name: [unclosed\n"))
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Errorf("error = %v, want a parse error", err)
	}
}

func TestLoadUnknownFieldIsRejected(t *testing.T) {
	// A typo ("prot") must fail loudly; silently ignoring it would run
	// the server on the default port while the user believes otherwise.
	_, err := Load(writeConfig(t, "server:\n  prot: 9090\n"))
	if err == nil {
		t.Fatal("unknown field should be rejected")
	}
	if !strings.Contains(err.Error(), "prot") {
		t.Errorf("error = %v, want it to name the unknown field", err)
	}
}

func TestLoadPortOutOfRange(t *testing.T) {
	_, err := Load(writeConfig(t, "server:\n  port: 70000\n"))
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error = %v, want a range error", err)
	}
}

func TestApplyEnvOverridesFile(t *testing.T) {
	cfg := Config{Server: Server{Port: 9090}}
	env := map[string]string{"GOFORGE_PORT": "7777"}
	ApplyEnv(&cfg, func(key string) string { return env[key] })
	if cfg.Server.Port != 7777 {
		t.Errorf("port = %d, want 7777 (env wins)", cfg.Server.Port)
	}
}

func TestApplyEnvIgnoresGarbage(t *testing.T) {
	cfg := Config{Server: Server{Port: 9090}}
	env := map[string]string{"GOFORGE_PORT": "not-a-number"}
	ApplyEnv(&cfg, func(key string) string { return env[key] })
	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090 (garbage ignored)", cfg.Server.Port)
	}
}

func TestApplyEnvWithRealGetenv(t *testing.T) {
	cfg := Config{Server: Server{Port: 9090}}
	t.Setenv("GOFORGE_PORT", "8081")
	ApplyEnv(&cfg, os.Getenv)
	if cfg.Server.Port != 8081 {
		t.Errorf("port = %d, want 8081", cfg.Server.Port)
	}
}
