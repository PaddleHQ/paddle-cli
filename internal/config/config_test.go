package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config"
)

func TestConfig_LoadAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		CurrentEnvironment: "test_env_apikey_only",
		Environments: map[string]config.EnvironmentConfig{
			"test_env_apikey_only": {
				AccessToken:  "test-access-token",
				RefreshToken: "test-refresh-token",
			},
		},
	}

	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.CurrentEnvironment != cfg.CurrentEnvironment {
		t.Errorf("CurrentEnvironment = %q, want %q", loaded.CurrentEnvironment, cfg.CurrentEnvironment)
	}

	sandboxEnv, ok := loaded.Environments["test_env_apikey_only"]
	if !ok {
		t.Fatal("expected test_env_apikey_only environment in loaded config")
	}
	if sandboxEnv.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", sandboxEnv.AccessToken, "test-access-token")
	}
	if sandboxEnv.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", sandboxEnv.RefreshToken, "test-refresh-token")
	}
}

func TestConfig_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error for missing file: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config for missing file")
	}
	if cfg.Environments == nil {
		t.Error("expected non-nil Environments map for missing file")
	}
}

func TestConfig_DefaultPath(t *testing.T) {
	path := config.DefaultPath()
	if path == "" {
		t.Error("DefaultPath() returned empty string")
	}
}

func TestConfig_SetCurrentEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		CurrentEnvironment: "test_env_apikey_only",
		Environments:       map[string]config.EnvironmentConfig{},
	}

	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	cfg.CurrentEnvironment = "test_env"
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.CurrentEnvironment != "test_env" {
		t.Errorf("CurrentEnvironment = %q, want %q", loaded.CurrentEnvironment, "test_env")
	}
}

func TestLoad_IgnoresUnknownKeys_DefaultFormatLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	legacy := []byte("current_environment: test_env_apikey_only\ndefault_format: csv\n")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("seeding legacy config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("loading legacy config must not error: %v", err)
	}
	if cfg.DefaultOutput != "" {
		t.Errorf("legacy default_format should be ignored, got DefaultOutput = %q", cfg.DefaultOutput)
	}
	if cfg.CurrentEnvironment != "test_env_apikey_only" {
		t.Errorf("rest of config must still load; CurrentEnvironment = %q", cfg.CurrentEnvironment)
	}
}

func TestConfig_EnvVarOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		CurrentEnvironment: "test_env_apikey_only",
		Environments:       map[string]config.EnvironmentConfig{},
	}

	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	t.Setenv("PADDLE_ENVIRONMENT", "test_env")

	resolved := config.ResolveConfig(loaded)

	if resolved.CurrentEnvironment != "test_env" {
		t.Errorf("CurrentEnvironment = %q after PADDLE_ENVIRONMENT override, want %q", resolved.CurrentEnvironment, "test_env")
	}

	// Original should not be mutated
	if loaded.CurrentEnvironment != "test_env_apikey_only" {
		t.Errorf("original CurrentEnvironment mutated to %q, want %q", loaded.CurrentEnvironment, "test_env_apikey_only")
	}

	// Clear the env var and verify it falls back
	t.Setenv("PADDLE_ENVIRONMENT", "")
	resolved2 := config.ResolveConfig(loaded)
	if resolved2.CurrentEnvironment != "test_env_apikey_only" {
		t.Errorf("CurrentEnvironment = %q without env override, want %q", resolved2.CurrentEnvironment, "test_env_apikey_only")
	}
}

func TestConfig_ColorField_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		CurrentEnvironment: "test_env_apikey_only",
		Color:              "always",
		Environments:       map[string]config.EnvironmentConfig{},
	}
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Color != "always" {
		t.Errorf("Color = %q, want %q", loaded.Color, "always")
	}
}

func TestConfig_ColorField_RejectsUnknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Hand-write the YAML so we exercise the validation path on Load.
	data := []byte("current_environment: test_env_apikey_only\ncolor: maybe\nenvironments: {}\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := config.Load(path); err == nil {
		t.Errorf("expected error for invalid color value, got nil")
	}
}

func TestConfig_ColorField_DefaultsToEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// File without color field: Load must succeed and Color must be empty
	// (the resolve layer treats empty as "auto").
	data := []byte("current_environment: test_env_apikey_only\nenvironments: {}\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Color != "" {
		t.Errorf("Color = %q, want empty", cfg.Color)
	}
}

func TestConfig_NoBannerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := config.Save(&config.Config{NoBanner: true, Environments: map[string]config.EnvironmentConfig{}}, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.NoBanner {
		t.Errorf("NoBanner = false, want true after round-trip")
	}
}

func TestResolveConfig_EmptyEnvironmentFallsBackToDefault(t *testing.T) {
	t.Setenv("PADDLE_ENVIRONMENT", "")
	resolved := config.ResolveConfig(&config.Config{})
	if resolved.CurrentEnvironment != config.DefaultEnvironment {
		t.Errorf("CurrentEnvironment = %q with no stored environment, want default %q",
			resolved.CurrentEnvironment, config.DefaultEnvironment)
	}
}

func TestResolveConfig_NoBannerEnvVar(t *testing.T) {
	t.Setenv("PADDLE_NO_BANNER", "1")
	resolved := config.ResolveConfig(&config.Config{})
	if !resolved.NoBanner {
		t.Errorf("NoBanner = false, want true with PADDLE_NO_BANNER=1")
	}
}

func TestConfig_APIVersionField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("api_version: \"2025-01-01\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIVersion != "2025-01-01" {
		t.Errorf("got APIVersion=%q, want %q", cfg.APIVersion, "2025-01-01")
	}
}

func TestResolveConfig_PaddleAPIVersionEnv(t *testing.T) {
	t.Setenv("PADDLE_API_VERSION", "2024-10-01")
	cfg := &config.Config{Environments: map[string]config.EnvironmentConfig{}}
	resolved := config.ResolveConfig(cfg)
	if resolved.APIVersion != "2024-10-01" {
		t.Errorf("got APIVersion=%q, want %q", resolved.APIVersion, "2024-10-01")
	}
}

func TestConfig_LoadAndSave_APIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		CurrentEnvironment: "test_env_apikey_only",
		Environments: map[string]config.EnvironmentConfig{
			"test_env_apikey_only": {
				APIKey: "test-api-key",
			},
		},
	}

	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	sandboxEnv, ok := loaded.Environments["test_env_apikey_only"]
	if !ok {
		t.Fatal("expected test_env_apikey_only environment in loaded config")
	}
	if sandboxEnv.APIKey != "test-api-key" {
		t.Errorf("APIKey = %q, want %q", sandboxEnv.APIKey, "test-api-key")
	}
	if sandboxEnv.AccessToken != "" {
		t.Errorf("AccessToken = %q, want empty when only an API key is stored", sandboxEnv.AccessToken)
	}
}

// The config file can hold plaintext credentials, so Save must always leave
// it owner-only, including when replacing a file created with looser bits.
func TestSave_FilePermissions(t *testing.T) {
	for name, existing := range map[string]os.FileMode{"new file": 0, "existing 0644 file": 0o644} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if existing != 0 {
				if err := os.WriteFile(path, nil, existing); err != nil {
					t.Fatal(err)
				}
			}
			if err := config.Save(&config.Config{Environments: map[string]config.EnvironmentConfig{}}, path); err != nil {
				t.Fatalf("Save: %v", err)
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := fi.Mode().Perm(); got != 0o600 {
				t.Errorf("config file mode = %o, want 600", got)
			}
		})
	}
}
