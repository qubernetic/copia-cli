package config

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// TestMain puts go-keyring on its in-memory mock, so no test of this package
// reaches the OS keyring of the machine it runs on.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

func TestKeyringStore_SetGetDelete(t *testing.T) {
	keyring.MockInit()
	store := NewKeyringStore()

	require.NoError(t, store.Set("app.copia.io", "john", "secret-1"))

	got, err := store.Get("app.copia.io", "john")
	require.NoError(t, err)
	assert.Equal(t, "secret-1", got)

	// The service names the host, so another host's entry is separate.
	_, err = store.Get("other.example.com", "john")
	assert.ErrorIs(t, err, ErrTokenNotFound)

	require.NoError(t, store.Delete("app.copia.io", "john"))
	_, err = store.Get("app.copia.io", "john")
	assert.ErrorIs(t, err, ErrTokenNotFound)

	assert.ErrorIs(t, store.Delete("app.copia.io", "john"), ErrTokenNotFound)
}

func TestKeyringStore_ServiceName(t *testing.T) {
	keyring.MockInit()
	require.NoError(t, NewKeyringStore().Set("app.copia.io", "john", "secret-1"))

	got, err := keyring.Get("copia-cli:app.copia.io", "john")
	require.NoError(t, err)
	assert.Equal(t, "secret-1", got)
}

func TestKeyringStore_Unavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service"))
	t.Cleanup(keyring.MockInit)
	store := NewKeyringStore()

	err := store.Set("app.copia.io", "john", "secret-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no secret service")

	_, err = store.Get("app.copia.io", "john")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTokenNotFound)

	assert.Error(t, store.Delete("app.copia.io", "john"))
}

func TestKeyringStore_ErrorNeverHoldsToken(t *testing.T) {
	keyring.MockInitWithError(errors.New("cannot store secret-1"))
	t.Cleanup(keyring.MockInit)

	err := NewKeyringStore().Set("app.copia.io", "john", "secret-1")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-1")
}

// blockingBackend never answers, like a locked Secret Service waiting for an
// unlock prompt nobody sees.
type blockingBackend struct{ release chan struct{} }

func (b blockingBackend) Get(string, string) (string, error) { <-b.release; return "", nil }
func (b blockingBackend) Set(string, string, string) error   { <-b.release; return nil }
func (b blockingBackend) Delete(string, string) error        { <-b.release; return nil }

func TestKeyringStore_Timeout(t *testing.T) {
	backend := blockingBackend{release: make(chan struct{})}
	t.Cleanup(func() { close(backend.release) })
	store := &KeyringStore{timeout: 10 * time.Millisecond, backend: backend}

	err := store.Set("app.copia.io", "john", "secret-1")
	assert.ErrorIs(t, err, ErrKeyringTimeout)

	_, err = store.Get("app.copia.io", "john")
	assert.ErrorIs(t, err, ErrKeyringTimeout)

	assert.ErrorIs(t, store.Delete("app.copia.io", "john"), ErrKeyringTimeout)
}

func TestFileStore_SetGetDelete(t *testing.T) {
	cfg := &Config{Hosts: map[string]*HostConfig{}}
	store := FileStore{Config: cfg}

	_, err := store.Get("app.copia.io", "john")
	assert.ErrorIs(t, err, ErrTokenNotFound)

	require.NoError(t, store.Set("app.copia.io", "john", "secret-1"))
	require.NoError(t, store.Set("app.copia.io", "jane", "secret-2"))
	assert.Equal(t, &HostConfig{Users: map[string]*UserConfig{"john": {Token: "secret-1"}, "jane": {Token: "secret-2"}}}, cfg.Hosts["app.copia.io"])

	got, err := store.Get("app.copia.io", "john")
	require.NoError(t, err)
	assert.Equal(t, "secret-1", got)

	_, err = store.Get("app.copia.io", "joe")
	assert.ErrorIs(t, err, ErrTokenNotFound)

	require.NoError(t, store.Delete("app.copia.io", "john"))
	assert.Equal(t, &HostConfig{Users: map[string]*UserConfig{"john": {}, "jane": {Token: "secret-2"}}}, cfg.Hosts["app.copia.io"])
	assert.ErrorIs(t, store.Delete("app.copia.io", "john"), ErrTokenNotFound)
}

func TestFileStore_OldShape(t *testing.T) {
	cfg := &Config{Hosts: map[string]*HostConfig{"app.copia.io": {Token: "secret-1", User: "john"}}}
	store := FileStore{Config: cfg}

	got, err := store.Get("app.copia.io", "john")
	require.NoError(t, err)
	assert.Equal(t, "secret-1", got)

	require.NoError(t, store.Set("app.copia.io", "jane", "secret-2"))
	assert.Equal(t, &HostConfig{User: "john", Users: map[string]*UserConfig{"john": {Token: "secret-1"}, "jane": {Token: "secret-2"}}}, cfg.Hosts["app.copia.io"])
}

func TestFileStore_SetKeepsUnknownKeys(t *testing.T) {
	cfg := &Config{Hosts: map[string]*HostConfig{
		"app.copia.io": {User: "john", Extra: map[string]any{"git_protocol": "ssh"}},
	}}

	require.NoError(t, FileStore{Config: cfg}.Set("app.copia.io", "john", "secret-1"))
	assert.Equal(t, map[string]any{"git_protocol": "ssh"}, cfg.Hosts["app.copia.io"].Extra)
}

func TestLookupToken(t *testing.T) {
	tests := []struct {
		name       string
		hosts      map[string]*HostConfig
		keyring    map[string]string // user -> token on app.copia.io
		noKeyring  bool
		wantToken  string
		wantSource string
		wantErr    error
	}{
		{
			name:       "config file token wins over keyring",
			hosts:      map[string]*HostConfig{"app.copia.io": {Token: "file-token", User: "john"}},
			keyring:    map[string]string{"john": "keyring-token"},
			wantToken:  "file-token",
			wantSource: SourceFile,
		},
		{
			name:       "keyring token",
			hosts:      map[string]*HostConfig{"app.copia.io": {User: "john"}},
			keyring:    map[string]string{"john": "keyring-token"},
			wantToken:  "keyring-token",
			wantSource: SourceKeyring,
		},
		{
			name: "keyring token of the active account",
			hosts: map[string]*HostConfig{"app.copia.io": {User: "jane", Users: map[string]*UserConfig{
				"john": {Token: "file-token"}, "jane": {},
			}}},
			keyring:    map[string]string{"john": "keyring-token-1", "jane": "keyring-token-2"},
			wantToken:  "keyring-token-2",
			wantSource: SourceKeyring,
		},
		{
			name: "config file token of the active account",
			hosts: map[string]*HostConfig{"app.copia.io": {User: "john", Users: map[string]*UserConfig{
				"john": {Token: "file-token"}, "jane": {},
			}}},
			keyring:    map[string]string{"jane": "keyring-token"},
			wantToken:  "file-token",
			wantSource: SourceFile,
		},
		{
			name:    "no active account",
			hosts:   map[string]*HostConfig{"app.copia.io": {Users: map[string]*UserConfig{"john": {Token: "file-token"}}}},
			keyring: map[string]string{"john": "keyring-token"},
			wantErr: ErrTokenNotFound,
		},
		{
			name:    "no token anywhere",
			hosts:   map[string]*HostConfig{"app.copia.io": {User: "john"}},
			wantErr: ErrTokenNotFound,
		},
		{
			name:    "host not configured",
			hosts:   map[string]*HostConfig{},
			keyring: map[string]string{"john": "keyring-token"},
			wantErr: ErrTokenNotFound,
		},
		{
			name:      "no keyring store",
			hosts:     map[string]*HostConfig{"app.copia.io": {User: "john"}},
			keyring:   map[string]string{"john": "keyring-token"},
			noKeyring: true,
			wantErr:   ErrTokenNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			for user, token := range tt.keyring {
				require.NoError(t, keyring.Set("copia-cli:app.copia.io", user, token))
			}
			var store TokenStore = NewKeyringStore()
			if tt.noKeyring {
				store = nil
			}

			token, source, err := LookupToken(&Config{Hosts: tt.hosts}, "app.copia.io", store)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
			assert.Equal(t, tt.wantSource, source)
		})
	}
}

func TestLookupToken_KeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service"))
	t.Cleanup(keyring.MockInit)
	cfg := &Config{Hosts: map[string]*HostConfig{"app.copia.io": {User: "john"}}}

	_, _, err := LookupToken(cfg, "app.copia.io", NewKeyringStore())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTokenNotFound)
	assert.Contains(t, err.Error(), "no secret service")
}

func TestSave_KeyringLoginLeavesNoTokenKey(t *testing.T) {
	path := t.TempDir() + "/config.yml"
	cfg := &Config{Hosts: map[string]*HostConfig{"app.copia.io": {User: "john"}}}

	require.NoError(t, Save(path, cfg))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "token")
}

func TestLookupUserToken(t *testing.T) {
	keyring.MockInit()
	require.NoError(t, keyring.Set("copia-cli:app.copia.io", "jane", "keyring-token"))
	cfg := &Config{Hosts: map[string]*HostConfig{"app.copia.io": {User: "john", Users: map[string]*UserConfig{
		"john": {Token: "file-token"}, "jane": {},
	}}}}

	token, source, err := LookupUserToken(cfg, "app.copia.io", "jane", NewKeyringStore())
	require.NoError(t, err)
	assert.Equal(t, "keyring-token", token)
	assert.Equal(t, SourceKeyring, source)

	token, source, err = LookupUserToken(cfg, "app.copia.io", "john", NewKeyringStore())
	require.NoError(t, err)
	assert.Equal(t, "file-token", token)
	assert.Equal(t, SourceFile, source)

	_, _, err = LookupUserToken(cfg, "app.copia.io", "joe", NewKeyringStore())
	assert.ErrorIs(t, err, ErrTokenNotFound)
}

func TestKeyringStore_EntryPerAccount(t *testing.T) {
	keyring.MockInit()
	store := NewKeyringStore()
	require.NoError(t, store.Set("app.copia.io", "john", "secret-1"))
	require.NoError(t, store.Set("app.copia.io", "jane", "secret-2"))

	require.NoError(t, store.Delete("app.copia.io", "john"))
	got, err := keyring.Get("copia-cli:app.copia.io", "jane")
	require.NoError(t, err)
	assert.Equal(t, "secret-2", got)
	_, err = keyring.Get("copia-cli:app.copia.io", "john")
	assert.ErrorIs(t, err, keyring.ErrNotFound)
}
