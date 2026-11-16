package apis

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nexgestion/server/system"
)

// RegisterGeneralAffairsRoutes wires docs/System/general-affairs-system.md.
// Routes whose access depends on the record ("own request, or its approver,
// or the manage permission") are authenticated at the route and authorized in
// the service, which receives the caller's effective permissions.
func RegisterGeneralAffairsRoutes(router *http.ServeMux, ga *system.GeneralAffairsService, users *system.UserService,
	protected func(string, http.HandlerFunc) http.HandlerFunc, authenticated func(http.HandlerFunc) http.HandlerFunc) {
	const base = "/api/general-affairs"

	actorOf := func(r *http.Request) (system.GAActor, error) {
		userID := authenticatedUserID(r)
		keys, err := users.EffectivePermissionKeys(r.Context(), userID)
		if err != nil {
			return system.GAActor{}, err
		}
		permissions := make(map[string]bool, len(keys))
		for _, key := range keys {
			permissions[key] = true
		}
		return system.GAActor{UserID: userID, Permissions: permissions}, nil
	}
	respond := func(w http.ResponseWriter, r *http.Request, status int, result any, err error) {
		if err != nil {
			writeGeneralAffairsError(w, err)
			return
		}
		if r.Method != http.MethodGet {
			recordRequestLog(r, "info", "general affairs "+r.Method+" "+r.URL.Path)
		}
		if result == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, status, result)
	}
	// handle registers a route guarded by one catalog permission.
	handle := func(method, path, permission string, status int, fn func(*http.Request, system.GAActor) (any, error)) {
		router.HandleFunc(method+" "+base+path, protected(permission, func(w http.ResponseWriter, r *http.Request) {
			actor, err := actorOf(r)
			if err != nil {
				respond(w, r, status, nil, err)
				return
			}
			result, err := fn(r, actor)
			respond(w, r, status, result, err)
		}))
	}
	// handleAny requires at least one of the listed permissions; an empty
	// list means "any signed-in user" with the service deciding.
	handleAny := func(method, path string, permissions []string, status int, fn func(*http.Request, system.GAActor) (any, error)) {
		router.HandleFunc(method+" "+base+path, authenticated(func(w http.ResponseWriter, r *http.Request) {
			actor, err := actorOf(r)
			if err != nil {
				respond(w, r, status, nil, err)
				return
			}
			if len(permissions) > 0 {
				allowed := false
				for _, permission := range permissions {
					allowed = allowed || actor.Has(permission)
				}
				if !allowed {
					recordRequestLog(r, "warning", "permission denied: "+r.Method+" "+r.URL.Path+" requires one of "+strings.Join(permissions, ", "))
					respond(w, r, status, nil, system.ErrPermissionDenied)
					return
				}
			}
			result, err := fn(r, actor)
			respond(w, r, status, result, err)
		}))
	}
	listItems := func(r *http.Request, fn func(system.ListQuery) (any, error)) (any, error) {
		query, err := parseListQuery(r)
		if err != nil {
			return nil, err
		}
		return fn(query)
	}
	decode := func(r *http.Request, destination any) error {
		if err := decodeJSON(nil, r, destination); err != nil {
			return gaBadRequest(err)
		}
		return nil
	}
	none := []string{}

	// --- 2.1 Assets & Custody ----------------------------------------------
	handle("GET", "/assets", "general_affairs.assets.read", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return listItems(r, func(q system.ListQuery) (any, error) {
			result, err := ga.ListAssets(r.Context(), q, "")
			return listResponse("items", result), err
		})
	})
	handle("GET", "/me/assets", "general_affairs.assets.read.self", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return listItems(r, func(q system.ListQuery) (any, error) {
			result, err := ga.ListAssets(r.Context(), q, a.UserID)
			return listResponse("items", result), err
		})
	})
	handleAny("GET", "/assets/{id}", []string{"general_affairs.assets.read", "general_affairs.assets.read.self"}, http.StatusOK,
		func(r *http.Request, a system.GAActor) (any, error) {
			return ga.GetAsset(r.Context(), a, r.PathValue("id"))
		})
	handle("POST", "/assets", "general_affairs.assets.manage", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.CreateGAAssetInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.CreateAsset(r.Context(), a, input)
	})
	handle("PATCH", "/assets/{id}", "general_affairs.assets.manage", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.UpdateGAAssetInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.UpdateAsset(r.Context(), a, r.PathValue("id"), input)
	})
	handle("POST", "/assets/{id}/events", "general_affairs.assets.manage", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.RecordGAAssetEventInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.RecordAssetEvent(r.Context(), a, r.PathValue("id"), input)
	})

	// --- 2.2 Office Supply Requisitions ------------------------------------
	requisitionList := func(scope string) func(*http.Request, system.GAActor) (any, error) {
		return func(r *http.Request, a system.GAActor) (any, error) {
			return listItems(r, func(q system.ListQuery) (any, error) {
				result, err := ga.ListRequisitions(r.Context(), a, scope, q)
				return listResponse("items", result), err
			})
		}
	}
	handle("POST", "/requisitions", "general_affairs.requisitions.submit", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.SubmitGARequisitionInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.SubmitRequisition(r.Context(), a, input)
	})
	handle("GET", "/me/requisitions", "general_affairs.requisitions.read.self", http.StatusOK, requisitionList("mine"))
	handle("GET", "/requisitions", "general_affairs.requisitions.manage", http.StatusOK, requisitionList("all"))
	handleAny("GET", "/requisitions/approvals", none, http.StatusOK, requisitionList("approvals"))
	handleAny("GET", "/requisitions/{id}", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.GetRequisition(r.Context(), a, r.PathValue("id"))
	})
	handleAny("POST", "/requisitions/{id}/decide", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GADecisionInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.DecideRequisition(r.Context(), a, r.PathValue("id"), input)
	})
	handle("POST", "/requisitions/{id}/cancel", "general_affairs.requisitions.submit", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.CancelRequisition(r.Context(), a, r.PathValue("id"))
	})
	handle("POST", "/requisitions/{id}/fulfill", "general_affairs.requisitions.manage", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.FulfillGARequisitionInput
		if err := decodeOptionalJSON(nil, r, &input); err != nil {
			return nil, gaBadRequest(err)
		}
		return ga.FulfillRequisition(r.Context(), a, r.PathValue("id"), input)
	})

	// --- 2.3 Resources, bookings, coordination -----------------------------
	handle("GET", "/resources", "general_affairs.bookings.read", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return listItems(r, func(q system.ListQuery) (any, error) {
			result, err := ga.ListResources(r.Context(), q)
			return listResponse("items", result), err
		})
	})
	handle("POST", "/resources", "general_affairs.bookings.manage", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GAResourceInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.CreateResource(r.Context(), input)
	})
	handle("PATCH", "/resources/{id}", "general_affairs.bookings.manage", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GAResourceInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.UpdateResource(r.Context(), r.PathValue("id"), input)
	})
	handle("GET", "/bookings", "general_affairs.bookings.read", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return listItems(r, func(q system.ListQuery) (any, error) {
			result, err := ga.ListBookings(r.Context(), a, q)
			return listResponse("items", result), err
		})
	})
	handle("GET", "/bookings/{id}", "general_affairs.bookings.read", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.GetBooking(r.Context(), r.PathValue("id"))
	})
	handle("POST", "/bookings", "general_affairs.bookings.book", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.CreateGABookingInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.CreateBooking(r.Context(), a, input)
	})
	bookingEditors := []string{"general_affairs.bookings.book", "general_affairs.bookings.manage"}
	handleAny("PATCH", "/bookings/{id}", bookingEditors, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.UpdateGABookingInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.UpdateBooking(r.Context(), a, r.PathValue("id"), input)
	})
	handleAny("POST", "/bookings/{id}/cancel", bookingEditors, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.CancelBooking(r.Context(), a, r.PathValue("id"))
	})
	handleAny("POST", "/bookings/{id}/return", bookingEditors, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.RecordBookingReturn(r.Context(), a, r.PathValue("id"))
	})
	handle("POST", "/bookings/{id}/response", "general_affairs.bookings.read", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input struct {
			Response string `json:"response"`
		}
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.RespondToInvitation(r.Context(), a, r.PathValue("id"), input.Response)
	})
	handleAny("GET", "/coordination-cases", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		if scope == "" {
			scope = "mine"
		}
		switch scope {
		case "mine":
			if !a.Has("general_affairs.bookings.book") {
				return nil, system.ErrPermissionDenied
			}
		case "all":
			if !a.Has("general_affairs.bookings.coordinate") && !a.Has("general_affairs.bookings.manage") {
				return nil, system.ErrPermissionDenied
			}
		default:
			return nil, gaBadRequest(errors.New("scope must be mine or all"))
		}
		cases, err := ga.ListCoordinationCases(r.Context(), a, scope, strings.TrimSpace(r.URL.Query().Get("status")))
		return map[string]any{"items": cases}, err
	})
	handle("POST", "/coordination-cases", "general_affairs.bookings.coordinate", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.CreateGACoordinationInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.CreateCoordinationCase(r.Context(), a, input)
	})
	handle("POST", "/coordination-cases/{id}/decide", "general_affairs.bookings.book", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input struct {
			Decision string `json:"decision"`
		}
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.DecideCoordinationCase(r.Context(), a, r.PathValue("id"), input.Decision)
	})
	handleAny("POST", "/coordination-cases/{id}/cancel", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.CancelCoordinationCase(r.Context(), a, r.PathValue("id"))
	})

	// --- 2.4 Seals ---------------------------------------------------------
	sealRequestList := func(scope string) func(*http.Request, system.GAActor) (any, error) {
		return func(r *http.Request, a system.GAActor) (any, error) {
			return listItems(r, func(q system.ListQuery) (any, error) {
				result, err := ga.ListSealRequests(r.Context(), a, scope, q)
				return listResponse("items", result), err
			})
		}
	}
	handleAny("GET", "/seals", []string{"general_affairs.seals.request", "general_affairs.seals.manage"}, http.StatusOK,
		func(r *http.Request, a system.GAActor) (any, error) {
			return listItems(r, func(q system.ListQuery) (any, error) {
				result, err := ga.ListSeals(r.Context(), q)
				return listResponse("items", result), err
			})
		})
	handle("POST", "/seals", "general_affairs.seals.manage", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GASealInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.CreateSeal(r.Context(), input)
	})
	handle("PATCH", "/seals/{id}", "general_affairs.seals.manage", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GASealInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.UpdateSeal(r.Context(), r.PathValue("id"), input)
	})
	handle("GET", "/seals/{id}/usage", "general_affairs.seals.manage", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		usage, err := ga.ListSealUsage(r.Context(), r.PathValue("id"))
		return map[string]any{"items": usage}, err
	})
	handle("POST", "/seal-requests", "general_affairs.seals.request", http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.SubmitGASealRequestInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.SubmitSealRequest(r.Context(), a, input)
	})
	handle("GET", "/me/seal-requests", "general_affairs.seals.request", http.StatusOK, sealRequestList("mine"))
	handle("GET", "/seal-requests", "general_affairs.seals.manage", http.StatusOK, sealRequestList("all"))
	handleAny("GET", "/seal-requests/approvals", none, http.StatusOK, sealRequestList("approvals"))
	handleAny("GET", "/seal-requests/{id}", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.GetSealRequest(r.Context(), a, r.PathValue("id"))
	})
	handleAny("POST", "/seal-requests/{id}/decide", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GADecisionInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.DecideSealRequest(r.Context(), a, r.PathValue("id"), input)
	})
	handle("POST", "/seal-requests/{id}/cancel", "general_affairs.seals.request", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.CancelSealRequest(r.Context(), a, r.PathValue("id"))
	})
	handle("POST", "/seal-requests/{id}/use", "general_affairs.seals.manage", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.RecordGASealUseInput
		if err := decodeOptionalJSON(nil, r, &input); err != nil {
			return nil, gaBadRequest(err)
		}
		return ga.RecordSealUse(r.Context(), a, r.PathValue("id"), input)
	})
	handle("POST", "/seal-requests/{id}/return", "general_affairs.seals.manage", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.RecordSealReturn(r.Context(), r.PathValue("id"))
	})

	// --- 2.5 / 2.6 Polls and meal orders -----------------------------------
	pollCreators := []string{"general_affairs.polls.create.organization", "general_affairs.polls.create.group"}
	handleAny("GET", "/polls", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		if scope == "" {
			scope = "respondable"
		}
		required := map[string][]string{
			"respondable": {"general_affairs.polls.respond"},
			"mine":        pollCreators,
			"all":         {"general_affairs.polls.read", "general_affairs.polls.settle"},
		}[scope]
		if required == nil {
			return nil, gaBadRequest(errors.New("scope must be respondable, mine or all"))
		}
		allowed := false
		for _, permission := range required {
			allowed = allowed || a.Has(permission)
		}
		if !allowed {
			return nil, system.ErrPermissionDenied
		}
		return listItems(r, func(q system.ListQuery) (any, error) {
			delete(q.Filters, "scope")
			result, err := ga.ListPolls(r.Context(), a, scope, q)
			return listResponse("items", result), err
		})
	})
	handleAny("POST", "/polls", pollCreators, http.StatusCreated, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GAPollInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.CreatePoll(r.Context(), a, input)
	})
	handleAny("GET", "/polls/{id}", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.GetPoll(r.Context(), a, r.PathValue("id"))
	})
	handleAny("PATCH", "/polls/{id}", pollCreators, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.GAPollInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.UpdatePoll(r.Context(), a, r.PathValue("id"), input)
	})
	handleAny("DELETE", "/polls/{id}", pollCreators, http.StatusNoContent, func(r *http.Request, a system.GAActor) (any, error) {
		return nil, ga.DeletePoll(r.Context(), a, r.PathValue("id"))
	})
	handleAny("POST", "/polls/{id}/open", pollCreators, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.OpenPoll(r.Context(), a, r.PathValue("id"))
	})
	handleAny("POST", "/polls/{id}/close", none, http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.ClosePoll(r.Context(), a, r.PathValue("id"))
	})
	handle("PUT", "/polls/{id}/response", "general_affairs.polls.respond", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		var input system.RespondGAPollInput
		if err := decode(r, &input); err != nil {
			return nil, err
		}
		return ga.RespondToPoll(r.Context(), a, r.PathValue("id"), input)
	})
	handle("POST", "/polls/{id}/settle", "general_affairs.polls.settle", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.SettlePoll(r.Context(), a, r.PathValue("id"))
	})
	handle("GET", "/salary-deductions", "general_affairs.polls.settle", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return listItems(r, func(q system.ListQuery) (any, error) {
			result, err := ga.ListSalaryDeductions(r.Context(), "", q)
			return listResponse("items", result), err
		})
	})
	handle("GET", "/me/salary-deductions", "general_affairs.polls.respond", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return listItems(r, func(q system.ListQuery) (any, error) {
			result, err := ga.ListSalaryDeductions(r.Context(), a.UserID, q)
			return listResponse("items", result), err
		})
	})
	handle("POST", "/salary-deductions/{id}/cancel", "general_affairs.polls.settle", http.StatusOK, func(r *http.Request, a system.GAActor) (any, error) {
		return ga.CancelSalaryDeduction(r.Context(), r.PathValue("id"))
	})
}

func gaBadRequest(err error) error {
	return fmt.Errorf("%w: %v", system.ErrGAInvalid, err)
}

func writeGeneralAffairsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, system.ErrGANotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrGAInvalid), errors.Is(err, system.ErrInvalidListQuery):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrGAConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrPermissionDenied):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrApprovalNotFound), errors.Is(err, system.ErrApprovalInvalidInput),
		errors.Is(err, system.ErrApprovalDecision), errors.Is(err, system.ErrApprovalPermission):
		writeApprovalError(w, err)
	default:
		writeSystemError(w, err)
	}
}
