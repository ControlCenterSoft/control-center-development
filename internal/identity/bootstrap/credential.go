package bootstrap

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	DefaultAdminPasswordPath = "/root/control-center-bootstrap-password"
	credentialBytes         = 32
)

// CredentialFile manages the one-time bootstrap credential without ever
// exposing it through logs, audit events, telemetry, or process arguments.
type CredentialFile struct {
	Path string
}

func DefaultCredentialFile() CredentialFile {
	return CredentialFile{Path: DefaultAdminPasswordPath}
}

// GeneratePassword returns a cryptographically random URL-safe bootstrap
// credential. The encoded value contains 256 bits of randomness.
func GeneratePassword() (string, error) {
	buffer := make([]byte, credentialBytes)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return "", fmt.Errorf("generate bootstrap credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// Write creates the bootstrap credential file exactly once. Existing files are
// never replaced, so a restart cannot silently rotate an outstanding bootstrap
// credential.
func (f CredentialFile) Write(password string) error {
	if f.Path == "" {
		return errors.New("bootstrap credential path is required")
	}
	if password == "" {
		return errors.New("bootstrap credential is required")
	}
	if filepath.Clean(f.Path) != f.Path || !filepath.IsAbs(f.Path) {
		return errors.New("bootstrap credential path must be absolute and clean")
	}
	file, err := os.OpenFile(f.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create bootstrap credential file: %w", err)
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(f.Path)
		}
	}()
	if _, err := file.WriteString(password + "\n"); err != nil {
		return fmt.Errorf("write bootstrap credential file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync bootstrap credential file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set bootstrap credential permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close bootstrap credential file: %w", err)
	}
	removeOnFailure = false
	return nil
}

func (f CredentialFile) Remove() error {
	if f.Path == "" {
		return errors.New("bootstrap credential path is required")
	}
	if err := os.Remove(f.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove bootstrap credential file: %w", err)
	}
	return nil
}
