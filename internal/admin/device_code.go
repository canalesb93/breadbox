//go:build !headless && !lite

package admin

import (
	"errors"
	"html/template"
	"net/http"
	"strings"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
)

// deviceCodePageTmpl renders the device-code approval surface. Kept
// inline (rather than as a templ component) because the page is a
// dead-simple two-button form — the rest of the dashboard chrome would
// be noise here, and CLI users following the verification_url often
// arrive from an unfamiliar browser context where the standard nav
// would be visually disorienting.
var deviceCodePageTmpl = template.Must(template.New("device-code").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Approve CLI access — Breadbox</title>
  <link rel="stylesheet" href="/static/css/styles.css">
</head>
<body class="min-h-screen flex items-center justify-center bg-base-200 p-4">
  <main class="card w-full max-w-md bg-base-100 shadow-lg">
    <div class="card-body">
      <h1 class="card-title text-2xl mb-2">Approve CLI access</h1>
      {{if .Flash}}<div class="alert alert-{{.FlashType}} mb-3">{{.Flash}}</div>{{end}}
      {{if .Error}}<div class="alert alert-error mb-3">{{.Error}}</div>{{end}}
      {{if .Approved}}
        <div class="alert alert-success">
          <div>
            <p class="font-semibold">Approved.</p>
            <p>The CLI that printed code <code>{{.DisplayCode}}</code> can finish logging in now.
            You can close this tab.</p>
          </div>
        </div>
      {{else if .Denied}}
        <div class="alert alert-warning">
          <p>Denied. The CLI will exit with an error.</p>
        </div>
      {{else}}
        <p class="mb-4 text-base-content/70">
          A Breadbox CLI is requesting access. Approving will mint an API key bound
          to <strong>{{.DefaultActor}}</strong> and return it to the waiting CLI process.
        </p>
        <form method="post" action="/auth/device" class="space-y-4">
          <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
          <input type="hidden" name="user_code" value="{{.UserCode}}">
          <div class="form-control">
            <label class="label"><span class="label-text font-medium">Code</span></label>
            <input class="input input-bordered" value="{{.DisplayCode}}" readonly>
          </div>
          <div class="form-control">
            <label class="label" for="actor_name"><span class="label-text font-medium">Actor name</span></label>
            <input id="actor_name" name="actor_name" class="input input-bordered"
              value="{{.DefaultActor}}" placeholder="e.g. workstation-prod">
            <span class="label-text-alt text-base-content/60 mt-1">Shown in audit logs as the user behind this key.</span>
          </div>
          <div class="form-control">
            <label class="label" for="scope"><span class="label-text font-medium">Scope</span></label>
            <select id="scope" name="scope" class="select select-bordered bg-base-100">
              <option value="read_only" selected>Read only</option>
              <option value="full_access">Full access</option>
            </select>
          </div>
          <div class="flex gap-2 pt-2">
            <button type="submit" name="action" value="approve" class="btn btn-primary flex-1">Approve</button>
            <button type="submit" name="action" value="deny" class="btn btn-ghost">Deny</button>
          </div>
        </form>
      {{end}}
      <p class="mt-6 text-xs text-base-content/50">
        Expecting a different page? <a class="link" href="/auth/device">Clear the code</a>
        or visit <a class="link" href="/settings/api-keys">API keys</a> directly.
      </p>
    </div>
  </main>
</body>
</html>`))

// deviceCodePageData is the view-model the inline template above
// consumes. Kept package-private; not part of any reusable contract.
type deviceCodePageData struct {
	UserCode     string // stored canonical form (no dash)
	DisplayCode  string // XXXX-XXXX form
	DefaultActor string
	CSRFToken    string
	Error        string
	Approved     bool
	Denied       bool
	Flash        string
	FlashType    string
}

// DeviceCodeApprovalHandler serves GET/POST /auth/device behind the
// admin session. GET renders the approval form (pre-populating the code
// from `?code=XXXX-XXXX`); POST processes approve/deny and either flips
// the row or renders an error.
//
// Mounted behind RequireAuth + CSRFMiddleware (see internal/admin/router.go).
func DeviceCodeApprovalHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			displayCode := strings.ToUpper(r.URL.Query().Get("code"))
			renderDeviceCodePage(w, r, sm, deviceCodePageData{
				UserCode:     stripUserCodeFormatting(displayCode),
				DisplayCode:  displayCode,
				DefaultActor: defaultActorFromRequest(r),
				CSRFToken:    GetCSRFToken(r),
			})
		case http.MethodPost:
			handleDeviceCodePost(w, r, sm, svc)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleDeviceCodePost(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, svc *service.Service) {
	action := r.FormValue("action")
	userCode := strings.TrimSpace(r.FormValue("user_code"))
	if userCode == "" {
		userCode = strings.TrimSpace(r.FormValue("code"))
	}
	displayCode := strings.ToUpper(userCode)
	if len(userCode) == 8 {
		displayCode = strings.ToUpper(userCode[:4]) + "-" + strings.ToUpper(userCode[4:])
	}

	if userCode == "" {
		renderDeviceCodePage(w, r, sm, deviceCodePageData{
			DisplayCode:  displayCode,
			DefaultActor: defaultActorFromRequest(r),
			CSRFToken:    GetCSRFToken(r),
			Error:        "Enter the code shown by the CLI.",
		})
		return
	}

	approvedBy := SessionAccountID(sm, r)
	switch action {
	case "approve":
		actorName := strings.TrimSpace(r.FormValue("actor_name"))
		if actorName == "" {
			actorName = defaultActorFromRequest(r)
		}
		scope := r.FormValue("scope")
		_, err := svc.ApproveDeviceCode(r.Context(), service.ApproveDeviceCodeParams{
			UserCode:   userCode,
			ActorName:  actorName,
			Scope:      scope,
			ApprovedBy: approvedBy,
		})
		if err != nil {
			renderDeviceCodePage(w, r, sm, deviceCodePageData{
				UserCode:     stripUserCodeFormatting(userCode),
				DisplayCode:  displayCode,
				DefaultActor: actorName,
				CSRFToken:    GetCSRFToken(r),
				Error:        deviceCodeErrorMessage(err),
			})
			return
		}
		renderDeviceCodePage(w, r, sm, deviceCodePageData{
			DisplayCode: displayCode,
			Approved:    true,
		})
	case "deny":
		err := svc.DenyDeviceCode(r.Context(), userCode, approvedBy)
		if err != nil {
			renderDeviceCodePage(w, r, sm, deviceCodePageData{
				UserCode:     stripUserCodeFormatting(userCode),
				DisplayCode:  displayCode,
				DefaultActor: defaultActorFromRequest(r),
				CSRFToken:    GetCSRFToken(r),
				Error:        deviceCodeErrorMessage(err),
			})
			return
		}
		renderDeviceCodePage(w, r, sm, deviceCodePageData{
			DisplayCode: displayCode,
			Denied:      true,
		})
	default:
		renderDeviceCodePage(w, r, sm, deviceCodePageData{
			UserCode:     stripUserCodeFormatting(userCode),
			DisplayCode:  displayCode,
			DefaultActor: defaultActorFromRequest(r),
			CSRFToken:    GetCSRFToken(r),
			Error:        "Pick Approve or Deny.",
		})
	}
}

// deviceCodeErrorMessage maps a service-layer error to friendly UI copy.
func deviceCodeErrorMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return "Unknown code — double-check the value the CLI printed."
	case errors.Is(err, service.ErrExpired):
		return "This code has expired. Restart the CLI to get a fresh one."
	case errors.Is(err, service.ErrInvalidState):
		return "This code has already been handled."
	case errors.Is(err, service.ErrInvalidParameter):
		return "Code format looks wrong — it should be 8 letters/digits like XXXX-XXXX."
	default:
		return "Something went wrong handling that code. Try again."
	}
}

// defaultActorFromRequest seeds a friendly default for the actor_name
// input. The browser hitting the verification page isn't necessarily on
// the same host as the CLI, so we deliberately pick a generic label
// rather than echo the polling User-Agent back into the form.
func defaultActorFromRequest(_ *http.Request) string {
	return "cli-device"
}

// stripUserCodeFormatting returns the canonical 8-char form (uppercase,
// no dash) from a raw input. Returns the input unchanged when it can't
// be normalized so callers still see something useful.
func stripUserCodeFormatting(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func renderDeviceCodePage(w http.ResponseWriter, r *http.Request, sm *scs.SessionManager, data deviceCodePageData) {
	if f := GetFlash(r.Context(), sm); f != nil {
		data.Flash = f.Message
		data.FlashType = f.Type
	}
	if data.DefaultActor == "" {
		data.DefaultActor = defaultActorFromRequest(r)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := deviceCodePageTmpl.Execute(w, data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}
