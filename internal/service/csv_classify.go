//go:build !lite

package service

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"
	"breadbox/internal/textmatch"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// CSVRowClassification is the per-row verdict produced when an analyzed CSV is
// compared against the transactions already in a resolved target account.
type CSVRowClassification string

const (
	CSVRowNew          CSVRowClassification = "new"           // not present — import it (default include)
	CSVRowExactDup     CSVRowClassification = "exact_dup"     // identical row already exists (default exclude)
	CSVRowProbableDup  CSVRowClassification = "probable_dup"  // same date+amount, similar name (default exclude)
	CSVRowConflict     CSVRowClassification = "conflict"      // would materially change an existing row
	CSVRowError        CSVRowClassification = "error"         // unparseable (default exclude)
	CSVRowNeedsAccount CSVRowClassification = "needs_account" // no account resolved yet
)

// DefaultDedupToleranceDays is the +/- day window for the fuzzy duplicate pass.
const DefaultDedupToleranceDays = 1

// CSVParsedRow is a single CSV data row after applying the column mapping.
type CSVParsedRow struct {
	RowIndex   int
	Raw        []string
	Date       time.Time
	Amount     decimal.Decimal
	Desc       string
	Merchant   string
	Category   string
	ParseError string // non-empty → classification is forced to error
}

// CSVClassifiedRow is a parsed row plus its verdict against a target account.
type CSVClassifiedRow struct {
	CSVParsedRow
	Classification CSVRowClassification
	MatchTxnID     string // existing transaction this row matched (exact/probable)
	MatchScore     int    // name-similarity score (0-3) for probable matches
	MatchReason    string
	ContentHash    string
	ProviderTxnID  string // stable upsert id, set once the account is known
	Include        bool   // default user intent
}

// CSVParseConfig captures everything needed to turn raw rows into parsed rows.
type CSVParseConfig struct {
	ColumnMapping   map[string]int
	DateFormat      string
	PositiveIsDebit bool
	HasDebitCredit  bool
}

// ParseCSVRows applies the column mapping to raw rows, producing CSVParsedRow
// values. Rows that fail to parse carry a ParseError instead of being dropped,
// so the preview can show and let the user fix them. This mirrors the per-row
// logic in ImportCSV but never touches the DB.
func ParseCSVRows(cfg CSVParseConfig, rows [][]string) []CSVParsedRow {
	dateCol := cfg.ColumnMapping["date"]
	amountCol := cfg.ColumnMapping["amount"]
	descCol := cfg.ColumnMapping["description"]
	catCol, hasCat := cfg.ColumnMapping["category"]
	merchantCol, hasMerchant := cfg.ColumnMapping["merchant_name"]
	debitCol := cfg.ColumnMapping["debit"]
	creditCol := cfg.ColumnMapping["credit"]

	out := make([]CSVParsedRow, 0, len(rows))
	for i, row := range rows {
		pr := CSVParsedRow{RowIndex: i, Raw: row}

		maxCol := max(dateCol, descCol)
		if cfg.HasDebitCredit {
			maxCol = max(maxCol, max(debitCol, creditCol))
		} else {
			maxCol = max(maxCol, amountCol)
		}
		if maxCol >= len(row) {
			pr.ParseError = "not enough columns"
			out = append(out, pr)
			continue
		}

		dateVal, err := csvpkg.ParseDate(row[dateCol], cfg.DateFormat)
		if err != nil {
			pr.ParseError = err.Error()
			out = append(out, pr)
			continue
		}
		pr.Date = dateVal

		var amount decimal.Decimal
		if cfg.HasDebitCredit {
			amount, err = csvpkg.ParseDualColumns(row[debitCol], row[creditCol])
		} else {
			amount, err = csvpkg.ParseAmount(row[amountCol])
		}
		if err != nil {
			pr.ParseError = err.Error()
			out = append(out, pr)
			continue
		}
		pr.Amount = csvpkg.NormalizeSign(amount, cfg.PositiveIsDebit)

		desc := strings.TrimSpace(row[descCol])
		if desc == "" {
			pr.ParseError = "empty description"
			out = append(out, pr)
			continue
		}
		if len(desc) > 500 {
			desc = desc[:500]
		}
		pr.Desc = desc

		if hasCat && catCol < len(row) {
			pr.Category = strings.TrimSpace(row[catCol])
		}
		if hasMerchant && merchantCol < len(row) {
			m := strings.TrimSpace(row[merchantCol])
			if len(m) > 200 {
				m = m[:200]
			}
			pr.Merchant = m
		}

		out = append(out, pr)
	}
	return out
}

// dedupCandidate is an existing transaction reduced to the fields the classifier
// compares against.
type dedupCandidate struct {
	id            string
	date          time.Time
	amount        decimal.Decimal
	name          string
	merchant      string
	contentHash   string
	providerTxnID string
}

// ClassifyCSVRows compares parsed rows against the live transactions in the
// resolved account and assigns each a classification. accountIDStr is the
// canonical account UUID string (used to derive the stable provider_transaction_id).
func (s *Service) ClassifyCSVRows(
	ctx context.Context,
	accountID pgtype.UUID,
	accountIDStr string,
	rows []CSVParsedRow,
	toleranceDays int,
) ([]CSVClassifiedRow, error) {
	if toleranceDays < 0 {
		toleranceDays = 0
	}

	// Date window covering all parseable rows, padded by the tolerance.
	var minDate, maxDate time.Time
	haveDate := false
	for _, r := range rows {
		if r.ParseError != "" {
			continue
		}
		if !haveDate || r.Date.Before(minDate) {
			minDate = r.Date
		}
		if !haveDate || r.Date.After(maxDate) {
			maxDate = r.Date
		}
		haveDate = true
	}

	// Existing-row indexes.
	byProviderTxnID := map[string]dedupCandidate{}
	byContentHash := map[string]dedupCandidate{}
	byAmount := map[string][]dedupCandidate{}

	if haveDate {
		tol := time.Duration(toleranceDays) * 24 * time.Hour
		existing, err := s.Queries.ListAccountTransactionsForDedup(ctx, db.ListAccountTransactionsForDedupParams{
			AccountID: accountID,
			Date:      pgconv.Date(minDate.Add(-tol)),
			Date_2:    pgconv.Date(maxDate.Add(tol)),
		})
		if err != nil {
			return nil, fmt.Errorf("load dedup candidates: %w", err)
		}
		for _, e := range existing {
			cand := dedupCandidate{
				id:            formatUUID(e.ID),
				date:          e.Date.Time,
				amount:        numericToDecimal(e.Amount),
				name:          e.ProviderName,
				merchant:      pgconv.TextOr(e.ProviderMerchantName, ""),
				contentHash:   pgconv.TextOr(e.ContentHash, ""),
				providerTxnID: e.ProviderTransactionID,
			}
			byProviderTxnID[cand.providerTxnID] = cand
			if cand.contentHash != "" {
				byContentHash[cand.contentHash] = cand
			}
			byAmount[cand.amount.String()] = append(byAmount[cand.amount.String()], cand)
		}
	}

	out := make([]CSVClassifiedRow, 0, len(rows))
	for _, pr := range rows {
		cr := CSVClassifiedRow{CSVParsedRow: pr}

		if pr.ParseError != "" {
			cr.Classification = CSVRowError
			cr.Include = false
			out = append(out, cr)
			continue
		}

		cr.ContentHash = csvpkg.GenerateContentHash(pr.Date, pr.Amount, pr.Desc)
		cr.ProviderTxnID = csvpkg.GenerateExternalID(accountIDStr, pr.Date, pr.Amount, pr.Desc)

		// Exact duplicate: identical row already imported (same provider id) or
		// identical content already present from any source.
		if cand, ok := byProviderTxnID[cr.ProviderTxnID]; ok {
			cr.Classification = CSVRowExactDup
			cr.MatchTxnID = cand.id
			cr.MatchReason = "already imported"
			cr.Include = false
			out = append(out, cr)
			continue
		}
		if cand, ok := byContentHash[cr.ContentHash]; ok {
			cr.Classification = CSVRowExactDup
			cr.MatchTxnID = cand.id
			cr.MatchReason = "identical transaction already exists"
			cr.Include = false
			out = append(out, cr)
			continue
		}

		// Probable duplicate: same amount, date within tolerance, name similar.
		best, bestScore, bestReason := bestFuzzyMatch(pr, byAmount[pr.Amount.String()], toleranceDays)
		if best != nil {
			cr.Classification = CSVRowProbableDup
			cr.MatchTxnID = best.id
			cr.MatchScore = bestScore
			cr.MatchReason = bestReason
			cr.Include = false
			out = append(out, cr)
			continue
		}

		cr.Classification = CSVRowNew
		cr.Include = true
		out = append(out, cr)
	}
	return out, nil
}

// bestFuzzyMatch finds the strongest same-amount, within-tolerance candidate for
// a parsed row. Returns nil when none qualifies. A candidate with an exact
// date+amount match qualifies even with no name overlap (score 0).
func bestFuzzyMatch(pr CSVParsedRow, sameAmount []dedupCandidate, toleranceDays int) (*dedupCandidate, int, string) {
	descLower := strings.ToLower(pr.Desc)
	merchLower := strings.ToLower(pr.Merchant)

	var best *dedupCandidate
	bestScore := -1
	bestExact := false
	var bestReason string

	for i := range sameAmount {
		cand := &sameAmount[i]
		dayDiff := daysBetween(pr.Date, cand.date)
		if dayDiff > toleranceDays {
			continue
		}
		exactDate := dayDiff == 0
		score, fields := textmatch.ScoreLowered(pr.Desc, descLower, pr.Merchant, merchLower, cand.name, cand.merchant)

		// Only a same-date+amount row qualifies on its own with no name overlap.
		if score == 0 && !exactDate {
			continue
		}

		// Prefer higher name score; tie-break toward an exact-date match.
		better := score > bestScore || (score == bestScore && exactDate && !bestExact)
		if best == nil || better {
			best = cand
			bestScore = score
			bestExact = exactDate
			bestReason = fuzzyReason(score, fields, exactDate)
		}
	}
	if best == nil {
		return nil, 0, ""
	}
	return best, bestScore, bestReason
}

func fuzzyReason(score int, fields []string, exactDate bool) string {
	datePart := "same amount"
	if exactDate {
		datePart = "same date & amount"
	} else {
		datePart = "same amount, nearby date"
	}
	if score >= 2 && len(fields) > 0 {
		return fmt.Sprintf("%s, %s matches", datePart, strings.ReplaceAll(fields[0], "_", " "))
	}
	if score == 1 {
		return datePart + ", similar name"
	}
	return datePart
}

// daysBetween returns the absolute whole-day difference between two dates.
func daysBetween(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	da := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	db := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	diff := int(da.Sub(db).Hours() / 24)
	if diff < 0 {
		diff = -diff
	}
	return diff
}

// numericToDecimal converts a pgtype.Numeric to a shopspring Decimal. Invalid /
// NaN / infinity values become zero (they never occur for transaction amounts).
func numericToDecimal(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.NaN || n.Int == nil || n.InfinityModifier != pgtype.Finite {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(new(big.Int).Set(n.Int), n.Exp)
}
