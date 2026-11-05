package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// TokenStore reads, writes and deletes the token of an account (the login)
// on a host.
type TokenStore interface {
	Get(host, user string) (string, error)
	Set(host, user, token string) error
	Delete(host, user string) error
}

// ErrTokenNotFound is returned when a store holds no token for the host and
// account.
var ErrTokenNotFound = errors.New("token not found")

// ErrKeyringTimeout is returned when the OS keyring does not answer in time,
// as a locked or absent Secret Service can hang instead of failing.
var ErrKeyringTimeout = errors.New("timed out waiting for the OS keyring")

// The token sources LookupToken reports.
const (
	SourceKeyring = "keyring"
	SourceFile    = "config file"
)

// keyringTimeout bounds every keyring call, as gh's internal/keyring does.
const keyringTimeout = 3 * time.Second

// keyringBackend is the part of go-keyring KeyringStore calls; tests swap it
// for one that never answers.
type keyringBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type goKeyring struct{}

func (goKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }
func (goKeyring) Set(service, user, password string) error { return keyring.Set(service, user, password) }
func (goKeyring) Delete(service, user string) error        { return keyring.Delete(service, user) }

// KeyringStore keeps tokens in the OS keyring (Keychain, Windows Credential
// Manager, Secret Service) under the service copia-cli:<host> with the login
// as the account.
type KeyringStore struct {
	timeout time.Duration
	backend keyringBackend
}

// NewKeyringStore returns the store on the OS keyring.
func NewKeyringStore() *KeyringStore {
	return &KeyringStore{timeout: keyringTimeout, backend: goKeyring{}}
}

func keyringService(host string) string {
	return "copia-cli:" + host
}

// Get returns the token of user on host.
func (k *KeyringStore) Get(host, user string) (string, error) {
	var token string
	err := k.call(func() error {
		var err error
		token, err = k.backend.Get(keyringService(host), user)
		return err
	})
	if err != nil {
		return "", keyringError(err, "")
	}
	return token, nil
}

// Set stores token as the token of user on host.
func (k *KeyringStore) Set(host, user, token string) error {
	err := k.call(func() error {
		return k.backend.Set(keyringService(host), user, token)
	})
	return keyringError(err, token)
}

// Delete removes the token of user on host.
func (k *KeyringStore) Delete(host, user string) error {
	err := k.call(func() error {
		return k.backend.Delete(keyringService(host), user)
	})
	return keyringError(err, "")
}

// call runs fn and gives up after the store's timeout. The goroutine of a
// call that timed out is left behind; the process ends soon after.
func (k *KeyringStore) call(fn func() error) error {
	ch := make(chan error, 1)
	go func() { ch <- fn() }()
	select {
	case err := <-ch:
		return err
	case <-time.After(k.timeout):
		return ErrKeyringTimeout
	}
}

// keyringError maps go-keyring's not-found error to ErrTokenNotFound and
// masks token in any other error, so a backend message that echoes the
// secret never reaches the terminal.
func keyringError(err error, token string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrTokenNotFound
	case errors.Is(err, ErrKeyringTimeout):
		return err
	}
	msg := err.Error()
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "****")
	}
	return fmt.Errorf("OS keyring: %s", msg)
}

// FileStore keeps tokens in the host entries of Config. Set and Delete change
// Config only; Save writes the change to the config file.
type FileStore struct {
	Config *Config
}

// Get returns the token of user on host.
func (s FileStore) Get(host, user string) (string, error) {
	hc := s.Config.Hosts[host]
	if hc == nil || hc.User != user || hc.Token == "" {
		return "", ErrTokenNotFound
	}
	return hc.Token, nil
}

// Set stores token as the token of user on host, creating the host entry
// when it is missing.
func (s FileStore) Set(host, user, token string) error {
	hc := s.Config.Hosts[host]
	if hc == nil {
		hc = &HostConfig{}
		s.Config.Hosts[host] = hc
	}
	hc.User = user
	hc.Token = token
	return nil
}

// Delete clears the token of user on host and keeps the host entry.
func (s FileStore) Delete(host, user string) error {
	if _, err := s.Get(host, user); err != nil {
		return err
	}
	s.Config.Hosts[host].Token = ""
	return nil
}

// LookupToken returns the stored token of host's account and where it came
// from: the config file first, so tokens stored there before the keyring
// keep working, then keyring when it is not nil. It returns ErrTokenNotFound
// when neither holds one.
func LookupToken(cfg *Config, host string, keyring TokenStore) (token, source string, err error) {
	hc := cfg.Hosts[host]
	if hc == nil {
		return "", "", ErrTokenNotFound
	}
	if hc.Token != "" {
		return hc.Token, SourceFile, nil
	}
	if keyring == nil || hc.User == "" {
		return "", "", ErrTokenNotFound
	}
	token, err = keyring.Get(host, hc.User)
	if err != nil {
		return "", "", err
	}
	return token, SourceKeyring, nil
}
