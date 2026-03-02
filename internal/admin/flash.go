package admin

import (
	"context"

	"github.com/alexedwards/scs/v2"
)

const (
	flashTypeKey    = "flash_type"
	flashMessageKey = "flash_message"
)

// SetFlash stores a flash message in the session. It will be displayed
// once on the next page load and then cleared.
func SetFlash(ctx context.Context, sm *scs.SessionManager, msgType, message string) {
	sm.Put(ctx, flashTypeKey, msgType)
	sm.Put(ctx, flashMessageKey, message)
}

// GetFlash retrieves and clears the flash message from the session.
// Returns nil if no flash message is set.
func GetFlash(ctx context.Context, sm *scs.SessionManager) *Flash {
	msgType := sm.PopString(ctx, flashTypeKey)
	message := sm.PopString(ctx, flashMessageKey)
	if message == "" {
		return nil
	}
	return &Flash{
		Type:    msgType,
		Message: message,
	}
}
