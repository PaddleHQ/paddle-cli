package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
)

// secretsInKeychain is the EnvironmentConfig.SecretsLocation value marking an
// environment whose secrets live in the OS keychain.
const secretsInKeychain = "keychain"

// Secrets are the EnvironmentConfig fields kept out of the config file when an
// OS credential store is available. The JSON tags are the on-keychain format:
// renaming one silently empties existing users' stored credentials.
type Secrets struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
}

func (s Secrets) empty() bool { return s == Secrets{} }

// CredentialStore persists one Secrets value per account (environment name).
type CredentialStore interface {
	Get(account string) (Secrets, error)
	Set(account string, s Secrets) error
	Delete(account string) error
}

// ErrCredentialNotFound is returned by CredentialStore.Get for an unknown account.
var ErrCredentialNotFound = errors.New("credential not found")

// keychainItem is what Load learnt about one environment's keychain item:
// its secrets, or why they couldn't be read.
type keychainItem struct {
	secrets Secrets
	readErr error
}

var credentialStore = defaultCredentialStore()

// defaultCredentialStore is nil under `go test` so no test can reach the
// developer's real keychain unless it installs a store explicitly.
func defaultCredentialStore() CredentialStore {
	if runtime.GOOS == "darwin" && !testing.Testing() {
		return newMacOSKeychain(keychainService)
	}
	return nil
}

// SetCredentialStore replaces the process-wide store (nil disables it) and
// returns a function restoring the previous one. Intended for tests.
func SetCredentialStore(s CredentialStore) (restore func()) {
	prev := credentialStore
	credentialStore = s
	return func() { credentialStore = prev }
}

var (
	errKeychainDisabled    = errors.New("keychain disabled by PADDLE_NO_KEYCHAIN")
	errKeychainUnsupported = errors.New("OS keychain not supported")
	errKeychainItemMissing = errors.New("credentials are missing from the macOS Keychain; run 'paddle login' again")
)

// keychainHint is appended to errors that leave a keychain-stored credential
// unchanged, so the user knows how to get unstuck.
const keychainHint = "unlock it with 'security unlock-keychain' and retry"

// usableStore returns the store to read and write, or a non-nil error saying
// why there isn't one. PADDLE_NO_KEYCHAIN=1 disables reads as well as writes
// so it can keep the CLI away from a locked or prompting keychain.
func usableStore() (CredentialStore, error) {
	if os.Getenv("PADDLE_NO_KEYCHAIN") == "1" {
		return nil, errKeychainDisabled
	}
	if credentialStore == nil {
		return nil, fmt.Errorf("%w on %s", errKeychainUnsupported, runtime.GOOS)
	}
	return credentialStore, nil
}

func secretsOf(e EnvironmentConfig) Secrets {
	return Secrets{AccessToken: e.AccessToken, RefreshToken: e.RefreshToken, APIKey: e.APIKey}
}

func withSecrets(e EnvironmentConfig, s Secrets) EnvironmentConfig {
	e.AccessToken, e.RefreshToken, e.APIKey = s.AccessToken, s.RefreshToken, s.APIKey
	return e
}

// hydrateSecrets fills in secrets for environments whose credentials live in
// the keychain. A read failure is recorded rather than returned so commands
// that don't need credentials still work.
func (c *Config) hydrateSecrets() {
	c.keychainItems = map[string]keychainItem{}
	for env, e := range c.Environments {
		if e.SecretsLocation != secretsInKeychain {
			continue
		}
		store, err := usableStore()
		if err != nil {
			c.keychainItems[env] = keychainItem{readErr: fmt.Errorf("%s credentials are in the macOS Keychain, which can't be used: %w", env, err)}
			continue
		}
		s, err := store.Get(env)
		switch {
		case errors.Is(err, ErrCredentialNotFound):
			c.keychainItems[env] = keychainItem{readErr: fmt.Errorf("%s %w", env, errKeychainItemMissing)}
		case err != nil:
			c.keychainItems[env] = keychainItem{readErr: fmt.Errorf("reading %s credentials from the macOS Keychain: %w", env, err)}
		default:
			c.keychainItems[env] = keychainItem{secrets: s}
			c.Environments[env] = withSecrets(e, s)
		}
	}
}

// syncCredentials writes and deletes keychain items so they match
// c.Environments, and returns the environments as they should be written to
// disk plus a commit func that updates c once that write succeeds.
//
// Any keychain failure that would otherwise lose track of an item, or move a
// keychain-stored credential into plaintext, aborts before the file is
// written, so an error from Save means the config file is unchanged. Keychain
// writes already made for other environments are kept; the next Save
// reconciles them.
func (c *Config) syncCredentials() (onDisk map[string]EnvironmentConfig, commit func(), err error) {
	store, unavailable := usableStore()
	envs := make(map[string]EnvironmentConfig, len(c.Environments))
	onDisk = make(map[string]EnvironmentConfig, len(c.Environments))
	plaintextReasons := map[string]error{}

	for env, e := range c.Environments {
		s := secretsOf(e)
		// Couldn't read it, and nothing replaced it: leave the item alone.
		if c.keychainItems[env].readErr != nil && s.empty() {
			envs[env], onDisk[env] = e, e
			continue
		}
		inKeychain, err := c.storeSecrets(store, unavailable, env, s, plaintextReasons)
		if err != nil {
			return nil, nil, err
		}
		e.SecretsLocation = ""
		onDisk[env] = e
		if inKeychain {
			e.SecretsLocation = secretsInKeychain
			onDisk[env] = withSecrets(e, Secrets{})
		}
		envs[env] = e
	}

	if err := c.deleteStaleItems(store, envs); err != nil {
		return nil, nil, err
	}
	return onDisk, c.commitFunc(envs, plaintextReasons), nil
}

// storeSecrets writes env's secrets to store unless the keychain already holds
// them, and reports whether they now live in the keychain. When they don't, it
// records why in plaintextReasons; an error means saving must abort.
func (c *Config) storeSecrets(
	store CredentialStore, unavailable error, env string, s Secrets, plaintextReasons map[string]error,
) (bool, error) {
	switch {
	case s.empty():
		// Nothing secret to store; the environment is written as-is.
		return false, nil
	case store == nil:
		plaintextReasons[env] = unavailable
		return false, nil
	}
	item, wasInKeychain := c.keychainItems[env]
	if wasInKeychain && item.readErr == nil && item.secrets == s {
		return true, nil
	}
	if err := store.Set(env, s); err != nil {
		// Never downgrade a keychain-stored credential to plaintext.
		if wasInKeychain {
			return false, fmt.Errorf("saving %s credentials to the macOS Keychain (%s): %w", env, keychainHint, err)
		}
		plaintextReasons[env] = fmt.Errorf("macOS Keychain unavailable: %w", err)
		return false, nil
	}
	return true, nil
}

// deleteStaleItems removes keychain items for environments whose secrets no
// longer live there.
func (c *Config) deleteStaleItems(store CredentialStore, envs map[string]EnvironmentConfig) error {
	// Without a usable store (e.g. PADDLE_NO_KEYCHAIN) items are left as is; a
	// later keychain save for the environment overwrites them.
	if store == nil {
		return nil
	}
	for env := range c.keychainItems {
		if envs[env].SecretsLocation == secretsInKeychain {
			continue
		}
		if err := store.Delete(env); err != nil && !errors.Is(err, ErrCredentialNotFound) {
			return fmt.Errorf("removing %s credentials from the macOS Keychain (%s): %w", env, keychainHint, err)
		}
	}
	return nil
}

// commitFunc returns the func that records a successful save in c.
func (c *Config) commitFunc(envs map[string]EnvironmentConfig, plaintextReasons map[string]error) func() {
	return func() {
		items := map[string]keychainItem{}
		for env, e := range envs {
			c.Environments[env] = e
			if e.SecretsLocation != secretsInKeychain {
				continue
			}
			if prev := c.keychainItems[env]; prev.readErr != nil && secretsOf(e).empty() {
				items[env] = prev // still unreadable; keep reporting why
			} else {
				items[env] = keychainItem{secrets: secretsOf(e)}
			}
		}
		c.keychainItems = items
		c.plaintextReasons = plaintextReasons
	}
}

// StoredInKeychain reports whether env's credentials were last saved to the
// OS keychain and, if not, why they went to the config file instead.
func (c *Config) StoredInKeychain(env string) (bool, error) {
	if c.Environments[env].SecretsLocation == secretsInKeychain {
		return true, nil
	}
	if err := c.plaintextReasons[env]; err != nil {
		return false, err
	}
	_, err := usableStore()
	return false, err
}

// CredentialReadError returns why env's keychain credentials couldn't be read
// at Load time, or nil.
func (c *Config) CredentialReadError(env string) error {
	return c.keychainItems[env].readErr
}
