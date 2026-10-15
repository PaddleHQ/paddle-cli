package configtest

import (
	"testing"

	"github.com/PaddleHQ/paddle-cli/internal/config"
)

// MemoryStore is an in-memory config.CredentialStore that records calls.
type MemoryStore struct {
	Items     map[string]config.Secrets
	Sets      []string
	Deletes   []string
	SetErr    error
	GetErr    error
	DeleteErr error
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{Items: map[string]config.Secrets{}}
}

// Install makes s the process-wide credential store until the test ends.
// The store is package-level state, so tests using it must not run in parallel.
func (s *MemoryStore) Install(t testing.TB) *MemoryStore {
	t.Cleanup(config.SetCredentialStore(s))
	return s
}

// Get returns GetErr if set, otherwise the stored item or config.ErrCredentialNotFound.
func (s *MemoryStore) Get(account string) (config.Secrets, error) {
	if s.GetErr != nil {
		return config.Secrets{}, s.GetErr
	}
	sec, ok := s.Items[account]
	if !ok {
		return config.Secrets{}, config.ErrCredentialNotFound
	}
	return sec, nil
}

// Set returns SetErr if set, otherwise stores sec and records account in Sets.
func (s *MemoryStore) Set(account string, sec config.Secrets) error {
	if s.SetErr != nil {
		return s.SetErr
	}
	s.Sets = append(s.Sets, account)
	s.Items[account] = sec
	return nil
}

// Delete returns DeleteErr if set, otherwise removes the item and records account in Deletes.
func (s *MemoryStore) Delete(account string) error {
	if s.DeleteErr != nil {
		return s.DeleteErr
	}
	s.Deletes = append(s.Deletes, account)
	delete(s.Items, account)
	return nil
}
