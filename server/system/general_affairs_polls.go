package system

// Section 2.5 Polls & Statistics and Section 2.6 Group Meal Ordering.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
)

type GAPoll struct {
	ID                    string           `json:"id"`
	Title                 string           `json:"title"`
	Description           *string          `json:"description"`
	PollType              string           `json:"poll_type"`
	AllowMultiple         bool             `json:"allow_multiple"`
	CreatedByUserID       string           `json:"created_by_user_id"`
	TargetScope           string           `json:"target_scope"`
	TargetGroupID         *string          `json:"target_group_id"`
	Status                string           `json:"status"`
	ClosesAt              *string          `json:"closes_at"`
	SettlementMethod      *string          `json:"settlement_method"`
	FinanceExpenseClaimID *string          `json:"finance_expense_claim_id"`
	SettledAt             *string          `json:"settled_at"`
	SettledByUserID       *string          `json:"settled_by_user_id"`
	ClosedAt              *string          `json:"closed_at"`
	CreatedAt             string           `json:"created_at"`
	UpdatedAt             string           `json:"updated_at"`
	Options               []GAPollOption   `json:"options,omitempty"`
	MyResponse            *GAPollResponse  `json:"my_response,omitempty"`
	Results               *GAPollResults   `json:"results,omitempty"`
	Responses             []GAPollResponse `json:"responses,omitempty"`
}

type GAPollOption struct {
	ID         string  `json:"id"`
	PollID     string  `json:"poll_id"`
	Label      string  `json:"label"`
	UnitAmount *string `json:"unit_amount"`
	SortOrder  int     `json:"sort_order"`
}

type GAPollResponse struct {
	ID                string            `json:"id"`
	PollID            string            `json:"poll_id"`
	RespondedByUserID string            `json:"responded_by_user_id"`
	Participating     bool              `json:"participating"`
	RespondedAt       string            `json:"responded_at"`
	Selections        []GAPollSelection `json:"selections"`
	LineTotal         string            `json:"line_total"`
}

type GAPollSelection struct {
	OptionID string `json:"option_id"`
	Quantity string `json:"quantity"`
}

type GAPollResults struct {
	ResponseCount      int                  `json:"response_count"`
	ParticipatingCount int                  `json:"participating_count"`
	GrandTotal         string               `json:"grand_total"`
	Options            []GAPollOptionResult `json:"options"`
}

type GAPollOptionResult struct {
	OptionID      string `json:"option_id"`
	Label         string `json:"label"`
	ResponseCount int    `json:"response_count"`
	TotalQuantity string `json:"total_quantity"`
	TotalAmount   string `json:"total_amount"`
}

type GAPollOptionInput struct {
	Label      string  `json:"label"`
	UnitAmount *string `json:"unit_amount"`
}

type GAPollInput struct {
	Title            *string              `json:"title"`
	Description      *string              `json:"description"`
	PollType         *string              `json:"poll_type"`
	AllowMultiple    *bool                `json:"allow_multiple"`
	TargetScope      *string              `json:"target_scope"`
	TargetGroupID    *string              `json:"target_group_id"`
	ClosesAt         *string              `json:"closes_at"`
	SettlementMethod *string              `json:"settlement_method"`
	Options          *[]GAPollOptionInput `json:"options"`
}

type RespondGAPollInput struct {
	Participating *bool             `json:"participating"`
	Selections    []GAPollSelection `json:"selections"`
}

type GASalaryDeduction struct {
	ID             string `json:"id"`
	PollID         string `json:"poll_id"`
	PollResponseID string `json:"poll_response_id"`
	UserID         string `json:"user_id"`
	Amount         string `json:"amount"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

const gaPollColumns = `id, title, description, poll_type, allow_multiple, created_by_user_id, target_scope, target_group_id, status,
	closes_at, settlement_method, finance_expense_claim_id, settled_at, settled_by_user_id, closed_at, created_at, updated_at`

func scanGAPoll(row rowScanner) (GAPoll, error) {
	var poll GAPoll
	var allowMultiple int
	err := row.Scan(&poll.ID, &poll.Title, &poll.Description, &poll.PollType, &allowMultiple, &poll.CreatedByUserID, &poll.TargetScope,
		&poll.TargetGroupID, &poll.Status, &poll.ClosesAt, &poll.SettlementMethod, &poll.FinanceExpenseClaimID, &poll.SettledAt,
		&poll.SettledByUserID, &poll.ClosedAt, &poll.CreatedAt, &poll.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return poll, ErrGANotFound
	}
	poll.AllowMultiple = allowMultiple == 1
	return poll, err
}

// effectiveStatus treats an open poll past closes_at as closed on read; the
// hourly maintenance job persists it (Section 2.5.1's open question, resolved
// as "both").
func (s *GeneralAffairsService) effectiveStatus(poll *GAPoll) {
	if poll.Status == "open" && poll.ClosesAt != nil && *poll.ClosesAt <= s.timestamp() {
		poll.Status = "closed"
		poll.ClosedAt = poll.ClosesAt
	}
}

// myGroupIDs returns the caller's active group memberships (group-system.md
// Section 3.2; no second membership list is kept here).
func (s *GeneralAffairsService) myGroupIDs(ctx context.Context, userID string) ([]string, error) {
	db, err := s.users.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT ug.group_id FROM user_groups ug JOIN groups g ON g.id = ug.group_id
		WHERE ug.user_id = ? AND ug.left_at IS NULL AND g.status = 'active'`, userID)
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

func (s *GeneralAffairsService) eligibleForPoll(ctx context.Context, poll *GAPoll, userID string) (bool, error) {
	if poll.TargetScope == "organization" {
		return true, nil
	}
	groups, err := s.myGroupIDs(ctx, userID)
	if err != nil {
		return false, err
	}
	return poll.TargetGroupID != nil && containsString(groups, *poll.TargetGroupID), nil
}

func canSeePollResults(actor GAActor, poll *GAPoll) bool {
	return poll.CreatedByUserID == actor.UserID || actor.Has("general_affairs.polls.read") || actor.Has("general_affairs.polls.settle")
}

// ListPolls scopes: "respondable" (open/closed polls targeted at the caller),
// "mine" (created by the caller), "all" (polls.read).
func (s *GeneralAffairsService) ListPolls(ctx context.Context, actor GAActor, scope string, query ListQuery) (ListResult[GAPoll], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GAPoll]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: "ga_polls", columns: gaPollColumns, keywordWhere: "title LIKE ?", keywordArgs: 1,
		sorts:       map[string]string{"created_at": "created_at", "closes_at": "closes_at", "title": "title COLLATE NOCASE"},
		defaultSort: "created_at", defaultOrder: "desc"}
	switch scope {
	case "respondable":
		groups, err := s.myGroupIDs(ctx, actor.UserID)
		if err != nil {
			return ListResult[GAPoll]{}, err
		}
		clause := "target_scope = 'organization'"
		if len(groups) > 0 {
			clause = "(target_scope = 'organization' OR target_group_id IN (?" + strings.Repeat(", ?", len(groups)-1) + "))"
			for _, group := range groups {
				spec.args = append(spec.args, group)
			}
		}
		spec.where = append(spec.where, "status IN ('open', 'closed')", clause)
	case "mine":
		spec.where, spec.args = append(spec.where, "created_by_user_id = ?"), append(spec.args, actor.UserID)
	case "all":
	default:
		return ListResult[GAPoll]{}, gaInvalid("unknown scope")
	}
	for _, filter := range []string{"status", "poll_type"} {
		if value := filterValue(query, filter); value != "" {
			spec.where, spec.args = append(spec.where, filter+" = ?"), append(spec.args, value)
		}
	}
	result, err := gaList(ctx, db, query, spec, scanGAPoll)
	for index := range result.Items {
		s.effectiveStatus(&result.Items[index])
	}
	return result, err
}

// GetPoll returns a poll with its options and the caller's own response; the
// creator and polls.read/settle holders also get aggregates and individual
// responses (Section 4).
func (s *GeneralAffairsService) GetPoll(ctx context.Context, actor GAActor, id string) (*GAPoll, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	poll, err := s.loadPoll(ctx, db, id)
	if err != nil {
		return nil, err
	}
	seesResults := canSeePollResults(actor, poll)
	if !seesResults {
		eligible, err := s.eligibleForPoll(ctx, poll, actor.UserID)
		if err != nil {
			return nil, err
		}
		if !eligible || poll.Status == "draft" {
			return nil, ErrGANotFound
		}
	}
	responses, err := s.pollResponses(ctx, db, poll)
	if err != nil {
		return nil, err
	}
	for index := range responses {
		if responses[index].RespondedByUserID == actor.UserID {
			poll.MyResponse = &responses[index]
		}
	}
	if seesResults {
		poll.Results = summarizePoll(poll, responses)
		poll.Responses = responses
	}
	return poll, nil
}

func (s *GeneralAffairsService) loadPoll(ctx context.Context, db *sql.DB, id string) (*GAPoll, error) {
	poll, err := scanGAPoll(db.QueryRowContext(ctx, `SELECT `+gaPollColumns+` FROM ga_polls WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	s.effectiveStatus(&poll)
	rows, err := db.QueryContext(ctx, `SELECT id, poll_id, label, unit_amount, sort_order FROM ga_poll_options WHERE poll_id = ? ORDER BY sort_order, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	poll.Options = []GAPollOption{}
	for rows.Next() {
		var option GAPollOption
		if err := rows.Scan(&option.ID, &option.PollID, &option.Label, &option.UnitAmount, &option.SortOrder); err != nil {
			return nil, err
		}
		poll.Options = append(poll.Options, option)
	}
	return &poll, rows.Err()
}

func (s *GeneralAffairsService) pollResponses(ctx context.Context, db *sql.DB, poll *GAPoll) ([]GAPollResponse, error) {
	unitAmounts := map[string]*big.Rat{}
	for _, option := range poll.Options {
		amount := new(big.Rat)
		if option.UnitAmount != nil {
			amount.SetString(*option.UnitAmount)
		}
		unitAmounts[option.ID] = amount
	}
	rows, err := db.QueryContext(ctx, `SELECT r.id, r.poll_id, r.responded_by_user_id, r.participating, r.responded_at, s.option_id, s.quantity
		FROM ga_poll_responses r LEFT JOIN ga_poll_response_selections s ON s.poll_response_id = r.id
		WHERE r.poll_id = ? ORDER BY r.responded_at, r.id`, poll.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	responses := []GAPollResponse{}
	index := map[string]int{}
	totals := map[string]*big.Rat{}
	for rows.Next() {
		var response GAPollResponse
		var participating int
		var optionID, quantity sql.NullString
		if err := rows.Scan(&response.ID, &response.PollID, &response.RespondedByUserID, &participating, &response.RespondedAt, &optionID, &quantity); err != nil {
			return nil, err
		}
		position, seen := index[response.ID]
		if !seen {
			response.Participating = participating == 1
			response.Selections = []GAPollSelection{}
			responses = append(responses, response)
			position = len(responses) - 1
			index[response.ID] = position
			totals[response.ID] = new(big.Rat)
		}
		if optionID.Valid {
			responses[position].Selections = append(responses[position].Selections, GAPollSelection{OptionID: optionID.String, Quantity: quantity.String})
			if q, ok := new(big.Rat).SetString(quantity.String); ok && unitAmounts[optionID.String] != nil {
				totals[response.ID].Add(totals[response.ID], new(big.Rat).Mul(q, unitAmounts[optionID.String]))
			}
		}
	}
	for position := range responses {
		responses[position].LineTotal = formatGAAmount(totals[responses[position].ID])
	}
	return responses, rows.Err()
}

func summarizePoll(poll *GAPoll, responses []GAPollResponse) *GAPollResults {
	results := &GAPollResults{ResponseCount: len(responses), Options: []GAPollOptionResult{}}
	quantities, counts := map[string]*big.Rat{}, map[string]int{}
	grand := new(big.Rat)
	for _, response := range responses {
		if !response.Participating {
			continue
		}
		results.ParticipatingCount++
		lineTotal, _ := new(big.Rat).SetString(response.LineTotal)
		grand.Add(grand, lineTotal)
		for _, selection := range response.Selections {
			q, _ := new(big.Rat).SetString(selection.Quantity)
			if quantities[selection.OptionID] == nil {
				quantities[selection.OptionID] = new(big.Rat)
			}
			quantities[selection.OptionID].Add(quantities[selection.OptionID], q)
			counts[selection.OptionID]++
		}
	}
	for _, option := range poll.Options {
		quantity := quantities[option.ID]
		if quantity == nil {
			quantity = new(big.Rat)
		}
		amount := new(big.Rat)
		if option.UnitAmount != nil {
			unit, _ := new(big.Rat).SetString(*option.UnitAmount)
			amount.Mul(quantity, unit)
		}
		results.Options = append(results.Options, GAPollOptionResult{OptionID: option.ID, Label: option.Label,
			ResponseCount: counts[option.ID], TotalQuantity: formatGADecimal(quantity, 2), TotalAmount: formatGAAmount(amount)})
	}
	results.GrandTotal = formatGAAmount(grand)
	return results
}

func (s *GeneralAffairsService) CreatePoll(ctx context.Context, actor GAActor, input GAPollInput) (*GAPoll, error) {
	if input.Title == nil || input.TargetScope == nil {
		return nil, gaInvalid("title and target_scope are required")
	}
	poll := GAPoll{ID: uuid.NewString(), PollType: "general", CreatedByUserID: actor.UserID, Status: "draft"}
	options, err := s.applyPollInput(ctx, actor, &poll, input, true)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	now := s.timestamp()
	poll.CreatedAt, poll.UpdatedAt = now, now
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO ga_polls (id, title, description, poll_type, allow_multiple, created_by_user_id, target_scope,
		target_group_id, status, closes_at, settlement_method, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'draft', ?, ?, ?, ?)`,
		poll.ID, poll.Title, poll.Description, poll.PollType, boolInt(poll.AllowMultiple), poll.CreatedByUserID, poll.TargetScope,
		poll.TargetGroupID, poll.ClosesAt, poll.SettlementMethod, now, now); err != nil {
		return nil, err
	}
	if err := replacePollOptions(ctx, tx, poll.ID, options); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPoll(ctx, actor, poll.ID)
}

// UpdatePoll edits a draft poll (creator only). Once opened, the question
// and options are frozen so existing responses keep their meaning.
func (s *GeneralAffairsService) UpdatePoll(ctx context.Context, actor GAActor, id string, input GAPollInput) (*GAPoll, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	poll, err := s.loadPoll(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if poll.CreatedByUserID != actor.UserID {
		return nil, ErrPermissionDenied
	}
	if poll.Status != "draft" {
		// The settlement method may still change on a closed food order
		// until it is settled.
		onlySettlement := input.Title == nil && input.Description == nil && input.PollType == nil && input.AllowMultiple == nil &&
			input.TargetScope == nil && input.TargetGroupID == nil && input.ClosesAt == nil && input.Options == nil
		if !(onlySettlement && input.SettlementMethod != nil && poll.SettledAt == nil) {
			return nil, gaConflict("only a draft poll can be edited")
		}
	}
	options, err := s.applyPollInput(ctx, actor, poll, input, false)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE ga_polls SET title = ?, description = ?, poll_type = ?, allow_multiple = ?, target_scope = ?,
		target_group_id = ?, closes_at = ?, settlement_method = ?, updated_at = ? WHERE id = ?`, poll.Title, poll.Description, poll.PollType,
		boolInt(poll.AllowMultiple), poll.TargetScope, poll.TargetGroupID, poll.ClosesAt, poll.SettlementMethod, s.timestamp(), id); err != nil {
		return nil, err
	}
	if options != nil {
		if err := replacePollOptions(ctx, tx, id, options); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPoll(ctx, actor, id)
}

// applyPollInput merges and validates input. It returns the replacement
// options when input carries them (nil means unchanged).
func (s *GeneralAffairsService) applyPollInput(ctx context.Context, actor GAActor, poll *GAPoll, input GAPollInput, creating bool) ([]GAPollOptionInput, error) {
	var err error
	if input.Title != nil {
		if poll.Title, err = requireText("title", *input.Title, 200); err != nil {
			return nil, err
		}
	}
	if input.Description != nil {
		if poll.Description, err = optionalText("description", input.Description, 2000); err != nil {
			return nil, err
		}
	}
	if input.PollType != nil {
		poll.PollType = strings.TrimSpace(*input.PollType)
	}
	if poll.PollType != "general" && poll.PollType != "food_order" {
		return nil, gaInvalid("poll_type must be general or food_order")
	}
	if input.AllowMultiple != nil {
		poll.AllowMultiple = *input.AllowMultiple
	}
	if poll.PollType == "food_order" {
		poll.AllowMultiple = true
	}
	if input.TargetScope != nil {
		poll.TargetScope = strings.TrimSpace(*input.TargetScope)
	}
	if input.TargetGroupID != nil {
		poll.TargetGroupID, _ = optionalText("target_group_id", input.TargetGroupID, 100)
	}
	switch poll.TargetScope {
	case "organization":
		poll.TargetGroupID = nil
		if !actor.Has("general_affairs.polls.create.organization") {
			return nil, ErrPermissionDenied
		}
	case "group":
		if !actor.Has("general_affairs.polls.create.group") {
			return nil, ErrPermissionDenied
		}
		if poll.TargetGroupID == nil {
			return nil, gaInvalid("target_group_id is required when target_scope is group")
		}
		if err := s.requireActivePollGroup(ctx, *poll.TargetGroupID); err != nil {
			return nil, err
		}
	default:
		return nil, gaInvalid("target_scope must be organization or group")
	}
	if input.ClosesAt != nil {
		if strings.TrimSpace(*input.ClosesAt) == "" {
			poll.ClosesAt = nil
		} else {
			closesAt, _, err := parseGATime("closes_at", *input.ClosesAt)
			if err != nil {
				return nil, err
			}
			poll.ClosesAt = &closesAt
		}
	}
	if input.SettlementMethod != nil {
		method := strings.TrimSpace(*input.SettlementMethod)
		poll.SettlementMethod = &method
	}
	if poll.PollType == "general" {
		poll.SettlementMethod = nil
	} else {
		if poll.SettlementMethod == nil || *poll.SettlementMethod == "" {
			none := "none"
			poll.SettlementMethod = &none
		}
		switch *poll.SettlementMethod {
		case "none", "salary_deduction", "finance_expense":
		default:
			return nil, gaInvalid("settlement_method must be none, finance_expense or salary_deduction")
		}
	}
	if input.Options == nil {
		if creating {
			return []GAPollOptionInput{}, nil
		}
		return nil, nil
	}
	if len(*input.Options) > 100 {
		return nil, gaInvalid("a poll can have at most 100 options")
	}
	options := make([]GAPollOptionInput, 0, len(*input.Options))
	for index, option := range *input.Options {
		label, err := requireText(fmt.Sprintf("options[%d].label", index), option.Label, 200)
		if err != nil {
			return nil, err
		}
		var unitAmount *string
		if poll.PollType == "food_order" {
			if option.UnitAmount == nil {
				return nil, gaInvalid("options[%d].unit_amount is required for a food order", index)
			}
			_, normalized, err := parseGADecimal(fmt.Sprintf("options[%d].unit_amount", index), *option.UnitAmount, 2, true)
			if err != nil {
				return nil, err
			}
			unitAmount = &normalized
		}
		options = append(options, GAPollOptionInput{Label: label, UnitAmount: unitAmount})
	}
	return options, nil
}

func (s *GeneralAffairsService) requireActivePollGroup(ctx context.Context, groupID string) error {
	db, err := s.users.open()
	if err != nil {
		return err
	}
	defer db.Close()
	var status string
	err = db.QueryRowContext(ctx, `SELECT status FROM groups WHERE id = ?`, groupID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return gaInvalid("target_group_id does not exist")
	}
	if err != nil {
		return err
	}
	if status != "active" {
		return gaInvalid("target group is not active")
	}
	return nil
}

func replacePollOptions(ctx context.Context, tx *sql.Tx, pollID string, options []GAPollOptionInput) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM ga_poll_options WHERE poll_id = ?`, pollID); err != nil {
		return err
	}
	for index, option := range options {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ga_poll_options (id, poll_id, label, unit_amount, sort_order) VALUES (?, ?, ?, ?, ?)`,
			uuid.NewString(), pollID, option.Label, option.UnitAmount, index+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *GeneralAffairsService) DeletePoll(ctx context.Context, actor GAActor, id string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	poll, err := s.loadPoll(ctx, db, id)
	if err != nil {
		return err
	}
	if poll.CreatedByUserID != actor.UserID {
		return ErrPermissionDenied
	}
	if poll.Status != "draft" {
		return gaConflict("only a draft poll can be deleted; close it instead")
	}
	_, err = db.ExecContext(ctx, `DELETE FROM ga_polls WHERE id = ? AND status = 'draft'`, id)
	return err
}

// OpenPoll publishes a draft and notifies its audience (Section 3.6).
func (s *GeneralAffairsService) OpenPoll(ctx context.Context, actor GAActor, id string) (*GAPoll, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	poll, err := s.loadPoll(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if poll.CreatedByUserID != actor.UserID {
		return nil, ErrPermissionDenied
	}
	if poll.Status != "draft" {
		return nil, gaConflict("the poll is already %s", poll.Status)
	}
	if len(poll.Options) == 0 {
		return nil, gaConflict("add at least one option before opening the poll")
	}
	if poll.ClosesAt != nil && *poll.ClosesAt <= s.timestamp() {
		return nil, gaConflict("closes_at is in the past")
	}
	if _, err := db.ExecContext(ctx, `UPDATE ga_polls SET status = 'open', updated_at = ? WHERE id = ? AND status = 'draft'`, s.timestamp(), id); err != nil {
		return nil, err
	}
	audience := NotificationAudienceInput{Scope: "organization"}
	if poll.TargetScope == "group" {
		audience = NotificationAudienceInput{Scope: "group", TargetGroupID: *poll.TargetGroupID}
	}
	message := "A new poll is open: " + poll.Title
	if poll.ClosesAt != nil {
		message += " (closes " + *poll.ClosesAt + ")"
	}
	s.notify(ctx, "info", "New poll: "+poll.Title, message, []NotificationAudienceInput{audience})
	return s.GetPoll(ctx, actor, id)
}

// ClosePoll ends responses early: the creator, or a polls.settle holder.
func (s *GeneralAffairsService) ClosePoll(ctx context.Context, actor GAActor, id string) (*GAPoll, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	poll, err := s.loadPoll(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if poll.CreatedByUserID != actor.UserID && !actor.Has("general_affairs.polls.settle") {
		return nil, ErrPermissionDenied
	}
	if err := s.closePoll(ctx, db, poll); err != nil {
		return nil, err
	}
	return s.GetPoll(ctx, actor, id)
}

func (s *GeneralAffairsService) closePoll(ctx context.Context, db sqlExecer, poll *GAPoll) error {
	switch poll.Status {
	case "draft":
		return gaConflict("a draft poll cannot be closed; delete it instead")
	case "closed":
		// Already closed (possibly only by deadline): persist it.
		closedAt := s.timestamp()
		if poll.ClosedAt != nil {
			closedAt = *poll.ClosedAt
		}
		_, err := db.ExecContext(ctx, `UPDATE ga_polls SET status = 'closed', closed_at = COALESCE(closed_at, ?), updated_at = ? WHERE id = ?`,
			closedAt, s.timestamp(), poll.ID)
		return err
	}
	now := s.timestamp()
	_, err := db.ExecContext(ctx, `UPDATE ga_polls SET status = 'closed', closed_at = ?, updated_at = ? WHERE id = ? AND status = 'open'`, now, now, poll.ID)
	poll.Status, poll.ClosedAt = "closed", &now
	return err
}

// RespondToPoll creates or replaces the caller's single response (Section
// 2.5.3). Responses are frozen once the poll closes.
func (s *GeneralAffairsService) RespondToPoll(ctx context.Context, actor GAActor, id string, input RespondGAPollInput) (*GAPoll, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	poll, err := s.loadPoll(ctx, db, id)
	if err != nil {
		return nil, err
	}
	eligible, err := s.eligibleForPoll(ctx, poll, actor.UserID)
	if err != nil {
		return nil, err
	}
	if !eligible || poll.Status == "draft" {
		return nil, ErrGANotFound
	}
	if poll.Status != "open" {
		return nil, gaConflict("the poll is closed")
	}
	participating := true
	if poll.PollType == "food_order" {
		if input.Participating == nil {
			return nil, gaInvalid("participating is required for a food order")
		}
		participating = *input.Participating
	}
	optionIDs := map[string]bool{}
	for _, option := range poll.Options {
		optionIDs[option.ID] = true
	}
	type selection struct{ optionID, quantity string }
	selections := []selection{}
	seen := map[string]bool{}
	for index, item := range input.Selections {
		optionID := strings.TrimSpace(item.OptionID)
		if !optionIDs[optionID] {
			return nil, gaInvalid("selections[%d].option_id is not an option of this poll", index)
		}
		if seen[optionID] {
			return nil, gaInvalid("option %s is selected twice", optionID)
		}
		seen[optionID] = true
		quantity := "1"
		if strings.TrimSpace(item.Quantity) != "" {
			if _, quantity, err = parseGADecimal(fmt.Sprintf("selections[%d].quantity", index), item.Quantity, 2, false); err != nil {
				return nil, err
			}
		}
		if poll.PollType == "general" && quantity != "1" {
			return nil, gaInvalid("quantities only apply to food orders")
		}
		selections = append(selections, selection{optionID, quantity})
	}
	switch {
	case participating && len(selections) == 0:
		return nil, gaInvalid("choose at least one option")
	case !participating && len(selections) > 0:
		return nil, gaInvalid("a non-participating response has no selections")
	case poll.PollType == "general" && !poll.AllowMultiple && len(selections) > 1:
		return nil, gaInvalid("this poll allows only one choice")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	var responseID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM ga_poll_responses WHERE poll_id = ? AND responded_by_user_id = ?`, id, actor.UserID).Scan(&responseID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		responseID = uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO ga_poll_responses (id, poll_id, responded_by_user_id, participating, responded_at)
			VALUES (?, ?, ?, ?, ?)`, responseID, id, actor.UserID, boolInt(participating), now); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE ga_poll_responses SET participating = ?, responded_at = ? WHERE id = ?`, boolInt(participating), now, responseID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM ga_poll_response_selections WHERE poll_response_id = ?`, responseID); err != nil {
			return nil, err
		}
	}
	for _, item := range selections {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ga_poll_response_selections (poll_response_id, option_id, quantity) VALUES (?, ?, ?)`,
			responseID, item.optionID, item.quantity); err != nil {
			return nil, err
		}
	}
	// Re-check inside the transaction: the poll may have closed meanwhile.
	var status string
	var closesAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status, closes_at FROM ga_polls WHERE id = ?`, id).Scan(&status, &closesAt); err != nil {
		return nil, err
	}
	if status != "open" || (closesAt.Valid && closesAt.String <= now) {
		return nil, gaConflict("the poll is closed")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPoll(ctx, actor, id)
}

// SettlePoll closes a food order (if still open) and settles it once
// (Section 2.6). finance_expense is refused until Finance has Expense claims
// (finance-system.md Section 6 is not implemented yet).
func (s *GeneralAffairsService) SettlePoll(ctx context.Context, actor GAActor, id string) (*GAPoll, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	poll, err := s.loadPoll(ctx, db, id)
	if err != nil {
		return nil, err
	}
	switch {
	case poll.PollType != "food_order":
		return nil, gaConflict("only a food order poll can be settled")
	case poll.SettledAt != nil:
		return nil, gaConflict("the poll is already settled")
	case poll.Status == "draft":
		return nil, gaConflict("a draft poll cannot be settled")
	case *poll.SettlementMethod == "finance_expense":
		return nil, gaConflict("Finance expense claims are not available yet; settle with salary_deduction or none")
	}
	responses, err := s.pollResponses(ctx, db, poll)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.closePoll(ctx, tx, poll); err != nil {
		return nil, err
	}
	now := s.timestamp()
	type charge struct{ userID, amount string }
	var charges []charge
	if *poll.SettlementMethod == "salary_deduction" {
		for _, response := range responses {
			amount, _ := new(big.Rat).SetString(response.LineTotal)
			if !response.Participating || amount.Sign() == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO ga_salary_deduction_requests (id, poll_id, poll_response_id, user_id, amount, status,
				created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)`, uuid.NewString(), poll.ID, response.ID,
				response.RespondedByUserID, response.LineTotal, now, now); err != nil {
				return nil, err
			}
			charges = append(charges, charge{response.RespondedByUserID, response.LineTotal})
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE ga_polls SET settled_at = ?, settled_by_user_id = ?, updated_at = ? WHERE id = ? AND settled_at IS NULL`,
		now, actor.UserID, now, poll.ID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, gaConflict("the poll was settled concurrently")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, item := range charges {
		s.notifyUsers(ctx, []string{item.userID}, "info", "Meal order settled",
			fmt.Sprintf("%s: %s will be deducted from your salary.", poll.Title, item.amount))
	}
	return s.GetPoll(ctx, actor, id)
}

const gaDeductionColumns = `id, poll_id, poll_response_id, user_id, amount, status, created_at, updated_at`

func scanGADeduction(row rowScanner) (GASalaryDeduction, error) {
	var d GASalaryDeduction
	err := row.Scan(&d.ID, &d.PollID, &d.PollResponseID, &d.UserID, &d.Amount, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrGANotFound
	}
	return d, err
}

// ListSalaryDeductions lists Salary Deduction Requests (Section 2.6.1).
// userID limits it to one person (the self view).
func (s *GeneralAffairsService) ListSalaryDeductions(ctx context.Context, userID string, query ListQuery) (ListResult[GASalaryDeduction], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GASalaryDeduction]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: "ga_salary_deduction_requests", columns: gaDeductionColumns,
		sorts: map[string]string{"created_at": "created_at", "amount": "CAST(amount AS REAL)"}, defaultSort: "created_at", defaultOrder: "desc"}
	if userID != "" {
		spec.where, spec.args = append(spec.where, "user_id = ?"), append(spec.args, userID)
	}
	for _, filter := range []string{"status", "poll_id", "user_id"} {
		if value := filterValue(query, filter); value != "" && !(filter == "user_id" && userID != "") {
			spec.where, spec.args = append(spec.where, filter+" = ?"), append(spec.args, value)
		}
	}
	return gaList(ctx, db, query, spec, scanGADeduction)
}

// CancelSalaryDeduction withdraws a pending deduction. Marking one applied
// belongs to the Salary settlement run, never to this module.
func (s *GeneralAffairsService) CancelSalaryDeduction(ctx context.Context, id string) (*GASalaryDeduction, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `UPDATE ga_salary_deduction_requests SET status = 'cancelled', updated_at = ? WHERE id = ? AND status = 'pending'`,
		s.timestamp(), id)
	if err != nil {
		return nil, err
	}
	deduction, err := scanGADeduction(db.QueryRowContext(ctx, `SELECT `+gaDeductionColumns+` FROM ga_salary_deduction_requests WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, gaConflict("only a pending deduction can be cancelled (status is %s)", deduction.Status)
	}
	return &deduction, nil
}
