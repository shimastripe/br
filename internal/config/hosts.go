package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultHostsFile = "br/hosts.yml"

type HostEntry struct {
	Token string `yaml:"token,omitempty"`
}

type HostsConfig struct {
	Hosts map[string]HostEntry `yaml:"hosts"`
}

type HostsManager struct {
	path string
}

func DefaultHostsPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, defaultHostsFile), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}

	return filepath.Join(home, ".config", defaultHostsFile), nil
}

func NewHostsManager(path string) (*HostsManager, error) {
	if path == "" {
		var err error
		path, err = DefaultHostsPath()
		if err != nil {
			return nil, err
		}
	}
	return &HostsManager{path: path}, nil
}

func (m *HostsManager) Path() string {
	return m.path
}

func (m *HostsManager) Load() (*HostsConfig, error) {
	cfg := &HostsConfig{Hosts: map[string]HostEntry{}}
	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read hosts config: %w", err)
	}

	if len(data) == 0 {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse hosts config: %w", err)
	}

	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostEntry{}
	}

	return cfg, nil
}

func (m *HostsManager) Save(cfg *HostsConfig) error {
	if cfg == nil {
		cfg = &HostsConfig{Hosts: map[string]HostEntry{}}
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostEntry{}
	}

	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal hosts config: %w", err)
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write hosts config temp file: %w", err)
	}

	if err := os.Rename(tmp, m.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace hosts config: %w", err)
	}

	return nil
}

func (m *HostsManager) SetToken(host string, token string) error {
	cfg, err := m.Load()
	if err != nil {
		return err
	}

	entry := cfg.Hosts[host]
	entry.Token = token
	cfg.Hosts[host] = entry
	return m.Save(cfg)
}

func (m *HostsManager) GetToken(host string) (string, bool, error) {
	cfg, err := m.Load()
	if err != nil {
		return "", false, err
	}

	entry, ok := cfg.Hosts[host]
	if !ok || entry.Token == "" {
		return "", false, nil
	}
	return entry.Token, true, nil
}

func (m *HostsManager) DeleteToken(host string) error {
	cfg, err := m.Load()
	if err != nil {
		return err
	}
	entry, ok := cfg.Hosts[host]
	if !ok {
		return nil
	}
	entry.Token = ""
	cfg.Hosts[host] = entry
	return m.Save(cfg)
}
