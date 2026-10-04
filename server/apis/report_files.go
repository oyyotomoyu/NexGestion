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
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		files, err := reports.List(query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeReportFileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("files", files))
	}
}

func downloadReportFile(reports *system.ReportFileService, security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relative := strings.TrimSpace(r.PathValue("path"))
		path, err := reports.Path(relative)
		if err != nil {
			writeReportFileError(w, err)
			return
		}
		recordRequestLog(r, "info", "downloaded report file "+relative)
		// Feeds the mass-export/data-exfiltration rule (SIEM.md Section 6) -
		// this was the one existing action in the wiring list with no audit
		// trail at all until now.
		_, _ = security.RecordEvent(r.Context(), system.SecurityEventInput{
			EventType: "report_download", Severity: "info", ActorUserID: authenticatedUserID(r), SourceIP: clientIP(r),
			Module: "reports", RecordRef: relative, Summary: "downloaded report file " + relative,
		})
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
