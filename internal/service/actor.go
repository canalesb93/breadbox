//go:build !lite

package service

import (
	"context"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
)

// Actor identifies who performed an action, used for audit logging and comments.
type Actor struct {
	Type string // "user", "agent", "system"
	ID   string // admin_account ID, API key ID, or ""
	Name string // display name
}

// SystemActor returns an Actor for system-initiated actions.
func SystemActor() Actor {
	return Actor{Type: "system", ID: "", Name: "Breadbox"}
}

type contextKey int

const (
	ctxKeyAPIKey contextKey = iota
)

// apiKeyCtxValue holds the slice of fields ActorFromContext needs out of an
// API key. We carry the whole db.ApiKey so callers don't need to plumb the
// fields independently — middleware and CLI bootstrap both already have the
// row.
type apiKeyCtxValue struct {
	id        string
	prefix    string
	name      string
	actorType string
	actorName string
}

// ContextWithAPIKey stores an API-key-derived actor identity in the context.
// Middleware and the stdio bootstrap both call this — there is one canonical
// shape so ActorFromContext can read it consistently.
//
// Pre-PR-03 callers passed (id, name) only. That shape is kept on
// ContextWithAPIKeyLegacy for the few in-tree paths that haven't been
// migrated yet (synthetic OAuth bearer tokens, tests that mint a fake key
// without DB access). Those callers attribute as `agent`.
func ContextWithAPIKey(ctx context.Context, key *db.ApiKey) context.Context {
	if key == nil {
		return ctx
	}
	v := apiKeyCtxValue{
		id:        pgconv.FormatUUID(key.ID),
		prefix:    key.KeyPrefix,
		name:      key.Name,
		actorType: key.ActorType,
	}
	if key.ActorName.Valid {
		v.actorName = key.ActorName.String
	}
	return context.WithValue(ctx, ctxKeyAPIKey, v)
}

// ContextWithAPIKeyLegacy is the pre-PR-03 shape — id + display name, no
// explicit actor fields. Callers that produce a synthetic API key (OAuth
// bearer tokens, in-memory test keys) use this and get attributed as
// `agent` by default.
func ContextWithAPIKeyLegacy(ctx context.Context, id, name string) context.Context {
	return context.WithValue(ctx, ctxKeyAPIKey, apiKeyCtxValue{
		id:        id,
		name:      name,
		actorType: "agent",
	})
}

// AgentRunShortIDFromContext returns the agent_runs.short_id this
// request is acting under, derived from the API key's name. Returns
// "" when the request isn't running under a per-run agent key.
//
// API keys minted by Orchestrator.MintRunAPIKey are named
// "agent:<slug>:<runShortID>" (see internal/service/agents.go). Any
// future change to that format must update this parser in lock-step.
func AgentRunShortIDFromContext(ctx context.Context) string {
	v, ok := ctx.Value(ctxKeyAPIKey).(apiKeyCtxValue)
	if !ok {
		return ""
	}
	const prefix = "agent:"
	if len(v.name) <= len(prefix) || v.name[:len(prefix)] != prefix {
		return ""
	}
	rest := v.name[len(prefix):]
	lastColon := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon < 0 || lastColon == len(rest)-1 {
		return ""
	}
	return rest[lastColon+1:]
}

// ParseAgentKeySlug extracts the agent slug from a per-run key name of
// the form "agent:<slug>:<runID>" (minted by Orchestrator.MintRunAPIKey).
// Returns ok=false for any other shape. This is the single canonical Go
// parser for that contract — the avatar handler and the actor resolver
// both call it so the format lives in one place. Keep it in lock-step
// with the key name built in MintRunAPIKey and the SPLIT_PART backfill.
func ParseAgentKeySlug(name string) (string, bool) {
	parts := strings.Split(name, ":")
	if len(parts) != 3 || parts[0] != "agent" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// IsAgentRunContext reports whether ctx is authenticated as a scheduled
// agent's per-run key: actor_type='agent' AND a parseable
// "agent:<slug>:<runID>" name. It gates behavior that must apply ONLY to
// real agent runs — notably never letting the MCP clientInfo rebind
// clobber the run key. The actor_type check is load-bearing: a non-agent
// key that merely happens to be named "agent:..." must NOT be treated as
// a run (otherwise an operator could spoof an agent identity).
func IsAgentRunContext(ctx context.Context) bool {
	v, ok := ctx.Value(ctxKeyAPIKey).(apiKeyCtxValue)
	if !ok || v.actorType != "agent" {
		return false
	}
	_, parsed := ParseAgentKeySlug(v.name)
	return parsed
}

// ActorFromContext builds an Actor from the request context.
// The actor's Type reflects the API key's actor_type column ('user',
// 'agent', or 'system'). Display name falls back to the API key's own
// name (and then its prefix) when actor_name is empty.
func ActorFromContext(ctx context.Context) Actor {
	v, ok := ctx.Value(ctxKeyAPIKey).(apiKeyCtxValue)
	if !ok || v.id == "" {
		return SystemActor()
	}
	actorType := v.actorType
	if actorType == "" {
		actorType = "agent"
	}
	name := v.actorName
	if name == "" {
		name = v.name
	}
	if name == "" {
		name = v.prefix
	}
	return Actor{Type: actorType, ID: v.id, Name: name}
}
