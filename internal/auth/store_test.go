package auth

import (
	"path/filepath"
	"testing"

	"github.com/shimastripe/br/internal/hostutil"
	"github.com/zalando/go-keyring"
)

type memoryKeyring struct {
	data map[string]string
}

func newMemoryKeyring() *memoryKeyring {
	return &memoryKeyring{data: map[string]string{}}
}

func (m *memoryKeyring) key(service string, user string) string {
	return service + "|" + user
}

func (m *memoryKeyring) Set(service string, user string, password string) error {
	m.data[m.key(service, user)] = password
	return nil
}

func (m *memoryKeyring) Get(service string, user string) (string, error) {
	v, ok := m.data[m.key(service, user)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (m *memoryKeyring) Delete(service string, user string) error {
	delete(m.data, m.key(service, user))
	return nil
}

func newTestStore(t *testing.T) (*Store, *memoryKeyring) {
	t.Helper()
	kr := newMemoryKeyring()
	store, err := NewStoreWithOptions(StoreOptions{
		Keyring:   kr,
		HostsPath: filepath.Join(t.TempDir(), "hosts.yml"),
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	return store, kr
}

func TestResolveTokenPrefersEnv(t *testing.T) {
	store, kr := newTestStore(t)

	_, host, err := hostutil.Normalize("api.bitrise.io")
	if err != nil {
		t.Fatalf("normalize host: %v", err)
	}
	kr.Set(keychainService, host, "keychain-token")
	if err := store.hosts.SetToken(host, "file-token"); err != nil {
		t.Fatalf("set file token: %v", err)
	}

	t.Setenv("BITRISE_TOKEN", "env-token")

	token, source, err := store.ResolveTokenWithSource(host)
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "env-token" || source != SourceEnv {
		t.Fatalf("token/source = %q/%q; want env-token/%q", token, source, SourceEnv)
	}
}

func TestSaveTokenSecureUsesKeychain(t *testing.T) {
	store, kr := newTestStore(t)
	host := "api.bitrise.io"

	if err := store.hosts.SetToken(host, "old-file-token"); err != nil {
		t.Fatalf("set file token: %v", err)
	}

	if err := store.SaveToken(host, "secure-token", false); err != nil {
		t.Fatalf("save secure token: %v", err)
	}

	value, err := kr.Get(keychainService, host)
	if err != nil {
		t.Fatalf("read keychain token: %v", err)
	}
	if value != "secure-token" {
		t.Fatalf("keychain token = %q; want secure-token", value)
	}

	if _, ok, err := store.hosts.GetToken(host); err != nil {
		t.Fatalf("read file token: %v", err)
	} else if ok {
		t.Fatal("file token should be removed after secure save")
	}
}

func TestSaveTokenInsecureUsesFile(t *testing.T) {
	store, kr := newTestStore(t)
	host := "api.bitrise.io"

	if err := kr.Set(keychainService, host, "old-keychain-token"); err != nil {
		t.Fatalf("set keychain token: %v", err)
	}

	if err := store.SaveToken(host, "plain-token", true); err != nil {
		t.Fatalf("save insecure token: %v", err)
	}

	if _, err := kr.Get(keychainService, host); err == nil {
		t.Fatal("keychain token should be deleted after insecure save")
	}

	value, ok, err := store.hosts.GetToken(host)
	if err != nil {
		t.Fatalf("read file token: %v", err)
	}
	if !ok || value != "plain-token" {
		t.Fatalf("file token = %q (ok=%t); want plain-token", value, ok)
	}
}
