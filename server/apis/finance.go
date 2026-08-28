package apis

import (
	"errors"
	"net/http"

	"nexgestion/server/system"
)

func listFinanceAccounts(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := finance.ListAccounts(r.Context())
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"accounts": items})
	}
}

func createFinanceAccount(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateFinanceAccountInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		item, err := finance.CreateAccount(r.Context(), input)
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func listAccountingPeriods(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := finance.ListPeriods(r.Context())
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"accounting_periods": items})
	}
}

func createAccountingPeriod(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateAccountingPeriodInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		item, err := finance.CreatePeriod(r.Context(), input)
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func closeAccountingPeriod(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := finance.ClosePeriod(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		recordRequestLog(r, "warning", "closed accounting period "+item.ID)
		writeJSON(w, http.StatusOK, item)
	}
}

func createJournalEntry(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateJournalEntryInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		item, err := finance.CreateJournalEntry(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func getJournalEntry(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := finance.GetJournalEntry(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func postJournalEntry(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := finance.PostJournalEntry(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		recordRequestLog(r, "info", "posted journal entry "+item.ID)
		writeJSON(w, http.StatusOK, item)
	}
}

func listFinanceVendors(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := finance.ListVendors(r.Context())
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"vendors": items})
	}
}

func createFinanceVendor(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateFinanceVendorInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		item, err := finance.CreateVendor(r.Context(), input)
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func createAPBill(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateAPBillInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		item, err := finance.CreateAPBill(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func approveAPBill(finance *system.FinanceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, err := finance.ApproveAPBill(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFinanceError(w, err)
			return
		}
		recordRequestLog(r, "info", "approved AP bill "+item.ID)
		writeJSON(w, http.StatusOK, item)
	}
}

func writeFinanceError(w http.ResponseWriter, err error) {
	code, status := "finance_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrFinanceNotFound), errors.Is(err, system.ErrUserNotFound):
		code, status = "finance_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrFinanceInvalid):
		code, status = "finance_invalid_input", http.StatusBadRequest
	case errors.Is(err, system.ErrFinanceState):
		code, status = "finance_invalid_state", http.StatusConflict
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
