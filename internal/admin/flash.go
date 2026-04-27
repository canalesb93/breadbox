package admin

import (
	"context"
	"net/http"

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

// FlashRedirect sets a flash message and writes a 303 See Other redirect.
// It collapses the SetFlash + http.Redirect early-return idiom in admin POST
// handlers; the caller still issues `return` after invoking it.
//
//	if err := svc.Foo(r.Context()); err != nil {
//	    FlashRedirect(w, r, sm, "error", "Failed: "+err.Error(), "/foo")
//	    return
//	}
func FlashRedirect(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, msgType, message, target string) {
	SetFlash(r.Context(), sm, msgType, message)
	http.Redirect(w, r, target, http.StatusSeeOther)
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
