package apis

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"nexgestion/server/system"
)

func listReportFiles(reports *system.ReportFileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files, err := reports.List()
		if err != nil {
			writeReportFileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": files})
	}
}

func downloadReportFile(reports *system.ReportFileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimSpace(r.PathValue("path"))
		path, err := reports.Path(relative)
		if err != nil {
			writeReportFileError(w, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
		http.ServeFile(w, r, path)
	}
}

func deleteReportFile(reports *system.ReportFileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := reports.Delete(r.PathValue("path")); err != nil {
			writeReportFileError(w, err)
			return
		}
		recordRequestLog(r, "warning", "deleted report file "+r.PathValue("path"))
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeReportFileError(w http.ResponseWriter, err error) {
	code, status := "report_file_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrReportFileInvalid):
		code, status = "report_file_invalid", http.StatusBadRequest
	case errors.Is(err, system.ErrReportFileMissing):
		code, status = "report_file_not_found", http.StatusNotFound
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
