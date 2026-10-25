package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const acmeStagingDirectory = "https://acme-staging-v02.api.letsencrypt.org/directory"

func newACMEManager(cfg *config.Config) (*autocert.Manager, error) {
	if !cfg.ACMEAcceptTOS {
		return nil, fmt.Errorf("ACME requires explicit acceptance of the CA terms of service")
	}
	if cfg.ACMEDomain == "" || strings.TrimSpace(cfg.ACMECacheDir) == "" {
		return nil, fmt.Errorf("ACME requires a domain and persistent cache directory")
	}
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" {
		return nil, fmt.Errorf("ACME cannot be combined with certificate and key files")
	}
	if cfg.TLSClientCAFile != "" {
		return nil, fmt.Errorf("ACME cannot be combined with client certificate authentication")
	}
	name, directory := "production", autocert.DefaultACMEDirectory
	if cfg.ACMEStaging {
		name, directory = "staging", acmeStagingDirectory
	}
	cacheDir := filepath.Join(cfg.ACMECacheDir, name)
	if err := prepareACMECache(cacheDir); err != nil {
		return nil, fmt.Errorf("prepare ACME cache: %w", err)
	}
	return &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.ACMEDomain),
		Cache:      autocert.DirCache(cacheDir),
		Client:     &acme.Client{DirectoryURL: directory},
	}, nil
}

func prepareACMECache(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		return fmt.Errorf("cache directory must be a private directory with 0700 permissions")
	}
	probe, err := os.CreateTemp(dir, ".echo-observatory-cache-")
	if err != nil {
		return err
	}
	name := probe.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := probe.Write([]byte("cache check")); err != nil {
		_ = probe.Close()
		return err
	}
	if err := probe.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, name+".ready"); err != nil {
		return err
	}
	name += ".ready"
	return os.Remove(name)
}
