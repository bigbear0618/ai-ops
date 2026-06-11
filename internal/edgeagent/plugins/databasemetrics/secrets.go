package databasemetrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ongridio/ongrid/internal/pkg/tunnel"
)

const (
	secretBaseDir    = "/var/lib/ongrid-edge/secrets"
	maxSecretContent = 16 << 10
)

// RegisterSecretHandler installs the manager->edge one-shot credential writer
// used by managed databasemetrics sources. It deliberately only accepts paths
// under /var/lib/ongrid-edge/secrets so plugin config cannot turn this RPC
// into a general-purpose file write primitive.
func RegisterSecretHandler(client tunnel.Client, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	client.RegisterHandler(tunnel.MethodWriteDatabaseMetricsSecret, func(ctx context.Context, _ tunnel.Session, _ string, body []byte) ([]byte, error) {
		var req tunnel.WriteDatabaseMetricsSecretRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("write database metrics secret: bad req: %w", err)
		}
		if err := writeManagedSecret(ctx, req.Path, req.Content); err != nil {
			return nil, err
		}
		log.Info("databasemetrics secret written",
			slog.String("source", req.SourceID),
			slog.String("path", req.Path))
		return json.Marshal(tunnel.WriteDatabaseMetricsSecretResponse{OK: true})
	})
}

func writeManagedSecret(ctx context.Context, path, content string) error {
	return writeManagedSecretInBase(ctx, secretBaseDir, path, content)
}

func writeManagedSecretInBase(ctx context.Context, baseDir, path, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("write database metrics secret: path required")
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("write database metrics secret: content required")
	}
	if len(content) > maxSecretContent {
		return fmt.Errorf("write database metrics secret: content too large")
	}
	cleanPath, err := cleanManagedSecretPath(baseDir, path)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(cleanPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("write database metrics secret: refusing symlink path")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("write database metrics secret: stat target: %w", err)
	}
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("write database metrics secret: mkdir: %w", err)
	}
	f, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("write database metrics secret: open: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return errors.Join(fmt.Errorf("write database metrics secret: write: %w", err), fmt.Errorf("write database metrics secret: close: %w", closeErr))
		}
		return fmt.Errorf("write database metrics secret: write: %w", err)
	}
	if !strings.HasSuffix(content, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			closeErr := f.Close()
			if closeErr != nil {
				return errors.Join(fmt.Errorf("write database metrics secret: write newline: %w", err), fmt.Errorf("write database metrics secret: close: %w", closeErr))
			}
			return fmt.Errorf("write database metrics secret: write newline: %w", err)
		}
	}
	if err := f.Chmod(0o600); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return errors.Join(fmt.Errorf("write database metrics secret: chmod: %w", err), fmt.Errorf("write database metrics secret: close: %w", closeErr))
		}
		return fmt.Errorf("write database metrics secret: chmod: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write database metrics secret: close: %w", err)
	}
	return nil
}

func cleanManagedSecretPath(baseDir, path string) (string, error) {
	cleanBase := filepath.Clean(baseDir)
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("write database metrics secret: path must be absolute")
	}
	rel, err := filepath.Rel(cleanBase, cleanPath)
	if err != nil {
		return "", fmt.Errorf("write database metrics secret: validate path: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("write database metrics secret: path outside allowed directory")
	}
	return cleanPath, nil
}
