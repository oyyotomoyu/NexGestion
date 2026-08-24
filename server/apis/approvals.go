package apis

import (
	"errors"
	"net/http"

	"nexgestion/server/system"
)

func listFlowTemplates(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		templates, err := approvals.ListFlowTemplates(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("flow_templates", templates))
	}
}

func createFlowTemplate(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateFlowTemplateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		template, err := approvals.CreateFlowTemplate(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		recordRequestLog(r, "info", "created approval flow template "+template.ID)
		writeJSON(w, http.StatusCreated, template)
	}
}

func getFlowTemplate(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		template, err := approvals.GetFlowTemplate(r.Context(), r.PathValue("id"))
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, template)
	}
}

func updateFlowTemplate(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateFlowTemplateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		template, err := approvals.UpdateFlowTemplate(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated approval flow template "+template.ID)
		writeJSON(w, http.StatusOK, template)
	}
}

func deleteFlowTemplate(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := approvals.DeleteFlowTemplate(r.Context(), authenticatedUserID(r), id); err != nil {
			writeApprovalError(w, err)
			return
		}
		recordRequestLog(r, "info", "deleted approval flow template "+id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func submitApprovalRequest(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.SubmitApprovalRequestInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		request, err := approvals.SubmitRequest(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		recordRequestLog(r, "info", "submitted approval request "+request.ID)
		writeJSON(w, http.StatusCreated, request)
	}
}

func listApprovalRequests(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		requests, err := approvals.ListRequests(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("approval_requests", requests))
	}
}

func listMyApprovalRequests(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		requests, err := approvals.ListMyRequests(r.Context(), authenticatedUserID(r), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("approval_requests", requests))
	}
}

func listMyApprovalAssignments(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		requests, err := approvals.ListMyAssignments(r.Context(), authenticatedUserID(r), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeApprovalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("approval_requests", requests))
	}
}

// getApprovalRequest is record-scoped: a requester or an assigned approver
// may always view their own request (approvals.read.self); anyone else needs
// the administrative approvals.read permission. See
// docs/System/approval-system.md Section 5.
func getApprovalRequest(approvals *system.ApprovalService, users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, err := approvals.GetRequest(r.Context(), r.PathValue("id"))
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		userID := authenticatedUserID(r)
		if !request.InvolvesUser(userID) {
			allowed, err := userHasPermission(r.Context(), users, userID, "approvals.read")
			if err != nil {
				writeApprovalError(w, err)
				return
			}
			if !allowed {
				writeJSON(w, http.StatusNotFound, map[string]string{"code": "approval_not_found", "error": system.ErrApprovalNotFound.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, request)
	}
}

func cancelApprovalRequest(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, err := approvals.CancelRequest(r.Context(), authenticatedUserID(r), r.PathValue("id"))
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		recordRequestLog(r, "info", "cancelled approval request "+request.ID)
		writeJSON(w, http.StatusOK, request)
	}
}

func decideApprovalRequest(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.DecideApprovalInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		request, err := approvals.DecideRequest(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		recordRequestLog(r, "info", "decided approval request "+request.ID+" as "+input.Decision)
		writeJSON(w, http.StatusOK, request)
	}
}

func reassignApprovalRequest(approvals *system.ApprovalService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.ReassignApprovalInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		request, err := approvals.ReassignApprover(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		recordRequestLog(r, "info", "reassigned approval request "+request.ID)
		writeJSON(w, http.StatusOK, request)
	}
}

func writeApprovalError(w http.ResponseWriter, err error) {
	code, status := "approval_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrApprovalNotFound), errors.Is(err, system.ErrUserNotFound),
		errors.Is(err, system.ErrRoleNotFound), errors.Is(err, system.ErrGroupNotFound):
		code, status = "approval_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrApprovalPermission):
		code, status = "approval_permission_denied", http.StatusForbidden
	case errors.Is(err, system.ErrApprovalTemplateInUse):
		code, status = "approval_template_in_use", http.StatusConflict
	case errors.Is(err, system.ErrApprovalDecision):
		code, status = "approval_decision_invalid", http.StatusConflict
	case errors.Is(err, system.ErrApprovalInvalidInput), errors.Is(err, system.ErrInvalidListQuery):
		code, status = "approval_invalid_input", http.StatusBadRequest
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
