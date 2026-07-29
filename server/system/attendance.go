package system

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	AttendanceNonWorking = "non_working"
	AttendanceWorking    = "working"
	defaultAttendanceTZ  = "Asia/Taipei"
)

var (
	ErrAttendanceConflict  = errors.New("attendance action conflicts with current status")
	ErrAttendanceNotFound  = errors.New("attendance record not found")
	ErrAttendanceMonth     = errors.New("invalid attendance month")
	ErrAttendanceMonthOpen = errors.New("attendance month is still open")
	ErrAttendanceExport    = errors.New("attendance CSV report not found")
)

type AttendanceSession struct {
	ID                     string  `json:"id"`
	SequenceNumber         int     `json:"sequence_number"`
	ContinuedFromSessionID *string `json:"continued_from_session_id"`
	SignInAt               string  `json:"sign_in_at"`
	SignOutAt              *string `json:"sign_out_at"`
	WorkedMinutes          *int    `json:"worked_minutes"`
}

type AttendanceDay struct {
	ID             string              `json:"id"`
	UserID         string              `json:"user_id"`
	AttendanceDate string              `json:"attendance_date"`
	Timezone       string              `json:"timezone"`
	Status         string              `json:"status"`
	WorkedHours    float64             `json:"worked_hours"`
	WorkedMinutes  int                 `json:"worked_minutes"`
	RequiresReview bool                `json:"requires_review"`
	Sessions       []AttendanceSession `json:"sessions"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

type AttendanceMonthlyReport struct {
	ID                string  `json:"id"`
	UserID            string  `json:"user_id"`
	EmployeeNumber    string  `json:"employee_number"`
	DisplayName       string  `json:"display_name"`
	Timezone          string  `json:"timezone"`
	ReportMonth       string  `json:"report_month"`
	ScheduledWorkDays int     `json:"scheduled_work_days"`
	PresentDays       int     `json:"present_days"`
	AbsentDays        int     `json:"absent_days"`
	IncompleteDays    int     `json:"incomplete_days"`
	WorkedHours       float64 `json:"worked_hours"`
	WorkedMinutes     int     `json:"worked_minutes"`
	GeneratedAt       string  `json:"generated_at"`
}

type AttendanceExport struct {
	ReportMonth  string `json:"report_month"`
	RelativePath string `json:"relative_path"`
	SHA256       string `json:"sha256"`
	RowCount     int    `json:"row_count"`
	GeneratedAt  string `json:"generated_at"`
}

type CorrectAttendanceSessionInput struct {
	SignInAt  string  `json:"sign_in_at"`
	SignOutAt *string `json:"sign_out_at"`
}

type CorrectAttendanceDayInput struct {
	Reason   string                          `json:"reason"`
	Sessions []CorrectAttendanceSessionInput `json:"sessions"`
}

type AttendanceService struct {
	databasePath string
	reportRoot   string
	users        *UserService
	now          func() time.Time
}

func NewAttendanceService(databaseDirectory, reportDirectory string, users *UserService) *AttendanceService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	if strings.TrimSpace(reportDirectory) == "" {
		reportDirectory = filepath.Join("reports", "attendance")
	}
	return &AttendanceService{
		databasePath: filepath.Join(databaseDirectory, "attendance.db"),
		reportRoot:   reportDirectory,
		users:        users,
		now:          time.Now,
	}
}

func (s *AttendanceService) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *AttendanceService) Today(ctx context.Context, userID string) (*AttendanceDay, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := attendanceMinute(s.now())
	timezone := attendanceTimezone(user)
	timezone, err = reconcileAttendance(ctx, tx, userID, timezone, now)
	if err != nil {
		return nil, err
	}
	date, err := attendanceDate(now, timezone)
	if err != nil {
		return nil, err
	}
	day, err := getOrCreateAttendanceDay(ctx, tx, userID, date, timezone, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getDay(ctx, db, day.ID)
}

func (s *AttendanceService) SignIn(ctx context.Context, userID string) (*AttendanceDay, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := attendanceMinute(s.now())
	timezone, err := reconcileAttendance(ctx, tx, userID, attendanceTimezone(user), now)
	if err != nil {
		return nil, err
	}
	date, err := attendanceDate(now, timezone)
	if err != nil {
		return nil, err
	}
	day, err := getOrCreateAttendanceDay(ctx, tx, userID, date, timezone, now)
	if err != nil {
		return nil, err
	}
	if day.Status != AttendanceNonWorking {
		return nil, ErrAttendanceConflict
	}
	var openCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM attendance_sessions s
		JOIN attendance_days d ON d.id=s.attendance_day_id
		WHERE d.user_id=? AND s.sign_out_at IS NULL`, userID).Scan(&openCount); err != nil {
		return nil, err
	}
	if openCount != 0 {
		return nil, ErrAttendanceConflict
	}
	var sequence int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence_number),0)+1 FROM attendance_sessions WHERE attendance_day_id=?`, day.ID).Scan(&sequence); err != nil {
		return nil, err
	}
	sessionID, stamp := uuid.NewString(), formatAttendanceTime(now)
	if _, err := tx.ExecContext(ctx, `INSERT INTO attendance_sessions
		(id,attendance_day_id,sequence_number,sign_in_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?)`, sessionID, day.ID, sequence, stamp, stamp, stamp); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attendance_days SET status=?,updated_at=? WHERE id=?`, AttendanceWorking, stamp, day.ID); err != nil {
		return nil, err
	}
	if err := insertAttendanceEvent(ctx, tx, day.ID, &sessionID, userID, userID, "sign_in", now, AttendanceNonWorking, AttendanceWorking, nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getDay(ctx, db, day.ID)
}

func (s *AttendanceService) SignOut(ctx context.Context, userID string) (*AttendanceDay, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := attendanceMinute(s.now())
	if _, err := reconcileAttendance(ctx, tx, userID, attendanceTimezone(user), now); err != nil {
		return nil, err
	}
	var sessionID, dayID, signInValue string
	err = tx.QueryRowContext(ctx, `SELECT s.id,d.id,s.sign_in_at FROM attendance_sessions s
		JOIN attendance_days d ON d.id=s.attendance_day_id
		WHERE d.user_id=? AND s.sign_out_at IS NULL`, userID).Scan(&sessionID, &dayID, &signInValue)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAttendanceConflict
	}
	if err != nil {
		return nil, err
	}
	signIn, err := parseAttendanceTime(signInValue)
	if err != nil || now.Before(signIn) {
		return nil, ErrAttendanceConflict
	}
	minutes := int(now.Sub(signIn) / time.Minute)
	stamp := formatAttendanceTime(now)
	if _, err := tx.ExecContext(ctx, `UPDATE attendance_sessions
		SET sign_out_at=?,worked_minutes=?,updated_at=? WHERE id=? AND sign_out_at IS NULL`,
		stamp, minutes, stamp, sessionID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attendance_days
		SET status=?,total_worked_minutes=total_worked_minutes+?,updated_at=? WHERE id=?`,
		AttendanceNonWorking, minutes, stamp, dayID); err != nil {
		return nil, err
	}
	if err := insertAttendanceEvent(ctx, tx, dayID, &sessionID, userID, userID, "sign_out", now, AttendanceWorking, AttendanceNonWorking, nil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getDay(ctx, db, dayID)
}

func (s *AttendanceService) ListDays(ctx context.Context, userID, month string) ([]AttendanceDay, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query := `SELECT id FROM attendance_days WHERE user_id=?`
	args := []any{userID}
	if month != "" {
		if _, err := parseAttendanceMonth(month); err != nil {
			return nil, err
		}
		query += ` AND attendance_date LIKE ?`
		args = append(args, month+"-%")
	}
	query += ` ORDER BY attendance_date DESC`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]AttendanceDay, 0, len(ids))
	for _, id := range ids {
		day, err := s.getDay(ctx, db, id)
		if err != nil {
			return nil, err
		}
		result = append(result, *day)
	}
	return result, nil
}

func (s *AttendanceService) CorrectDay(ctx context.Context, actorUserID, dayID string, input CorrectAttendanceDayInput) (*AttendanceDay, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, errors.New("attendance correction reason is required")
	}
	type correctedSession struct {
		signIn  time.Time
		signOut *time.Time
		minutes *int
	}
	sessions := make([]correctedSession, 0, len(input.Sessions))
	var previousEnd *time.Time
	openSeen := false
	total := 0
	for index, candidate := range input.Sessions {
		signIn, err := parseAttendanceTime(candidate.SignInAt)
		if err != nil || signIn.Second() != 0 || signIn.Nanosecond() != 0 {
			return nil, errors.New("attendance session sign_in_at must be an ISO 8601 minute")
		}
		if previousEnd != nil && signIn.Before(*previousEnd) {
			return nil, errors.New("attendance sessions cannot overlap")
		}
		session := correctedSession{signIn: signIn}
		if candidate.SignOutAt == nil {
			if openSeen || index != len(input.Sessions)-1 {
				return nil, errors.New("only the final attendance session may be open")
			}
			openSeen = true
		} else {
			signOut, err := parseAttendanceTime(*candidate.SignOutAt)
			if err != nil || signOut.Second() != 0 || signOut.Nanosecond() != 0 || signOut.Before(signIn) {
				return nil, errors.New("attendance session sign_out_at must be a valid ISO 8601 minute")
			}
			minutes := int(signOut.Sub(signIn) / time.Minute)
			session.signOut, session.minutes = &signOut, &minutes
			total += minutes
			previousEnd = &signOut
		}
		sessions = append(sessions, session)
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var userID, previousStatus, attendanceDateValue, timezone string
	if err := tx.QueryRowContext(ctx, `SELECT user_id,status,attendance_date,timezone
		FROM attendance_days WHERE id=?`, dayID).Scan(
		&userID, &previousStatus, &attendanceDateValue, &timezone,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAttendanceNotFound
	} else if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	dayStart, err := time.ParseInLocation("2006-01-02", attendanceDateValue, location)
	if err != nil {
		return nil, err
	}
	dayStart, dayEnd := dayStart.UTC(), dayStart.AddDate(0, 0, 1).UTC()
	for _, session := range sessions {
		if session.signIn.Before(dayStart) || !session.signIn.Before(dayEnd) ||
			(session.signOut != nil && session.signOut.After(dayEnd)) {
			return nil, errors.New("attendance session must remain within its attendance day")
		}
	}
	if openSeen {
		currentDate, err := attendanceDate(attendanceMinute(s.now()), timezone)
		if err != nil {
			return nil, err
		}
		if currentDate != attendanceDateValue {
			return nil, errors.New("only the current attendance day may contain an open session")
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM attendance_sessions WHERE attendance_day_id=?`, dayID); err != nil {
		return nil, err
	}
	stamp := formatAttendanceTime(s.now())
	for index, session := range sessions {
		var signOut any
		if session.signOut != nil {
			signOut = formatAttendanceTime(*session.signOut)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO attendance_sessions
			(id,attendance_day_id,sequence_number,sign_in_at,sign_out_at,worked_minutes,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?)`, uuid.NewString(), dayID, index+1, formatAttendanceTime(session.signIn),
			signOut, session.minutes, stamp, stamp); err != nil {
			return nil, err
		}
	}
	status := AttendanceNonWorking
	review := 0
	if openSeen {
		status, review = AttendanceWorking, 1
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attendance_days SET status=?,total_worked_minutes=?,
		requires_review=?,updated_at=? WHERE id=?`, status, total, review, stamp, dayID); err != nil {
		return nil, err
	}
	if err := insertAttendanceEvent(ctx, tx, dayID, nil, userID, actorUserID, "correction",
		attendanceMinute(s.now()), previousStatus, status, &reason); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getDay(ctx, db, dayID)
}

func (s *AttendanceService) getDay(ctx context.Context, db *sql.DB, id string) (*AttendanceDay, error) {
	var day AttendanceDay
	var review int
	err := db.QueryRowContext(ctx, `SELECT id,user_id,attendance_date,timezone,status,total_worked_minutes,
		requires_review,created_at,updated_at FROM attendance_days WHERE id=?`, id).Scan(
		&day.ID, &day.UserID, &day.AttendanceDate, &day.Timezone, &day.Status, &day.WorkedMinutes,
		&review, &day.CreatedAt, &day.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAttendanceNotFound
	}
	if err != nil {
		return nil, err
	}
	day.RequiresReview = review == 1
	day.WorkedHours = roundedHours(day.WorkedMinutes)
	day.Sessions = []AttendanceSession{}
	rows, err := db.QueryContext(ctx, `SELECT id,sequence_number,continued_from_session_id,sign_in_at,sign_out_at,worked_minutes
		FROM attendance_sessions WHERE attendance_day_id=? ORDER BY sequence_number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var session AttendanceSession
		if err := rows.Scan(&session.ID, &session.SequenceNumber, &session.ContinuedFromSessionID,
			&session.SignInAt, &session.SignOutAt, &session.WorkedMinutes); err != nil {
			return nil, err
		}
		day.Sessions = append(day.Sessions, session)
	}
	return &day, rows.Err()
}

type attendanceDayRow struct {
	ID     string
	Status string
}

func getOrCreateAttendanceDay(ctx context.Context, tx *sql.Tx, userID, date, timezone string, now time.Time) (*attendanceDayRow, error) {
	var day attendanceDayRow
	err := tx.QueryRowContext(ctx, `SELECT id,status FROM attendance_days WHERE user_id=? AND attendance_date=?`, userID, date).Scan(&day.ID, &day.Status)
	if err == nil {
		return &day, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	day = attendanceDayRow{ID: uuid.NewString(), Status: AttendanceNonWorking}
	stamp := formatAttendanceTime(now)
	if _, err := tx.ExecContext(ctx, `INSERT INTO attendance_days
		(id,user_id,attendance_date,timezone,status,total_worked_minutes,requires_review,created_at,updated_at)
		VALUES(?,?,?,?,?,0,0,?,?)`, day.ID, userID, date, timezone, day.Status, stamp, stamp); err != nil {
		return nil, err
	}
	return &day, nil
}

func reconcileAttendance(ctx context.Context, tx *sql.Tx, userID, fallbackTimezone string, now time.Time) (string, error) {
	var sessionID, dayID, signInValue, timezone string
	err := tx.QueryRowContext(ctx, `SELECT s.id,d.id,s.sign_in_at,d.timezone FROM attendance_sessions s
		JOIN attendance_days d ON d.id=s.attendance_day_id
		WHERE d.user_id=? AND s.sign_out_at IS NULL`, userID).Scan(&sessionID, &dayID, &signInValue, &timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return fallbackTimezone, nil
	}
	if err != nil {
		return "", err
	}
	signIn, err := parseAttendanceTime(signInValue)
	if err != nil {
		return "", err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", err
	}
	for {
		local := signIn.In(location)
		nextMidnightLocal := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location)
		boundary := nextMidnightLocal.UTC()
		if boundary.After(now) {
			break
		}
		minutes := int(boundary.Sub(signIn) / time.Minute)
		stamp := formatAttendanceTime(boundary)
		if _, err := tx.ExecContext(ctx, `UPDATE attendance_sessions SET sign_out_at=?,worked_minutes=?,updated_at=? WHERE id=?`,
			stamp, minutes, stamp, sessionID); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE attendance_days SET status=?,total_worked_minutes=total_worked_minutes+?,updated_at=? WHERE id=?`,
			AttendanceNonWorking, minutes, stamp, dayID); err != nil {
			return "", err
		}
		nextDate := nextMidnightLocal.Format("2006-01-02")
		nextDay, err := getOrCreateAttendanceDay(ctx, tx, userID, nextDate, timezone, boundary)
		if err != nil {
			return "", err
		}
		var sequence int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence_number),0)+1 FROM attendance_sessions WHERE attendance_day_id=?`, nextDay.ID).Scan(&sequence); err != nil {
			return "", err
		}
		nextSessionID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO attendance_sessions
			(id,attendance_day_id,sequence_number,continued_from_session_id,sign_in_at,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?)`, nextSessionID, nextDay.ID, sequence, sessionID, stamp, stamp, stamp); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE attendance_days SET status=?,updated_at=? WHERE id=?`,
			AttendanceWorking, stamp, nextDay.ID); err != nil {
			return "", err
		}
		if err := insertAttendanceEvent(ctx, tx, nextDay.ID, &nextSessionID, userID, userID,
			"midnight_rollover", boundary, AttendanceWorking, AttendanceWorking, nil); err != nil {
			return "", err
		}
		sessionID, dayID, signIn = nextSessionID, nextDay.ID, boundary
	}
	return timezone, nil
}

func insertAttendanceEvent(ctx context.Context, tx *sql.Tx, dayID string, sessionID *string, userID, actorID, eventType string, occurred time.Time, previous, next string, reason *string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO attendance_events
		(id,attendance_day_id,attendance_session_id,user_id,actor_user_id,event_type,occurred_at,previous_status,new_status,reason)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		uuid.NewString(), dayID, sessionID, userID, actorID, eventType, formatAttendanceTime(occurred), previous, next, reason)
	return err
}

func attendanceTimezone(user *User) string {
	if user != nil && user.Timezone != nil {
		value := strings.TrimSpace(*user.Timezone)
		if value != "" {
			if _, err := time.LoadLocation(value); err == nil {
				return value
			}
		}
	}
	return defaultAttendanceTZ
}

func attendanceDate(now time.Time, timezone string) (string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", err
	}
	return now.In(location).Format("2006-01-02"), nil
}

func attendanceMinute(value time.Time) time.Time {
	return value.UTC().Truncate(time.Minute)
}

func formatAttendanceTime(value time.Time) string {
	return attendanceMinute(value).Format(time.RFC3339)
}

func parseAttendanceTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339, value)
}

func roundedHours(minutes int) float64 {
	value, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", float64(minutes)/60), 64)
	return value
}

func parseAttendanceMonth(value string) (time.Time, error) {
	if len(value) != 7 {
		return time.Time{}, ErrAttendanceMonth
	}
	result, err := time.Parse("2006-01", value)
	if err != nil {
		return time.Time{}, ErrAttendanceMonth
	}
	return result, nil
}

func (s *AttendanceService) GenerateMonthlyCSV(ctx context.Context, month string) (*AttendanceExport, error) {
	monthTime, err := parseAttendanceMonth(month)
	if err != nil {
		return nil, err
	}
	users, err := s.users.List(ctx)
	if err != nil {
		return nil, err
	}
	now := attendanceMinute(s.now())
	for index := range users {
		location, _ := time.LoadLocation(attendanceTimezone(&users[index]))
		closeTime := time.Date(monthTime.Year(), monthTime.Month()+1, 1, 0, 0, 0, 0, location).UTC()
		if now.Before(closeTime) {
			return nil, ErrAttendanceMonthOpen
		}
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	reports := make([]AttendanceMonthlyReport, 0, len(users))
	for index := range users {
		report, err := s.calculateMonthlyReport(ctx, db, &users[index], month, now)
		if err != nil {
			return nil, err
		}
		reports = append(reports, *report)
	}
	relativePath := filepath.Join(strconv.Itoa(monthTime.Year()), "attendance-"+month+".csv")
	finalPath := filepath.Join(s.reportRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o750); err != nil {
		return nil, err
	}
	temp, err := os.CreateTemp(filepath.Dir(finalPath), ".attendance-*.csv")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	hash := sha256.New()
	writer := io.MultiWriter(temp, hash)
	if _, err := writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		temp.Close()
		return nil, err
	}
	csvWriter := csv.NewWriter(writer)
	csvWriter.UseCRLF = true
	header := []string{"report_month", "user_id", "employee_number", "display_name", "timezone",
		"scheduled_work_days", "present_days", "absent_days", "incomplete_days",
		"worked_hours", "worked_hours_minutes", "worked_minutes"}
	if err := csvWriter.Write(header); err != nil {
		temp.Close()
		return nil, err
	}
	for _, report := range reports {
		hoursMinutes := fmt.Sprintf("%d:%02d", report.WorkedMinutes/60, report.WorkedMinutes%60)
		row := []string{report.ReportMonth, report.UserID, report.EmployeeNumber, report.DisplayName, report.Timezone,
			strconv.Itoa(report.ScheduledWorkDays), strconv.Itoa(report.PresentDays), strconv.Itoa(report.AbsentDays),
			strconv.Itoa(report.IncompleteDays), fmt.Sprintf("%.2f", report.WorkedHours), hoursMinutes, strconv.Itoa(report.WorkedMinutes)}
		if err := csvWriter.Write(row); err != nil {
			temp.Close()
			return nil, err
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		temp.Close()
		return nil, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return nil, err
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		return nil, err
	}
	generated := formatAttendanceTime(now)
	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE attendance_report_exports SET is_active=0 WHERE report_month=? AND format='csv'`, month); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO attendance_report_exports
		(id,report_month,format,relative_path,sha256,row_count,generated_at,source_updated_at,is_active)
		VALUES(?,?,'csv',?,?,?,?,?,1)`, uuid.NewString(), month, relativePath, checksum, len(reports), generated, generated); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &AttendanceExport{ReportMonth: month, RelativePath: relativePath, SHA256: checksum, RowCount: len(reports), GeneratedAt: generated}, nil
}

func (s *AttendanceService) calculateMonthlyReport(ctx context.Context, db *sql.DB, user *User, month string, now time.Time) (*AttendanceMonthlyReport, error) {
	var present, incomplete, minutes int
	var source sql.NullString
	err := db.QueryRowContext(ctx, `SELECT
		COUNT(CASE WHEN EXISTS (SELECT 1 FROM attendance_sessions s WHERE s.attendance_day_id=d.id) THEN 1 END),
		COUNT(CASE WHEN d.requires_review=1 OR d.status='working' THEN 1 END),
		COALESCE(SUM(d.total_worked_minutes),0),
		MAX(d.updated_at)
		FROM attendance_days d WHERE d.user_id=? AND d.attendance_date LIKE ?`,
		user.ID, month+"-%").Scan(&present, &incomplete, &minutes, &source)
	if err != nil {
		return nil, err
	}
	monthTime, _ := parseAttendanceMonth(month)
	scheduled := weekdayCount(monthTime.Year(), monthTime.Month())
	absent := scheduled - present
	if absent < 0 {
		absent = 0
	}
	generated := formatAttendanceTime(now)
	sourceValue := generated
	if source.Valid {
		sourceValue = source.String
	}
	reportTimezone := attendanceTimezone(user)
	_ = db.QueryRowContext(ctx, `SELECT timezone FROM attendance_days
		WHERE user_id=? AND attendance_date LIKE ? ORDER BY attendance_date DESC LIMIT 1`,
		user.ID, month+"-%").Scan(&reportTimezone)
	report := &AttendanceMonthlyReport{
		ID: uuid.NewString(), UserID: user.ID, DisplayName: user.DisplayName, Timezone: reportTimezone,
		ReportMonth: month, ScheduledWorkDays: scheduled, PresentDays: present, AbsentDays: absent,
		IncompleteDays: incomplete, WorkedMinutes: minutes, WorkedHours: roundedHours(minutes), GeneratedAt: generated,
	}
	if user.EmployeeProfile != nil {
		report.EmployeeNumber = user.EmployeeProfile.EmployeeNumber
	}
	_, err = db.ExecContext(ctx, `INSERT INTO attendance_monthly_reports
		(id,user_id,report_month,timezone,scheduled_work_days,present_days,absent_days,incomplete_days,
		worked_minutes,worked_hours,generated_at,source_updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id,report_month) DO UPDATE SET
		timezone=excluded.timezone,scheduled_work_days=excluded.scheduled_work_days,present_days=excluded.present_days,
		absent_days=excluded.absent_days,incomplete_days=excluded.incomplete_days,
		worked_minutes=excluded.worked_minutes,worked_hours=excluded.worked_hours,
		generated_at=excluded.generated_at,source_updated_at=excluded.source_updated_at`,
		report.ID, report.UserID, report.ReportMonth, report.Timezone, report.ScheduledWorkDays, report.PresentDays,
		report.AbsentDays, report.IncompleteDays, report.WorkedMinutes, report.WorkedHours, generated, sourceValue)
	return report, err
}

func weekdayCount(year int, month time.Month) int {
	count := 0
	for day := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC); day.Month() == month; day = day.AddDate(0, 0, 1) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			count++
		}
	}
	return count
}

func (s *AttendanceService) MonthlyReports(ctx context.Context, month string) ([]AttendanceMonthlyReport, error) {
	if _, err := parseAttendanceMonth(month); err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	users, err := s.users.List(ctx)
	if err != nil {
		return nil, err
	}
	userMap := make(map[string]User, len(users))
	for _, user := range users {
		userMap[user.ID] = user
	}
	rows, err := db.QueryContext(ctx, `SELECT id,user_id,report_month,timezone,scheduled_work_days,present_days,
		absent_days,incomplete_days,worked_minutes,worked_hours,generated_at
		FROM attendance_monthly_reports WHERE report_month=? ORDER BY user_id`, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AttendanceMonthlyReport{}
	for rows.Next() {
		var report AttendanceMonthlyReport
		if err := rows.Scan(&report.ID, &report.UserID, &report.ReportMonth, &report.Timezone, &report.ScheduledWorkDays,
			&report.PresentDays, &report.AbsentDays, &report.IncompleteDays, &report.WorkedMinutes,
			&report.WorkedHours, &report.GeneratedAt); err != nil {
			return nil, err
		}
		if user, ok := userMap[report.UserID]; ok {
			report.DisplayName = user.DisplayName
			if user.EmployeeProfile != nil {
				report.EmployeeNumber = user.EmployeeProfile.EmployeeNumber
			}
		}
		result = append(result, report)
	}
	return result, rows.Err()
}

func (s *AttendanceService) CSVPath(ctx context.Context, month string) (string, error) {
	if _, err := parseAttendanceMonth(month); err != nil {
		return "", err
	}
	db, err := s.open()
	if err != nil {
		return "", err
	}
	defer db.Close()
	var relative string
	err = db.QueryRowContext(ctx, `SELECT relative_path FROM attendance_report_exports
		WHERE report_month=? AND format='csv' AND is_active=1`, month).Scan(&relative)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAttendanceExport
	}
	if err != nil {
		return "", err
	}
	cleanRelative := filepath.Clean(relative)
	if filepath.IsAbs(cleanRelative) || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", ErrAttendanceExport
	}
	path := filepath.Join(s.reportRoot, cleanRelative)
	if _, err := os.Stat(path); err != nil {
		return "", ErrAttendanceExport
	}
	return path, nil
}

func (s *AttendanceService) Cleanup(ctx context.Context) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	cutoff := attendanceMinute(s.now()).UTC().AddDate(0, -6, 0).Format("2006-01-02")
	_, err = db.ExecContext(ctx, `DELETE FROM attendance_days
		WHERE attendance_date < ? AND requires_review=0
		AND EXISTS (
			SELECT 1 FROM attendance_report_exports e
			WHERE e.report_month=substr(attendance_days.attendance_date,1,7)
			AND e.format='csv' AND e.is_active=1
		)`, cutoff)
	return err
}

func (s *AttendanceService) RunMaintenance(ctx context.Context) error {
	users, err := s.users.List(ctx)
	if err != nil {
		return err
	}
	for index := range users {
		if _, err := s.Today(ctx, users[index].ID); err != nil {
			return err
		}
	}
	now := attendanceMinute(s.now())
	previousMonth := now.AddDate(0, -1, 0).Format("2006-01")
	if _, err := s.CSVPath(ctx, previousMonth); errors.Is(err, ErrAttendanceExport) {
		if _, err := s.GenerateMonthlyCSV(ctx, previousMonth); err != nil && !errors.Is(err, ErrAttendanceMonthOpen) {
			return err
		}
	} else if err != nil {
		return err
	}
	return s.Cleanup(ctx)
}
