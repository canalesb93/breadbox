//go:build !lite

package simplefin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"
)

const (
	// maxWindowDays is the SimpleFIN Bridge's hard cap on the date range of a
	// single /accounts request (start-date..end-date). Larger spans must be
	// fetched in chunks.
	maxWindowDays = 90
	// initialBackfillDays bounds the first sync. ~1 year ≈ 5 windowed requests,
	// well under the bridge's ~24-requests/day budget.
	initialBackfillDays = 365
	// overlapDays re-fetches recent history on incremental syncs to catch
	// late-posting transactions. Idempotent upserts absorb the duplicates.
	overlapDays = 10
)

// SyncTransactions fetches transactions via date-range polling, chunked into
// windows no larger than the bridge's 90-day limit. The cursor is the RFC3339
// timestamp of the last sync (same scheme as Teller).
func (p *SimpleFINProvider) SyncTransactions(ctx context.Context, conn provider.Connection, cursor string) (provider.SyncResult, error) {
	accessURLBytes, err := crypto.Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return provider.SyncResult{}, fmt.Errorf("simplefin: decrypt access URL: %w", err)
	}
	accessURL := string(accessURLBytes)

	now := time.Now().UTC()
	fromDate, err := syncStart(cursor, now)
	if err != nil {
		return provider.SyncResult{}, err
	}

	var allTxns []provider.Transaction
	for _, w := range windows(fromDate, now, maxWindowDays) {
		txns, err := p.fetchWindow(ctx, accessURL, w.start, w.end)
		if err != nil {
			return provider.SyncResult{}, err
		}
		allTxns = append(allTxns, txns...)
	}

	return provider.SyncResult{
		Added:   allTxns,
		HasMore: false,
		Cursor:  now.Format(time.RFC3339),
	}, nil
}

// syncStart computes the start of the sync window from the stored cursor.
func syncStart(cursor string, now time.Time) (time.Time, error) {
	if cursor == "" {
		return now.AddDate(0, 0, -initialBackfillDays), nil
	}
	parsed, err := time.Parse(time.RFC3339, cursor)
	if err != nil {
		return time.Time{}, fmt.Errorf("simplefin: parse cursor %q: %w", cursor, err)
	}
	return parsed.AddDate(0, 0, -overlapDays), nil
}

// fetchWindow fetches one date-bounded /accounts page and maps every nested
// transaction. start-date is inclusive, end-date is exclusive.
func (p *SimpleFINProvider) fetchWindow(ctx context.Context, accessURL string, start, end time.Time) ([]provider.Transaction, error) {
	query := strings.Join([]string{
		"start-date=" + strconv.FormatInt(start.Unix(), 10),
		"end-date=" + strconv.FormatInt(end.Unix(), 10),
		"pending=1",
	}, "&")

	set, err := p.fetchAccountSet(ctx, accessURL, query)
	if err != nil {
		return nil, err
	}

	var txns []provider.Transaction
	for _, acct := range set.Accounts {
		for _, t := range acct.Transactions {
			mapped, err := t.toTransaction(acct.ID, acct.Currency)
			if err != nil {
				p.logger.WarnContext(ctx, "simplefin: skipping transaction with parse error",
					"transaction_id", t.ID, "account_id", acct.ID, "error", err)
				continue
			}
			txns = append(txns, mapped)
		}
	}
	return txns, nil
}

type window struct {
	start time.Time
	end   time.Time
}

// windows splits [from, to) into consecutive spans no longer than maxDays. The
// windows are contiguous and non-overlapping; since SimpleFIN treats end-date as
// exclusive and start-date as inclusive, boundary transactions are not double
// counted across adjacent windows.
func windows(from, to time.Time, maxDays int) []window {
	if !from.Before(to) {
		return nil
	}
	var out []window
	span := time.Duration(maxDays) * 24 * time.Hour
	for start := from; start.Before(to); {
		end := start.Add(span)
		if end.After(to) {
			end = to
		}
		out = append(out, window{start: start, end: end})
		start = end
	}
	return out
}
