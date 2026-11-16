package system

// Section 2.3 Resource Booking & Item Borrowing, including invitees (2.3.3)
// and Space Coordination (2.3.4). The no-overlap rule for confirmed bookings
// is enforced by triggers in general_affairs_database.go; this code checks
// first to give a useful error and maps the trigger's abort to a conflict.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const gaMaxBookingDuration = 31 * 24 * time.Hour

type GAResource struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Category       *string `json:"category"`
	RequiresReturn bool    `json:"requires_return"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type GAResourceInput struct {
	Name           *string `json:"name"`
	Category       *string `json:"category"`
	RequiresReturn *bool   `json:"requires_return"`
	Status         *string `json:"status"`
}

type GABooking struct {
	ID                string               `json:"id"`
	ResourceID        string               `json:"resource_id"`
	ResourceName      string               `json:"resource_name"`
	RequiresReturn    bool                 `json:"requires_return"`
	BookedByUserID    string               `json:"booked_by_user_id"`
	Purpose           *string              `json:"purpose"`
	StartsAt          string               `json:"starts_at"`
	EndsAt            string               `json:"ends_at"`
	Status            string               `json:"status"`
	ReturnedAt        *string              `json:"returned_at"`
	Overdue           bool                 `json:"overdue"`
	CreatedByUserID   string               `json:"created_by_user_id"`
	CreatedAt         string               `json:"created_at"`
	UpdatedAt         string               `json:"updated_at"`
	Invitees          []GABookingInvitee   `json:"invitees,omitempty"`
	CoordinationCases []GACoordinationCase `json:"coordination_cases,omitempty"`
}

type GABookingInvitee struct {
	BookingID      string  `json:"booking_id"`
	InvitedUserID  string  `json:"invited_user_id"`
	ResponseStatus string  `json:"response_status"`
	RespondedAt    *string `json:"responded_at"`
}

type GACoordinationCase struct {
	ID                     string  `json:"id"`
	RequestingBookingID    string  `json:"requesting_booking_id"`
	ConflictingBookingID   string  `json:"conflicting_booking_id"`
	Reason                 *string `json:"reason"`
	InitiatedByUserID      string  `json:"initiated_by_user_id"`
	OriginalBookerDecision string  `json:"original_booker_decision"`
	DecidedAt              *string `json:"decided_at"`
	Status                 string  `json:"status"`
	CreatedAt              string  `json:"created_at"`
	UpdatedAt              string  `json:"updated_at"`
}

type CreateGABookingInput struct {
	ResourceID     string   `json:"resource_id"`
	StartsAt       string   `json:"starts_at"`
	EndsAt         string   `json:"ends_at"`
	Purpose        *string  `json:"purpose"`
	InviteeUserIDs []string `json:"invitee_user_ids"`
	// BookedByUserID books on someone else's behalf; requires
	// general_affairs.bookings.manage or .coordinate.
	BookedByUserID *string `json:"booked_by_user_id"`
	// Coordinate raises a Space Coordination case instead of rejecting a
	// conflict; requires general_affairs.bookings.coordinate.
	Coordinate *GACoordinateInput `json:"coordinate"`
}

type GACoordinateInput struct {
	Reason string `json:"reason"`
}

type UpdateGABookingInput struct {
	ResourceID     *string   `json:"resource_id"`
	StartsAt       *string   `json:"starts_at"`
	EndsAt         *string   `json:"ends_at"`
	Purpose        *string   `json:"purpose"`
	InviteeUserIDs *[]string `json:"invitee_user_ids"`
}

type CreateGACoordinationInput struct {
	RequestingBookingID string `json:"requesting_booking_id"`
	Reason              string `json:"reason"`
}

// ---------------------------------------------------------------------------
// Resources
// ---------------------------------------------------------------------------

const gaResourceColumns = `id, name, category, requires_return, status, created_at, updated_at`

func scanGAResource(row rowScanner) (GAResource, error) {
	var resource GAResource
	var requiresReturn int
	err := row.Scan(&resource.ID, &resource.Name, &resource.Category, &requiresReturn, &resource.Status, &resource.CreatedAt, &resource.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return resource, ErrGANotFound
	}
	resource.RequiresReturn = requiresReturn == 1
	return resource, err
}

func (s *GeneralAffairsService) ListResources(ctx context.Context, query ListQuery) (ListResult[GAResource], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GAResource]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: "ga_resources", columns: gaResourceColumns, keywordWhere: "(name LIKE ? OR category LIKE ?)", keywordArgs: 2,
		sorts: map[string]string{"name": "name COLLATE NOCASE", "category": "category", "created_at": "created_at"}, defaultSort: "name", defaultOrder: "asc"}
	for _, filter := range []string{"status", "category"} {
		if value := filterValue(query, filter); value != "" {
			spec.where, spec.args = append(spec.where, filter+" = ?"), append(spec.args, value)
		}
	}
	return gaList(ctx, db, query, spec, scanGAResource)
}

func (s *GeneralAffairsService) CreateResource(ctx context.Context, input GAResourceInput) (*GAResource, error) {
	if input.Name == nil {
		return nil, gaInvalid("name is required")
	}
	return s.saveResource(ctx, "", input)
}

func (s *GeneralAffairsService) UpdateResource(ctx context.Context, id string, input GAResourceInput) (*GAResource, error) {
	return s.saveResource(ctx, id, input)
}

func (s *GeneralAffairsService) saveResource(ctx context.Context, id string, input GAResourceInput) (*GAResource, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	resource := GAResource{Status: "active"}
	if id != "" {
		if resource, err = scanGAResource(db.QueryRowContext(ctx, `SELECT `+gaResourceColumns+` FROM ga_resources WHERE id = ?`, id)); err != nil {
			return nil, err
		}
	}
	if input.Name != nil {
		if resource.Name, err = requireText("name", *input.Name, 200); err != nil {
			return nil, err
		}
	}
	if input.Category != nil {
		if resource.Category, err = optionalText("category", input.Category, 100); err != nil {
			return nil, err
		}
	}
	if input.RequiresReturn != nil {
		if id != "" && *input.RequiresReturn != resource.RequiresReturn {
			var open int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ga_bookings WHERE resource_id = ? AND status <> 'cancelled' AND ends_at > ?`,
				id, s.timestamp()).Scan(&open); err != nil {
				return nil, err
			}
			if open > 0 {
				return nil, gaConflict("requires_return cannot change while the resource has upcoming bookings")
			}
		}
		resource.RequiresReturn = *input.RequiresReturn
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status != "active" && status != "inactive" {
			return nil, gaInvalid("status must be active or inactive")
		}
		resource.Status = status
	}
	now := s.timestamp()
	if id == "" {
		resource.ID, resource.CreatedAt = uuid.NewString(), now
		resource.UpdatedAt = now
		_, err = db.ExecContext(ctx, `INSERT INTO ga_resources (`+gaResourceColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			resource.ID, resource.Name, resource.Category, boolInt(resource.RequiresReturn), resource.Status, now, now)
	} else {
		resource.UpdatedAt = now
		_, err = db.ExecContext(ctx, `UPDATE ga_resources SET name = ?, category = ?, requires_return = ?, status = ?, updated_at = ? WHERE id = ?`,
			resource.Name, resource.Category, boolInt(resource.RequiresReturn), resource.Status, now, id)
	}
	if err != nil {
		return nil, err
	}
	return &resource, nil
}

// ---------------------------------------------------------------------------
// Bookings
// ---------------------------------------------------------------------------

const gaBookingColumns = `b.id, b.resource_id, r.name, r.requires_return, b.booked_by_user_id, b.purpose, b.starts_at, b.ends_at,
	b.status, b.returned_at, b.created_by_user_id, b.created_at, b.updated_at`

const gaBookingFrom = `ga_bookings b JOIN ga_resources r ON r.id = b.resource_id`

func (s *GeneralAffairsService) scanGABooking(row rowScanner) (GABooking, error) {
	var booking GABooking
	var requiresReturn int
	err := row.Scan(&booking.ID, &booking.ResourceID, &booking.ResourceName, &requiresReturn, &booking.BookedByUserID, &booking.Purpose,
		&booking.StartsAt, &booking.EndsAt, &booking.Status, &booking.ReturnedAt, &booking.CreatedByUserID, &booking.CreatedAt, &booking.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return booking, ErrGANotFound
	}
	booking.RequiresReturn = requiresReturn == 1
	// Overdue is derived on read, never stored (Section 2.3.2).
	booking.Overdue = booking.RequiresReturn && booking.Status == "confirmed" && booking.ReturnedAt == nil && booking.EndsAt <= s.timestamp()
	return booking, err
}

// ListBookings filters: resource_id, status, booked_by_user_id, from/to (an
// overlap window, RFC3339), mine=true, invited=true, overdue=true.
func (s *GeneralAffairsService) ListBookings(ctx context.Context, actor GAActor, query ListQuery) (ListResult[GABooking], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GABooking]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: gaBookingFrom, columns: gaBookingColumns, keywordWhere: "(b.purpose LIKE ? OR r.name LIKE ?)", keywordArgs: 2,
		sorts:       map[string]string{"starts_at": "b.starts_at", "created_at": "b.created_at", "resource": "r.name COLLATE NOCASE"},
		defaultSort: "starts_at", defaultOrder: "asc"}
	add := func(clause string, args ...any) {
		spec.where, spec.args = append(spec.where, clause), append(spec.args, args...)
	}
	for _, filter := range []struct{ key, column string }{{"resource_id", "b.resource_id"}, {"status", "b.status"}, {"booked_by_user_id", "b.booked_by_user_id"}} {
		if value := filterValue(query, filter.key); value != "" {
			add(filter.column+" = ?", value)
		}
	}
	if raw := filterValue(query, "from"); raw != "" {
		from, _, err := parseGATime("from", raw)
		if err != nil {
			return ListResult[GABooking]{}, err
		}
		add("b.ends_at > ?", from)
	}
	if raw := filterValue(query, "to"); raw != "" {
		to, _, err := parseGATime("to", raw)
		if err != nil {
			return ListResult[GABooking]{}, err
		}
		add("b.starts_at < ?", to)
	}
	if filterValue(query, "mine") == "true" {
		add("b.booked_by_user_id = ?", actor.UserID)
	}
	if filterValue(query, "invited") == "true" {
		add("b.id IN (SELECT booking_id FROM ga_booking_invitees WHERE invited_user_id = ?)", actor.UserID)
	}
	if filterValue(query, "overdue") == "true" {
		add("r.requires_return = 1 AND b.status = 'confirmed' AND b.returned_at IS NULL AND b.ends_at <= ?", s.timestamp())
	}
	return gaList(ctx, db, query, spec, s.scanGABooking)
}

func (s *GeneralAffairsService) GetBooking(ctx context.Context, id string) (*GABooking, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.loadBooking(ctx, db, id)
}

func (s *GeneralAffairsService) loadBooking(ctx context.Context, db *sql.DB, id string) (*GABooking, error) {
	booking, err := s.scanGABooking(db.QueryRowContext(ctx, `SELECT `+gaBookingColumns+` FROM `+gaBookingFrom+` WHERE b.id = ?`, id))
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT booking_id, invited_user_id, response_status, responded_at FROM ga_booking_invitees
		WHERE booking_id = ? ORDER BY invited_user_id`, id)
	if err != nil {
		return nil, err
	}
	booking.Invitees = []GABookingInvitee{}
	for rows.Next() {
		var invitee GABookingInvitee
		if err := rows.Scan(&invitee.BookingID, &invitee.InvitedUserID, &invitee.ResponseStatus, &invitee.RespondedAt); err != nil {
			rows.Close()
			return nil, err
		}
		booking.Invitees = append(booking.Invitees, invitee)
	}
	rows.Close()
	cases, err := s.queryCases(ctx, db, `requesting_booking_id = ? OR conflicting_booking_id = ?`, id, id)
	if err != nil {
		return nil, err
	}
	booking.CoordinationCases = cases
	return &booking, nil
}

type gaSlot struct {
	resourceID string
	startsAt   string
	endsAt     string
}

func (s *GeneralAffairsService) parseSlot(ctx context.Context, db *sql.DB, resourceID, startsRaw, endsRaw string) (gaSlot, *GAResource, error) {
	resource, err := scanGAResource(db.QueryRowContext(ctx, `SELECT `+gaResourceColumns+` FROM ga_resources WHERE id = ?`, strings.TrimSpace(resourceID)))
	if errors.Is(err, ErrGANotFound) {
		return gaSlot{}, nil, gaInvalid("resource_id does not exist")
	}
	if err != nil {
		return gaSlot{}, nil, err
	}
	if resource.Status != "active" {
		return gaSlot{}, nil, gaConflict("resource %q is inactive", resource.Name)
	}
	starts, startTime, err := parseGATime("starts_at", startsRaw)
	if err != nil {
		return gaSlot{}, nil, err
	}
	ends, endTime, err := parseGATime("ends_at", endsRaw)
	if err != nil {
		return gaSlot{}, nil, err
	}
	if !endTime.After(startTime) {
		return gaSlot{}, nil, gaInvalid("ends_at must be after starts_at")
	}
	if endTime.Sub(startTime) > gaMaxBookingDuration {
		return gaSlot{}, nil, gaInvalid("a booking can last at most 31 days")
	}
	if !endTime.After(s.now()) {
		return gaSlot{}, nil, gaInvalid("a booking must end in the future")
	}
	return gaSlot{resourceID: resource.ID, startsAt: starts, endsAt: ends}, &resource, nil
}

// confirmedConflicts lists confirmed bookings overlapping the slot.
func confirmedConflicts(ctx context.Context, db *sql.DB, slot gaSlot, excludeID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM ga_bookings WHERE resource_id = ? AND status = 'confirmed' AND id <> ?
		AND starts_at < ? AND ? < ends_at ORDER BY starts_at`, slot.resourceID, excludeID, slot.endsAt, slot.startsAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func isOverlapAbort(err error) bool {
	return err != nil && strings.Contains(err.Error(), "ga_booking_overlap")
}

func overlapConflict(ids []string) error {
	return gaConflict("the resource is already booked for an overlapping time (booking %s)", strings.Join(ids, ", "))
}

func (s *GeneralAffairsService) CreateBooking(ctx context.Context, actor GAActor, input CreateGABookingInput) (*GABooking, error) {
	purpose, err := optionalText("purpose", input.Purpose, 500)
	if err != nil {
		return nil, err
	}
	bookedBy := actor.UserID
	if input.BookedByUserID != nil && strings.TrimSpace(*input.BookedByUserID) != "" && strings.TrimSpace(*input.BookedByUserID) != actor.UserID {
		if !actor.Has("general_affairs.bookings.manage") && !actor.Has("general_affairs.bookings.coordinate") {
			return nil, ErrPermissionDenied
		}
		bookedBy = strings.TrimSpace(*input.BookedByUserID)
		if err := s.requireActiveUser(ctx, "booked_by_user_id", bookedBy); err != nil {
			return nil, err
		}
	}
	if input.Coordinate != nil && !actor.Has("general_affairs.bookings.coordinate") {
		return nil, ErrPermissionDenied
	}
	invitees, err := s.normalizeInvitees(ctx, input.InviteeUserIDs, bookedBy)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	slot, resource, err := s.parseSlot(ctx, db, input.ResourceID, input.StartsAt, input.EndsAt)
	if err != nil {
		return nil, err
	}
	conflicts, err := confirmedConflicts(ctx, db, slot, "")
	if err != nil {
		return nil, err
	}
	status := "confirmed"
	var reason *string
	if len(conflicts) > 0 {
		if input.Coordinate == nil {
			return nil, overlapConflict(conflicts)
		}
		if len(conflicts) > 1 {
			return nil, gaConflict("the slot overlaps %d confirmed bookings; Space Coordination resolves one conflict at a time", len(conflicts))
		}
		if reason, err = optionalText("coordinate.reason", &input.Coordinate.Reason, 500); err != nil {
			return nil, err
		}
		status = "pending_coordination"
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO ga_bookings (id, resource_id, booked_by_user_id, purpose, starts_at, ends_at, status,
		created_by_user_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, slot.resourceID, bookedBy, purpose, slot.startsAt, slot.endsAt, status, actor.UserID, now, now); err != nil {
		if isOverlapAbort(err) {
			return nil, overlapConflict(conflicts)
		}
		return nil, err
	}
	for _, invitee := range invitees {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ga_booking_invitees (booking_id, invited_user_id) VALUES (?, ?)`, id, invitee); err != nil {
			return nil, err
		}
	}
	var conflictingBooker string
	if status == "pending_coordination" {
		if conflictingBooker, err = s.insertCase(ctx, tx, id, conflicts[0], reason, actor.UserID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.notifyUsers(ctx, invitees, "info", "Meeting invitation", "You are invited: "+resource.Name+" "+slot.startsAt+" – "+slot.endsAt+".")
	if bookedBy != actor.UserID {
		s.notifyUsers(ctx, []string{bookedBy}, "info", "Booking made for you", resource.Name+" "+slot.startsAt+" – "+slot.endsAt+" ("+status+").")
	}
	if conflictingBooker != "" {
		s.notifyCoordination(ctx, conflictingBooker, resource.Name, slot)
	}
	return s.loadBooking(ctx, db, id)
}

func (s *GeneralAffairsService) normalizeInvitees(ctx context.Context, raw []string, bookedBy string) ([]string, error) {
	seen := map[string]bool{}
	invitees := []string{}
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || id == bookedBy || seen[id] {
			continue
		}
		if err := s.requireActiveUser(ctx, "invitee_user_ids", id); err != nil {
			return nil, err
		}
		seen[id] = true
		invitees = append(invitees, id)
	}
	if len(invitees) > 200 {
		return nil, gaInvalid("at most 200 invitees per booking")
	}
	return invitees, nil
}

func (s *GeneralAffairsService) requireBookingOwner(actor GAActor, booking *GABooking) error {
	if booking.BookedByUserID != actor.UserID && !actor.Has("general_affairs.bookings.manage") {
		return ErrPermissionDenied
	}
	return nil
}

// UpdateBooking edits a booking the caller owns (or any, with bookings.manage).
// Moving a pending_coordination booking to a free slot confirms it and closes
// its open coordination case.
func (s *GeneralAffairsService) UpdateBooking(ctx context.Context, actor GAActor, id string, input UpdateGABookingInput) (*GABooking, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	booking, err := s.loadBooking(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireBookingOwner(actor, booking); err != nil {
		return nil, err
	}
	if booking.Status == "cancelled" {
		return nil, gaConflict("a cancelled booking cannot be edited")
	}
	purpose := booking.Purpose
	if input.Purpose != nil {
		if purpose, err = optionalText("purpose", input.Purpose, 500); err != nil {
			return nil, err
		}
	}
	slotChanged := input.ResourceID != nil || input.StartsAt != nil || input.EndsAt != nil
	slot := gaSlot{resourceID: booking.ResourceID, startsAt: booking.StartsAt, endsAt: booking.EndsAt}
	resourceName := booking.ResourceName
	if slotChanged {
		if booking.ReturnedAt != nil {
			return nil, gaConflict("the item was already returned; this booking can no longer be moved")
		}
		resourceID, starts, ends := booking.ResourceID, booking.StartsAt, booking.EndsAt
		if input.ResourceID != nil {
			resourceID = *input.ResourceID
		}
		if input.StartsAt != nil {
			starts = *input.StartsAt
		}
		if input.EndsAt != nil {
			ends = *input.EndsAt
		}
		var resource *GAResource
		if slot, resource, err = s.parseSlot(ctx, db, resourceID, starts, ends); err != nil {
			return nil, err
		}
		resourceName = resource.Name
	}
	var invitees []string
	if input.InviteeUserIDs != nil {
		if invitees, err = s.normalizeInvitees(ctx, *input.InviteeUserIDs, booking.BookedByUserID); err != nil {
			return nil, err
		}
	}
	status := booking.Status
	var conflicts []string
	if slotChanged {
		if conflicts, err = confirmedConflicts(ctx, db, slot, id); err != nil {
			return nil, err
		}
		if status == "confirmed" && len(conflicts) > 0 {
			return nil, overlapConflict(conflicts)
		}
		if status == "pending_coordination" && len(conflicts) == 0 {
			status = "confirmed"
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `UPDATE ga_bookings SET resource_id = ?, starts_at = ?, ends_at = ?, purpose = ?, status = ?,
		overdue_notified_at = CASE WHEN ? THEN NULL ELSE overdue_notified_at END, updated_at = ? WHERE id = ?`,
		slot.resourceID, slot.startsAt, slot.endsAt, purpose, status, slotChanged, now, id); err != nil {
		if isOverlapAbort(err) {
			return nil, overlapConflict(conflicts)
		}
		return nil, err
	}
	if slotChanged && booking.Status == "pending_coordination" {
		// The slot under negotiation no longer exists.
		if _, err := tx.ExecContext(ctx, `UPDATE ga_coordination_cases SET status = 'cancelled', updated_at = ?
			WHERE requesting_booking_id = ? AND status = 'pending'`, now, id); err != nil {
			return nil, err
		}
	}
	var added []string
	if input.InviteeUserIDs != nil {
		existing := map[string]bool{}
		for _, invitee := range booking.Invitees {
			existing[invitee.InvitedUserID] = true
		}
		keep := map[string]bool{}
		for _, invitee := range invitees {
			keep[invitee] = true
			if !existing[invitee] {
				added = append(added, invitee)
				if _, err := tx.ExecContext(ctx, `INSERT INTO ga_booking_invitees (booking_id, invited_user_id) VALUES (?, ?)`, id, invitee); err != nil {
					return nil, err
				}
			}
		}
		for invitee := range existing {
			if !keep[invitee] {
				if _, err := tx.ExecContext(ctx, `DELETE FROM ga_booking_invitees WHERE booking_id = ? AND invited_user_id = ?`, id, invitee); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.notifyUsers(ctx, added, "info", "Meeting invitation", "You are invited: "+resourceName+" "+slot.startsAt+" – "+slot.endsAt+".")
	if slotChanged {
		others := []string{}
		for _, invitee := range booking.Invitees {
			if !containsString(added, invitee.InvitedUserID) {
				others = append(others, invitee.InvitedUserID)
			}
		}
		s.notifyUsers(ctx, others, "info", "Booking changed", resourceName+" moved to "+slot.startsAt+" – "+slot.endsAt+".")
	}
	return s.loadBooking(ctx, db, id)
}

// CancelBooking cancels a booking. Open coordination cases it was part of are
// cancelled too; a booking that was waiting for this slot is confirmed when
// the slot is now free.
func (s *GeneralAffairsService) CancelBooking(ctx context.Context, actor GAActor, id string) (*GABooking, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	booking, err := s.loadBooking(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireBookingOwner(actor, booking); err != nil {
		return nil, err
	}
	if booking.Status == "cancelled" {
		return booking, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `UPDATE ga_bookings SET status = 'cancelled', updated_at = ? WHERE id = ?`, now, id); err != nil {
		return nil, err
	}
	var waiting []string
	for _, c := range booking.CoordinationCases {
		if c.Status != "pending" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE ga_coordination_cases SET status = 'cancelled', updated_at = ? WHERE id = ?`, now, c.ID); err != nil {
			return nil, err
		}
		if c.ConflictingBookingID == id {
			waiting = append(waiting, c.RequestingBookingID)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, waitingID := range waiting {
		var booker string
		if err := db.QueryRowContext(ctx, `SELECT booked_by_user_id FROM ga_bookings WHERE id = ?`, waitingID).Scan(&booker); err != nil {
			return nil, err
		}
		_, err := db.ExecContext(ctx, `UPDATE ga_bookings SET status = 'confirmed', updated_at = ? WHERE id = ? AND status = 'pending_coordination'`, now, waitingID)
		switch {
		case isOverlapAbort(err):
			// Still blocked by some other booking; stays pending.
		case err != nil:
			return nil, err
		default:
			s.notifyUsers(ctx, []string{booker}, "success", "Booking confirmed", "The slot you were waiting for on "+booking.ResourceName+" is now yours.")
		}
	}
	invitees := []string{}
	for _, invitee := range booking.Invitees {
		invitees = append(invitees, invitee.InvitedUserID)
	}
	s.notifyUsers(ctx, invitees, "info", "Booking cancelled", booking.ResourceName+" "+booking.StartsAt+" has been cancelled.")
	return s.loadBooking(ctx, db, id)
}

// RecordBookingReturn stamps returned_at on a requires_return booking
// (Section 2.3.2): the booker for their own booking, or bookings.manage.
func (s *GeneralAffairsService) RecordBookingReturn(ctx context.Context, actor GAActor, id string) (*GABooking, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	booking, err := s.loadBooking(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireBookingOwner(actor, booking); err != nil {
		return nil, err
	}
	now := s.timestamp()
	switch {
	case !booking.RequiresReturn:
		return nil, gaConflict("%s is used in place; there is nothing to return", booking.ResourceName)
	case booking.Status != "confirmed":
		return nil, gaConflict("only a confirmed booking can be returned")
	case booking.ReturnedAt != nil:
		return nil, gaConflict("the return is already recorded")
	case booking.StartsAt > now:
		return nil, gaConflict("the booking has not started yet")
	}
	if _, err := db.ExecContext(ctx, `UPDATE ga_bookings SET returned_at = ?, return_recorded_by_user_id = ?, updated_at = ? WHERE id = ?`,
		now, actor.UserID, now, id); err != nil {
		return nil, err
	}
	return s.loadBooking(ctx, db, id)
}

func (s *GeneralAffairsService) RespondToInvitation(ctx context.Context, actor GAActor, bookingID, response string) (*GABooking, error) {
	response = strings.TrimSpace(response)
	if response != "accepted" && response != "declined" {
		return nil, gaInvalid("response must be accepted or declined")
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `UPDATE ga_booking_invitees SET response_status = ?, responded_at = ?
		WHERE booking_id = ? AND invited_user_id = ? AND booking_id IN (SELECT id FROM ga_bookings WHERE status <> 'cancelled')`,
		response, s.timestamp(), bookingID, actor.UserID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, ErrGANotFound
	}
	return s.loadBooking(ctx, db, bookingID)
}

// ---------------------------------------------------------------------------
// Space Coordination (Section 2.3.4)
// ---------------------------------------------------------------------------

const gaCaseColumns = `id, requesting_booking_id, conflicting_booking_id, reason, initiated_by_user_id, original_booker_decision,
	decided_at, status, created_at, updated_at`

func scanGACase(row rowScanner) (GACoordinationCase, error) {
	var c GACoordinationCase
	err := row.Scan(&c.ID, &c.RequestingBookingID, &c.ConflictingBookingID, &c.Reason, &c.InitiatedByUserID, &c.OriginalBookerDecision,
		&c.DecidedAt, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrGANotFound
	}
	return c, err
}

func (s *GeneralAffairsService) queryCases(ctx context.Context, db *sql.DB, where string, args ...any) ([]GACoordinationCase, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+gaCaseColumns+` FROM ga_coordination_cases WHERE `+where+` ORDER BY created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cases := []GACoordinationCase{}
	for rows.Next() {
		c, err := scanGACase(rows)
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, rows.Err()
}

// insertCase opens a case and returns the original booker to notify.
func (s *GeneralAffairsService) insertCase(ctx context.Context, tx *sql.Tx, requestingID, conflictingID string, reason *string, actorID string) (string, error) {
	var booker string
	if err := tx.QueryRowContext(ctx, `SELECT booked_by_user_id FROM ga_bookings WHERE id = ?`, conflictingID).Scan(&booker); err != nil {
		return "", err
	}
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO ga_coordination_cases (`+gaCaseColumns+`) VALUES (?, ?, ?, ?, ?, 'pending', NULL, 'pending', ?, ?)`,
		uuid.NewString(), requestingID, conflictingID, reason, actorID, now, now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return "", gaConflict("this booking already has an open coordination case")
		}
		return "", err
	}
	return booker, nil
}

func (s *GeneralAffairsService) notifyCoordination(ctx context.Context, booker, resourceName string, slot gaSlot) {
	s.notifyUsers(ctx, []string{booker}, "important", "Space coordination request",
		"General Affairs asks whether you can give up your booking of "+resourceName+" ("+slot.startsAt+" – "+slot.endsAt+"). Please accept or decline.")
}

// CreateCoordinationCase raises a case for an existing pending_coordination
// booking, e.g. after an earlier case was declined or cancelled.
func (s *GeneralAffairsService) CreateCoordinationCase(ctx context.Context, actor GAActor, input CreateGACoordinationInput) (*GACoordinationCase, error) {
	reason, err := optionalText("reason", &input.Reason, 500)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	booking, err := s.loadBooking(ctx, db, strings.TrimSpace(input.RequestingBookingID))
	if err != nil {
		return nil, err
	}
	if booking.Status != "pending_coordination" {
		return nil, gaConflict("only a booking waiting for coordination can be coordinated (status is %s)", booking.Status)
	}
	slot := gaSlot{resourceID: booking.ResourceID, startsAt: booking.StartsAt, endsAt: booking.EndsAt}
	conflicts, err := confirmedConflicts(ctx, db, slot, booking.ID)
	if err != nil {
		return nil, err
	}
	switch len(conflicts) {
	case 0:
		if _, err := db.ExecContext(ctx, `UPDATE ga_bookings SET status = 'confirmed', updated_at = ? WHERE id = ? AND status = 'pending_coordination'`,
			s.timestamp(), booking.ID); err != nil {
			return nil, err
		}
		return nil, gaConflict("the slot is free now; the booking was confirmed without coordination")
	case 1:
	default:
		return nil, gaConflict("the slot overlaps %d confirmed bookings; resolve them one at a time", len(conflicts))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	booker, err := s.insertCase(ctx, tx, booking.ID, conflicts[0], reason, actor.UserID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.notifyCoordination(ctx, booker, booking.ResourceName, slot)
	cases, err := s.queryCases(ctx, db, `requesting_booking_id = ? AND status = 'pending'`, booking.ID)
	if err != nil || len(cases) == 0 {
		return nil, err
	}
	return &cases[0], nil
}

// ListCoordinationCases: scope "mine" (cases on bookings the caller owns,
// either side) or "all" (coordinate/manage).
func (s *GeneralAffairsService) ListCoordinationCases(ctx context.Context, actor GAActor, scope, status string) ([]GACoordinationCase, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	where, args := "1 = 1", []any{}
	if scope == "mine" {
		where = `(requesting_booking_id IN (SELECT id FROM ga_bookings WHERE booked_by_user_id = ?)
			OR conflicting_booking_id IN (SELECT id FROM ga_bookings WHERE booked_by_user_id = ?))`
		args = append(args, actor.UserID, actor.UserID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	return s.queryCases(ctx, db, where, args...)
}

// DecideCoordinationCase is the original booker's answer. Accepting swaps the
// two bookings atomically: the existing one is cancelled and the requesting
// one confirmed.
func (s *GeneralAffairsService) DecideCoordinationCase(ctx context.Context, actor GAActor, caseID, decision string) (*GACoordinationCase, error) {
	decision = strings.TrimSpace(decision)
	if decision != "accept" && decision != "decline" {
		return nil, gaInvalid("decision must be accept or decline")
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	c, err := scanGACase(db.QueryRowContext(ctx, `SELECT `+gaCaseColumns+` FROM ga_coordination_cases WHERE id = ?`, caseID))
	if err != nil {
		return nil, err
	}
	conflicting, err := s.loadBooking(ctx, db, c.ConflictingBookingID)
	if err != nil {
		return nil, err
	}
	if conflicting.BookedByUserID != actor.UserID {
		return nil, ErrPermissionDenied
	}
	if c.Status != "pending" {
		return nil, gaConflict("the case is already %s", c.Status)
	}
	requesting, err := s.loadBooking(ctx, db, c.RequestingBookingID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	status := "declined"
	if decision == "accept" {
		status = "accepted"
		if _, err := tx.ExecContext(ctx, `UPDATE ga_bookings SET status = 'cancelled', updated_at = ? WHERE id = ? AND status = 'confirmed'`, now, conflicting.ID); err != nil {
			return nil, err
		}
		result, err := tx.ExecContext(ctx, `UPDATE ga_bookings SET status = 'confirmed', updated_at = ? WHERE id = ? AND status = 'pending_coordination'`, now, requesting.ID)
		if isOverlapAbort(err) {
			return nil, gaConflict("another booking now overlaps the requested slot; the swap was not applied")
		}
		if err != nil {
			return nil, err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return nil, gaConflict("the requesting booking is no longer waiting for coordination")
		}
		// Other open cases against the slot just given up are moot.
		if _, err := tx.ExecContext(ctx, `UPDATE ga_coordination_cases SET status = 'cancelled', updated_at = ?
			WHERE conflicting_booking_id = ? AND status = 'pending' AND id <> ?`, now, conflicting.ID, c.ID); err != nil {
			return nil, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE ga_coordination_cases SET original_booker_decision = ?, decided_at = ?, status = ?, updated_at = ?
		WHERE id = ? AND status = 'pending'`, decision, now, status, now, c.ID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, gaConflict("the case was decided concurrently")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if decision == "accept" {
		s.notifyUsers(ctx, []string{requesting.BookedByUserID, c.InitiatedByUserID}, "success", "Space coordination accepted",
			requesting.ResourceName+" "+requesting.StartsAt+" – "+requesting.EndsAt+" is now confirmed for you.")
	} else {
		s.notifyUsers(ctx, []string{requesting.BookedByUserID, c.InitiatedByUserID}, "warning", "Space coordination declined",
			"The current booker of "+requesting.ResourceName+" declined to give up "+requesting.StartsAt+" – "+requesting.EndsAt+".")
	}
	updated, err := scanGACase(db.QueryRowContext(ctx, `SELECT `+gaCaseColumns+` FROM ga_coordination_cases WHERE id = ?`, caseID))
	return &updated, err
}

// CancelCoordinationCase withdraws an open case: its initiator, the
// requesting booker, or a coordinate/manage holder.
func (s *GeneralAffairsService) CancelCoordinationCase(ctx context.Context, actor GAActor, caseID string) (*GACoordinationCase, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	c, err := scanGACase(db.QueryRowContext(ctx, `SELECT `+gaCaseColumns+` FROM ga_coordination_cases WHERE id = ?`, caseID))
	if err != nil {
		return nil, err
	}
	var requester string
	if err := db.QueryRowContext(ctx, `SELECT booked_by_user_id FROM ga_bookings WHERE id = ?`, c.RequestingBookingID).Scan(&requester); err != nil {
		return nil, err
	}
	if c.InitiatedByUserID != actor.UserID && requester != actor.UserID &&
		!actor.Has("general_affairs.bookings.coordinate") && !actor.Has("general_affairs.bookings.manage") {
		return nil, ErrPermissionDenied
	}
	if c.Status != "pending" {
		return nil, gaConflict("the case is already %s", c.Status)
	}
	now := s.timestamp()
	if _, err := db.ExecContext(ctx, `UPDATE ga_coordination_cases SET status = 'cancelled', updated_at = ? WHERE id = ? AND status = 'pending'`, now, caseID); err != nil {
		return nil, err
	}
	c.Status, c.UpdatedAt = "cancelled", now
	return &c, nil
}
