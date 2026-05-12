package api

import (
	"errors"
	"net/http"
	"strings"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
)

// deviceCodeResponse is the body returned by POST /auth/device-code.
// `device_code` is the opaque polling token; `user_code` is the human-
// facing approval code (formatted `XXXX-XXXX`). `interval` and
// `expires_in` are in seconds.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// CreateDeviceCodeHandler serves POST /api/v1/auth/device-code. No auth.
// Returns a fresh pending device-code pair the CLI can poll on.
func CreateDeviceCodeHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dc, err := svc.CreateDeviceCode(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to initiate device-code flow")
			return
		}
		writeJSON(w, http.StatusCreated, deviceCodeResponse{
			DeviceCode:      dc.DeviceCode,
			UserCode:        service.FormatUserCode(dc.UserCode),
			VerificationURL: buildDeviceVerificationURL(r),
			ExpiresIn:       int(service.DeviceCodeTTL.Seconds()),
			Interval:        int(service.DeviceCodePollInterval.Seconds()),
		})
	}
}

// deviceCodePollRequest is the request body for the poll endpoint.
type deviceCodePollRequest struct {
	DeviceCode string `json:"device_code"`
}

// deviceCodePollResponse is the JSON body for a 200 poll response.
// `status` is one of "authorization_pending" or "approved"; `token` is
// populated only when status="approved" and only on the first successful
// poll after approval.
type deviceCodePollResponse struct {
	Status string `json:"status"`
	Token  string `json:"token,omitempty"`
}

// PollDeviceCodeHandler serves POST /api/v1/auth/device-code/poll. No
// auth — the device_code itself is the credential.
//
// Maps service-layer outcomes to the OAuth-2-style status/error split:
//   - pending → 200 authorization_pending
//   - approved → 200 approved + token (one-shot)
//   - expired  → 400 EXPIRED
//   - denied   → 400 DENIED
//   - missing  → 404 INVALID_DEVICE_CODE
func PollDeviceCodeHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req deviceCodePollRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		req.DeviceCode = strings.TrimSpace(req.DeviceCode)
		if req.DeviceCode == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "device_code is required")
			return
		}

		dc, err := svc.PollDeviceCode(r.Context(), req.DeviceCode)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNotFound):
				mw.WriteError(w, http.StatusNotFound, "INVALID_DEVICE_CODE", "Unknown device_code")
			case errors.Is(err, service.ErrExpired):
				mw.WriteError(w, http.StatusBadRequest, "EXPIRED", "Device code has expired")
			case errors.Is(err, service.ErrInvalidState):
				mw.WriteError(w, http.StatusBadRequest, "DENIED", "Device code was denied by the operator")
			default:
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to poll device code")
			}
			return
		}
		if dc.Status == "approved" {
			writeJSON(w, http.StatusOK, deviceCodePollResponse{Status: "approved", Token: dc.Token})
			return
		}
		writeJSON(w, http.StatusOK, deviceCodePollResponse{Status: "authorization_pending"})
	}
}

// buildDeviceVerificationURL constructs the absolute URL the end user
// should open in a browser. Mirrors the scheme/host inference used by
// buildHostedLinkURL so reverse proxies are handled the same way.
func buildDeviceVerificationURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/auth/device"
}
