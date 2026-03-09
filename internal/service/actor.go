package service

import "context"

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
	ctxKeyAPIKeyID   contextKey = iota
	ctxKeyAPIKeyName contextKey = iota
)

// ContextWithAPIKey stores API key identity in the context.
func ContextWithAPIKey(ctx context.Context, id, name string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyAPIKeyID, id)
	ctx = context.WithValue(ctx, ctxKeyAPIKeyName, name)
	return ctx
}

// ActorFromContext builds an Actor from the request context.
// If API key info is present (set by APIKeyAuth middleware), returns an agent actor.
// Otherwise returns a system actor.
func ActorFromContext(ctx context.Context) Actor {
	id, _ := ctx.Value(ctxKeyAPIKeyID).(string)
	name, _ := ctx.Value(ctxKeyAPIKeyName).(string)
	if id != "" {
		return Actor{Type: "agent", ID: id, Name: name}
	}
	return SystemActor()
}
