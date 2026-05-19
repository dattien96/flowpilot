package runner

import (
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

type SecretStore interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
}

type keyringSecretStore struct {
	service string
}

func newDefaultSecretStore() SecretStore {
	return &keyringSecretStore{service: "flowpilot-runner"}
}

func (s *keyringSecretStore) Set(key, value string) error {
	return keyring.Set(s.service, key, value)
}

func (s *keyringSecretStore) Get(key string) (string, error) {
	return keyring.Get(s.service, key)
}

func (s *keyringSecretStore) Delete(key string) error {
	err := keyring.Delete(s.service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

func normalizeSecretKey(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalized = append(normalized, part)
	}
	return strings.Join(normalized, ":")
}
