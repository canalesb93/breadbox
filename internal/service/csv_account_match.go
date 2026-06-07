//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Account-detection scoring weights. Deterministic and explainable — every point
// awarded carries a human-readable reason.
const (
	scoreMaskMatch        = 40 // file's last-4 matches the account mask
	scoreOverlapMax       = 40 // proportional to how many rows already exist there
	scoreInstitution      = 15 // template bank name appears in the account name
	scoreFilenameHint     = 5  // filename mentions the account
	overlapSampleSize     = 200
	preselectMinScore     = 70 // top must reach this…
	preselectMinGap       = 20 // …and lead the runner-up by this
	confidenceHighScore   = 70
	confidenceMediumScore = 40
)

// CSVAccountMatch is one ranked account suggestion for an uploaded file.
type CSVAccountMatch struct {
	AccountID    string   `json:"account_id"`
	ShortID      string   `json:"short_id"`
	AccountName  string   `json:"account_name"`
	Institution  string   `json:"institution"`
	Mask         string   `json:"mask"`
	Currency     string   `json:"currency"`
	Score        int      `json:"score"`
	Confidence   string   `json:"confidence"` // high | medium | low
	Reasons      []string `json:"reasons"`
	ProfileMatch bool     `json:"profile_match"`
}

// CSVAccountSuggestion is the full account-detection result: ranked matches plus
// whether one is confident enough to pre-select.
type CSVAccountSuggestion struct {
	Matches   []CSVAccountMatch `json:"matches"`
	Preselect string            `json:"preselect"` // account id to auto-select, "" = ask the user
	ProfileID string            `json:"profile_id"`
}

// CSVDetectionSignals are the file-derived inputs to account detection.
type CSVDetectionSignals struct {
	Filename         string
	Headers          []string
	DetectedTemplate string // bank template name, "" if none matched
	Mask             string // last-4 extracted from the file/filename, "" if none
}

// MatchCSVAccounts ranks existing accounts for an uploaded file using the file
// signals plus how strongly the parsed rows overlap each account's existing
// transactions. A saved profile (matched by header fingerprint) short-circuits
// to its remembered account.
func (s *Service) MatchCSVAccounts(
	ctx context.Context,
	userID pgtype.UUID,
	signals CSVDetectionSignals,
	parsed []CSVParsedRow,
) (*CSVAccountSuggestion, error) {
	accounts, err := s.Queries.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	// Profile lookup by header fingerprint (the strongest possible signal).
	fingerprint := csvpkg.HeaderFingerprint(signals.Headers)
	var profileAccountID, profileID string
	if prof, err := s.Queries.GetCSVImportProfileByFingerprint(ctx, db.GetCSVImportProfileByFingerprintParams{
		UserID:            userID,
		HeaderFingerprint: fingerprint,
	}); err == nil {
		profileID = formatUUID(prof.ID)
		if prof.DefaultAccountID.Valid {
			profileAccountID = formatUUID(prof.DefaultAccountID)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		s.Logger.Warn("csv account match: profile lookup failed", "error", err)
	}

	overlap := s.overlapRatios(ctx, accounts, parsed)
	fileTokens := csvpkg.FilenameTokens(signals.Filename)

	matches := make([]CSVAccountMatch, 0, len(accounts))
	for _, a := range accounts {
		accID := formatUUID(a.ID)
		mask := pgconv.TextOr(a.Mask, "")
		institution := pgconv.TextOr(a.InstitutionName, "")

		m := CSVAccountMatch{
			AccountID:   accID,
			ShortID:     a.ShortID,
			AccountName: a.Name,
			Institution: institution,
			Mask:        mask,
			Currency:    pgconv.TextOr(a.IsoCurrencyCode, "USD"),
		}

		// Mask / last-4.
		if signals.Mask != "" && mask != "" && signals.Mask == mask {
			m.Score += scoreMaskMatch
			m.Reasons = append(m.Reasons, "card ending "+mask+" matches")
		}

		// Transaction overlap.
		if ratio := overlap[accID]; ratio > 0 {
			pts := int(ratio*float64(scoreOverlapMax) + 0.5)
			if pts > 0 {
				m.Score += pts
				m.Reasons = append(m.Reasons, fmt.Sprintf("%d%% of rows already here", int(ratio*100+0.5)))
			}
		}

		// Institution name (template bank name appears in the account name).
		if signals.DetectedTemplate != "" && institutionOverlap(signals.DetectedTemplate, a.Name, institution) {
			m.Score += scoreInstitution
			m.Reasons = append(m.Reasons, "institution looks like "+signals.DetectedTemplate)
		}

		// Filename mentions the account.
		if tokensOverlap(fileTokens, csvpkg.InstitutionTokens(a.Name)) {
			m.Score += scoreFilenameHint
			m.Reasons = append(m.Reasons, "filename mentions this account")
		}

		// Profile-remembered account wins outright.
		if profileAccountID != "" && accID == profileAccountID {
			m.ProfileMatch = true
			m.Score = 100
			m.Reasons = append([]string{"saved import profile"}, m.Reasons...)
		}

		m.Confidence = confidenceFor(m.Score)
		matches = append(matches, m)
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].AccountName < matches[j].AccountName
	})

	return &CSVAccountSuggestion{
		Matches:   matches,
		Preselect: choosePreselect(matches, profileAccountID),
		ProfileID: profileID,
	}, nil
}

// overlapRatios returns, per account id, the fraction of sampled parseable rows
// whose (date, amount) already exists in that account.
func (s *Service) overlapRatios(ctx context.Context, accounts []db.ListAccountsRow, parsed []CSVParsedRow) map[string]float64 {
	sample := sampleParsedRows(parsed, overlapSampleSize)
	if len(sample) == 0 {
		return nil
	}

	var minDate, maxDate time.Time
	for i, r := range sample {
		if i == 0 || r.Date.Before(minDate) {
			minDate = r.Date
		}
		if i == 0 || r.Date.After(maxDate) {
			maxDate = r.Date
		}
	}

	keys, err := s.Queries.ListTransactionKeysInRange(ctx, db.ListTransactionKeysInRangeParams{
		Date:   pgconv.Date(minDate),
		Date_2: pgconv.Date(maxDate),
	})
	if err != nil {
		s.Logger.Warn("csv account match: overlap query failed", "error", err)
		return nil
	}

	// Per-account set of date|amount keys present in the DB.
	present := map[string]map[string]struct{}{}
	for _, k := range keys {
		if !k.AccountID.Valid {
			continue
		}
		acc := formatUUID(k.AccountID)
		set := present[acc]
		if set == nil {
			set = map[string]struct{}{}
			present[acc] = set
		}
		set[dateAmountKey(k.Date.Time, numericToDecimal(k.Amount).String())] = struct{}{}
	}

	ratios := make(map[string]float64, len(accounts))
	for _, a := range accounts {
		acc := formatUUID(a.ID)
		set := present[acc]
		if len(set) == 0 {
			continue
		}
		matched := 0
		for _, r := range sample {
			if _, ok := set[dateAmountKey(r.Date, r.Amount.String())]; ok {
				matched++
			}
		}
		if matched > 0 {
			ratios[acc] = float64(matched) / float64(len(sample))
		}
	}
	return ratios
}

func dateAmountKey(d time.Time, amount string) string {
	return d.Format("2006-01-02") + "|" + amount
}

// sampleParsedRows returns up to n parseable rows, strided to spread coverage
// across the file rather than clustering at the start.
func sampleParsedRows(parsed []CSVParsedRow, n int) []CSVParsedRow {
	valid := make([]CSVParsedRow, 0, len(parsed))
	for _, r := range parsed {
		if r.ParseError == "" {
			valid = append(valid, r)
		}
	}
	if len(valid) <= n {
		return valid
	}
	out := make([]CSVParsedRow, 0, n)
	stride := float64(len(valid)) / float64(n)
	for i := 0; i < n; i++ {
		out = append(out, valid[int(float64(i)*stride)])
	}
	return out
}

func institutionOverlap(template, name, institution string) bool {
	tmplTokens := csvpkg.InstitutionTokens(template)
	target := csvpkg.InstitutionTokens(name)
	target = append(target, csvpkg.InstitutionTokens(institution)...)
	return tokensOverlap(tmplTokens, target)
}

func tokensOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(b))
	for _, t := range b {
		set[t] = struct{}{}
	}
	for _, t := range a {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}

func confidenceFor(score int) string {
	switch {
	case score >= confidenceHighScore:
		return "high"
	case score >= confidenceMediumScore:
		return "medium"
	default:
		return "low"
	}
}

// choosePreselect returns the account id to auto-select, or "" to ask the user.
// A profile-remembered account always pre-selects; otherwise the top match must
// be both strong and a clear leader.
func choosePreselect(matches []CSVAccountMatch, profileAccountID string) string {
	if profileAccountID != "" {
		for _, m := range matches {
			if m.AccountID == profileAccountID {
				return profileAccountID
			}
		}
	}
	if len(matches) == 0 {
		return ""
	}
	top := matches[0]
	second := 0
	if len(matches) > 1 {
		second = matches[1].Score
	}
	if top.Score >= preselectMinScore && top.Score-second >= preselectMinGap {
		return top.AccountID
	}
	return ""
}
