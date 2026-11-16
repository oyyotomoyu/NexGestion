package apis

import (
	"errors"
	"io"
	"net/http"

	"nexgestion/server/system"
)

// RegisterSchedulingRoutes wires docs/System/scheduling-system.md Section 10.
// Validation overrides additionally require scheduling.override, checked per
// request because the override is optional on the same route.
func RegisterSchedulingRoutes(router *http.ServeMux, scheduling *system.SchedulingService, users *system.UserService, protected func(string, http.HandlerFunc) http.HandlerFunc) {
	handle := func(pattern, permission string, status int, fn func(*http.Request) (any, error)) {
		router.HandleFunc(pattern, protected(permission, func(w http.ResponseWriter, r *http.Request) {
			result, err := fn(r)
			if err != nil {
				writeSchedulingError(w, err)
				return
			}
			if r.Method != http.MethodGet {
				recordRequestLog(r, "info", "scheduling operation "+r.Method+" "+r.URL.Path)
			}
			if result == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			writeJSON(w, status, result)
		}))
	}
	// canOverride rejects an override reason from a caller without
	// scheduling.override instead of silently ignoring it.
	canOverride := func(r *http.Request, reason string) (bool, error) {
		allowed, err := userHasPermission(r.Context(), users, authenticatedUserID(r), "scheduling.override")
		if err != nil {
			return false, err
		}
		if reason != "" && !allowed {
			return false, system.ErrPermissionDenied
		}
		return allowed, nil
	}

	handle("GET /api/scheduling/config", "scheduling.read", http.StatusOK, func(r *http.Request) (any, error) {
		return scheduling.Config(r.Context())
	})
	handle("PATCH /api/scheduling/config", "scheduling.config.manage", http.StatusOK, func(r *http.Request) (any, error) {
		var input system.UpdateSchedulingConfigInput
		if err := decodeSchedulingJSON(r, &input, false); err != nil {
			return nil, err
		}
		return scheduling.UpdateConfig(r.Context(), authenticatedUserID(r), input)
	})

	handle("GET /api/scheduling/groups", "scheduling.manage", http.StatusOK, func(r *http.Request) (any, error) {
		query, err := parseListQuery(r)
		if err != nil {
			return nil, err
		}
		result, err := scheduling.Groups(r.Context(), query)
		if err != nil {
			return nil, err
		}
		return listResponse("groups", result), nil
	})
	handle("GET /api/scheduling/periods", "scheduling.read", http.StatusOK, func(r *http.Request) (any, error) {
		query, err := parseListQuery(r)
		if err != nil {
			return nil, err
		}
		result, err := scheduling.ListPeriods(r.Context(), query)
		if err != nil {
			return nil, err
		}
		return listResponse("periods", result), nil
	})
	handle("POST /api/scheduling/periods", "scheduling.manage", http.StatusCreated, func(r *http.Request) (any, error) {
		var input system.CreateSchedulePeriodInput
		if err := decodeSchedulingJSON(r, &input, false); err != nil {
			return nil, err
		}
		return scheduling.CreatePeriod(r.Context(), authenticatedUserID(r), input)
	})
	handle("GET /api/scheduling/periods/{id}", "scheduling.read", http.StatusOK, func(r *http.Request) (any, error) {
		return scheduling.GetPeriod(r.Context(), r.PathValue("id"))
	})
	handle("PATCH /api/scheduling/periods/{id}", "scheduling.manage", http.StatusOK, func(r *http.Request) (any, error) {
		var input system.UpdateSchedulePeriodInput
		if err := decodeSchedulingJSON(r, &input, false); err != nil {
			return nil, err
		}
		return scheduling.UpdatePeriod(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
	})
	for _, action := range []string{"publish", "lock", "archive", "cancel"} {
		action := action
		handle("POST /api/scheduling/periods/{id}/"+action, "scheduling.publish", http.StatusOK, func(r *http.Request) (any, error) {
			var input struct {
				Reason string `json:"reason"`
			}
			if err := decodeSchedulingJSON(r, &input, true); err != nil {
				return nil, err
			}
			override := false
			if action == "publish" {
				var err error
				if override, err = canOverride(r, input.Reason); err != nil {
					return nil, err
				}
			}
			period, warnings, err := scheduling.TransitionPeriod(r.Context(), authenticatedUserID(r), r.PathValue("id"), action, input.Reason, override)
			if err != nil {
				return nil, err
			}
			return map[string]any{"period": period, "warnings": warnings}, nil
		})
	}
	handle("GET /api/scheduling/periods/{id}/conflicts", "scheduling.read", http.StatusOK, func(r *http.Request) (any, error) {
		conflicts, err := scheduling.Conflicts(r.Context(), r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"conflicts": conflicts}, nil
	})
	handle("GET /api/scheduling/periods/{id}/events", "scheduling.read", http.StatusOK, func(r *http.Request) (any, error) {
		events, err := scheduling.PeriodEvents(r.Context(), r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"events": events}, nil
	})

	handle("POST /api/scheduling/periods/{id}/shifts", "scheduling.manage", http.StatusCreated, func(r *http.Request) (any, error) {
		var input system.CreateScheduleShiftInput
		if err := decodeSchedulingJSON(r, &input, false); err != nil {
			return nil, err
		}
		return scheduling.CreateShift(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
	})
	handle("PATCH /api/scheduling/shifts/{id}", "scheduling.manage", http.StatusOK, func(r *http.Request) (any, error) {
		var input system.UpdateScheduleShiftInput
		if err := decodeSchedulingJSON(r, &input, false); err != nil {
			return nil, err
		}
		override, err := canOverride(r, input.OverrideReason)
		if err != nil {
			return nil, err
		}
		shift, warnings, err := scheduling.UpdateShift(r.Context(), authenticatedUserID(r), r.PathValue("id"), input, override)
		if err != nil {
			return nil, err
		}
		return map[string]any{"shift": shift, "warnings": warnings}, nil
	})
	handle("POST /api/scheduling/shifts/{id}/cancel", "scheduling.manage", http.StatusOK, func(r *http.Request) (any, error) {
		var input struct {
			Reason string `json:"reason"`
		}
		if err := decodeSchedulingJSON(r, &input, true); err != nil {
			return nil, err
		}
		return scheduling.CancelShift(r.Context(), authenticatedUserID(r), r.PathValue("id"), input.Reason)
	})
	handle("GET /api/scheduling/shifts/{id}/candidates", "scheduling.manage", http.StatusOK, func(r *http.Request) (any, error) {
		candidates, err := scheduling.Candidates(r.Context(), r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"candidates": candidates}, nil
	})
	handle("POST /api/scheduling/shifts/{id}/assignments", "scheduling.manage", http.StatusCreated, func(r *http.Request) (any, error) {
		var input system.CreateScheduleAssignmentInput
		if err := decodeSchedulingJSON(r, &input, false); err != nil {
			return nil, err
		}
		override, err := canOverride(r, input.OverrideReason)
		if err != nil {
			return nil, err
		}
		assignment, warnings, err := scheduling.AssignUser(r.Context(), authenticatedUserID(r), r.PathValue("id"), input, override)
		if err != nil {
			return nil, err
		}
		return map[string]any{"assignment": assignment, "warnings": warnings}, nil
	})
	handle("DELETE /api/scheduling/assignments/{id}", "scheduling.manage", http.StatusNoContent, func(r *http.Request) (any, error) {
		return nil, scheduling.CancelAssignment(r.Context(), authenticatedUserID(r), r.PathValue("id"))
	})

	handle("GET /api/scheduling/me/shifts", "scheduling.read.self", http.StatusOK, func(r *http.Request) (any, error) {
		shifts, err := scheduling.MyShifts(r.Context(), authenticatedUserID(r), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"shifts": shifts}, nil
	})
	handle("PATCH /api/scheduling/me/assignments/{id}", "scheduling.read.self", http.StatusOK, func(r *http.Request) (any, error) {
		var input struct {
			AssignmentStatus string `json:"assignment_status"`
			Note             string `json:"note"`
		}
		if err := decodeSchedulingJSON(r, &input, false); err != nil {
			return nil, err
		}
		return scheduling.RespondToAssignment(r.Context(), authenticatedUserID(r), r.PathValue("id"), input.AssignmentStatus, input.Note)
	})
}

// decodeSchedulingJSON decodes a request body; optional bodies may be empty.
func decodeSchedulingJSON(r *http.Request, destination any, optional bool) error {
	if optional && (r.Body == nil || r.ContentLength == 0) {
		return nil
	}
	if err := decodeJSON(nil, r, destination); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return nil
		}
		return errors.Join(system.ErrSchedulingInvalid, err)
	}
	return nil
}

func writeSchedulingError(w http.ResponseWriter, err error) {
	var conflictErr *system.SchedulingConflictError
	if errors.As(err, &conflictErr) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":      "scheduling_conflicts",
			"error":     err.Error(),
			"conflicts": conflictErr.Conflicts,
		})
		return
	}
	code, status := "scheduling_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrSchedulingInvalid), errors.Is(err, system.ErrInvalidListQuery):
		code, status = "scheduling_invalid_input", http.StatusBadRequest
	case errors.Is(err, system.ErrSchedulingNotFound):
		code, status = "scheduling_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrSchedulingState):
		code, status = "scheduling_invalid_state", http.StatusConflict
	case errors.Is(err, system.ErrPermissionDenied):
		code, status = "scheduling_override_forbidden", http.StatusForbidden
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
