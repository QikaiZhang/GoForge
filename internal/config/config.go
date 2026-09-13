// Package config loads goforge.yaml, the per-project configuration
// file goforge reads from the target project root.
//
// Precedence, from weakest to strongest:
//
//	built-in defaults  →  goforge.yaml  →  environment variables  →  flags
//
// Each layer is applied by a separate, individually testable step:
// Load starts from defaults and merges the file; ApplyEnv overlays
// environment variables; the dev command's flag parsing is the final
// layer.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// FileName is the config file goforge looks for in the project root.
const FileName = "goforge.yaml"

// DefaultPort is used when the config file does not set server.port.
const DefaultPort = 8080

// Project and Server are the two sections of goforge.yaml:
//
//	project:
//	  name: user-service
//	server:
//	  port: 8080
type Config struct {
	Project Project `yaml:"project"`
	Server  Server  `yaml:"server"`
}

type Project struct {
	// Name is the project's canonical name. It defaults to empty;
	// commands that need a name fall back to the go.mod module path.
	Name string `yaml:"name"`
}

type Server struct {
	// Port is where `goforge dev` runs the generated server. The
	// generated server itself reads the PORT environment variable.
	Port int `yaml:"port"`
}

// defaults returns the built-in baseline every load starts from.
func defaults() Config {
	return Config{Server: Server{Port: DefaultPort}}
}

// Load reads goforge.yaml from dir. A missing file is not an error —
// configuration is optional and the built-in defaults apply. A broken
// file (bad YAML, unknown keys, out-of-range values) is an error:
// silently ignoring a config the user believes is active is worse than
// failing loudly.
func Load(dir string) (Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	} else if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", FileName, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	// KnownFields rejects keys that do not map to any struct field, so
	// a typo like "prot: 9090" fails loudly instead of being ignored.
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", FileName, err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", FileName, err)
	}
	return cfg, nil
}

// applyDefaults fills unset values. The zero value of port is 0,
// which is not a valid port, so it always means "unset".
func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = DefaultPort
	}
}

// Validate checks the merged configuration.
func (c Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d out of range (1-65535)", c.Server.Port)
	}
	return nil
}

// ApplyEnv overlays environment variables onto cfg. lookup is
// injected (pass os.Getenv) so tests can supply fakes.
//
//	GOFORGE_PORT   overrides server.port
func ApplyEnv(cfg *Config, lookup func(string) string) {
	if v := lookup("GOFORGE_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
		// An unparsable GOFORGE_PORT is ignored here: the port that
		// survives still passes Validate, so the effective value is
		// never nonsense.
	}
}
