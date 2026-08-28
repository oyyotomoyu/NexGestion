package apis

import (
	"errors"
	"net/http"

	"nexgestion/server/system"
)

func listCheckoutTransactions(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := checkout.ListTransactions(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("checkout_transactions", items))
	}
}

func createCheckoutTransaction(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCheckoutTransactionInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		transaction, err := checkout.CreateTransaction(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		recordRequestLog(r, "info", "created checkout transaction "+transaction.ID)
		writeJSON(w, http.StatusCreated, transaction)
	}
}

func getCheckoutTransaction(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transaction, err := checkout.GetTransaction(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, transaction)
	}
}

func addCheckoutLine(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.AddCheckoutLineInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		transaction, err := checkout.AddLine(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, transaction)
	}
}

func addCheckoutDiscount(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.AddCheckoutDiscountInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		transaction, err := checkout.AddDiscount(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, transaction)
	}
}

func addCheckoutPayment(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.AddCheckoutPaymentInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		transaction, err := checkout.AddPayment(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, transaction)
	}
}

func completeCheckoutTransaction(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transaction, err := checkout.CompleteTransaction(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		recordRequestLog(r, "info", "completed checkout transaction "+transaction.ID)
		writeJSON(w, http.StatusOK, transaction)
	}
}

func voidCheckoutTransaction(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		transaction, err := checkout.VoidTransaction(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		recordRequestLog(r, "warning", "voided checkout transaction "+transaction.ID)
		writeJSON(w, http.StatusOK, transaction)
	}
}

func createCheckoutCoupon(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCheckoutCouponInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		coupon, err := checkout.CreateCoupon(r.Context(), input)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, coupon)
	}
}

func createCheckoutPromotionRule(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCheckoutPromotionRuleInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		rule, err := checkout.CreatePromotionRule(r.Context(), input)
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	}
}

func resolveCheckoutScan(checkout *system.CheckoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := checkout.ResolveScanCode(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			writeCheckoutError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writeCheckoutError(w http.ResponseWriter, err error) {
	code, status := "checkout_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrCheckoutNotFound):
		code, status = "checkout_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrCheckoutInvalid), errors.Is(err, system.ErrInvalidListQuery):
		code, status = "checkout_invalid_input", http.StatusBadRequest
	case errors.Is(err, system.ErrCheckoutState):
		code, status = "checkout_invalid_state", http.StatusConflict
	case errors.Is(err, system.ErrUserNotFound):
		code, status = "checkout_user_not_found", http.StatusNotFound
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
