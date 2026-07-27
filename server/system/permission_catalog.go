package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const permissionConfigEnvironment = "NEXGESTION_PERMISSION_CONFIG"

type PermissionDefinition struct {
	Key         string `json:"key"`
	Module      string `json:"module"`
	Description string `json:"description"`
}

type PermissionCatalog struct {
	Permissions []PermissionDefinition `json:"permissions"`
}

func LoadPermissionCatalog() (*PermissionCatalog, error) {
	path, err := permissionCatalogPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read permission catalog: %w", err)
	}
	var catalog PermissionCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("decode permission catalog: %w", err)
	}
	if len(catalog.Permissions) == 0 {
		return nil, errors.New("permission catalog must contain at least one permission")
	}
	seen := make(map[string]struct{}, len(catalog.Permissions))
	for index := range catalog.Permissions {
		item := &catalog.Permissions[index]
		item.Key = strings.TrimSpace(item.Key)
		item.Module = strings.TrimSpace(item.Module)
		item.Description = strings.TrimSpace(item.Description)
		if item.Key == "" || item.Module == "" {
			return nil, fmt.Errorf("permission catalog entry %d requires key and module", index)
		}
		if _, exists := seen[item.Key]; exists {
			return nil, fmt.Errorf("duplicate permission key %q", item.Key)
		}
		seen[item.Key] = struct{}{}
	}
	return &catalog, nil
}

func (c *PermissionCatalog) Contains(key string) bool {
	for _, permission := range c.Permissions {
		if permission.Key == key {
			return true
		}
	}
	return false
}

func permissionCatalogPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(permissionConfigEnvironment)); configured != "" {
		return configured, nil
	}
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), "config", "permission.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	candidates := []string{
		filepath.Join("config", "permission.json"),
		filepath.Join("..", "config", "permission.json"),
		filepath.Join(filepath.Dir(sourceFile), "..", "..", "config", "permission.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("config/permission.json not found")
}

func syncPermissionCatalog(ctx context.Context, tx *sql.Tx) error {
	catalog, err := LoadPermissionCatalog()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, permission := range catalog.Permissions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO permissions
			(id, permission_key, module, description, created_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(permission_key) DO UPDATE SET
				module = excluded.module,
				description = excluded.description`,
			permission.Key, permission.Key, permission.Module, nullableCatalogDescription(permission.Description), now,
		); err != nil {
			return fmt.Errorf("sync permission %q: %w", permission.Key, err)
		}
	}
	placeholders := make([]string, len(catalog.Permissions))
	args := make([]any, len(catalog.Permissions))
	for index, permission := range catalog.Permissions {
		placeholders[index] = "?"
		args[index] = permission.Key
	}
	notInCatalog := `SELECT id FROM permissions WHERE permission_key NOT IN (` + strings.Join(placeholders, ",") + `)`
	for _, table := range []string{"role_permissions", "group_permissions"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE permission_id IN (`+notInCatalog+`)`, args...); err != nil {
			return fmt.Errorf("remove obsolete %s grants: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM permissions WHERE permission_key NOT IN (`+strings.Join(placeholders, ",")+`)`, args...); err != nil {
		return fmt.Errorf("remove obsolete permissions: %w", err)
	}
	return nil
}

func nullableCatalogDescription(value string) any {
	if value == "" {
		return nil
	}
	return value
}
