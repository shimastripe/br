package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shimastripe/br/internal/config"
	"github.com/shimastripe/br/internal/hostutil"
	"github.com/zalando/go-keyring"
)

const keychainService = "br"

var ErrNoToken = errors.New("no token configured")

type Source string

const (
	SourceEnv      Source = "env"
	SourceKeychain Source = "keychain"
	SourceFile     Source = "file"
)

type KeyringBackend interface {
	Set(service string, user string, password string) error
	Get(service string, user string) (string, error)
	Delete(service string, user string) error
}

type defaultKeyring struct{}

func (defaultKeyring) Set(service string, user string, password string) error {
	return keyring.Set(service, user, password)
}

func (defaultKeyring) Get(service string, user string) (string, error) {
	return keyring.Get(service, user)
}

func (defaultKeyring) Delete(service string, user string) error {
	return keyring.Delete(service, user)
}

type Store struct {
	keyring KeyringBackend
	hosts   *config.HostsManager
}

type StoreOptions struct {
	Keyring   KeyringBackend
	HostsPath string
}

func NewStore() (*Store, error) {
	return NewStoreWithOptions(StoreOptions{})
}

func NewStoreWithOptions(opts StoreOptions) (*Store, error) {
	hostsMgr, err := config.NewHostsManager(opts.HostsPath)
	if err != nil {
		return nil, err
	}
	backend := opts.Keyring
	if backend == nil {
		backend = defaultKeyring{}
	}
	return &Store{keyring: backend, hosts: hostsMgr}, nil
}

func (s *Store) SaveToken(hostInput string, token string, insecure bool) error {
	_, host, err := hostutil.Normalize(hostInput)
	if err != nil {
		return err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token is empty")
	}

	if insecure {
		if err := s.hosts.SetToken(host, token); err != nil {
			return fmt.Errorf("save token to config file: %w", err)
		}
		_ = s.keyring.Delete(keychainService, host)
		return nil
	}

	if err := s.keyring.Set(keychainService, host, token); err != nil {
		return fmt.Errorf("save token to keychain: %w", err)
	}

	if err := s.hosts.DeleteToken(host); err != nil {
		return fmt.Errorf("remove plaintext token after keychain save: %w", err)
	}

	return nil
}

func (s *Store) ResolveToken(hostInput string) (string, error) {
	token, _, err := s.ResolveTokenWithSource(hostInput)
	return token, err
}

func (s *Store) ResolveTokenWithSource(hostInput string) (string, Source, error) {
	if env := strings.TrimSpace(os.Getenv("BITRISE_TOKEN")); env != "" {
		return env, SourceEnv, nil
	}

	_, host, err := hostutil.Normalize(hostInput)
	if err != nil {
		return "", "", err
	}

	token, err := s.keyring.Get(keychainService, host)
	if err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token), SourceKeychain, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		// fall back to file storage for usability even if keychain lookup failed
	}

	fileToken, ok, err := s.hosts.GetToken(host)
	if err != nil {
		return "", "", fmt.Errorf("read token from config: %w", err)
	}
	if ok && strings.TrimSpace(fileToken) != "" {
		return strings.TrimSpace(fileToken), SourceFile, nil
	}

	return "", "", ErrNoToken
}

func (s *Store) DeleteToken(hostInput string) error {
	_, host, err := hostutil.Normalize(hostInput)
	if err != nil {
		return err
	}

	if err := s.keyring.Delete(keychainService, host); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("delete token from keychain: %w", err)
	}
	if err := s.hosts.DeleteToken(host); err != nil {
		return fmt.Errorf("delete token from config: %w", err)
	}

	return nil
}

func (s *Store) HostsPath() string {
	if s.hosts == nil {
		return ""
	}
	return s.hosts.Path()
}
