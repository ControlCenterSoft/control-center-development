package bootstrap

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSecretIsDeterministicForInjectedReaderAndPolicySized(t *testing.T) {
	secret, err := GenerateSecret(bytes.NewReader(bytes.Repeat([]byte{0x5a}, secretBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 43 {
		t.Fatalf("unexpected encoded length: %d", len(secret))
	}
	if strings.ContainsAny(secret, "\r\n \t") {
		t.Fatal("credential contains whitespace")
	}
}

func TestGenerateSecretRejectsShortEntropySource(t *testing.T) {
	if _, err := GenerateSecret(bytes.NewReader(make([]byte, secretBytes-1))); err == nil {
		t.Fatal("short entropy source unexpectedly accepted")
	}
}

func TestWriteOnceCreates0600FileAndNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-password")
	store := NewStore(path)
	if err := store.WriteOnce("first-secret-value"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode=%#o want 0600", got)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first-secret-value\n" {
		t.Fatalf("unexpected content: %q", content)
	}

	if err := store.WriteOnce("replacement-secret-value"); !errors.Is(err, ErrCredentialAlreadyExists) {
		t.Fatalf("overwrite err=%v want ErrCredentialAlreadyExists", err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "first-secret-value\n" {
		t.Fatal("existing bootstrap credential was modified")
	}
}

func TestPrepareWritesGeneratedSecretExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-password")
	store := NewStore(path)
	secret, err := store.Prepare(bytes.NewReader(bytes.Repeat([]byte{0x21}, secretBytes)))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != secret+"\n" {
		t.Fatal("persisted bootstrap credential does not match returned secret")
	}

	if _, err := store.Prepare(bytes.NewReader(bytes.Repeat([]byte{0x22}, secretBytes))); !errors.Is(err, ErrCredentialAlreadyExists) {
		t.Fatalf("second prepare err=%v want ErrCredentialAlreadyExists", err)
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-password")
	store := NewStore(path)
	if err := store.WriteOnce("one-time-secret"); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credential still exists: %v", err)
	}
	if err := store.Remove(); err != nil {
		t.Fatalf("idempotent remove failed: %v", err)
	}
}

func TestStoreRejectsRelativePathAndEmptySecret(t *testing.T) {
	store := NewStore("relative/bootstrap-password")
	if err := store.WriteOnce("secret"); err == nil {
		t.Fatal("relative path unexpectedly accepted")
	}
	store = NewStore(filepath.Join(t.TempDir(), "bootstrap-password"))
	if err := store.WriteOnce("   "); err == nil {
		t.Fatal("empty credential unexpectedly accepted")
	}
}
