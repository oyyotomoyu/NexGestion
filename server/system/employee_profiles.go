package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EmploymentProfileChange is the UserSystem-side half of an HR employment
// event (docs/System/hr-system.md Section 3). HR owns the narrative history;
// employee_profiles stays the source of truth for the current status.
type EmploymentProfileChange struct {
	EmploymentStatus string
	// EmployeeNumber is required when the user has no employee profile yet
	// (onboarding creates one per user-system.md Section 3).
	EmployeeNumber   string
	JobTitle         *string
	HireDate         *string
	TerminationDate  *string
	ClearTermination bool
	// Offboard disables login, revokes sessions, and ends active group
	// memberships via user_groups.left_at (Section 3.7 steps 3-4).
	Offboard bool
	// Reenable restores login for a rehired employee.
	Reenable bool
}

// ApplyEmploymentChange updates (or, for onboarding, creates) the employee
// profile and applies the matching account-side effects in one transaction.
func (s *UserService) ApplyEmploymentChange(ctx context.Context, userID string, change EmploymentProfileChange) (*User, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	user, err := getUser(ctx, db, userID)
	if err != nil {
		return nil, err
	}
	if user.IsProtected && (change.Offboard || change.EmploymentStatus == "suspended" || change.EmploymentStatus == "terminated") {
		return nil, errors.New("protected user cannot be suspended or terminated")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if user.EmployeeProfile == nil {
		number := strings.TrimSpace(change.EmployeeNumber)
		if number == "" {
			return nil, errors.New("employee_number is required to create an employee profile")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO employee_profiles
			(id, user_id, employee_number, job_title, employment_status, hire_date, termination_date, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), userID, number, normalizeOptionalText(change.JobTitle), change.EmploymentStatus,
			change.HireDate, change.TerminationDate, now, now); err != nil {
			return nil, fmt.Errorf("create employee profile: %w", err)
		}
	} else {
		sets, args := []string{"employment_status = ?", "updated_at = ?"}, []any{change.EmploymentStatus, now}
		if change.JobTitle != nil {
			sets = append(sets, "job_title = ?")
			args = append(args, normalizeOptionalText(change.JobTitle))
		}
		if change.HireDate != nil {
			sets = append(sets, "hire_date = ?")
			args = append(args, *change.HireDate)
		}
		if change.TerminationDate != nil {
			sets = append(sets, "termination_date = ?")
			args = append(args, *change.TerminationDate)
		} else if change.ClearTermination {
			sets = append(sets, "termination_date = NULL")
		}
		args = append(args, userID)
		if _, err := tx.ExecContext(ctx, `UPDATE employee_profiles SET `+strings.Join(sets, ", ")+` WHERE user_id = ?`, args...); err != nil {
			return nil, fmt.Errorf("update employee profile: %w", err)
		}
	}

	if change.Offboard {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET status = 'disabled', updated_at = ? WHERE id = ?`, now, userID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, now, userID); err != nil {
			return nil, err
		}
		// Group-derived roles go with the membership; the membership row
		// itself is kept with left_at so history is preserved.
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = ? AND role_id IN (
			SELECT gr.role_id FROM group_roles gr JOIN user_groups ug ON ug.group_id = gr.group_id
			WHERE ug.user_id = ? AND ug.left_at IS NULL)`, userID, userID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE user_groups SET left_at = ?, is_primary_organization = 0
			WHERE user_id = ? AND left_at IS NULL`, now, userID); err != nil {
			return nil, err
		}
	}
	if change.Reenable {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET status = 'active', updated_at = ? WHERE id = ?`, now, userID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getUser(ctx, db, userID)
}

// ManagesUser reports whether managerID is a manager of employeeID's primary
// organization group or any of its ancestors - the reporting line resolved
// the same way as leave approval (group-system.md Section 5), extended to
// indirect reports.
func (s *UserService) ManagesUser(ctx context.Context, managerID, employeeID string) (bool, error) {
	if managerID == "" || managerID == employeeID {
		return false, nil
	}
	db, err := s.open()
	if err != nil {
		return false, err
	}
	defer db.Close()
	var groupID string
	err = db.QueryRowContext(ctx, `SELECT g.id FROM user_groups ug
		JOIN groups g ON g.id=ug.group_id
		WHERE ug.user_id=? AND ug.left_at IS NULL AND ug.is_primary_organization=1
			AND g.type='organization' AND g.status='active'`, employeeID).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	visited := map[string]bool{}
	for groupID != "" && !visited[groupID] {
		visited[groupID] = true
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM group_roles gr
			JOIN user_roles ur ON ur.role_id=gr.role_id
			WHERE gr.group_id=? AND gr.kind='manager' AND ur.user_id=?`, groupID, managerID).Scan(&count); err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
		var parent *string
		if err := db.QueryRowContext(ctx, `SELECT parent_group_id FROM groups WHERE id=?`, groupID).Scan(&parent); err != nil {
			return false, err
		}
		if parent == nil {
			break
		}
		groupID = *parent
	}
	return false, nil
}
