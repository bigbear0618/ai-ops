package databasemetrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteManagedSecretInBaseWritesStrictFile(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "mysql-prod.my.cnf")

	if err := writeManagedSecretInBase(context.Background(), base, path, "[client]\nuser=u"); err != nil {
		t.Fatalf("writeManagedSecretInBase() error = %v", err)
	}

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(blob); got != "[client]\nuser=u\n" {
		t.Fatalf("content = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}
}

func TestWriteManagedSecretInBaseRejectsPathOutsideBase(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "..", "outside.dsn")

	err := writeManagedSecretInBase(context.Background(), base, path, "redis://127.0.0.1:6379/0")
	if err == nil || !strings.Contains(err.Error(), "outside allowed directory") {
		t.Fatalf("error = %v, want outside allowed directory", err)
	}
}

func TestWriteManagedSecretInBaseRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target.dsn")
	link := filepath.Join(base, "redis.dsn")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := writeManagedSecretInBase(context.Background(), base, link, "redis://127.0.0.1:6379/0")
	if err == nil || !strings.Contains(err.Error(), "refusing symlink path") {
		t.Fatalf("error = %v, want refusing symlink path", err)
	}
}
