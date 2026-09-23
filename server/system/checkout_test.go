package system

import (
	"context"
	"testing"
)

func newCheckoutTestServices(t *testing.T) (*CheckoutService, *CRMService, *UserService, string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	cashier, err := users.Create(context.Background(), adminUserID, CreateUserInput{
		DisplayName: "Cashier",
		Email:       "cashier@example.com",
		Password:    "a-secure-user-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	crm := NewCRMService(directory)
	checkout := NewCheckoutService(directory, users, crm)
	return checkout, crm, users, cashier.ID
}

func completeSimpleCheckout(t *testing.T, checkout *CheckoutService, cashierID, crmCustomerID string, unitPrice string) *CheckoutTransaction {
	t.Helper()
	ctx := context.Background()
	transaction, err := checkout.CreateTransaction(ctx, cashierID, CreateCheckoutTransactionInput{
		WarehouseID:   "main-warehouse",
		CRMCustomerID: crmCustomerID,
		Currency:      "TWD",
	})
	if err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}
	if _, err := checkout.AddLine(ctx, transaction.ID, AddCheckoutLineInput{
		Description: "Widget",
		Quantity:    "1",
		UnitPrice:   unitPrice,
	}); err != nil {
		t.Fatalf("AddLine: %v", err)
	}
	if _, err := checkout.AddPayment(ctx, transaction.ID, AddCheckoutPaymentInput{
		Method: "cash",
		Amount: unitPrice,
	}); err != nil {
		t.Fatalf("AddPayment: %v", err)
	}
	completed, err := checkout.CompleteTransaction(ctx, transaction.ID)
	if err != nil {
		t.Fatalf("CompleteTransaction: %v", err)
	}
	return completed
}

// TestCompleteTransactionWithoutCRMCustomerSkipsPoints covers the anonymous
// checkout path (checkout-system.md §4.5): no crm_customer_id means
// completion never touches CRM at all.
func TestCompleteTransactionWithoutCRMCustomerSkipsPoints(t *testing.T) {
	checkout, _, _, cashierID := newCheckoutTestServices(t)
	completed := completeSimpleCheckout(t, checkout, cashierID, "", "100")
	if completed.Status != CheckoutStatusCompleted {
		t.Fatalf("expected completed status, got %q", completed.Status)
	}
	if completed.CRMCustomerID != nil {
		t.Fatalf("expected no crm_customer_id, got %v", *completed.CRMCustomerID)
	}
}

// TestCompleteTransactionEarnsPointsForLinkedMember covers crm-system.md
// §3.2 / checkout-system.md §4.5: completing a transaction linked to a CRM
// member posts an "earned" ledger row sized by the tier's Points Earning
// Rule against total_amount (post-discount, here equal to the unit price).
func TestCompleteTransactionEarnsPointsForLinkedMember(t *testing.T) {
	checkout, crm, _, cashierID := newCheckoutTestServices(t)
	ctx := context.Background()

	tier, err := crm.CreateMembershipTier(ctx, CreateCRMMembershipTierInput{Name: "Gold", Status: "active"})
	if err != nil {
		t.Fatalf("CreateMembershipTier: %v", err)
	}
	customer, err := crm.CreateCustomer(ctx, CreateCRMCustomerInput{
		PartyType: "individual",
		Segment:   "b2c",
		Name:      "Member One",
		Status:    "active",
	})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if _, err := crm.CreateMembership(ctx, CreateCRMMembershipInput{
		CustomerID:       customer.ID,
		MembershipTierID: tier.ID,
		JoinedAt:         "2026-01-01",
		Status:           "active",
	}); err != nil {
		t.Fatalf("CreateMembership: %v", err)
	}
	if _, err := crm.CreatePointsEarningRule(ctx, CreateCRMPointsEarningRuleInput{
		MembershipTierID:      tier.ID,
		PointsPerCurrencyUnit: "2",
		Status:                "active",
	}); err != nil {
		t.Fatalf("CreatePointsEarningRule: %v", err)
	}

	completed := completeSimpleCheckout(t, checkout, cashierID, customer.ID, "100")
	if completed.Status != CheckoutStatusCompleted {
		t.Fatalf("expected completed status, got %q", completed.Status)
	}

	balance, err := crm.GetPointsBalance(ctx, customer.ID)
	if err != nil {
		t.Fatalf("GetPointsBalance: %v", err)
	}
	if balance.Balance != 200 {
		t.Fatalf("expected 200 points earned (100 * 2/unit), got %d", balance.Balance)
	}

	entries, err := crm.ListPointsLedger(ctx, customer.ID, ListQuery{})
	if err != nil {
		t.Fatalf("ListPointsLedger: %v", err)
	}
	if len(entries.Items) != 1 {
		t.Fatalf("expected exactly one ledger entry, got %d", len(entries.Items))
	}
	entry := entries.Items[0]
	if entry.EntryType != "earned" || entry.SourceModule == nil || *entry.SourceModule != "checkout" ||
		entry.SourceReferenceID == nil || *entry.SourceReferenceID != completed.ID {
		t.Fatalf("unexpected ledger entry: %+v", entry)
	}
}

// TestCompleteTransactionSkipsPointsWithoutMatchingRule covers a member
// linked to a tier with no configured Points Earning Rule and no
// tier-unset default rule: completion still succeeds, just with zero points.
func TestCompleteTransactionSkipsPointsWithoutMatchingRule(t *testing.T) {
	checkout, crm, _, cashierID := newCheckoutTestServices(t)
	ctx := context.Background()

	tier, err := crm.CreateMembershipTier(ctx, CreateCRMMembershipTierInput{Name: "Silver", Status: "active"})
	if err != nil {
		t.Fatalf("CreateMembershipTier: %v", err)
	}
	customer, err := crm.CreateCustomer(ctx, CreateCRMCustomerInput{
		PartyType: "individual",
		Segment:   "b2c",
		Name:      "Member Two",
		Status:    "active",
	})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if _, err := crm.CreateMembership(ctx, CreateCRMMembershipInput{
		CustomerID:       customer.ID,
		MembershipTierID: tier.ID,
		JoinedAt:         "2026-01-01",
		Status:           "active",
	}); err != nil {
		t.Fatalf("CreateMembership: %v", err)
	}

	completed := completeSimpleCheckout(t, checkout, cashierID, customer.ID, "50")
	if completed.Status != CheckoutStatusCompleted {
		t.Fatalf("expected completed status, got %q", completed.Status)
	}

	balance, err := crm.GetPointsBalance(ctx, customer.ID)
	if err != nil {
		t.Fatalf("GetPointsBalance: %v", err)
	}
	if balance.Balance != 0 {
		t.Fatalf("expected 0 points with no matching rule, got %d", balance.Balance)
	}
}
