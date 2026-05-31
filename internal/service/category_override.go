//go:build !lite

package service

// Category-override source levels — who set a transaction's current category.
// Stored in transactions.category_override (a TEXT enum). Precedence is
// user > agent > rule:
//   - rules write only 'none' rows (they never overwrite an agent or user);
//   - agents write where the level is not 'user', stamping 'agent';
//   - users write anywhere, stamping 'user' (sacred — nothing auto-overwrites).
const (
	CategoryOverrideNone  = "none"
	CategoryOverrideAgent = "agent"
	CategoryOverrideUser  = "user"
)
