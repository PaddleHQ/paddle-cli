// Package config loads and saves the CLI configuration file, keeps credentials
// in the OS keychain where available, and defines the known Paddle environments.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk CLI configuration. It carries private keychain
// bookkeeping from Load to Save, so save the *Config you loaded rather than a
// shallow copy of it (e.g. ResolveConfig's result).
type Config struct {
	CurrentEnvironment string                       `yaml:"current_environment"`
	DefaultOutput      string                       `yaml:"default_output,omitempty"`
	Color              string                       `yaml:"color,omitempty"`
	NoBanner           bool                         `yaml:"no_banner,omitempty"`
	APIVersion         string                       `yaml:"api_version,omitempty"`
	Environments       map[string]EnvironmentConfig `yaml:"environments"`

	// keychainItems records, per environment, the keychain item Load found:
	// Save compares against it to skip unchanged writes and find items to delete.
	keychainItems map[string]keychainItem
	// plaintextReasons says why the last Save put an environment's secrets in
	// this file instead of the keychain; read via StoredInKeychain.
	plaintextReasons map[string]error
}

// EnvironmentConfig holds the stored credentials and login state for one environment.
type EnvironmentConfig struct {
	AccessToken  string `yaml:"access_token,omitempty"`
	RefreshToken string `yaml:"refresh_token,omitempty"`
	ExpiresAt    string `yaml:"expires_at,omitempty"`
	APIKey       string `yaml:"api_key,omitempty"`
	// Scope is the space-separated OAuth scopes granted at login. Not a
	// secret, so it stays in this file even when secrets move to the keychain.
	Scope string `yaml:"scope,omitempty"`
	// SecretsLocation is "keychain" when the secret fields above live in the
	// OS keychain instead of this file. Set by Save.
	SecretsLocation string `yaml:"credential_store,omitempty"`
}

// DefaultPath returns ~/.config/paddle/config.yaml, or "" if the home directory is unknown.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "paddle", "config.yaml")
}

// validColorValues lists the values accepted by the `color` config key.
// Empty is allowed and treated as "auto" by the style.Resolve layer.
var validColorValues = map[string]struct{}{
	"":       {},
	"auto":   {},
	"always": {},
	"never":  {},
}

var errInvalidColor = errors.New("invalid color")

// Load reads the config at path, returning an empty config if the file doesn't exist.
// Keychain read failures are recorded rather than returned; see CredentialReadError.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the user's chosen config file
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Environments: map[string]EnvironmentConfig{}}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if _, ok := validColorValues[cfg.Color]; !ok {
		return nil, fmt.Errorf("%w %q in config: must be one of auto, always, never", errInvalidColor, cfg.Color)
	}
	if cfg.Environments == nil {
		cfg.Environments = map[string]EnvironmentConfig{}
	}
	cfg.hydrateSecrets()
	return &cfg, nil
}

// Save writes cfg to path, moving secrets into the OS keychain where possible.
// It updates cfg in place (each environment's SecretsLocation, plus the
// bookkeeping StoredInKeychain reads), so check StoredInKeychain on the same
// *Config afterwards. An error means the file at path is unchanged.
func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if cfg.Environments == nil {
		cfg.Environments = map[string]EnvironmentConfig{}
	}
	envs, commit, err := cfg.syncCredentials()
	if err != nil {
		return err
	}
	onDisk := *cfg
	onDisk.Environments = envs
	data, err := yaml.Marshal(&onDisk)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	commit()
	return nil
}

// writeFileAtomic replaces path in one rename so a crash or concurrent reader
// never sees a truncated config, which would lose the keychain markers.
// Symlinks are resolved first so a linked config (e.g. from dotfiles) stays linked.
//
//nolint:wrapcheck // os errors already name the op and path; Save adds "writing config".
func writeFileAtomic(path string, data []byte) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ResolveConfig returns a shallow copy of cfg with PADDLE_* environment
// variable overrides applied. Before login no environment is stored, so the
// resolved environment falls back to DefaultEnvironment; the stored config is
// never written with that fallback.
func ResolveConfig(cfg *Config) *Config {
	resolved := *cfg
	if env := os.Getenv("PADDLE_ENVIRONMENT"); env != "" {
		resolved.CurrentEnvironment = env
	}
	if resolved.CurrentEnvironment == "" {
		resolved.CurrentEnvironment = DefaultEnvironment
	}
	if f := os.Getenv("PADDLE_OUTPUT"); f != "" {
		resolved.DefaultOutput = f
	}
	if v := os.Getenv("PADDLE_NO_BANNER"); v == "1" {
		resolved.NoBanner = true
	}
	if v := os.Getenv("PADDLE_API_VERSION"); v != "" {
		resolved.APIVersion = v
	}
	return &resolved
}
