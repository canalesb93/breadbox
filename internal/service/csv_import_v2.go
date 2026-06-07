//go:build !lite

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// importSessionTTL is how long an un-applied session lives before the sweeper
// may reap it.
const importSessionTTL = 24 * time.Hour

// Session status values.
const (
	importStatusAnalyzing       = "analyzing"
	importStatusAwaitingAccount = "awaiting_account"
	importStatusPreviewed       = "previewed"
	importStatusApplied         = "applied"
)

// ErrImportSessionNotFound is returned when a session id/short id doesn't resolve.
var ErrImportSessionNotFound = errors.New("import session not found")

// ImportSession is the service-layer view of a csv_import_sessions row.
type ImportSession struct {
	ID                   string           `json:"id"`
	ShortID              string           `json:"short_id"`
	UserID               string           `json:"user_id"`
	Status               string           `json:"status"`
	Filename             string           `json:"filename"`
	Delimiter            string           `json:"delimiter"`
	Headers              []string         `json:"headers"`
	RowCount             int              `json:"row_count"`
	ResolvedAccountID    string           `json:"resolved_account_id"`
	ResolvedConnectionID string           `json:"resolved_connection_id"`
	DetectedTemplate     string           `json:"detected_template"`
	ColumnMapping        map[string]int   `json:"column_mapping"`
	DateFormat           string           `json:"date_format"`
	PositiveIsDebit      bool             `json:"positive_is_debit"`
	HasDebitCredit       bool             `json:"has_debit_credit"`
	IsoCurrencyCode      string           `json:"iso_currency_code"`
	ProfileID            string           `json:"profile_id"`
	Result               *CSVImportResult `json:"result,omitempty"`
}

// ImportRowView is the service-layer view of one staged row.
type ImportRowView struct {
	ID             string   `json:"id"`
	RowIndex       int      `json:"row_index"`
	Raw            []string `json:"raw"`
	Date           string   `json:"date"`
	Amount         string   `json:"amount"`
	Desc           string   `json:"description"`
	Merchant       string   `json:"merchant"`
	Category       string   `json:"category"`
	Classification string   `json:"classification"`
	MatchTxnID     string   `json:"match_txn_id"`
	MatchScore     int      `json:"match_score"`
	MatchReason    string   `json:"match_reason"`
	ParseError     string   `json:"parse_error"`
	Include        bool     `json:"include"`
	UserEdited     bool     `json:"user_edited"`
}

// ImportSummary is the per-classification breakdown for a session.
type ImportSummary struct {
	Total         int            `json:"total"`
	Counts        map[string]int `json:"counts"`
	IncludedCount int            `json:"included_count"`
}

// ImportAnalysis is returned from CreateImportSession: the session plus account
// suggestions plus a first summary.
type ImportAnalysis struct {
	Session  *ImportSession        `json:"session"`
	Accounts *CSVAccountSuggestion `json:"accounts"`
	Summary  ImportSummary         `json:"summary"`
}

// CreateImportSessionParams are the inputs for analyzing a dropped file.
type CreateImportSessionParams struct {
	UserID   string
	Filename string
	Data     []byte
}

// CreateImportSession parses + analyzes a dropped CSV, persists a staging
// session, applies any saved profile, runs account detection, and (when an
// account can be confidently chosen) classifies rows so the preview is ready.
func (s *Service) CreateImportSession(ctx context.Context, p CreateImportSessionParams) (*ImportAnalysis, error) {
	userID, err := s.resolveUserID(ctx, p.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user: %w", err)
	}

	pf, err := csvpkg.ParseFile(p.Data)
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}

	cfg, detectedTemplate := detectParseConfig(pf)
	currency := detectCurrency(pf.Rows, cfg.ColumnMapping)

	// Apply a saved profile (keyed by header fingerprint), if any.
	fingerprint := csvpkg.HeaderFingerprint(pf.Headers)
	var profileID pgtype.UUID
	if prof, perr := s.Queries.GetCSVImportProfileByFingerprint(ctx, db.GetCSVImportProfileByFingerprintParams{
		UserID:            userID,
		HeaderFingerprint: fingerprint,
	}); perr == nil {
		profileID = prof.ID
		if m := decodeMapping(prof.ColumnMapping); len(m) > 0 {
			cfg.ColumnMapping = m
		}
		cfg.DateFormat = prof.DateFormat
		cfg.PositiveIsDebit = prof.PositiveIsDebit
		cfg.HasDebitCredit = prof.HasDebitCredit
		if prof.IsoCurrencyCode != "" {
			currency = prof.IsoCurrencyCode
		}
		if prof.DetectedTemplate.Valid {
			detectedTemplate = prof.DetectedTemplate.String
		}
	} else if !errors.Is(perr, pgx.ErrNoRows) {
		s.Logger.Warn("csv import: profile lookup failed", "error", perr)
	}

	headersJSON, _ := json.Marshal(pf.Headers)
	mappingJSON, _ := json.Marshal(cfg.ColumnMapping)
	sum := sha256.Sum256(p.Data)

	sess, err := s.Queries.CreateCSVImportSession(ctx, db.CreateCSVImportSessionParams{
		UserID:           userID,
		Status:           importStatusAnalyzing,
		Filename:         p.Filename,
		FileSha256:       hex.EncodeToString(sum[:]),
		Delimiter:        string(pf.Delimiter),
		Headers:          headersJSON,
		RawBlob:          p.Data,
		RowCount:         int32(len(pf.Rows)),
		DetectedTemplate: pgconv.TextIfNotEmpty(detectedTemplate),
		ColumnMapping:    mappingJSON,
		DateFormat:       cfg.DateFormat,
		PositiveIsDebit:  cfg.PositiveIsDebit,
		HasDebitCredit:   cfg.HasDebitCredit,
		IsoCurrencyCode:  currency,
		ProfileID:        profileID,
		ExpiresAt:        pgconv.Timestamptz(time.Now().Add(importSessionTTL)),
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	parsed := ParseCSVRows(cfg, pf.Rows)

	// Account detection.
	suggestion, err := s.MatchCSVAccounts(ctx, userID, CSVDetectionSignals{
		Filename:         p.Filename,
		Headers:          pf.Headers,
		DetectedTemplate: detectedTemplate,
		Mask:             csvpkg.ExtractMask(p.Filename, pf.Headers, sampleRaw(pf.Rows, 50)),
	}, parsed)
	if err != nil {
		return nil, err
	}

	// If we can confidently pick an account, resolve + classify now so the
	// preview is immediately ready; otherwise stage the rows pending an account.
	if suggestion.Preselect != "" {
		if _, err := s.resolveAndClassify(ctx, sess, suggestion.Preselect, currency, cfg, pf.Rows); err != nil {
			return nil, err
		}
	} else {
		if err := s.stageNeedsAccount(ctx, sess.ID, cfg, pf.Rows); err != nil {
			return nil, err
		}
		_ = s.Queries.UpdateCSVImportSessionStatus(ctx, db.UpdateCSVImportSessionStatusParams{
			ID:     sess.ID,
			Status: importStatusAwaitingAccount,
		})
	}

	out, err := s.GetImportSession(ctx, sess.ShortID)
	if err != nil {
		return nil, err
	}
	summary, err := s.importSummary(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	return &ImportAnalysis{Session: out, Accounts: suggestion, Summary: summary}, nil
}

// ResolveImportAccountParams selects (or creates) the target account.
type ResolveImportAccountParams struct {
	AccountID string // existing account (UUID or short id)

	// Create-new (used when AccountID is empty).
	CreateNew   bool
	NewName     string
	NewType     string
	NewSubtype  string
	NewCurrency string
}

// ResolveImportAccount binds the session to an account (existing or newly
// created) and reclassifies every row against it. Status → previewed.
func (s *Service) ResolveImportAccount(ctx context.Context, sessionIDOrShort string, p ResolveImportAccountParams) (*ImportSession, error) {
	sess, err := s.getSession(ctx, sessionIDOrShort)
	if err != nil {
		return nil, err
	}
	rows, err := s.rawRows(sess)
	if err != nil {
		return nil, err
	}
	cfg := sessionParseConfig(sess)

	accountID := p.AccountID
	currency := sess.IsoCurrencyCode
	if p.CreateNew {
		name := strings.TrimSpace(p.NewName)
		if name == "" {
			name = "CSV Import"
		}
		cur := p.NewCurrency
		if cur == "" {
			cur = currency
		}
		_, acctID, err := s.createCSVConnectionAccount(ctx, sess.UserID, name, cur, orDefault(p.NewType, "depository"), orDefault(p.NewSubtype, "checking"))
		if err != nil {
			return nil, err
		}
		accountID = formatUUID(acctID)
		currency = cur
	} else {
		// Adopt the chosen account's currency when the file didn't detect one.
		if accID, err := s.resolveAccountID(ctx, accountID); err == nil {
			if acc, err := s.Queries.GetAccount(ctx, accID); err == nil {
				if c := pgconv.TextOr(acc.IsoCurrencyCode, ""); c != "" {
					currency = c
				}
			}
		}
	}

	if _, err := s.resolveAndClassify(ctx, sess, accountID, currency, cfg, rows); err != nil {
		return nil, err
	}
	return s.GetImportSession(ctx, sess.ShortID)
}

// resolveAndClassify binds the account on the session, classifies all rows
// against it, and persists them. Returns the number of rows persisted.
func (s *Service) resolveAndClassify(ctx context.Context, sess db.CsvImportSession, accountIDOrStr, currency string, cfg CSVParseConfig, rows [][]string) (int, error) {
	accountID, err := s.resolveAccountID(ctx, accountIDOrStr)
	if err != nil {
		return 0, fmt.Errorf("resolve account: %w", err)
	}
	acctStr := formatUUID(accountID)

	acc, err := s.Queries.GetAccount(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("get account: %w", err)
	}

	parsed := ParseCSVRows(cfg, rows)
	classified, err := s.ClassifyCSVRows(ctx, accountID, acctStr, parsed, DefaultDedupToleranceDays)
	if err != nil {
		return 0, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	if _, err := q.ResolveCSVImportSessionAccount(ctx, db.ResolveCSVImportSessionAccountParams{
		ID:                   sess.ID,
		ResolvedAccountID:    accountID,
		ResolvedConnectionID: acc.ConnectionID,
		IsoCurrencyCode:      currency,
		Status:               importStatusPreviewed,
	}); err != nil {
		return 0, err
	}
	if err := q.DeleteCSVImportRows(ctx, sess.ID); err != nil {
		return 0, err
	}
	if _, err := q.CreateCSVImportRows(ctx, classifiedToParams(sess.ID, classified)); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(classified), nil
}

// stageNeedsAccount persists parsed rows in the needs_account state (no account
// resolved yet) so the row count + parse errors are visible before resolution.
func (s *Service) stageNeedsAccount(ctx context.Context, sessionID pgtype.UUID, cfg CSVParseConfig, rows [][]string) error {
	parsed := ParseCSVRows(cfg, rows)
	staged := make([]CSVClassifiedRow, 0, len(parsed))
	for _, pr := range parsed {
		cr := CSVClassifiedRow{CSVParsedRow: pr, Include: pr.ParseError == ""}
		if pr.ParseError != "" {
			cr.Classification = CSVRowError
		} else {
			cr.Classification = CSVRowNeedsAccount
			cr.ContentHash = csvpkg.GenerateContentHash(pr.Date, pr.Amount, pr.Desc)
		}
		staged = append(staged, cr)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	if err := q.DeleteCSVImportRows(ctx, sessionID); err != nil {
		return err
	}
	if _, err := q.CreateCSVImportRows(ctx, classifiedToParams(sessionID, staged)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListImportRows returns a page of staged rows plus the full per-classification
// summary. classFilter "" returns all classifications.
func (s *Service) ListImportRows(ctx context.Context, sessionIDOrShort, classFilter string, page, pageSize int) ([]ImportRowView, ImportSummary, error) {
	sess, err := s.getSession(ctx, sessionIDOrShort)
	if err != nil {
		return nil, ImportSummary{}, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 100
	}
	rows, err := s.Queries.GetCSVImportRowsPage(ctx, db.GetCSVImportRowsPageParams{
		SessionID: sess.ID,
		Column2:   classFilter,
		Limit:     int32(pageSize),
		Offset:    int32((page - 1) * pageSize),
	})
	if err != nil {
		return nil, ImportSummary{}, err
	}
	views := make([]ImportRowView, len(rows))
	for i, r := range rows {
		views[i] = rowToView(r)
	}
	summary, err := s.importSummary(ctx, sess.ID)
	if err != nil {
		return nil, ImportSummary{}, err
	}
	return views, summary, nil
}

// EditImportRowParams carries the editable fields of a single row.
type EditImportRowParams struct {
	Date     string // "2006-01-02"
	Amount   string // decimal string
	Desc     string
	Merchant string
	Include  *bool
}

// EditImportRow updates a single row's parsed fields and reclassifies just that
// row against the resolved account.
func (s *Service) EditImportRow(ctx context.Context, sessionIDOrShort, rowID string, p EditImportRowParams) (*ImportRowView, error) {
	sess, err := s.getSession(ctx, sessionIDOrShort)
	if err != nil {
		return nil, err
	}
	if !sess.ResolvedAccountID.Valid {
		return nil, errors.New("resolve an account before editing rows")
	}
	rowUUID, err := pgconv.ParseUUID(rowID)
	if err != nil {
		return nil, fmt.Errorf("invalid row id: %w", err)
	}
	existing, err := s.Queries.GetCSVImportRow(ctx, rowUUID)
	if err != nil {
		return nil, fmt.Errorf("get row: %w", err)
	}

	pr := CSVParsedRow{RowIndex: int(existing.RowIndex)}
	_ = json.Unmarshal(existing.Raw, &pr.Raw)

	date, derr := time.Parse("2006-01-02", strings.TrimSpace(p.Date))
	amount, aerr := decimal.NewFromString(strings.TrimSpace(p.Amount))
	pr.Desc = strings.TrimSpace(p.Desc)
	pr.Merchant = strings.TrimSpace(p.Merchant)
	switch {
	case derr != nil:
		pr.ParseError = "unparseable date"
	case aerr != nil:
		pr.ParseError = "unparseable amount"
	case pr.Desc == "":
		pr.ParseError = "empty description"
	default:
		pr.Date, pr.Amount = date, amount
	}

	acctStr := formatUUID(sess.ResolvedAccountID)
	classified, err := s.ClassifyCSVRows(ctx, sess.ResolvedAccountID, acctStr, []CSVParsedRow{pr}, DefaultDedupToleranceDays)
	if err != nil {
		return nil, err
	}
	cr := classified[0]

	include := cr.Include
	if p.Include != nil {
		include = *p.Include
	}

	updated, err := s.Queries.UpdateCSVImportRow(ctx, db.UpdateCSVImportRowParams{
		ID:             rowUUID,
		ParsedDate:     dateOrNull(cr.Date, cr.ParseError == ""),
		ParsedAmount:   numericOrNull(cr.Amount, cr.ParseError == ""),
		ParsedDesc:     pgconv.TextIfNotEmpty(cr.Desc),
		ParsedMerchant: pgconv.TextIfNotEmpty(cr.Merchant),
		ParsedCategory: pgconv.TextIfNotEmpty(cr.Category),
		Classification: string(cr.Classification),
		MatchTxnID:     optionalUUID(cr.MatchTxnID),
		MatchScore:     pgconv.Int4(int32(cr.MatchScore)),
		MatchReason:    pgconv.TextIfNotEmpty(cr.MatchReason),
		ParseError:     pgconv.TextIfNotEmpty(cr.ParseError),
		ContentHash:    pgconv.TextIfNotEmpty(cr.ContentHash),
		ProviderTxnID:  pgconv.TextIfNotEmpty(cr.ProviderTxnID),
		Include:        include,
		CategoryID:     existing.CategoryID,
	})
	if err != nil {
		return nil, err
	}
	v := rowToView(updated)
	return &v, nil
}

// ImportBulkOp describes a bulk preview action.
type ImportBulkOp struct {
	Op             string // "include" | "exclude" | "set_category" | "remap"
	Classification string // for include/exclude scoping ("" = all dup classes)
	CategoryID     string // for set_category

	// for remap
	ColumnMapping   map[string]int
	DateFormat      string
	PositiveIsDebit bool
	HasDebitCredit  bool
}

// BulkImportOp applies a bulk action to a session's staged rows.
func (s *Service) BulkImportOp(ctx context.Context, sessionIDOrShort string, op ImportBulkOp) error {
	sess, err := s.getSession(ctx, sessionIDOrShort)
	if err != nil {
		return err
	}
	switch op.Op {
	case "include", "exclude":
		include := op.Op == "include"
		classes := []string{op.Classification}
		if op.Classification == "" {
			classes = []string{string(CSVRowExactDup), string(CSVRowProbableDup)}
		}
		for _, c := range classes {
			if err := s.Queries.SetCSVImportRowsIncludeByClassification(ctx, db.SetCSVImportRowsIncludeByClassificationParams{
				SessionID:      sess.ID,
				Classification: c,
				Include:        include,
			}); err != nil {
				return err
			}
		}
		return nil
	case "set_category":
		var catID pgtype.UUID
		if op.CategoryID != "" {
			id, err := s.resolveCategoryID(ctx, op.CategoryID)
			if err != nil {
				return fmt.Errorf("resolve category: %w", err)
			}
			catID = id
		}
		return s.Queries.SetCSVImportRowsCategoryAll(ctx, db.SetCSVImportRowsCategoryAllParams{
			SessionID:  sess.ID,
			CategoryID: catID,
		})
	case "remap":
		if !sess.ResolvedAccountID.Valid {
			return errors.New("resolve an account before remapping")
		}
		mappingJSON, _ := json.Marshal(op.ColumnMapping)
		if _, err := s.Queries.UpdateCSVImportSessionParse(ctx, db.UpdateCSVImportSessionParseParams{
			ID:               sess.ID,
			ColumnMapping:    mappingJSON,
			DateFormat:       op.DateFormat,
			PositiveIsDebit:  op.PositiveIsDebit,
			HasDebitCredit:   op.HasDebitCredit,
			DetectedTemplate: sess.DetectedTemplate,
		}); err != nil {
			return err
		}
		fresh, err := s.Queries.GetCSVImportSession(ctx, sess.ID)
		if err != nil {
			return err
		}
		rows, err := s.rawRows(fresh)
		if err != nil {
			return err
		}
		_, err = s.resolveAndClassify(ctx, fresh, formatUUID(fresh.ResolvedAccountID), fresh.IsoCurrencyCode, sessionParseConfig(fresh), rows)
		return err
	default:
		return fmt.Errorf("unknown bulk op %q", op.Op)
	}
}

// ApplyImportSession upserts the exact included set into the resolved account in
// one transaction, idempotently, and saves/updates the source profile.
func (s *Service) ApplyImportSession(ctx context.Context, sessionIDOrShort string, actor Actor) (*CSVImportResult, error) {
	sess, err := s.getSession(ctx, sessionIDOrShort)
	if err != nil {
		return nil, err
	}
	if sess.Status == importStatusApplied {
		return nil, errors.New("session already applied")
	}
	if !sess.ResolvedAccountID.Valid || !sess.ResolvedConnectionID.Valid {
		return nil, errors.New("resolve an account before applying")
	}
	acctStr := formatUUID(sess.ResolvedAccountID)

	included, err := s.Queries.ListIncludedCSVImportRows(ctx, sess.ID)
	if err != nil {
		return nil, err
	}

	var uncategorizedID pgtype.UUID
	_ = s.Pool.QueryRow(ctx, "SELECT id FROM categories WHERE slug = 'uncategorized'").Scan(&uncategorizedID)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	now := time.Now().UTC()
	syncLog, err := q.CreateSyncLog(ctx, db.CreateSyncLogParams{
		ConnectionID: sess.ResolvedConnectionID,
		Trigger:      db.SyncTriggerManual,
		Status:       db.SyncStatusInProgress,
		StartedAt:    pgconv.Timestamptz(now),
	})
	if err != nil {
		return nil, fmt.Errorf("create sync log: %w", err)
	}

	result := &CSVImportResult{
		ConnectionID: formatUUID(sess.ResolvedConnectionID),
		AccountID:    acctStr,
		TotalRows:    int(sess.RowCount),
	}

	seen := map[string]int{} // provider_txn_id base → next occurrence
	for _, r := range included {
		amount := numericToDecimal(r.ParsedAmount)
		desc := pgconv.TextOr(r.ParsedDesc, "")
		date := r.ParsedDate.Time

		// Disambiguate genuine same-key duplicates so "import anyway" inserts a
		// distinct row instead of no-op upserting onto the original.
		base := pgconv.TextOr(r.ProviderTxnID, csvpkg.GenerateExternalID(acctStr, date, amount, desc))
		occ := seen[base]
		if r.Classification == string(CSVRowExactDup) && occ == 0 {
			occ = 1 // explicit re-import of an identical row → force a new id
		}
		seen[base] = occ + 1
		providerTxnID := csvpkg.GenerateExternalIDWithOccurrence(acctStr, date, amount, desc, occ)

		categoryID := uncategorizedID
		if r.CategoryID.Valid {
			categoryID = r.CategoryID
		}

		var amountNumeric pgtype.Numeric
		_ = amountNumeric.Scan(amount.String())

		up, err := q.UpsertTransactionV2(ctx, db.UpsertTransactionV2Params{
			AccountID:               sess.ResolvedAccountID,
			ProviderTransactionID:   providerTxnID,
			Amount:                  amountNumeric,
			IsoCurrencyCode:         pgconv.Text(sess.IsoCurrencyCode),
			Date:                    pgconv.Date(date),
			ProviderName:            desc,
			ProviderMerchantName:    r.ParsedMerchant,
			ProviderCategoryPrimary: r.ParsedCategory,
			ProviderPaymentChannel:  pgconv.Text("other"),
			Pending:                 false,
			CategoryID:              categoryID,
			ProviderRaw:             r.Raw,
			ContentHash:             pgconv.TextIfNotEmpty(pgconv.TextOr(r.ContentHash, csvpkg.GenerateContentHash(date, amount, desc))),
		})
		if err != nil {
			result.SkippedCount++
			result.SkipReasons = append(result.SkipReasons, fmt.Sprintf("row %d: %s", r.RowIndex+2, err.Error()))
			continue
		}
		if up.Inserted {
			result.NewCount++
		} else {
			result.UpdatedCount++
		}
	}

	syncStatus := db.SyncStatusSuccess
	var errMsg pgtype.Text
	if result.NewCount+result.UpdatedCount == 0 && result.SkippedCount > 0 {
		syncStatus = db.SyncStatusError
		errMsg = pgconv.Text(fmt.Sprintf("all %d rows skipped", result.SkippedCount))
	}
	if err := q.UpdateSyncLog(ctx, db.UpdateSyncLogParams{
		ID:            syncLog.ID,
		Status:        syncStatus,
		CompletedAt:   pgconv.Timestamptz(time.Now().UTC()),
		AddedCount:    int32(result.NewCount),
		ModifiedCount: int32(result.UpdatedCount),
		RemovedCount:  0,
		ErrorMessage:  errMsg,
	}); err != nil {
		s.Logger.Error("csv import: update sync log", "error", err)
	}

	// Save / refresh the source profile so future imports of this layout are
	// one click. Best-effort — never fails the apply.
	if err := s.upsertProfileFromSession(ctx, q, sess); err != nil {
		s.Logger.Warn("csv import: profile upsert failed", "error", err)
	}

	resultJSON, _ := json.Marshal(result)
	if err := q.FinalizeCSVImportSession(ctx, db.FinalizeCSVImportSessionParams{
		ID:        sess.ID,
		Result:    resultJSON,
		SyncLogID: syncLog.ID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// GetImportSession returns the service view of a session by id or short id.
func (s *Service) GetImportSession(ctx context.Context, sessionIDOrShort string) (*ImportSession, error) {
	sess, err := s.getSession(ctx, sessionIDOrShort)
	if err != nil {
		return nil, err
	}
	v := sessionToView(sess)
	return &v, nil
}

// --- helpers ---

func (s *Service) getSession(ctx context.Context, idOrShort string) (db.CsvImportSession, error) {
	if len(idOrShort) == 8 {
		sess, err := s.Queries.GetCSVImportSessionByShortID(ctx, idOrShort)
		if err != nil {
			return db.CsvImportSession{}, ErrImportSessionNotFound
		}
		return sess, nil
	}
	uid, err := pgconv.ParseUUID(idOrShort)
	if err != nil {
		return db.CsvImportSession{}, ErrImportSessionNotFound
	}
	sess, err := s.Queries.GetCSVImportSession(ctx, uid)
	if err != nil {
		return db.CsvImportSession{}, ErrImportSessionNotFound
	}
	return sess, nil
}

func (s *Service) importSummary(ctx context.Context, sessionID pgtype.UUID) (ImportSummary, error) {
	counts, err := s.Queries.CountCSVImportRowsByClassification(ctx, sessionID)
	if err != nil {
		return ImportSummary{}, err
	}
	out := ImportSummary{Counts: map[string]int{}}
	for _, c := range counts {
		out.Counts[c.Classification] = int(c.Count)
		out.Total += int(c.Count)
	}
	included, err := s.Queries.CountIncludedCSVImportRows(ctx, sessionID)
	if err != nil {
		return ImportSummary{}, err
	}
	out.IncludedCount = int(included)
	return out, nil
}

// rawRows re-parses the stored file blob into rows.
func (s *Service) rawRows(sess db.CsvImportSession) ([][]string, error) {
	if len(sess.RawBlob) == 0 {
		return nil, errors.New("session file is no longer available")
	}
	pf, err := csvpkg.ParseFile(sess.RawBlob)
	if err != nil {
		return nil, fmt.Errorf("re-parse csv: %w", err)
	}
	return pf.Rows, nil
}

// createCSVConnectionAccount mints a fresh CSV connection + account (generalized
// from the legacy ImportCSV new-import branch).
func (s *Service) createCSVConnectionAccount(ctx context.Context, userID pgtype.UUID, name, currency, accType, subtype string) (pgtype.UUID, pgtype.UUID, error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	externalID := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])

	conn, err := s.Queries.CreateBankConnection(ctx, db.CreateBankConnectionParams{
		UserID:          userID,
		Provider:        db.ProviderTypeCsv,
		InstitutionName: pgconv.Text(name),
		ExternalID:      pgconv.Text(externalID),
		Status:          db.ConnectionStatusActive,
	})
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("create connection: %w", err)
	}
	acct, err := s.Queries.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID:      conn.ID,
		ExternalAccountID: externalID,
		Name:              name,
		Type:              accType,
		Subtype:           pgconv.Text(subtype),
		IsoCurrencyCode:   pgconv.Text(currency),
	})
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("create account: %w", err)
	}
	return conn.ID, acct.ID, nil
}

func (s *Service) upsertProfileFromSession(ctx context.Context, q *db.Queries, sess db.CsvImportSession) error {
	headers := decodeHeaders(sess.Headers)
	name := strings.TrimSpace(sess.Filename)
	if name == "" {
		name = "CSV import"
	}
	_, err := q.UpsertCSVImportProfile(ctx, db.UpsertCSVImportProfileParams{
		UserID:            sess.UserID,
		Name:              name,
		HeaderFingerprint: csvpkg.HeaderFingerprint(headers),
		Headers:           sess.Headers,
		DetectedTemplate:  sess.DetectedTemplate,
		ColumnMapping:     sess.ColumnMapping,
		DateFormat:        sess.DateFormat,
		Delimiter:         sess.Delimiter,
		PositiveIsDebit:   sess.PositiveIsDebit,
		HasDebitCredit:    sess.HasDebitCredit,
		IsoCurrencyCode:   sess.IsoCurrencyCode,
		DefaultAccountID:  sess.ResolvedAccountID,
	})
	return err
}

func classifiedToParams(sessionID pgtype.UUID, rows []CSVClassifiedRow) []db.CreateCSVImportRowsParams {
	out := make([]db.CreateCSVImportRowsParams, len(rows))
	for i, cr := range rows {
		rawJSON, _ := json.Marshal(cr.Raw)
		ok := cr.ParseError == ""
		out[i] = db.CreateCSVImportRowsParams{
			SessionID:      sessionID,
			RowIndex:       int32(cr.RowIndex),
			Raw:            rawJSON,
			ParsedDate:     dateOrNull(cr.Date, ok),
			ParsedAmount:   numericOrNull(cr.Amount, ok),
			ParsedDesc:     pgconv.TextIfNotEmpty(cr.Desc),
			ParsedMerchant: pgconv.TextIfNotEmpty(cr.Merchant),
			ParsedCategory: pgconv.TextIfNotEmpty(cr.Category),
			Classification: string(cr.Classification),
			MatchTxnID:     optionalUUID(cr.MatchTxnID),
			MatchScore:     pgconv.Int4(int32(cr.MatchScore)),
			MatchReason:    pgconv.TextIfNotEmpty(cr.MatchReason),
			ParseError:     pgconv.TextIfNotEmpty(cr.ParseError),
			ContentHash:    pgconv.TextIfNotEmpty(cr.ContentHash),
			ProviderTxnID:  pgconv.TextIfNotEmpty(cr.ProviderTxnID),
			Include:        cr.Include,
			UserEdited:     false,
		}
	}
	return out
}

func rowToView(r db.CsvImportRow) ImportRowView {
	var raw []string
	_ = json.Unmarshal(r.Raw, &raw)
	v := ImportRowView{
		ID:             formatUUID(r.ID),
		RowIndex:       int(r.RowIndex),
		Raw:            raw,
		Desc:           pgconv.TextOr(r.ParsedDesc, ""),
		Merchant:       pgconv.TextOr(r.ParsedMerchant, ""),
		Category:       pgconv.TextOr(r.ParsedCategory, ""),
		Classification: r.Classification,
		MatchReason:    pgconv.TextOr(r.MatchReason, ""),
		ParseError:     pgconv.TextOr(r.ParseError, ""),
		Include:        r.Include,
		UserEdited:     r.UserEdited,
	}
	if r.MatchTxnID.Valid {
		v.MatchTxnID = formatUUID(r.MatchTxnID)
	}
	if r.MatchScore.Valid {
		v.MatchScore = int(r.MatchScore.Int32)
	}
	if r.ParsedDate.Valid {
		v.Date = r.ParsedDate.Time.Format("2006-01-02")
	}
	if r.ParsedAmount.Valid {
		v.Amount = numericToDecimal(r.ParsedAmount).String()
	}
	return v
}

func sessionToView(s db.CsvImportSession) ImportSession {
	v := ImportSession{
		ID:               formatUUID(s.ID),
		ShortID:          s.ShortID,
		UserID:           formatUUID(s.UserID),
		Status:           s.Status,
		Filename:         s.Filename,
		Delimiter:        s.Delimiter,
		Headers:          decodeHeaders(s.Headers),
		RowCount:         int(s.RowCount),
		DetectedTemplate: pgconv.TextOr(s.DetectedTemplate, ""),
		ColumnMapping:    decodeMapping(s.ColumnMapping),
		DateFormat:       s.DateFormat,
		PositiveIsDebit:  s.PositiveIsDebit,
		HasDebitCredit:   s.HasDebitCredit,
		IsoCurrencyCode:  s.IsoCurrencyCode,
	}
	if s.ResolvedAccountID.Valid {
		v.ResolvedAccountID = formatUUID(s.ResolvedAccountID)
	}
	if s.ResolvedConnectionID.Valid {
		v.ResolvedConnectionID = formatUUID(s.ResolvedConnectionID)
	}
	if s.ProfileID.Valid {
		v.ProfileID = formatUUID(s.ProfileID)
	}
	if len(s.Result) > 0 {
		var res CSVImportResult
		if json.Unmarshal(s.Result, &res) == nil {
			v.Result = &res
		}
	}
	return v
}

func sessionParseConfig(s db.CsvImportSession) CSVParseConfig {
	return CSVParseConfig{
		ColumnMapping:   decodeMapping(s.ColumnMapping),
		DateFormat:      s.DateFormat,
		PositiveIsDebit: s.PositiveIsDebit,
		HasDebitCredit:  s.HasDebitCredit,
	}
}

// detectParseConfig builds a parse config from a parsed file using the bank
// template (if matched) or generic column detection. Returns the template name.
func detectParseConfig(pf *csvpkg.ParsedFile) (CSVParseConfig, string) {
	cfg := CSVParseConfig{ColumnMapping: map[string]int{}}
	if t := csvpkg.DetectTemplate(pf.Headers); t != nil && t.HeaderPatterns != nil {
		idx := func(name string) (int, bool) {
			for i, h := range pf.Headers {
				if strings.EqualFold(strings.TrimSpace(h), name) {
					return i, true
				}
			}
			return 0, false
		}
		if i, ok := idx(t.DateColumn); ok {
			cfg.ColumnMapping["date"] = i
		}
		if i, ok := idx(t.AmountColumn); ok {
			cfg.ColumnMapping["amount"] = i
		}
		if i, ok := idx(t.DescriptionColumn); ok {
			cfg.ColumnMapping["description"] = i
		}
		if t.CategoryColumn != "" {
			if i, ok := idx(t.CategoryColumn); ok {
				cfg.ColumnMapping["category"] = i
			}
		}
		if t.MerchantColumn != "" {
			if i, ok := idx(t.MerchantColumn); ok {
				cfg.ColumnMapping["merchant_name"] = i
			}
		}
		if t.HasDebitCredit {
			if i, ok := idx(t.DebitColumn); ok {
				cfg.ColumnMapping["debit"] = i
			}
			if i, ok := idx(t.CreditColumn); ok {
				cfg.ColumnMapping["credit"] = i
			}
		}
		cfg.DateFormat = t.DateFormat
		cfg.PositiveIsDebit = t.PositiveIsDebit
		cfg.HasDebitCredit = t.HasDebitCredit
		return cfg, t.Name
	}

	cfg.ColumnMapping = csvpkg.DetectColumns(pf.Headers)
	if dateCol, ok := cfg.ColumnMapping["date"]; ok {
		samples := make([]string, 0, 20)
		for _, r := range pf.Rows {
			if dateCol < len(r) {
				samples = append(samples, r[dateCol])
			}
			if len(samples) >= 20 {
				break
			}
		}
		if df, err := csvpkg.DetectDateFormat(samples); err == nil {
			cfg.DateFormat = df
		}
	}
	return cfg, ""
}

// detectCurrency guesses the ISO currency from symbols in the amount column,
// defaulting to USD.
func detectCurrency(rows [][]string, mapping map[string]int) string {
	col, ok := mapping["amount"]
	if !ok {
		return "USD"
	}
	for i, r := range rows {
		if i >= 50 || col >= len(r) {
			continue
		}
		switch {
		case strings.Contains(r[col], "€"):
			return "EUR"
		case strings.Contains(r[col], "£"):
			return "GBP"
		case strings.Contains(r[col], "¥"):
			return "JPY"
		}
	}
	return "USD"
}

func decodeHeaders(b []byte) []string {
	var h []string
	_ = json.Unmarshal(b, &h)
	return h
}

func decodeMapping(b []byte) map[string]int {
	m := map[string]int{}
	_ = json.Unmarshal(b, &m)
	return m
}

func sampleRaw(rows [][]string, n int) [][]string {
	if len(rows) <= n {
		return rows
	}
	return rows[:n]
}

func dateOrNull(t time.Time, ok bool) pgtype.Date {
	if !ok {
		return pgtype.Date{}
	}
	return pgconv.Date(t)
}

func numericOrNull(d decimal.Decimal, ok bool) pgtype.Numeric {
	if !ok {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func optionalUUID(idStr string) pgtype.UUID {
	if idStr == "" {
		return pgtype.UUID{}
	}
	u, err := pgconv.ParseUUID(idStr)
	if err != nil {
		return pgtype.UUID{}
	}
	return u
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
