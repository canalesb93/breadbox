//go:build !lite

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
)

// MCPClientFallbackFingerprint is the reserved fingerprint for the
// single "Local MCP" identity used when an MCP session doesn't send a
// clientInfo block on initialize. The relabelled stdio singleton row
// carries this value (see migration
// 20260526092106_api_keys_client_fingerprint.sql).
const MCPClientFallbackFingerprint = "unknown@@stdio"

// MCPClientFallbackActorName is the actor_name used both by the
// fallback singleton and by per-client rows whose clientInfo carried
// no usable label. Mirrored in the migration's UPDATE.
const MCPClientFallbackActorName = "Local MCP"

// MCPClientFingerprint normalises a clientInfo + transport pair into
// the stable identity key that EnsureMCPClientAgentKey uses to look
// up (or create) the per-client api_keys row. The format is
// `lower(label) + "@" + lower(host) + "@" + transport`:
//
//   - label = Title || Name (whichever is non-empty); collapsed
//     whitespace + lowercased so case/whitespace variants don't
//     fragment the identity
//   - host = lowercased host portion of WebsiteURL (falls back to ""
//     when the URL is empty or unparseable). Including host
//     disambiguates two unrelated clients that share a name (e.g.
//     "claude" — see reviewer concern in the planning doc).
//   - transport = "stdio" | "http"
//
// Returns MCPClientFallbackFingerprint when there is no usable label
// at all; the caller's EnsureMCPClientAgentKey will then return the
// pre-relabelled singleton fallback row.
func MCPClientFingerprint(client MCPClientInfo, transport string) string {
	label := strings.TrimSpace(client.Title)
	if label == "" {
		label = strings.TrimSpace(client.Name)
	}
	if label == "" {
		return MCPClientFallbackFingerprint
	}
	label = collapseWhitespace(strings.ToLower(label))
	host := ""
	if client.WebsiteURL != "" {
		if u, err := url.Parse(client.WebsiteURL); err == nil {
			host = strings.ToLower(u.Hostname())
		}
	}
	return fmt.Sprintf("%s@%s@%s", label, host, transport)
}

// collapseWhitespace folds any run of whitespace down to a single
// underscore so "Claude Desktop" and "claude  desktop" fingerprint
// identically. Underscore (not space) so the fingerprint itself never
// contains a space — it lands in api_keys.name + key_prefix where
// surrounding tooling treats whitespace as a delimiter.
func collapseWhitespace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace && b.Len() > 0 {
				b.WriteRune('_')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	out := b.String()
	return strings.TrimRight(out, "_")
}

// EnsureMCPClientAgentKey returns the per-client agent identity row
// for a given (clientInfo, transport) pair. Idempotent: the first
// call creates the row, every later call returns it. On concurrent
// first-call from the same client, both goroutines see the partial
// unique index conflict and the loser re-reads the winner's row.
//
// On total failure (DB down, unexpected error), the caller gets the
// fallback "Local MCP" singleton so the request still has a valid
// actor — same shape as the existing pre-PR behaviour, just no
// humanoid avatar. The error is returned for logging; the dispatcher
// chooses to swallow it.
func (s *Service) EnsureMCPClientAgentKey(ctx context.Context, client MCPClientInfo, transport string) (*db.ApiKey, error) {
	fingerprint := MCPClientFingerprint(client, transport)

	if row, err := s.Queries.GetApiKeyByClientFingerprint(ctx, pgconv.TextIfNotEmpty(fingerprint)); err == nil {
		_ = s.Queries.UpdateApiKeyLastUsed(ctx, row.ID)
		return &row, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return s.fallbackMCPClientKey(ctx, fingerprint, err)
	}

	actorName := strings.TrimSpace(client.Title)
	if actorName == "" {
		actorName = strings.TrimSpace(client.Name)
	}
	if actorName == "" {
		actorName = MCPClientFallbackActorName
	}

	row, err := s.Queries.CreateMCPClientApiKey(ctx, db.CreateMCPClientApiKeyParams{
		Name:              "mcp-client:" + fingerprint,
		KeyHash:           "mcp-client-not-a-real-credential:" + fingerprint,
		KeyPrefix:         "bb_mcp_" + first8(sha256Hex(fingerprint)),
		ActorName:         pgconv.TextIfNotEmpty(actorName),
		ClientFingerprint: pgconv.TextIfNotEmpty(fingerprint),
	})
	if err == nil {
		return &row, nil
	}

	// Insert race: a concurrent first-call already created the row.
	// The partial unique index kicked the loser out; re-read the
	// winner's row.
	if row2, lookupErr := s.Queries.GetApiKeyByClientFingerprint(ctx, pgconv.TextIfNotEmpty(fingerprint)); lookupErr == nil {
		_ = s.Queries.UpdateApiKeyLastUsed(ctx, row2.ID)
		return &row2, nil
	}
	return s.fallbackMCPClientKey(ctx, fingerprint, err)
}

// fallbackMCPClientKey returns the relabelled stdio singleton (the
// reserved fingerprint = MCPClientFallbackFingerprint) when an
// otherwise-clean lookup/insert path errors out. The intent is to
// hand the dispatcher *some* valid actor instead of falling back to
// SystemActor — which would render even more confusingly than the
// pre-fix singleton path.
func (s *Service) fallbackMCPClientKey(ctx context.Context, attemptedFingerprint string, cause error) (*db.ApiKey, error) {
	slog.Warn("EnsureMCPClientAgentKey: falling back to Local MCP singleton",
		"fingerprint", attemptedFingerprint, "error", cause)
	row, err := s.Queries.GetApiKeyByClientFingerprint(ctx, pgconv.TextIfNotEmpty(MCPClientFallbackFingerprint))
	if err != nil {
		// The migration that relabels the singleton hasn't applied
		// yet (e.g. mid-deploy on a fresh install with no stdio
		// history). Surface the original error so the caller can
		// fall back to its pre-PR behaviour.
		return nil, fmt.Errorf("ensure mcp client agent key: %w", cause)
	}
	return &row, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func first8(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}
