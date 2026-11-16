package system

// Section 2.2 Office Supply Requisitions and Section 2.4 Seal & Stamp
// Management. Both are "request → approval → fulfilment" records routed
// through routeApproval (Section 3.1).

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Requisitions
// ---------------------------------------------------------------------------

type GARequisition struct {
	ID                         string   `json:"id"`
	RequestedByUserID          string   `json:"requested_by_user_id"`
	Description                string   `json:"description"`
	Quantity                   *string  `json:"quantity"`
	EstimatedAmount            *string  `json:"estimated_amount"`
	Status                     string   `json:"status"`
	ApprovalRequestID          *string  `json:"approval_request_id"`
	ProcurementPurchaseOrderID *string  `json:"procurement_purchase_order_id"`
	InventoryOutboundID        *string  `json:"inventory_outbound_id"`
	FulfillmentLinkStatus      *string  `json:"fulfillment_link_status"`
	DecidedByUserID            *string  `json:"decided_by_user_id"`
	DecidedAt                  *string  `json:"decided_at"`
	DecisionComment            *string  `json:"decision_comment"`
	FulfilledByUserID          *string  `json:"fulfilled_by_user_id"`
	FulfilledAt                *string  `json:"fulfilled_at"`
	FallbackApproverUserIDs    []string `json:"fallback_approver_user_ids"`
	CreatedAt                  string   `json:"created_at"`
	UpdatedAt                  string   `json:"updated_at"`
}

type SubmitGARequisitionInput struct {
	Description     string  `json:"description"`
	Quantity        *string `json:"quantity"`
	EstimatedAmount *string `json:"estimated_amount"`
}

type GADecisionInput struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

type FulfillGARequisitionInput struct {
	ProcurementPurchaseOrderID *string `json:"procurement_purchase_order_id"`
	InventoryOutboundID        *string `json:"inventory_outbound_id"`
}

const gaRequisitionColumns = `id, requested_by_user_id, description, quantity, estimated_amount, status, approval_request_id,
	procurement_purchase_order_id, inventory_outbound_id, decided_by_user_id, decided_at, decision_comment,
	fulfilled_by_user_id, fulfilled_at, created_at, updated_at`

func scanGARequisition(row rowScanner) (GARequisition, error) {
	var r GARequisition
	err := row.Scan(&r.ID, &r.RequestedByUserID, &r.Description, &r.Quantity, &r.EstimatedAmount, &r.Status, &r.ApprovalRequestID,
		&r.ProcurementPurchaseOrderID, &r.InventoryOutboundID, &r.DecidedByUserID, &r.DecidedAt, &r.DecisionComment,
		&r.FulfilledByUserID, &r.FulfilledAt, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrGANotFound
	}
	return r, err
}

func (s *GeneralAffairsService) SubmitRequisition(ctx context.Context, actor GAActor, input SubmitGARequisitionInput) (*GARequisition, error) {
	description, err := requireText("description", input.Description, 2000)
	if err != nil {
		return nil, err
	}
	quantity, err := optionalDecimal("quantity", input.Quantity, 3)
	if err != nil {
		return nil, err
	}
	amount, err := optionalDecimal("estimated_amount", input.EstimatedAmount, 2)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id, now := uuid.NewString(), s.timestamp()
	if _, err := db.ExecContext(ctx, `INSERT INTO ga_requisitions (id, requested_by_user_id, description, quantity, estimated_amount,
		status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'pending_approval', ?, ?)`, id, actor.UserID, description, quantity, amount, now, now); err != nil {
		return nil, err
	}
	approvalID, err := s.routeApproval(ctx, db, GARequestTypeRequisition, "requisition", id, actor.UserID, amount)
	if err != nil {
		_, _ = db.ExecContext(ctx, `DELETE FROM ga_fallback_approvers WHERE subject_type = 'requisition' AND subject_id = ?`, id)
		_, _ = db.ExecContext(ctx, `DELETE FROM ga_requisitions WHERE id = ?`, id)
		return nil, err
	}
	if approvalID != nil {
		if _, err := db.ExecContext(ctx, `UPDATE ga_requisitions SET approval_request_id = ? WHERE id = ?`, *approvalID, id); err != nil {
			return nil, err
		}
	}
	return s.loadRequisition(ctx, db, id)
}

// loadRequisition reads one requisition and applies the read-derived updates:
// the Approval engine's outcome, and fulfilment through a linked Purchase
// Order (closed) or Outbound (shipped).
func (s *GeneralAffairsService) loadRequisition(ctx context.Context, db *sql.DB, id string) (*GARequisition, error) {
	r, err := scanGARequisition(db.QueryRowContext(ctx, `SELECT `+gaRequisitionColumns+` FROM ga_requisitions WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := s.syncRequisition(ctx, db, &r); err != nil {
		return nil, err
	}
	if r.FallbackApproverUserIDs, err = s.fallbackApprovers(ctx, db, "requisition", r.ID); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *GeneralAffairsService) syncRequisition(ctx context.Context, db *sql.DB, r *GARequisition) error {
	now := s.timestamp()
	if r.Status == "pending_approval" && r.ApprovalRequestID != nil {
		outcome, err := s.approvalOutcome(ctx, *r.ApprovalRequestID)
		if err != nil {
			return err
		}
		if outcome != "" {
			if _, err := db.ExecContext(ctx, `UPDATE ga_requisitions SET status = ?, decided_at = ?, updated_at = ?
				WHERE id = ? AND status = 'pending_approval'`, outcome, now, now, r.ID); err != nil {
				return err
			}
			r.Status, r.DecidedAt, r.UpdatedAt = outcome, &now, now
		}
	}
	var linkStatus string
	var err error
	switch {
	case r.ProcurementPurchaseOrderID != nil:
		linkStatus, err = s.purchaseOrderStatus(ctx, *r.ProcurementPurchaseOrderID)
	case r.InventoryOutboundID != nil:
		linkStatus, err = s.outboundStatus(ctx, *r.InventoryOutboundID)
	default:
		return nil
	}
	switch {
	case errors.Is(err, errGAModuleAbsent):
		linkStatus = "module_not_installed"
	case errors.Is(err, ErrGANotFound):
		linkStatus = "missing"
	case err != nil:
		return err
	}
	r.FulfillmentLinkStatus = &linkStatus
	if r.Status == "approved" && (linkStatus == "closed" || linkStatus == "shipped") {
		if _, err := db.ExecContext(ctx, `UPDATE ga_requisitions SET status = 'fulfilled', fulfilled_at = ?, updated_at = ?
			WHERE id = ? AND status = 'approved'`, now, now, r.ID); err != nil {
			return err
		}
		r.Status, r.FulfilledAt, r.UpdatedAt = "fulfilled", &now, now
	}
	return nil
}

// GetRequisition is visible to the requester, its fallback approvers, and
// holders of general_affairs.requisitions.manage.
func (s *GeneralAffairsService) GetRequisition(ctx context.Context, actor GAActor, id string) (*GARequisition, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadRequisition(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if r.RequestedByUserID != actor.UserID && !actor.Has("general_affairs.requisitions.manage") && !containsString(r.FallbackApproverUserIDs, actor.UserID) {
		return nil, ErrGANotFound
	}
	return r, nil
}

// ListRequisitions scopes: "mine" (own requests), "approvals" (awaiting the
// caller's fallback decision), "all" (requisitions.manage).
func (s *GeneralAffairsService) ListRequisitions(ctx context.Context, actor GAActor, scope string, query ListQuery) (ListResult[GARequisition], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GARequisition]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: "ga_requisitions", columns: gaRequisitionColumns,
		keywordWhere: "description LIKE ?", keywordArgs: 1,
		sorts:       map[string]string{"created_at": "created_at", "updated_at": "updated_at", "status": "status"},
		defaultSort: "created_at", defaultOrder: "desc"}
	switch scope {
	case "mine":
		spec.where, spec.args = append(spec.where, "requested_by_user_id = ?"), append(spec.args, actor.UserID)
	case "approvals":
		spec.where = append(spec.where, "status = 'pending_approval'", "approval_request_id IS NULL",
			"id IN (SELECT subject_id FROM ga_fallback_approvers WHERE subject_type = 'requisition' AND user_id = ?)")
		spec.args = append(spec.args, actor.UserID)
	case "all":
		if requester := filterValue(query, "requested_by_user_id"); requester != "" {
			spec.where, spec.args = append(spec.where, "requested_by_user_id = ?"), append(spec.args, requester)
		}
	default:
		return ListResult[GARequisition]{}, gaInvalid("unknown scope")
	}
	if status := filterValue(query, "status"); status != "" {
		spec.where, spec.args = append(spec.where, "status = ?"), append(spec.args, status)
	}
	result, err := gaList(ctx, db, query, spec, scanGARequisition)
	if err != nil {
		return result, err
	}
	for index := range result.Items {
		if err := s.syncRequisition(ctx, db, &result.Items[index]); err != nil {
			return result, err
		}
	}
	return result, nil
}

// DecideRequisition is the fallback decision path. Requests routed through
// the Approval engine are decided there instead.
func (s *GeneralAffairsService) DecideRequisition(ctx context.Context, actor GAActor, id string, input GADecisionInput) (*GARequisition, error) {
	status, comment, err := parseGADecision(input)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadRequisition(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if err := requireFallbackDecision(r.Status, r.ApprovalRequestID, r.RequestedByUserID, actor.UserID,
		containsString(r.FallbackApproverUserIDs, actor.UserID)); err != nil {
		return nil, err
	}
	now := s.timestamp()
	result, err := db.ExecContext(ctx, `UPDATE ga_requisitions SET status = ?, decided_by_user_id = ?, decided_at = ?, decision_comment = ?,
		updated_at = ? WHERE id = ? AND status = 'pending_approval'`, status, actor.UserID, now, comment, now, id)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, gaConflict("the requisition was decided concurrently")
	}
	s.notifyUsers(ctx, []string{r.RequestedByUserID}, decisionNoticeType(status), "Supply requisition "+status,
		"Your supply requisition \""+truncate(r.Description, 80)+"\" was "+status+".")
	return s.loadRequisition(ctx, db, id)
}

func (s *GeneralAffairsService) CancelRequisition(ctx context.Context, actor GAActor, id string) (*GARequisition, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadRequisition(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if r.RequestedByUserID != actor.UserID {
		return nil, ErrPermissionDenied
	}
	if r.Status != "pending_approval" && r.Status != "approved" {
		return nil, gaConflict("a %s requisition cannot be cancelled", r.Status)
	}
	if r.Status == "pending_approval" && r.ApprovalRequestID != nil && s.approvals != nil {
		if _, err := s.approvals.CancelRequest(ctx, actor.UserID, *r.ApprovalRequestID); err != nil {
			return nil, err
		}
	}
	now := s.timestamp()
	if _, err := db.ExecContext(ctx, `UPDATE ga_requisitions SET status = 'cancelled', updated_at = ? WHERE id = ? AND status IN ('pending_approval', 'approved')`, now, id); err != nil {
		return nil, err
	}
	return s.loadRequisition(ctx, db, id)
}

// FulfillRequisition marks an approved requisition fulfilled directly, or
// links it to the Purchase Order / Outbound that will fulfil it (Sections
// 3.4, 3.7); a linked requisition becomes fulfilled when that record reaches
// closed / shipped.
func (s *GeneralAffairsService) FulfillRequisition(ctx context.Context, actor GAActor, id string, input FulfillGARequisitionInput) (*GARequisition, error) {
	poID, err := optionalText("procurement_purchase_order_id", input.ProcurementPurchaseOrderID, 100)
	if err != nil {
		return nil, err
	}
	outboundID, err := optionalText("inventory_outbound_id", input.InventoryOutboundID, 100)
	if err != nil {
		return nil, err
	}
	if poID != nil && outboundID != nil {
		return nil, gaInvalid("a requisition is fulfilled by a purchase order or a stock outbound, never both")
	}
	check := func(module string, lookup func(context.Context, string) (string, error), ref string) error {
		status, err := lookup(ctx, ref)
		switch {
		case errors.Is(err, errGAModuleAbsent):
			return gaInvalid("%s is not installed", module)
		case errors.Is(err, ErrGANotFound):
			return gaInvalid("%s record %q does not exist", module, ref)
		case err != nil:
			return err
		case status == "cancelled":
			return gaConflict("%s record %q is cancelled", module, ref)
		}
		return nil
	}
	if poID != nil {
		if err := check("Procurement", s.purchaseOrderStatus, *poID); err != nil {
			return nil, err
		}
	}
	if outboundID != nil {
		if err := check("Inventory", s.outboundStatus, *outboundID); err != nil {
			return nil, err
		}
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadRequisition(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if r.Status != "approved" {
		return nil, gaConflict("only an approved requisition can be fulfilled (status is %s)", r.Status)
	}
	now := s.timestamp()
	if poID == nil && outboundID == nil {
		_, err = db.ExecContext(ctx, `UPDATE ga_requisitions SET status = 'fulfilled', fulfilled_by_user_id = ?, fulfilled_at = ?,
			updated_at = ? WHERE id = ? AND status = 'approved'`, actor.UserID, now, now, id)
	} else {
		_, err = db.ExecContext(ctx, `UPDATE ga_requisitions SET procurement_purchase_order_id = ?, inventory_outbound_id = ?,
			fulfilled_by_user_id = ?, updated_at = ? WHERE id = ? AND status = 'approved'`, poID, outboundID, actor.UserID, now, id)
	}
	if err != nil {
		return nil, err
	}
	updated, err := s.loadRequisition(ctx, db, id)
	if err == nil && updated.Status == "fulfilled" {
		s.notifyUsers(ctx, []string{r.RequestedByUserID}, "success", "Supply requisition fulfilled",
			"Your supply requisition \""+truncate(r.Description, 80)+"\" has been fulfilled.")
	}
	return updated, err
}

// ---------------------------------------------------------------------------
// Seals
// ---------------------------------------------------------------------------

type GASeal struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	CustodianUserID string `json:"custodian_user_id"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type GASealInput struct {
	Name            *string `json:"name"`
	CustodianUserID *string `json:"custodian_user_id"`
	Status          *string `json:"status"`
}

type GASealRequest struct {
	ID                      string          `json:"id"`
	SealID                  string          `json:"seal_id"`
	RequestedByUserID       string          `json:"requested_by_user_id"`
	DocumentReference       string          `json:"document_reference"`
	RequestedUseDate        string          `json:"requested_use_date"`
	Status                  string          `json:"status"`
	ApprovalRequestID       *string         `json:"approval_request_id"`
	DecidedByUserID         *string         `json:"decided_by_user_id"`
	DecidedAt               *string         `json:"decided_at"`
	DecisionComment         *string         `json:"decision_comment"`
	FallbackApproverUserIDs []string        `json:"fallback_approver_user_ids"`
	Usage                   *GASealUsageLog `json:"usage"`
	CreatedAt               string          `json:"created_at"`
	UpdatedAt               string          `json:"updated_at"`
}

type GASealUsageLog struct {
	ID                 string  `json:"id"`
	SealUsageRequestID string  `json:"seal_usage_request_id"`
	SealID             string  `json:"seal_id"`
	UsedByUserID       string  `json:"used_by_user_id"`
	UsedAt             string  `json:"used_at"`
	ReturnedAt         *string `json:"returned_at"`
	RecordedByUserID   string  `json:"recorded_by_user_id"`
}

type SubmitGASealRequestInput struct {
	SealID            string `json:"seal_id"`
	DocumentReference string `json:"document_reference"`
	RequestedUseDate  string `json:"requested_use_date"`
}

type RecordGASealUseInput struct {
	UsedByUserID *string `json:"used_by_user_id"`
	UsedAt       *string `json:"used_at"`
}

const gaSealColumns = `id, name, custodian_user_id, status, created_at, updated_at`

func scanGASeal(row rowScanner) (GASeal, error) {
	var seal GASeal
	err := row.Scan(&seal.ID, &seal.Name, &seal.CustodianUserID, &seal.Status, &seal.CreatedAt, &seal.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return seal, ErrGANotFound
	}
	return seal, err
}

func (s *GeneralAffairsService) ListSeals(ctx context.Context, query ListQuery) (ListResult[GASeal], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GASeal]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: "ga_seals", columns: gaSealColumns, keywordWhere: "name LIKE ?", keywordArgs: 1,
		sorts: map[string]string{"name": "name COLLATE NOCASE", "status": "status", "created_at": "created_at"}, defaultSort: "name", defaultOrder: "asc"}
	if status := filterValue(query, "status"); status != "" {
		spec.where, spec.args = append(spec.where, "status = ?"), append(spec.args, status)
	}
	return gaList(ctx, db, query, spec, scanGASeal)
}

func (s *GeneralAffairsService) CreateSeal(ctx context.Context, input GASealInput) (*GASeal, error) {
	if input.Name == nil || input.CustodianUserID == nil {
		return nil, gaInvalid("name and custodian_user_id are required")
	}
	name, err := requireText("name", *input.Name, 100)
	if err != nil {
		return nil, err
	}
	custodian := strings.TrimSpace(*input.CustodianUserID)
	if err := s.requireActiveUser(ctx, "custodian_user_id", custodian); err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id, now := uuid.NewString(), s.timestamp()
	if _, err := db.ExecContext(ctx, `INSERT INTO ga_seals (`+gaSealColumns+`) VALUES (?, ?, ?, 'available', ?, ?)`, id, name, custodian, now, now); err != nil {
		return nil, err
	}
	seal, err := scanGASeal(db.QueryRowContext(ctx, `SELECT `+gaSealColumns+` FROM ga_seals WHERE id = ?`, id))
	return &seal, err
}

// UpdateSeal changes name/custodian, or marks a seal lost or found again.
// in_use is only ever set by recording an actual use.
func (s *GeneralAffairsService) UpdateSeal(ctx context.Context, id string, input GASealInput) (*GASeal, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	seal, err := scanGASeal(db.QueryRowContext(ctx, `SELECT `+gaSealColumns+` FROM ga_seals WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		if seal.Name, err = requireText("name", *input.Name, 100); err != nil {
			return nil, err
		}
	}
	if input.CustodianUserID != nil {
		custodian := strings.TrimSpace(*input.CustodianUserID)
		if err := s.requireActiveUser(ctx, "custodian_user_id", custodian); err != nil {
			return nil, err
		}
		seal.CustodianUserID = custodian
	}
	if input.Status != nil {
		next := strings.TrimSpace(*input.Status)
		switch {
		case next != "lost" && next != "available":
			return nil, gaInvalid("status can only be set to lost or available; in_use follows recorded usage")
		case seal.Status == "in_use":
			return nil, gaConflict("record the seal's return before changing its status")
		}
		seal.Status = next
	}
	seal.UpdatedAt = s.timestamp()
	if _, err := db.ExecContext(ctx, `UPDATE ga_seals SET name = ?, custodian_user_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		seal.Name, seal.CustodianUserID, seal.Status, seal.UpdatedAt, id); err != nil {
		return nil, err
	}
	return &seal, nil
}

const gaSealRequestColumns = `id, seal_id, requested_by_user_id, document_reference, requested_use_date, status, approval_request_id,
	decided_by_user_id, decided_at, decision_comment, created_at, updated_at`

func scanGASealRequest(row rowScanner) (GASealRequest, error) {
	var r GASealRequest
	err := row.Scan(&r.ID, &r.SealID, &r.RequestedByUserID, &r.DocumentReference, &r.RequestedUseDate, &r.Status, &r.ApprovalRequestID,
		&r.DecidedByUserID, &r.DecidedAt, &r.DecisionComment, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrGANotFound
	}
	return r, err
}

func (s *GeneralAffairsService) SubmitSealRequest(ctx context.Context, actor GAActor, input SubmitGASealRequestInput) (*GASealRequest, error) {
	reference, err := requireText("document_reference", input.DocumentReference, 500)
	if err != nil {
		return nil, err
	}
	useDate, err := parseGADate("requested_use_date", input.RequestedUseDate)
	if err != nil {
		return nil, err
	}
	if useDate < s.now().Format("2006-01-02") {
		return nil, gaInvalid("requested_use_date cannot be in the past")
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	seal, err := scanGASeal(db.QueryRowContext(ctx, `SELECT `+gaSealColumns+` FROM ga_seals WHERE id = ?`, strings.TrimSpace(input.SealID)))
	if errors.Is(err, ErrGANotFound) {
		return nil, gaInvalid("seal_id does not exist")
	}
	if err != nil {
		return nil, err
	}
	if seal.Status == "lost" {
		return nil, gaConflict("this seal is marked lost")
	}
	id, now := uuid.NewString(), s.timestamp()
	if _, err := db.ExecContext(ctx, `INSERT INTO ga_seal_requests (id, seal_id, requested_by_user_id, document_reference,
		requested_use_date, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'pending_approval', ?, ?)`,
		id, seal.ID, actor.UserID, reference, useDate, now, now); err != nil {
		return nil, err
	}
	approvalID, err := s.routeApproval(ctx, db, GARequestTypeSealUsage, "seal_usage", id, actor.UserID, nil)
	if err != nil {
		_, _ = db.ExecContext(ctx, `DELETE FROM ga_fallback_approvers WHERE subject_type = 'seal_usage' AND subject_id = ?`, id)
		_, _ = db.ExecContext(ctx, `DELETE FROM ga_seal_requests WHERE id = ?`, id)
		return nil, err
	}
	if approvalID != nil {
		if _, err := db.ExecContext(ctx, `UPDATE ga_seal_requests SET approval_request_id = ? WHERE id = ?`, *approvalID, id); err != nil {
			return nil, err
		}
	}
	return s.loadSealRequest(ctx, db, id)
}

func (s *GeneralAffairsService) loadSealRequest(ctx context.Context, db *sql.DB, id string) (*GASealRequest, error) {
	r, err := scanGASealRequest(db.QueryRowContext(ctx, `SELECT `+gaSealRequestColumns+` FROM ga_seal_requests WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := s.syncSealRequest(ctx, db, &r); err != nil {
		return nil, err
	}
	if r.FallbackApproverUserIDs, err = s.fallbackApprovers(ctx, db, "seal_usage", r.ID); err != nil {
		return nil, err
	}
	var usage GASealUsageLog
	err = db.QueryRowContext(ctx, `SELECT id, seal_usage_request_id, seal_id, used_by_user_id, used_at, returned_at, recorded_by_user_id
		FROM ga_seal_usage_log WHERE seal_usage_request_id = ?`, id).Scan(&usage.ID, &usage.SealUsageRequestID, &usage.SealID,
		&usage.UsedByUserID, &usage.UsedAt, &usage.ReturnedAt, &usage.RecordedByUserID)
	if err == nil {
		r.Usage = &usage
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return &r, nil
}

func (s *GeneralAffairsService) syncSealRequest(ctx context.Context, db *sql.DB, r *GASealRequest) error {
	if r.Status != "pending_approval" || r.ApprovalRequestID == nil {
		return nil
	}
	outcome, err := s.approvalOutcome(ctx, *r.ApprovalRequestID)
	if err != nil || outcome == "" {
		return err
	}
	now := s.timestamp()
	if _, err := db.ExecContext(ctx, `UPDATE ga_seal_requests SET status = ?, decided_at = ?, updated_at = ?
		WHERE id = ? AND status = 'pending_approval'`, outcome, now, now, r.ID); err != nil {
		return err
	}
	r.Status, r.DecidedAt, r.UpdatedAt = outcome, &now, now
	return nil
}

func (s *GeneralAffairsService) GetSealRequest(ctx context.Context, actor GAActor, id string) (*GASealRequest, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadSealRequest(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if r.RequestedByUserID != actor.UserID && !actor.Has("general_affairs.seals.manage") && !containsString(r.FallbackApproverUserIDs, actor.UserID) {
		return nil, ErrGANotFound
	}
	return r, nil
}

// ListSealRequests scopes: "mine", "approvals" (awaiting the caller's
// fallback decision), "all" (seals.manage).
func (s *GeneralAffairsService) ListSealRequests(ctx context.Context, actor GAActor, scope string, query ListQuery) (ListResult[GASealRequest], error) {
	db, err := s.open()
	if err != nil {
		return ListResult[GASealRequest]{}, err
	}
	defer db.Close()
	spec := gaListSpec{from: "ga_seal_requests", columns: gaSealRequestColumns, keywordWhere: "document_reference LIKE ?", keywordArgs: 1,
		sorts:       map[string]string{"created_at": "created_at", "requested_use_date": "requested_use_date", "status": "status"},
		defaultSort: "created_at", defaultOrder: "desc"}
	switch scope {
	case "mine":
		spec.where, spec.args = append(spec.where, "requested_by_user_id = ?"), append(spec.args, actor.UserID)
	case "approvals":
		spec.where = append(spec.where, "status = 'pending_approval'", "approval_request_id IS NULL",
			"(id IN (SELECT subject_id FROM ga_fallback_approvers WHERE subject_type = 'seal_usage' AND user_id = ?) OR ?)")
		spec.args = append(spec.args, actor.UserID, actor.Has("general_affairs.seals.manage"))
	case "all":
	default:
		return ListResult[GASealRequest]{}, gaInvalid("unknown scope")
	}
	for _, filter := range []string{"status", "seal_id"} {
		if value := filterValue(query, filter); value != "" {
			spec.where, spec.args = append(spec.where, filter+" = ?"), append(spec.args, value)
		}
	}
	result, err := gaList(ctx, db, query, spec, scanGASealRequest)
	if err != nil {
		return result, err
	}
	for index := range result.Items {
		if err := s.syncSealRequest(ctx, db, &result.Items[index]); err != nil {
			return result, err
		}
	}
	return result, nil
}

// DecideSealRequest is the non-engine decision path: the requester's
// resolved manager, or a seals.manage holder ("approve usage outside the
// Approval System fallback", Section 4). Nobody decides their own request.
func (s *GeneralAffairsService) DecideSealRequest(ctx context.Context, actor GAActor, id string, input GADecisionInput) (*GASealRequest, error) {
	status, comment, err := parseGADecision(input)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadSealRequest(ctx, db, id)
	if err != nil {
		return nil, err
	}
	allowed := containsString(r.FallbackApproverUserIDs, actor.UserID) || actor.Has("general_affairs.seals.manage")
	if err := requireFallbackDecision(r.Status, r.ApprovalRequestID, r.RequestedByUserID, actor.UserID, allowed); err != nil {
		return nil, err
	}
	now := s.timestamp()
	result, err := db.ExecContext(ctx, `UPDATE ga_seal_requests SET status = ?, decided_by_user_id = ?, decided_at = ?, decision_comment = ?,
		updated_at = ? WHERE id = ? AND status = 'pending_approval'`, status, actor.UserID, now, comment, now, id)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, gaConflict("the request was decided concurrently")
	}
	s.notifyUsers(ctx, []string{r.RequestedByUserID}, decisionNoticeType(status), "Seal usage request "+status,
		"Your request to use a seal for \""+truncate(r.DocumentReference, 80)+"\" was "+status+".")
	return s.loadSealRequest(ctx, db, id)
}

func (s *GeneralAffairsService) CancelSealRequest(ctx context.Context, actor GAActor, id string) (*GASealRequest, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadSealRequest(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if r.RequestedByUserID != actor.UserID {
		return nil, ErrPermissionDenied
	}
	if r.Status != "pending_approval" && r.Status != "approved" {
		return nil, gaConflict("a %s seal request cannot be cancelled", r.Status)
	}
	if r.Status == "pending_approval" && r.ApprovalRequestID != nil && s.approvals != nil {
		if _, err := s.approvals.CancelRequest(ctx, actor.UserID, *r.ApprovalRequestID); err != nil {
			return nil, err
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE ga_seal_requests SET status = 'cancelled', updated_at = ? WHERE id = ? AND status IN ('pending_approval', 'approved')`,
		s.timestamp(), id); err != nil {
		return nil, err
	}
	return s.loadSealRequest(ctx, db, id)
}

// RecordSealUse writes the usage log entry for an approved request (Section
// 2.4.3) and marks the seal in use until its return is recorded.
func (s *GeneralAffairsService) RecordSealUse(ctx context.Context, actor GAActor, requestID string, input RecordGASealUseInput) (*GASealRequest, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadSealRequest(ctx, db, requestID)
	if err != nil {
		return nil, err
	}
	if r.Status != "approved" {
		return nil, gaConflict("only an approved request can be used (status is %s)", r.Status)
	}
	usedBy := r.RequestedByUserID
	if input.UsedByUserID != nil && strings.TrimSpace(*input.UsedByUserID) != "" {
		usedBy = strings.TrimSpace(*input.UsedByUserID)
		if err := s.requireActiveUser(ctx, "used_by_user_id", usedBy); err != nil {
			return nil, err
		}
	}
	usedAt := s.timestamp()
	if input.UsedAt != nil && strings.TrimSpace(*input.UsedAt) != "" {
		var parsed string
		if parsed, _, err = parseGATime("used_at", *input.UsedAt); err != nil {
			return nil, err
		}
		if parsed > usedAt {
			return nil, gaInvalid("used_at cannot be in the future")
		}
		usedAt = parsed
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	result, err := tx.ExecContext(ctx, `UPDATE ga_seals SET status = 'in_use', updated_at = ? WHERE id = ? AND status = 'available'`, now, r.SealID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, gaConflict("the seal is not available (in use or lost)")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ga_seal_usage_log (id, seal_usage_request_id, seal_id, used_by_user_id, used_at, recorded_by_user_id)
		VALUES (?, ?, ?, ?, ?, ?)`, uuid.NewString(), requestID, r.SealID, usedBy, usedAt, actor.UserID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ga_seal_requests SET status = 'used', updated_at = ? WHERE id = ? AND status = 'approved'`, now, requestID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.loadSealRequest(ctx, db, requestID)
}

func (s *GeneralAffairsService) RecordSealReturn(ctx context.Context, requestID string) (*GASealRequest, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	r, err := s.loadSealRequest(ctx, db, requestID)
	if err != nil {
		return nil, err
	}
	if r.Usage == nil || r.Usage.ReturnedAt != nil {
		return nil, gaConflict("there is no outstanding use of the seal for this request")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `UPDATE ga_seal_usage_log SET returned_at = ? WHERE id = ? AND returned_at IS NULL`, now, r.Usage.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ga_seals SET status = 'available', updated_at = ? WHERE id = ? AND status = 'in_use'`, now, r.SealID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.loadSealRequest(ctx, db, requestID)
}

func (s *GeneralAffairsService) ListSealUsage(ctx context.Context, sealID string) ([]GASealUsageLog, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, seal_usage_request_id, seal_id, used_by_user_id, used_at, returned_at, recorded_by_user_id
		FROM ga_seal_usage_log WHERE seal_id = ? ORDER BY used_at DESC LIMIT 500`, sealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []GASealUsageLog{}
	for rows.Next() {
		var entry GASealUsageLog
		if err := rows.Scan(&entry.ID, &entry.SealUsageRequestID, &entry.SealID, &entry.UsedByUserID, &entry.UsedAt, &entry.ReturnedAt, &entry.RecordedByUserID); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}

// ---------------------------------------------------------------------------
// shared decision helpers
// ---------------------------------------------------------------------------

func parseGADecision(input GADecisionInput) (string, *string, error) {
	var status string
	switch strings.TrimSpace(input.Decision) {
	case "approve", "approved":
		status = "approved"
	case "reject", "rejected":
		status = "rejected"
	default:
		return "", nil, gaInvalid("decision must be approve or reject")
	}
	comment, err := optionalText("comment", &input.Comment, 1000)
	return status, comment, err
}

func requireFallbackDecision(status string, approvalRequestID *string, requesterID, actorID string, allowed bool) error {
	switch {
	case approvalRequestID != nil:
		return gaConflict("this request is routed through the Approval System; decide it there")
	case status != "pending_approval":
		return gaConflict("the request is already %s", status)
	case requesterID == actorID:
		return ErrPermissionDenied
	case !allowed:
		return ErrPermissionDenied
	}
	return nil
}

func decisionNoticeType(status string) string {
	if status == "approved" {
		return "success"
	}
	return "warning"
}
