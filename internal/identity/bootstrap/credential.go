package bootstrap

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultCredentialPath = "/root/control-center-bootstrap-password"
	secretBytes           = 32
)

var ErrCredentialAlreadyExists = errors.New("bootstrap credential already exists")

// Store owns the one-time local bootstrap credential file. The credential is
// intentionally write-once: an existing file is never replaced on restart or
// upgrade.
type Store struct {
	Path string
}

func NewStore(path string) Store {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultCredentialPath
	}
	return Store{Path: path}
}

// GenerateSecret returns a cryptographically strong URL-safe credential. The
// result is suitable for the normal password hashing policy without any
// hard-coded bootstrap password exception.
func GenerateSecret(reader io.Reader) (string, error) {
	if reader == nil {
		reader = rand.Reader
	}
	raw := make([]byte, secretBytes)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", fmt.Errorf("generate bootstrap credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s Store) WriteOnce(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("bootstrap credential is required")
	}
	if !filepath.IsAbs(s.Path) {
		return errors.New("bootstrap credential path must be absolute")
	}

	file, err := os.OpenFile(s.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrCredentialAlreadyExists
		}
		return fmt.Errorf("create bootstrap credential file: %w", err)
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(s.Path)
		}
	}()

	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set bootstrap credential permissions: %w", err)
	}
	if _, err := io.WriteString(file, secret+"\n"); err != nil {
		return fmt.Errorf("write bootstrap credential: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync bootstrap credential: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close bootstrap credential: %w", err)
	}
	committed = true
	return nil
}

func (s Store) Remove() error {
	if !filepath.IsAbs(s.Path) {
		return errors.New("bootstrap credential path must be absolute")
	}
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove bootstrap credential: %w", err)
	}
	return nil
}

// Prepare creates one new credential and persists it exactly once. Callers must
// never log, audit, expose through APIs, or otherwise persist the returned value
// beyond hashing it for the initial admin account.
func (s Store) Prepare(reader io.Reader) (string, error) {
	secret, err := GenerateSecret(reader)
	if err != nil {
		return "", err
	}
	if err := s.WriteOnce(secret); err != nil {
		return "", err
	}
	return secret, nil
}
