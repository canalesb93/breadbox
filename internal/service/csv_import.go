package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// CSVImportParams holds the parameters for a CSV import.
type CSVImportParams struct {
	UserID          string
	AccountName     string         // defaults to "CSV Import"
	ColumnMapping   map[string]int // "date"→col, "amount"→col, "description"→col, optional: "category", "merchant_name"
	Rows            [][]string
	PositiveIsDebit bool
	DateFormat      string
	ConnectionID    string // empty = new connection, non-empty = re-import
	HasDebitCredit  bool   // if true, use "debit" and "credit" column mappings
}

// CSVImportResult holds the result of a CSV import.
type CSVImportResult struct {
	ConnectionID string
	AccountID    string
	TotalRows    int
	NewCount     int
	UpdatedCount int
	SkippedCount int
	SkipReasons  []string
}

// ImportCSV orchestrates the full CSV import flow: create/reuse connection+account,
// parse rows, and upsert transactions.
func (s *Service) ImportCSV(ctx context.Context, params CSVImportParams) (*CSVImportResult, error) {
	if params.AccountName == "" {
		params.AccountName = "CSV Import"
	}

	userID, err := parseUUID(params.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	var connID pgtype.UUID
	var accountID pgtype.UUID

	if params.ConnectionID != "" {
		// Re-import: load existing connection and get first account.
		connID, err = parseUUID(params.ConnectionID)
		if err != nil {
			return nil, fmt.Errorf("invalid connection ID: %w", err)
		}

		conn, err := s.Queries.GetBankConnection(ctx, connID)
		if err != nil {
			return nil, fmt.Errorf("get connection: %w", err)
		}
		if string(conn.Provider) != "csv" {
			return nil, fmt.Errorf("connection is not a CSV connection")
		}

		accounts, err := s.Queries.ListAccountsByConnection(ctx, connID)
		if err != nil {
			return nil, fmt.Errorf("list accounts: %w", err)
		}
		if len(accounts) == 0 {
			return nil, fmt.Errorf("no accounts found for connection")
		}
		accountID = accounts[0].ID
	} else {
		// New import: create connection + account.
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		externalID := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])

		conn, err := s.Queries.CreateBankConnection(ctx, db.CreateBankConnectionParams{
			UserID:               userID,
			Provider:             db.ProviderTypeCsv,
			InstitutionID:        pgtype.Text{},
			InstitutionName:      pgconv.Text(params.AccountName),
			ExternalID:           pgconv.Text(externalID),
			EncryptedCredentials: nil,
			Status:               db.ConnectionStatusActive,
		})
		if err != nil {
			return nil, fmt.Errorf("create connection: %w", err)
		}
		connID = conn.ID

		acct, err := s.Queries.UpsertAccount(ctx, db.UpsertAccountParams{
			ConnectionID:      conn.ID,
			ExternalAccountID: externalID,
			Name:              params.AccountName,
			Type:              "depository",
			Subtype:           pgconv.Text("checking"),
			IsoCurrencyCode:   pgconv.Text("USD"),
		})
		if err != nil {
			return nil, fmt.Errorf("create account: %w", err)
		}
		accountID = acct.ID
	}

	accountIDStr := formatUUID(accountID)

	// Create sync log entry.
	now := time.Now().UTC()
	syncLog, err := s.Queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: connID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusInProgress,
		StartedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create sync log: %w", err)
	}

	result := &CSVImportResult{
		ConnectionID: formatUUID(connID),
		AccountID:    accountIDStr,
		TotalRows:    len(params.Rows),
	}

	// Load uncategorized category ID for CSV imports.
	// Transaction rules will categorize on next sync if matching rules exist.
	var uncategorizedID pgtype.UUID
	if err := s.Pool.QueryRow(ctx, "SELECT id FROM categories WHERE slug = 'uncategorized'").Scan(&uncategorizedID); err != nil {
		s.Logger.Warn("failed to load uncategorized category for CSV import", "error", err)
	}

	dateCol := params.ColumnMapping["date"]
	amountCol := params.ColumnMapping["amount"]
	descCol := params.ColumnMapping["description"]
	catCol, hasCat := params.ColumnMapping["category"]
	merchantCol, hasMerchant := params.ColumnMapping["merchant_name"]
	debitCol := params.ColumnMapping["debit"]
	creditCol := params.ColumnMapping["credit"]

	for i, row := range params.Rows {
		rowNum := i + 2 // 1-indexed, +1 for header row

		// Validate column bounds.
		maxCol := max(dateCol, descCol)
		if !params.HasDebitCredit {
			maxCol = max(maxCol, amountCol)
		} else {
			maxCol = max(maxCol, max(debitCol, creditCol))
		}
		if hasCat {
			maxCol = max(maxCol, catCol)
		}
		if hasMerchant {
			maxCol = max(maxCol, merchantCol)
		}

		if maxCol >= len(row) {
			result.SkippedCount++
			result.SkipReasons = append(result.SkipReasons, fmt.Sprintf("row %d: not enough columns", rowNum))
			continue
		}

		// Parse date.
		dateVal, err := csvpkg.ParseDate(row[dateCol], params.DateFormat)
		if err != nil {
			result.SkippedCount++
			result.SkipReasons = append(result.SkipReasons, fmt.Sprintf("row %d: %s", rowNum, err.Error()))
			continue
		}

		// Parse amount.
		var amount decimal.Decimal
		if params.HasDebitCredit {
			amount, err = csvpkg.ParseDualColumns(row[debitCol], row[creditCol])
		} else {
			amount, err = csvpkg.ParseAmount(row[amountCol])
		}
		if err != nil {
			result.SkippedCount++
			result.SkipReasons = append(result.SkipReasons, fmt.Sprintf("row %d: %s", rowNum, err.Error()))
			continue
		}

		amount = csvpkg.NormalizeSign(amount, params.PositiveIsDebit)

		// Parse description.
		desc := strings.TrimSpace(row[descCol])
		if desc == "" {
			result.SkippedCount++
			result.SkipReasons = append(result.SkipReasons, fmt.Sprintf("row %d: empty description", rowNum))
			continue
		}
		if len(desc) > 500 {
			desc = desc[:500]
		}

		// Optional fields.
		var category, merchant string
		if hasCat && catCol < len(row) {
			category = strings.TrimSpace(row[catCol])
		}
		if hasMerchant && merchantCol < len(row) {
			merchant = strings.TrimSpace(row[merchantCol])
			if len(merchant) > 200 {
				merchant = merchant[:200]
			}
		}

		// Generate dedup hash.
		externalTxnID := csvpkg.GenerateExternalID(accountIDStr, dateVal, amount, desc)

		// Convert amount to pgtype.Numeric.
		var amountNumeric pgtype.Numeric
		_ = amountNumeric.Scan(amount.String())

		// Set to uncategorized — transaction rules will categorize on next sync.
		categoryID := uncategorizedID

		// Marshal raw CSV row as JSON for audit purposes.
		var providerRaw []byte
		if raw, err := json.Marshal(row); err == nil {
			providerRaw = raw
		}

		txn, err := s.Queries.UpsertTransaction(ctx, db.UpsertTransactionParams{
			AccountID:               accountID,
			ProviderTransactionID:   externalTxnID,
			Amount:                  amountNumeric,
			IsoCurrencyCode:         pgconv.Text("USD"),
			Date:                    pgtype.Date{Time: dateVal, Valid: true},
			ProviderName:            desc,
			ProviderMerchantName:    pgtype.Text{String: merchant, Valid: merchant != ""},
			ProviderCategoryPrimary: pgtype.Text{String: category, Valid: category != ""},
			ProviderPaymentChannel:  pgconv.Text("other"),
			Pending:                 false,
			CategoryID:              categoryID,
			ProviderRaw:             providerRaw,
		})
		if err != nil {
			result.SkippedCount++
			result.SkipReasons = append(result.SkipReasons, fmt.Sprintf("row %d: database error: %s", rowNum, err.Error()))
			continue
		}

		// Determine if this was an insert or update by comparing timestamps.
		if txn.CreatedAt.Valid && txn.UpdatedAt.Valid && txn.UpdatedAt.Time.Sub(txn.CreatedAt.Time) < time.Second {
			result.NewCount++
		} else {
			result.UpdatedCount++
		}
	}

	// Finalize sync log.
	completedAt := time.Now().UTC()
	syncStatus := db.SyncStatusSuccess
	var errorMsg pgtype.Text
	if result.SkippedCount > 0 && result.NewCount+result.UpdatedCount == 0 {
		syncStatus = db.SyncStatusError
		errorMsg = pgtype.Text{String: fmt.Sprintf("all %d rows skipped", result.SkippedCount), Valid: true}
	}

	err = s.Queries.UpdateSyncLog(ctx, db.UpdateSyncLogParams{
		ID:            syncLog.ID,
		Status:        syncStatus,
		CompletedAt:   pgtype.Timestamptz{Time: completedAt, Valid: true},
		AddedCount:    int32(result.NewCount),
		ModifiedCount: int32(result.UpdatedCount),
		RemovedCount:  0,
		ErrorMessage:  errorMsg,
	})
	if err != nil {
		s.Logger.Error("update sync log", "error", err)
	}

	// Update connection last_synced_at.
	err = s.Queries.UpdateBankConnectionCursor(ctx, db.UpdateBankConnectionCursorParams{
		ID:         connID,
		SyncCursor: pgtype.Text{},
	})
	if err != nil {
		s.Logger.Error("update connection cursor", "error", err)
	}

	return result, nil
}
