package system

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPermissionCatalogCoversDocumentedSystems(t *testing.T) {
	catalog, err := LoadPermissionCatalog()
	if err != nil {
		t.Fatalf("load permission catalog: %v", err)
	}
	modules := map[string]bool{}
	for _, permission := range catalog.Permissions {
		modules[permission.Module] = true
	}
	matches, err := filepath.Glob(filepath.Join("..", "..", "docs", "System", "*-system.md"))
	if err != nil {
		t.Fatalf("glob system docs: %v", err)
	}
	exceptions := map[string]string{
		"approval":        "approvals",
		"group":           "groups",
		"notification":    "notifications",
		"order":           "orders",
		"permission":      "permissions",
		"role":            "roles",
		"template":        "templates",
		"user":            "users",
		"general-affairs": "general_affairs",
	}
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), "-system.md")
		module := strings.ReplaceAll(name, "-", "_")
		if mapped, ok := exceptions[name]; ok {
			module = mapped
		}
		if !modules[module] {
			t.Fatalf("documented system %q has no permission catalog module %q", filepath.Base(path), module)
		}
	}
}

func TestPermissionCatalogHasModuleAccessPermissions(t *testing.T) {
	catalog, err := LoadPermissionCatalog()
	if err != nil {
		t.Fatalf("load permission catalog: %v", err)
	}
	modules := map[string]bool{}
	for _, permission := range catalog.Permissions {
		modules[permission.Module] = true
	}
	for module := range modules {
		if module == "logs" || module == "reports" {
			continue
		}
		if !catalog.Contains(module + ".access") {
			t.Fatalf("module %q has no %q access permission", module, module+".access")
		}
	}
}
