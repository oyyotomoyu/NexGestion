package apis

import (
	"errors"
	"net/http"

	"nexgestion/server/system"
)

// --- Customers ---

func listCRMCustomers(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := crm.ListCustomers(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("customers", items))
	}
}

func createCRMCustomer(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCRMCustomerInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		customer, err := crm.CreateCustomer(r.Context(), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		recordRequestLog(r, "info", "created crm customer "+customer.ID)
		writeJSON(w, http.StatusCreated, customer)
	}
}

func getCRMCustomer(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customer, err := crm.GetCustomer(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, customer)
	}
}

func updateCRMCustomer(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateCRMCustomerInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		customer, err := crm.UpdateCustomer(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, customer)
	}
}

// --- Customer Tiers ---

func listCRMCustomerTiers(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := crm.ListCustomerTiers(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("customer_tiers", items))
	}
}

func createCRMCustomerTier(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCRMCustomerTierInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		tier, err := crm.CreateCustomerTier(r.Context(), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, tier)
	}
}

func getCRMCustomerTier(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tier, err := crm.GetCustomerTier(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tier)
	}
}

func updateCRMCustomerTier(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateCRMCustomerTierInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		tier, err := crm.UpdateCustomerTier(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tier)
	}
}

// --- Membership Tiers ---

func listCRMMembershipTiers(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := crm.ListMembershipTiers(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("membership_tiers", items))
	}
}

func createCRMMembershipTier(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCRMMembershipTierInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		tier, err := crm.CreateMembershipTier(r.Context(), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, tier)
	}
}

func getCRMMembershipTier(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tier, err := crm.GetMembershipTier(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tier)
	}
}

func updateCRMMembershipTier(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateCRMMembershipTierInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		tier, err := crm.UpdateMembershipTier(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tier)
	}
}

// --- Memberships ---

func listCRMMemberships(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := crm.ListMemberships(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("memberships", items))
	}
}

func createCRMMembership(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCRMMembershipInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		membership, err := crm.CreateMembership(r.Context(), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, membership)
	}
}

func getCRMMembership(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		membership, err := crm.GetMembership(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, membership)
	}
}

func updateCRMMembership(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateCRMMembershipInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		membership, err := crm.UpdateMembership(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, membership)
	}
}

func resolveCRMMemberNumber(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		membership, err := crm.ResolveMemberNumber(r.Context(), r.URL.Query().Get("member_number"))
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, membership)
	}
}

// --- Price Lists ---

func listCRMPriceLists(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := crm.ListPriceLists(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("price_lists", items))
	}
}

func createCRMPriceList(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCRMPriceListInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		priceList, err := crm.CreatePriceList(r.Context(), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, priceList)
	}
}

func getCRMPriceList(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		priceList, err := crm.GetPriceList(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, priceList)
	}
}

func updateCRMPriceList(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateCRMPriceListInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		priceList, err := crm.UpdatePriceList(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, priceList)
	}
}

func addCRMPriceListItem(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.AddCRMPriceListItemInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		priceList, err := crm.AddPriceListItem(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, priceList)
	}
}

// --- Points Earning Rules ---

func listCRMPointsEarningRules(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := crm.ListPointsEarningRules(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("points_earning_rules", items))
	}
}

func createCRMPointsEarningRule(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateCRMPointsEarningRuleInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		rule, err := crm.CreatePointsEarningRule(r.Context(), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	}
}

func updateCRMPointsEarningRule(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateCRMPointsEarningRuleInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		rule, err := crm.UpdatePointsEarningRule(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rule)
	}
}

// --- Loyalty Points ---

func postCRMPointsLedgerEntry(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.PostCRMPointsLedgerEntryInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		entry, err := crm.PostPointsLedgerEntry(r.Context(), input)
		if err != nil {
			writeCRMError(w, err)
			return
		}
		recordRequestLog(r, "info", "posted crm points ledger entry "+entry.ID)
		writeJSON(w, http.StatusCreated, entry)
	}
}

func getCRMPointsBalance(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		balance, err := crm.GetPointsBalance(r.Context(), r.PathValue("id"))
		if err != nil {
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, balance)
	}
}

func listCRMPointsLedger(crm *system.CRMService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := crm.ListPointsLedger(r.Context(), r.PathValue("id"), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeCRMError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("points_ledger", items))
	}
}

func writeCRMError(w http.ResponseWriter, err error) {
	code, status := "crm_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrCRMNotFound):
		code, status = "crm_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrCRMInvalid), errors.Is(err, system.ErrInvalidListQuery):
		code, status = "crm_invalid_input", http.StatusBadRequest
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
