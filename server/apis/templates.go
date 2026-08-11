package apis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"nexgestion/server/system"
)

// templateMultipartMemory is the in-memory buffer ParseMultipartForm uses
// before spilling additional parts to temporary files.
const templateMultipartMemory = 10 << 20

// templateUploadFormOverhead allows room for the non-file form fields
// (description, audiences) on top of the configured file size limit.
const templateUploadFormOverhead = 64 << 10

func listTemplates(templates *system.TemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := templates.ListVisible(r.Context(), authenticatedUserID(r), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("templates", items))
	}
}

func uploadTemplate(templates *system.TemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maxFileBytes, err := templates.MaxFileBytes(r.Context())
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxFileBytes+templateUploadFormOverhead)
		if err := r.ParseMultipartForm(templateMultipartMemory); err != nil {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
				"code":  "template_file_too_large",
				"error": "file exceeds the maximum allowed size",
			})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": "file is required"})
			return
		}
		defer file.Close()

		var input system.UploadTemplateInput
		input.Description = r.FormValue("description")
		if raw := strings.TrimSpace(r.FormValue("audiences")); raw != "" {
			if err := json.Unmarshal([]byte(raw), &input.Audiences); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": "audiences must be a valid JSON array"})
				return
			}
		}

		item, err := templates.Upload(r.Context(), authenticatedUserID(r), input, header.Filename, header.Header.Get("Content-Type"), file)
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		recordRequestLog(r, "info", "uploaded template file "+item.ID)
		writeJSON(w, http.StatusCreated, item)
	}
}

func downloadTemplate(templates *system.TemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path, filename, err := templates.DownloadPath(r.Context(), authenticatedUserID(r), r.PathValue("id"))
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		setDownloadFilename(w, filename)
		http.ServeFile(w, r, path)
	}
}

func deleteTemplate(templates *system.TemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := templates.Delete(r.Context(), authenticatedUserID(r), r.PathValue("id")); err != nil {
			writeTemplateError(w, err)
			return
		}
		recordRequestLog(r, "warning", "deleted template file "+r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	}
}

func templateStorageUsage(templates *system.TemplateService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		usage, err := templates.StorageUsage(r.Context())
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, usage)
	}
}

// setDownloadFilename sets Content-Disposition with both a sanitized ASCII
// fallback and an RFC 5987 UTF-8 filename, since template names are often
// non-ASCII (e.g. 會員資料填寫表.docx).
func setDownloadFilename(w http.ResponseWriter, filename string) {
	ascii := sanitizeASCIIFilename(filename)
	w.Header().Set("Content-Disposition", `attachment; filename="`+ascii+`"; filename*=UTF-8''`+url.PathEscape(filename))
}

func sanitizeASCIIFilename(name string) string {
	var builder strings.Builder
	for _, r := range name {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			builder.WriteRune('_')
			continue
		}
		builder.WriteRune(r)
	}
	result := builder.String()
	if strings.TrimSpace(result) == "" {
		return "download"
	}
	return result
}

func writeTemplateError(w http.ResponseWriter, err error) {
	code, status := "template_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrTemplateInvalid):
		code, status = "template_invalid", http.StatusBadRequest
	case errors.Is(err, system.ErrTemplateNotFound):
		code, status = "template_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrTemplatePermission):
		code, status = "template_permission_denied", http.StatusForbidden
	case errors.Is(err, system.ErrTemplateFileTooLarge):
		code, status = "template_file_too_large", http.StatusRequestEntityTooLarge
	case errors.Is(err, system.ErrTemplateStorageFull):
		code, status = "template_storage_limit_exceeded", http.StatusInsufficientStorage
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
