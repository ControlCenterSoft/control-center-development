package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePasswordIsStrongAndDistinct(t *testing.T) {
	first, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) < 40 || len(second) < 40 {
		t.Fatalf("generated bootstrap credential is unexpectedly short: %d/%d", len(first), len(second))
	}
	if first == second {
		t.Fatal("generated bootstrap credentials unexpectedly match")
	}
	if strings.ContainsAny(first, "\r\n\t ") || strings.ContainsAny(second, "\r\n\t ") {
		t.Fatal("generated bootstrap credential contains whitespace")
	}
}

func TestCredentialFileIsExclusiveMode0600AndRemovable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-password")
	credential := CredentialFile{Path: path}
	const secret = "synthetic-bootstrap-secret-value"

	if err := credential.Write(secret); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("bootstrap credential mode=%#o want=0600", got)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != secret+"\n" {
		t.Fatal("bootstrap credential file content mismatch")
	}
	if err := credential.Write("replacement-must-not-win"); err == nil {
		t.Fatal("existing bootstrap credential file was overwritten")
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != secret+"\n" {
		t.Fatal("failed overwrite attempt changed bootstrap credential")
	}
	if err := credential.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("bootstrap credential file remains after removal: %v", err)
	}
	if err := credential.Remove(); err != nil {
		t.Fatalf("idempotent removal failed: %v", err)
	}
}
