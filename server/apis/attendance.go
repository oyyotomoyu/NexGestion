package apis

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"nexgestion/server/system"
)

func attendanceToday(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		day, err := attendance.Today(r.Context(), authenticatedUserID(r))
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, day)
	}
}

func attendanceSignIn(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		day, err := attendance.SignIn(r.Context(), authenticatedUserID(r))
		if err != nil {
			recordRequestLog(r, "warning", "attendance sign in rejected")
			writeAttendanceError(w, err)
			return
		}
		recordRequestLog(r, "info", "attendance signed in")
		writeJSON(w, http.StatusOK, day)
	}
}

func attendanceSignOut(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		day, err := attendance.SignOut(r.Context(), authenticatedUserID(r))
		if err != nil {
			recordRequestLog(r, "warning", "attendance sign out rejected")
			writeAttendanceError(w, err)
			return
		}
		recordRequestLog(r, "info", "attendance signed out")
		writeJSON(w, http.StatusOK, day)
	}
}

func attendanceDays(attendance *system.AttendanceService, self bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("userId")
		if self {
			userID = authenticatedUserID(r)
		}
		days, err := attendance.ListDays(r.Context(), userID, strings.TrimSpace(r.URL.Query().Get("month")))
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"days": days})
	}
}

func attendanceLeaveTypes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		types, err := system.LoadLeaveTypes()
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"leave_types": types})
	}
}

func attendanceLeaveRequests(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requests, err := attendance.ListLeaveRequests(r.Context(), authenticatedUserID(r))
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"leave_requests": requests})
	}
}

func applyAttendanceLeave(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.ApplyLeaveInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		request, err := attendance.ApplyLeave(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		recordRequestLog(r, "info", "applied for leave "+request.ID)
		writeJSON(w, http.StatusCreated, request)
	}
}

func attendanceLeaveApprovals(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requests, err := attendance.ListLeaveApprovals(r.Context(), authenticatedUserID(r))
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"leave_requests": requests})
	}
}

func decideAttendanceLeave(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.DecideLeaveInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		request, err := attendance.DecideLeave(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		recordRequestLog(r, "info", "decided leave request "+request.ID+" as "+request.Status)
		writeJSON(w, http.StatusOK, request)
	}
}

func attendanceSelfMonthly(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reports, err := attendance.MonthlyReports(r.Context(), r.PathValue("month"))
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		userID := authenticatedUserID(r)
		for _, report := range reports {
			if report.UserID == userID {
				writeJSON(w, http.StatusOK, report)
				return
			}
		}
		writeAttendanceError(w, system.ErrAttendanceNotFound)
	}
}

func attendanceMonthlyReports(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reports, err := attendance.MonthlyReports(r.Context(), r.PathValue("month"))
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
	}
}

func generateAttendanceMonthlyReport(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := attendance.GenerateMonthlyCSV(r.Context(), r.PathValue("month"))
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		recordRequestLog(r, "info", "generated attendance CSV "+report.ReportMonth)
		writeJSON(w, http.StatusOK, report)
	}
}

func correctAttendanceDay(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CorrectAttendanceDayInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		day, err := attendance.CorrectDay(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		recordRequestLog(r, "warning", "corrected attendance day "+day.ID)
		writeJSON(w, http.StatusOK, day)
	}
}

func downloadAttendanceCSV(attendance *system.AttendanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		month := r.PathValue("month")
		path, err := attendance.CSVPath(r.Context(), month)
		if err != nil {
			writeAttendanceError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="attendance-`+month+`.csv"`)
		http.ServeFile(w, r, filepath.Clean(path))
	}
}

func writeAttendanceError(w http.ResponseWriter, err error) {
	code, status := "attendance_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrAttendanceConflict):
		code, status = "attendance_status_conflict", http.StatusConflict
	case errors.Is(err, system.ErrAttendanceNotFound), errors.Is(err, system.ErrAttendanceExport):
		code, status = "attendance_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrAttendanceMonth):
		code, status = "attendance_invalid_month", http.StatusBadRequest
	case errors.Is(err, system.ErrAttendanceMonthOpen):
		code, status = "attendance_month_open", http.StatusConflict
	case errors.Is(err, system.ErrInvalidLeaveRequest):
		code, status = "attendance_invalid_leave_request", http.StatusBadRequest
	case errors.Is(err, system.ErrLeaveDecision):
		code, status = "attendance_leave_decision_conflict", http.StatusConflict
	case errors.Is(err, system.ErrUserNotFound):
		code, status = "user_not_found", http.StatusNotFound
	case strings.Contains(err.Error(), "attendance correction"), strings.Contains(err.Error(), "attendance session"),
		strings.Contains(err.Error(), "attendance day"):
		code, status = "attendance_invalid_correction", http.StatusBadRequest
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
