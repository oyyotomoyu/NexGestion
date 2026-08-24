package apis

import (
	"errors"
	"net/http"

	"nexgestion/server/system"
)

func createCompensationRecord(salary *system.SalaryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCompensationRecordInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		userID := r.PathValue("userId")
		record, err := salary.CreateCompensationRecord(r.Context(), authenticatedUserID(r), userID, input)
		if err != nil {
			writeSalaryError(w, err)
			return
		}
		recordRequestLog(r, "info", "created compensation record "+record.ID+" for user "+userID)
		writeJSON(w, http.StatusCreated, record)
	}
}

func listCompensationRecords(salary *system.SalaryService, self bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("userId")
		if self {
			userID = authenticatedUserID(r)
		}
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		records, err := salary.ListCompensationRecords(r.Context(), userID, query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeSalaryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("compensation_records", records))
	}
}

func currentCompensationRecord(salary *system.SalaryService, self bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("userId")
		if self {
			userID = authenticatedUserID(r)
		}
		record, err := salary.CurrentCompensationRecord(r.Context(), userID)
		if err != nil {
			writeSalaryError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	}
}

func writeSalaryError(w http.ResponseWriter, err error) {
	code, status := "salary_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrSalaryNotFound), errors.Is(err, system.ErrUserNotFound):
		code, status = "salary_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrSalaryOverlap):
		code, status = "salary_overlap", http.StatusConflict
	case errors.Is(err, system.ErrSalaryInvalidInput), errors.Is(err, system.ErrInvalidListQuery):
		code, status = "salary_invalid_input", http.StatusBadRequest
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
