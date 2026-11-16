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

// FileStore keeps tokens in the accounts of Config. Set and Delete change
// Config only; Save writes the change to the config file.
type FileStore struct {
	Config *Config
}

// Get returns the token of user on host.
func (s FileStore) Get(host, user string) (string, error) {
	hc := s.Config.Hosts[host]
	if hc == nil {
		return "", ErrTokenNotFound
	}
	account := hc.migrated().Users[user]
	if account == nil || account.Token == "" {
		return "", ErrTokenNotFound
	}
	return account.Token, nil
}

// Set stores token as the token of user on host, creating the host entry
// and the account when they are missing. It does not change the host's
// active account.
func (s FileStore) Set(host, user, token string) error {
	hc := s.Config.Hosts[host]
	if hc == nil {
		hc = &HostConfig{}
	}
	hc = hc.migrated()
	if hc.Users == nil {
		hc.Users = map[string]*UserConfig{}
	}
	account := hc.Users[user]
	if account == nil {
		account = &UserConfig{}
		hc.Users[user] = account
	}
	account.Token = token
	s.Config.Hosts[host] = hc
	return nil
}

// Delete clears the token of user on host and keeps the account.
func (s FileStore) Delete(host, user string) error {
	if _, err := s.Get(host, user); err != nil {
		return err
	}
	hc := s.Config.Hosts[host].migrated()
	hc.Users[user].Token = ""
	s.Config.Hosts[host] = hc
	return nil
}

// LookupToken returns the stored token of the active account of host and
// where it came from, as LookupUserToken does.
func LookupToken(cfg *Config, host string, keyring TokenStore) (token, source string, err error) {
	hc := cfg.Hosts[host]
	switch {
	case hc == nil:
		return "", "", ErrTokenNotFound
	case hc.User == "" && hc.Token != "":
		// The old shape without a login has no account to move the token
		// to, so it stays where it is.
		return hc.Token, SourceFile, nil
	case hc.User == "":
		return "", "", ErrTokenNotFound
	}
	return LookupUserToken(cfg, host, hc.User, keyring)
}

// LookupUserToken returns the stored token of the account user on host and
// where it came from: the config file first, so tokens stored there before
// the keyring keep working, then keyring when it is not nil. It returns
// ErrTokenNotFound when the host has no such account or neither holds a
// token.
func LookupUserToken(cfg *Config, host, user string, keyring TokenStore) (token, source string, err error) {
	hc := cfg.Hosts[host]
	if hc == nil {
		return "", "", ErrTokenNotFound
	}
	account := hc.migrated().Users[user]
	if account == nil {
		return "", "", ErrTokenNotFound
	}
	if account.Token != "" {
		return account.Token, SourceFile, nil
	}
	if keyring == nil {
		return "", "", ErrTokenNotFound
	}
	token, err = keyring.Get(host, user)
	if err != nil {
		return "", "", err
	}
	return token, SourceKeyring, nil
}
