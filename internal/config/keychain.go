package config

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const keychainService = "paddle-cli"

// securityMaxLine is just under the 4096-byte line buffer of `security -i`;
// anything longer is split and the tail is parsed as further commands.
const securityMaxLine = 4000

// securityNotFound is the exit status `security` uses for a missing item.
const securityNotFound = 44

// securityTimeout bounds each `security` call. A locked keychain on a GUI
// session prompts for the password and would otherwise block indefinitely;
// this still leaves a person time to type it.
const securityTimeout = 30 * time.Second

var validAccount = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var (
	errUnsupportedKeychainPath = errors.New("unsupported default keychain path")
	errNotFound                = errors.New("not found")
	errCredentialsTooLarge     = errors.New("credentials too large for the keychain")
	errSecurityRejected        = errors.New("security rejected the add-generic-password command")
	errTimedOut                = errors.New("timed out")
	errInvalidAccount          = errors.New("can't be used as a keychain account")
)

// macOSKeychain stores credentials in the login keychain via /usr/bin/security.
// Going through the system binary (rather than Security.framework via cgo)
// means the item's ACL trusts `security`, so rebuilding paddle doesn't
// trigger "allow access" prompts.
type macOSKeychain struct {
	bin     string
	service string
	timeout time.Duration

	readyOnce sync.Once
	readyErr  error
	// keychain is the default keychain's path, passed to every command so
	// reads, writes and deletes all target the same keychain even when it
	// isn't on the search list.
	keychain string
}

func newMacOSKeychain(service string) *macOSKeychain {
	return &macOSKeychain{bin: "/usr/bin/security", service: service, timeout: securityTimeout}
}

// NewMacOSKeychain returns a store backed by the login keychain under the
// given service name. Tests use a throwaway service to avoid real items.
func NewMacOSKeychain(service string) CredentialStore {
	return newMacOSKeychain(service)
}

// ready fails when there's no usable default keychain. Without this check,
// add-generic-password pops a blocking "keychain cannot be found" dialog.
func (k *macOSKeychain) ready() error {
	k.readyOnce.Do(func() {
		out, err := k.run("", "default-keychain")
		if err != nil {
			k.readyErr = fmt.Errorf("no default keychain: %w", err)
			return
		}
		path := strings.Trim(strings.TrimSpace(string(out)), `"`)
		// The path is %q-quoted into the `security -i` line, which doesn't
		// understand Go's escapes (same reasoning as validateAccount).
		if strings.ContainsAny(path, "\"\\\n") {
			k.readyErr = fmt.Errorf("%w %q", errUnsupportedKeychainPath, path)
			return
		}
		if _, err := os.Stat(path); err != nil {
			k.readyErr = fmt.Errorf("default keychain %s %w", path, errNotFound)
			return
		}
		k.keychain = path
	})
	return k.readyErr
}

// Get reads and decodes the item for account, returning ErrCredentialNotFound if none exists.
func (k *macOSKeychain) Get(account string) (Secrets, error) {
	if err := validateAccount(account); err != nil {
		return Secrets{}, err
	}
	if err := k.ready(); err != nil {
		return Secrets{}, err
	}
	out, err := k.run("", "find-generic-password", "-s", k.service, "-a", account, "-w", k.keychain)
	if err != nil {
		return Secrets{}, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return Secrets{}, fmt.Errorf("decoding keychain item: %w", err)
	}
	var s Secrets
	if err := json.Unmarshal(raw, &s); err != nil {
		return Secrets{}, fmt.Errorf("decoding keychain item: %w", err)
	}
	return s, nil
}

// Set writes via `security -i` on stdin so the secret never appears in argv.
func (k *macOSKeychain) Set(account string, s Secrets) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	raw, err := json.Marshal(s) //nolint:gosec // storing the secrets in the keychain is the point
	if err != nil {
		return fmt.Errorf("encoding keychain item: %w", err)
	}
	if err := k.ready(); err != nil {
		return err
	}
	line := fmt.Sprintf("add-generic-password -U -s %q -a %q -w %s %q\n",
		k.service, account, base64.StdEncoding.EncodeToString(raw), k.keychain)
	if len(line) > securityMaxLine {
		return fmt.Errorf("%w (%d-byte command, limit %d)", errCredentialsTooLarge, len(line), securityMaxLine)
	}
	if _, err := k.run(line, "-i"); err != nil {
		// An overlong line's tail is echoed back as an "unknown command",
		// which would put part of the secret in the error.
		if strings.Contains(err.Error(), "unknown command") {
			return errSecurityRejected
		}
		return err
	}
	return nil
}

// Delete removes the item for account, returning ErrCredentialNotFound if none exists.
func (k *macOSKeychain) Delete(account string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if err := k.ready(); err != nil {
		return err
	}
	_, err := k.run("", "delete-generic-password", "-s", k.service, "-a", account, k.keychain)
	return err
}

// run executes security with a timeout, mapping its not-found exit status to
// ErrCredentialNotFound and attaching its stderr to other failures.
func (k *macOSKeychain) run(stdin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, k.bin, args...) //nolint:gosec // bin is /usr/bin/security; args are fixed or validated
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("%w after %s waiting for the macOS Keychain (is it locked?)", errTimedOut, k.timeout)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == securityNotFound {
		return nil, ErrCredentialNotFound
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err //nolint:wrapcheck // callers add keychain context; same shape as the stderr branch
	}
	return stdout.Bytes(), nil
}

// validateAccount keeps account names to characters `security -i` tokenises
// the same way Go's %q quotes them.
func validateAccount(account string) error {
	if !validAccount.MatchString(account) {
		return fmt.Errorf("environment name %q %w", account, errInvalidAccount)
	}
	return nil
}
