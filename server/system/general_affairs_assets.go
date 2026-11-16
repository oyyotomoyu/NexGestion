package system

// Section 2.1 Assets & Custody. Every custody change is an append-only asset
// event; the asset row's status/custodian/location are a cache of the latest
// event, written in the same transaction.

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type GAAsset struct {
	ID                     string         `json:"id"`
	Name                   string         `json:"name"`
	Category               *string        `json:"category"`
	Status                 string         `json:"status"`
	CurrentCustodianUserID *string        `json:"current_custodian_user_id"`
	CurrentLocation        *string        `json:"current_location"`
	CreatedAt              string         `json:"created_at"`
	UpdatedAt              string         `json:"updated_at"`
	Events                 []GAAssetEvent `json:"events,omitempty"`
}

type GAAssetEvent struct {
	ID                       string  `json:"id"`
	AssetID                  string  `json:"asset_id"`
	EventType                string  `json:"event_type"`
	UserID                   *string `json:"user_id"`
	Location                 *string `json:"location"`
	RelatedApprovalRequestID *string `json:"related_approval_request_id"`
	RelatedHRTaskID          *string `json:"related_hr_task_id"`
	ConditionNote            *string `json:"condition_note"`
	ActorUserID              string  `json:"actor_user_id"`
	OccurredAt               string  `json:"occurred_at"`
}

type CreateGAAssetInput struct {
	Name          string  `json:"name"`
	Category      *string `json:"category"`
	Location      *string `json:"location"`
	ConditionNote *string `json:"condition_note"`
}

type UpdateGAAssetInput struct {
	Name     *string `json:"name"`
	Category *string `json:"category"`
	Location *string `json:"location"`
}

type RecordGAAssetEventInput struct {
	EventType                string  `json:"event_type"`
	UserID                   *string `json:"user_id"`
	Location                 *string `json:"location"`
	ConditionNote            *string `json:"condition_note"`
	RelatedApprovalRequestID *string `json:"related_approval_request_id"`
	RelatedHRTaskID          *string `json:"related_hr_task_id"`
}

const gaAssetColumns = `id, name, category, status, current_custodian_user_id, current_location, created_at, updated_at`

func scanGAAsset(row rowScanner) (GAAsset, error) {
	var asset GAAsset
	err := row.Scan(&asset.ID, &asset.Name, &asset.Category, &asset.Status, &asset.CurrentCustodianUserID,
		&asset.CurrentLocation, &asset.CreatedAt, &asset.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return asset, ErrGANotFound
	}
	return asset, err
}

func (s *GeneralAffairsService) ListAssets(ctx context.Context, query ListQuery, custodianOnly string) (ListResult[GAAsset], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GAAsset]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: "ga_assets", columns: gaAssetColumns,
		keywordWhere: "(name LIKE ? OR category LIKE ? OR id LIKE ?)", keywordArgs: 3,
		sorts:       map[string]string{"name": "name COLLATE NOCASE", "status": "status", "created_at": "created_at", "updated_at": "updated_at"},
		defaultSort: "name", defaultOrder: "asc"}
	if custodianOnly != "" {
		spec.where, spec.args = append(spec.where, "current_custodian_user_id = ?"), append(spec.args, custodianOnly)
	}
	for _, filter := range []struct{ key, column string }{{"status", "status"}, {"category", "category"}, {"custodian_user_id", "current_custodian_user_id"}} {
		if value := filterValue(query, filter.key); value != "" {
			spec.where, spec.args = append(spec.where, filter.column+" = ?"), append(spec.args, value)
		}
	}
	return gaList(ctx, db, query, spec, scanGAAsset)
}

// GetAsset returns the asset with its full event history. A caller without
// general_affairs.assets.read may only see an asset they currently hold.
func (s *GeneralAffairsService) GetAsset(ctx context.Context, actor GAActor, id string) (*GAAsset, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	asset, err := scanGAAsset(db.QueryRowContext(ctx, `SELECT `+gaAssetColumns+` FROM ga_assets WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if !actor.Has("general_affairs.assets.read") {
		if asset.CurrentCustodianUserID == nil || *asset.CurrentCustodianUserID != actor.UserID {
			return nil, ErrGANotFound
		}
	}
	rows, err := db.QueryContext(ctx, `SELECT id, asset_id, event_type, user_id, location, related_approval_request_id,
		related_hr_task_id, condition_note, actor_user_id, occurred_at FROM ga_asset_events WHERE asset_id = ?
		ORDER BY occurred_at, rowid`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	asset.Events = []GAAssetEvent{}
	for rows.Next() {
		var event GAAssetEvent
		if err := rows.Scan(&event.ID, &event.AssetID, &event.EventType, &event.UserID, &event.Location, &event.RelatedApprovalRequestID,
			&event.RelatedHRTaskID, &event.ConditionNote, &event.ActorUserID, &event.OccurredAt); err != nil {
			return nil, err
		}
		asset.Events = append(asset.Events, event)
	}
	return &asset, rows.Err()
}

func (s *GeneralAffairsService) CreateAsset(ctx context.Context, actor GAActor, input CreateGAAssetInput) (*GAAsset, error) {
	name, err := requireText("name", input.Name, 200)
	if err != nil {
		return nil, err
	}
	category, err := optionalText("category", input.Category, 100)
	if err != nil {
		return nil, err
	}
	location, err := optionalText("location", input.Location, 200)
	if err != nil {
		return nil, err
	}
	note, err := optionalText("condition_note", input.ConditionNote, 1000)
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
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO ga_assets (`+gaAssetColumns+`) VALUES (?, ?, ?, 'in_stock', NULL, ?, ?, ?)`,
		id, name, category, location, now, now); err != nil {
		return nil, err
	}
	if err := insertAssetEvent(ctx, tx, id, "created", nil, location, note, nil, nil, actor.UserID, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetAsset(ctx, actor, id)
}

func (s *GeneralAffairsService) UpdateAsset(ctx context.Context, actor GAActor, id string, input UpdateGAAssetInput) (*GAAsset, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	asset, err := scanGAAsset(db.QueryRowContext(ctx, `SELECT `+gaAssetColumns+` FROM ga_assets WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		if asset.Name, err = requireText("name", *input.Name, 200); err != nil {
			return nil, err
		}
	}
	if input.Category != nil {
		if asset.Category, err = optionalText("category", input.Category, 100); err != nil {
			return nil, err
		}
	}
	if input.Location != nil {
		// Location of an issued asset is "with its custodian"; moving a
		// stored asset is metadata, not a custody event.
		if asset.Status == "issued" {
			return nil, gaConflict("an issued asset's location follows its custodian; record a return first")
		}
		if asset.CurrentLocation, err = optionalText("location", input.Location, 200); err != nil {
			return nil, err
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE ga_assets SET name = ?, category = ?, current_location = ?, updated_at = ? WHERE id = ?`,
		asset.Name, asset.Category, asset.CurrentLocation, s.timestamp(), id); err != nil {
		return nil, err
	}
	return s.GetAsset(ctx, actor, id)
}

// assetTransitions lists, for each event type, the statuses it may start
// from and the status it leaves the asset in.
var assetTransitions = map[string]struct {
	from []string
	to   string
}{
	"issued":      {from: []string{"in_stock"}, to: "issued"},
	"transferred": {from: []string{"issued"}, to: "issued"},
	"returned":    {from: []string{"issued", "under_repair", "lost"}, to: "in_stock"},
	"repaired":    {from: []string{"in_stock", "issued"}, to: "under_repair"},
	"lost":        {from: []string{"in_stock", "issued"}, to: "lost"},
	"retired":     {from: []string{"in_stock", "under_repair", "lost"}, to: "retired"},
}

// RecordAssetEvent applies one custody event (Section 2.1.1). "repaired"
// means the asset went in for repair; a later "returned" brings it back to
// stock, as it does for a found lost asset.
func (s *GeneralAffairsService) RecordAssetEvent(ctx context.Context, actor GAActor, id string, input RecordGAAssetEventInput) (*GAAsset, error) {
	eventType := strings.TrimSpace(input.EventType)
	transition, ok := assetTransitions[eventType]
	if !ok {
		return nil, gaInvalid("event_type must be issued, transferred, returned, repaired, lost or retired")
	}
	location, err := optionalText("location", input.Location, 200)
	if err != nil {
		return nil, err
	}
	note, err := optionalText("condition_note", input.ConditionNote, 1000)
	if err != nil {
		return nil, err
	}
	approvalID, err := optionalText("related_approval_request_id", input.RelatedApprovalRequestID, 100)
	if err != nil {
		return nil, err
	}
	hrTaskID, err := optionalText("related_hr_task_id", input.RelatedHRTaskID, 100)
	if err != nil {
		return nil, err
	}
	var userID *string
	if eventType == "issued" || eventType == "transferred" {
		if input.UserID == nil {
			return nil, gaInvalid("user_id is required to %s an asset", strings.TrimSuffix(eventType, "ed"))
		}
		trimmed := strings.TrimSpace(*input.UserID)
		if err := s.requireActiveUser(ctx, "user_id", trimmed); err != nil {
			return nil, err
		}
		userID = &trimmed
	} else if input.UserID != nil && strings.TrimSpace(*input.UserID) != "" {
		return nil, gaInvalid("user_id only applies to issued and transferred events")
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
	asset, err := scanGAAsset(tx.QueryRowContext(ctx, `SELECT `+gaAssetColumns+` FROM ga_assets WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if !containsString(transition.from, asset.Status) {
		return nil, gaConflict("cannot record %s on an asset that is %s", eventType, asset.Status)
	}
	previousCustodian := asset.CurrentCustodianUserID
	if eventType == "transferred" && previousCustodian != nil && *previousCustodian == *userID {
		return nil, gaConflict("the asset is already held by this user")
	}
	// The returning employee is the event's user on "returned" (Section
	// 2.1.1), recorded from the custody being closed.
	eventUser := userID
	if eventType == "returned" || eventType == "repaired" || eventType == "lost" {
		eventUser = previousCustodian
	}
	newLocation := asset.CurrentLocation
	if transition.to == "issued" {
		newLocation = nil
	} else if location != nil {
		newLocation = location
	}
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `UPDATE ga_assets SET status = ?, current_custodian_user_id = ?, current_location = ?, updated_at = ? WHERE id = ?`,
		transition.to, userID, newLocation, now, id); err != nil {
		return nil, err
	}
	if err := insertAssetEvent(ctx, tx, id, eventType, eventUser, location, note, approvalID, hrTaskID, actor.UserID, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	switch eventType {
	case "issued":
		s.notifyUsers(ctx, []string{*userID}, "info", "Asset issued to you", asset.Name+" has been issued to you.")
	case "transferred":
		s.notifyUsers(ctx, []string{*userID}, "info", "Asset transferred to you", asset.Name+" has been transferred to you.")
		if previousCustodian != nil {
			s.notifyUsers(ctx, []string{*previousCustodian}, "info", "Asset transferred", asset.Name+" is no longer in your custody.")
		}
	case "returned":
		if previousCustodian != nil {
			s.notifyUsers(ctx, []string{*previousCustodian}, "info", "Asset return recorded", "The return of "+asset.Name+" has been recorded.")
		}
	}
	return s.GetAsset(ctx, GAActor{UserID: actor.UserID, Permissions: map[string]bool{"general_affairs.assets.read": true}}, id)
}

func insertAssetEvent(ctx context.Context, tx *sql.Tx, assetID, eventType string, userID, location, note, approvalID, hrTaskID *string, actorID, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO ga_asset_events (id, asset_id, event_type, user_id, location,
		related_approval_request_id, related_hr_task_id, condition_note, actor_user_id, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), assetID, eventType, userID, location, approvalID, hrTaskID, note, actorID, now)
	return err
}
