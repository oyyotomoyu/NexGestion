package apis

import (
	"errors"
	"net/http"
	"strings"

	"nexgestion/server/system"
)

// hrActor resolves the caller's effective permissions. HR visibility depends
// on reporting line and case assignment as well as the route permission, so
// the service receives the full permission set.
func hrActor(r *http.Request, users *system.UserService) (system.HRActor, error) {
	userID := authenticatedUserID(r)
	keys, err := users.EffectivePermissionKeys(r.Context(), userID)
	if err != nil {
		return system.HRActor{}, err
	}
	return system.NewHRActor(userID, keys), nil
}

// hrHandler wraps the common actor-resolution step for HR endpoints.
func hrHandler(users *system.UserService, handle func(w http.ResponseWriter, r *http.Request, actor system.HRActor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, err := hrActor(r, users)
		if err != nil {
			writeHRError(w, err)
			return
		}
		handle(w, r, actor)
	}
}

func decodeHRInput(w http.ResponseWriter, r *http.Request, destination any) bool {
	if err := decodeJSON(w, r, destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
		return false
	}
	return true
}

func hrListQuery(w http.ResponseWriter, r *http.Request) (system.ListQuery, bool) {
	query, err := parseListQuery(r)
	if err != nil {
		writeListQueryError(w, err)
		return query, false
	}
	return query, true
}

// --- Settings ---

func getHRSettings(hr *system.HRService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := hr.Settings(r.Context())
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func updateHRSettings(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.UpdateHRSettingsInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		settings, err := hr.UpdateSettings(r.Context(), actor, input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated hr settings")
		writeJSON(w, http.StatusOK, settings)
	})
}

// --- Employment lifecycle ---

func getEmploymentRecord(hr *system.HRService, self bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.PathValue("userId")
		if self {
			userID = authenticatedUserID(r)
		}
		record, err := hr.EmploymentRecord(r.Context(), userID)
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	}
}

func recordEmploymentEvent(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.RecordEmploymentEventInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		userID := r.PathValue("userId")
		event, err := hr.RecordEmploymentEvent(r.Context(), actor, userID, input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "recorded employment event "+event.EventType+" "+event.ID+" for user "+userID)
		writeJSON(w, http.StatusCreated, event)
	})
}

func listHRTasks(hr *system.HRService, self bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, ok := hrListQuery(w, r)
		if !ok {
			return
		}
		employeeID, assigneeID := r.PathValue("userId"), ""
		if self {
			employeeID, assigneeID = "", authenticatedUserID(r)
		}
		tasks, err := hr.ListTasks(r.Context(), employeeID, assigneeID, query)
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("tasks", tasks))
	}
}

func createHRTask(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.CreateHRTaskInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		task, err := hr.CreateTask(r.Context(), actor, r.PathValue("userId"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "created hr task "+task.ID)
		writeJSON(w, http.StatusCreated, task)
	})
}

func updateHRTask(hr *system.HRService, users *system.UserService, asAssignee bool) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.UpdateHRTaskInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		task, err := hr.UpdateTask(r.Context(), actor, r.PathValue("id"), input, asAssignee)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated hr task "+task.ID)
		writeJSON(w, http.StatusOK, task)
	})
}

// --- Performance ---

func listPerformanceCycles(hr *system.HRService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, ok := hrListQuery(w, r)
		if !ok {
			return
		}
		cycles, err := hr.ListPerformanceCycles(r.Context(), query)
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("cycles", cycles))
	}
}

func createPerformanceCycle(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.CreatePerformanceCycleInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		cycle, err := hr.CreatePerformanceCycle(r.Context(), actor, input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "created performance cycle "+cycle.ID)
		writeJSON(w, http.StatusCreated, cycle)
	})
}

func updatePerformanceCycle(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.UpdatePerformanceCycleInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		cycle, err := hr.UpdatePerformanceCycle(r.Context(), actor, r.PathValue("id"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated performance cycle "+cycle.ID)
		writeJSON(w, http.StatusOK, cycle)
	})
}

func listPerformanceGoals(hr *system.HRService, users *system.UserService, self bool) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		query, ok := hrListQuery(w, r)
		if !ok {
			return
		}
		userID := r.PathValue("userId")
		if self {
			userID = actor.UserID
		}
		goals, err := hr.ListPerformanceGoals(r.Context(), actor, userID, query)
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("goals", goals))
	})
}

func createPerformanceGoal(hr *system.HRService, users *system.UserService, self bool) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.CreatePerformanceGoalInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		userID := r.PathValue("userId")
		if self {
			userID = actor.UserID
		}
		goal, err := hr.CreatePerformanceGoal(r.Context(), actor, userID, input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "created performance goal "+goal.ID)
		writeJSON(w, http.StatusCreated, goal)
	})
}

func updatePerformanceGoal(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.UpdatePerformanceGoalInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		goal, err := hr.UpdatePerformanceGoal(r.Context(), actor, r.PathValue("id"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated performance goal "+goal.ID)
		writeJSON(w, http.StatusOK, goal)
	})
}

func listPerformanceReviews(hr *system.HRService, users *system.UserService, self bool) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		userID := r.PathValue("userId")
		if self {
			userID = actor.UserID
		}
		reviews, err := hr.ListPerformanceReviews(r.Context(), actor, userID)
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews})
	})
}

func submitSelfReview(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.SubmitSelfReviewInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		review, err := hr.SubmitSelfReview(r.Context(), actor, r.PathValue("cycleId"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "submitted self review "+review.ID)
		writeJSON(w, http.StatusOK, review)
	})
}

func submitManagerReview(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.SubmitManagerReviewInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		review, err := hr.SubmitManagerReview(r.Context(), actor, r.PathValue("userId"), r.PathValue("cycleId"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "submitted manager review "+review.ID)
		writeJSON(w, http.StatusOK, review)
	})
}

func finalizePerformanceReview(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		review, err := hr.FinalizePerformanceReview(r.Context(), actor, r.PathValue("userId"), r.PathValue("cycleId"))
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "finalized performance review "+review.ID)
		writeJSON(w, http.StatusOK, review)
	})
}

func correctPerformanceReview(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.CorrectPerformanceReviewInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		review, err := hr.CorrectPerformanceReview(r.Context(), actor, r.PathValue("id"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "corrected performance review "+review.ID)
		writeJSON(w, http.StatusCreated, review)
	})
}

// --- Employee Relations ---

func listMyEmployeeRelationsCases(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		cases, err := hr.ListMyEmployeeRelationsCases(r.Context(), actor)
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cases": cases})
	})
}

func fileGrievance(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.FileGrievanceInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		item, err := hr.FileGrievance(r.Context(), actor, input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		// Pointer-only log entry: never record case content.
		recordRequestLog(r, "info", "filed employee relations case "+item.ID)
		writeJSON(w, http.StatusCreated, item)
	})
}

func listEmployeeRelationsCases(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		query, ok := hrListQuery(w, r)
		if !ok {
			return
		}
		cases, err := hr.ListEmployeeRelationsCases(r.Context(), actor, query)
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("cases", cases))
	})
}

func createEmployeeRelationsCase(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.CreateEmployeeRelationsCaseInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		item, err := hr.CreateEmployeeRelationsCase(r.Context(), actor, input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "created employee relations case "+item.ID)
		writeJSON(w, http.StatusCreated, item)
	})
}

func getEmployeeRelationsCase(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		item, err := hr.GetEmployeeRelationsCase(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			writeHRError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
}

func updateEmployeeRelationsCase(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.UpdateEmployeeRelationsCaseInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		item, err := hr.UpdateEmployeeRelationsCase(r.Context(), actor, r.PathValue("id"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated employee relations case "+item.ID)
		writeJSON(w, http.StatusOK, item)
	})
}

func transitionEmployeeRelationsCase(hr *system.HRService, users *system.UserService) http.HandlerFunc {
	return hrHandler(users, func(w http.ResponseWriter, r *http.Request, actor system.HRActor) {
		var input system.TransitionEmployeeRelationsCaseInput
		if !decodeHRInput(w, r, &input) {
			return
		}
		item, err := hr.TransitionEmployeeRelationsCase(r.Context(), actor, r.PathValue("id"), input)
		if err != nil {
			writeHRError(w, err)
			return
		}
		recordRequestLog(r, "info", "employee relations case "+item.ID+" action "+input.Action)
		writeJSON(w, http.StatusOK, item)
	})
}

func writeHRError(w http.ResponseWriter, err error) {
	code, status := "hr_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrHRNotFound), errors.Is(err, system.ErrUserNotFound), errors.Is(err, system.ErrGroupNotFound):
		code, status = "hr_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrHRForbidden):
		code, status = "hr_forbidden", http.StatusForbidden
	case errors.Is(err, system.ErrHRInvalidTransition):
		code, status = "hr_invalid_transition", http.StatusConflict
	case errors.Is(err, system.ErrHRInvalidInput), errors.Is(err, system.ErrInvalidListQuery):
		code, status = "hr_invalid_input", http.StatusBadRequest
	case strings.Contains(err.Error(), "UNIQUE constraint failed"):
		code, status = "hr_conflict", http.StatusConflict
	case strings.Contains(err.Error(), "protected user"), strings.Contains(err.Error(), "employee_number is required"):
		code, status = "hr_invalid_input", http.StatusBadRequest
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
