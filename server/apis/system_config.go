package apis

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"nexgestion/server/system"
)

// RegisterSystemConfigRoutes wires docs/System/system-config.md. Branding is
// public read-only (the login page needs it before anyone signs in); every
// edit requires a system_config.* permission.
func RegisterSystemConfigRoutes(router *http.ServeMux, config *system.SystemConfigService, protected func(string, http.HandlerFunc) http.HandlerFunc) {
	router.HandleFunc("GET /api/branding", func(w http.ResponseWriter, r *http.Request) {
		branding, err := config.Branding(r.Context())
		if err != nil {
			writeSystemConfigError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		writeJSON(w, http.StatusOK, branding)
	})
	router.HandleFunc("GET /api/branding/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		contentType, data, etag, err := config.BrandingImageData(r.Context(), r.PathValue("name"))
		if err != nil {
			writeSystemConfigError(w, err)
			return
		}
		quoted := `"` + etag + `"`
		w.Header().Set("ETag", quoted)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Uploaded SVGs are validated, but are still served inert.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
		if r.Header.Get("If-None-Match") == quoted {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})

	handle := func(pattern, permission string, fn func(*http.Request) (any, error)) {
		router.HandleFunc(pattern, protected(permission, func(w http.ResponseWriter, r *http.Request) {
			result, err := fn(r)
			if err != nil {
				writeSystemConfigError(w, err)
				return
			}
			if r.Method != http.MethodGet {
				recordRequestLog(r, "info", "system config "+r.Method+" "+r.URL.Path)
			}
			writeJSON(w, http.StatusOK, result)
		}))
	}
	decode := func(r *http.Request, destination any) error {
		if err := decodeJSON(nil, r, destination); err != nil {
			return errors.Join(system.ErrConfigInvalid, err)
		}
		return nil
	}
	leaveTypes := func(items []system.ConfigLeaveType, err error) (any, error) {
		return map[string]any{"items": items}, err
	}
	const catalog = "/api/system-config/catalogs/leave-types"

	handle("GET "+catalog, "system_config.read", func(r *http.Request) (any, error) {
		return leaveTypes(config.ListLeaveTypes(r.Context()))
	})
	handle("POST "+catalog, "system_config.catalogs.manage", func(r *http.Request) (any, error) {
		var input system.CreateLeaveTypeInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return leaveTypes(config.CreateLeaveType(r.Context(), authenticatedUserID(r), input))
	})
	handle("PATCH "+catalog+"/{key}", "system_config.catalogs.manage", func(r *http.Request) (any, error) {
		var input system.UpdateLeaveTypeInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return leaveTypes(config.UpdateLeaveType(r.Context(), authenticatedUserID(r), r.PathValue("key"), input))
	})
	handle("DELETE "+catalog+"/{key}", "system_config.catalogs.manage", func(r *http.Request) (any, error) {
		return leaveTypes(config.DeleteLeaveType(r.Context(), authenticatedUserID(r), r.PathValue("key")))
	})
	handle("PUT "+catalog+"/order", "system_config.catalogs.manage", func(r *http.Request) (any, error) {
		var input struct {
			Keys []string `json:"keys"`
		}
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return leaveTypes(config.ReorderLeaveTypes(r.Context(), authenticatedUserID(r), input.Keys))
	})
	handle("POST "+catalog+"/restore-defaults", "system_config.catalogs.manage", func(r *http.Request) (any, error) {
		return leaveTypes(config.RestoreLeaveTypeDefaults(r.Context(), authenticatedUserID(r)))
	})

	handle("GET /api/system-config/permissions", "system_config.read", func(r *http.Request) (any, error) {
		permissions, err := config.PermissionCatalog()
		return map[string]any{"items": permissions}, err
	})

	handle("PUT /api/system-config/branding/colors", "system_config.branding.manage", func(r *http.Request) (any, error) {
		var input struct {
			Colors map[string]string `json:"colors"`
		}
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return config.SetBrandingColors(r.Context(), authenticatedUserID(r), input.Colors)
	})
	handle("DELETE /api/system-config/branding/colors", "system_config.branding.manage", func(r *http.Request) (any, error) {
		return config.SetBrandingColors(r.Context(), authenticatedUserID(r), nil)
	})
	// The image is sent as the raw request body.
	handle("PUT /api/system-config/branding/images/{name}", "system_config.branding.manage", func(r *http.Request) (any, error) {
		data, err := io.ReadAll(io.LimitReader(r.Body, 256<<10+1))
		if err != nil {
			return nil, errors.Join(system.ErrConfigInvalid, err)
		}
		return config.SetBrandingImage(r.Context(), authenticatedUserID(r), r.PathValue("name"), data)
	})
	handle("DELETE /api/system-config/branding/images/{name}", "system_config.branding.manage", func(r *http.Request) (any, error) {
		return config.ResetBrandingImage(r.Context(), authenticatedUserID(r), r.PathValue("name"))
	})

	handle("GET /api/system-config/audit", "system_config.read", func(r *http.Request) (any, error) {
		limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
		events, err := config.ListAudit(r.Context(), strings.TrimSpace(r.URL.Query().Get("surface")), limit)
		return map[string]any{"items": events}, err
	})
}

func writeSystemConfigError(w http.ResponseWriter, err error) {
	message := strings.ReplaceAll(err.Error(), "\n", ": ")
	switch {
	case errors.Is(err, system.ErrConfigNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": message})
	case errors.Is(err, system.ErrConfigInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
	case errors.Is(err, system.ErrConfigConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": message})
	default:
		writeSystemError(w, err)
	}
}
