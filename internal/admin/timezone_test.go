package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserLocation(t *testing.T) {
	tests := []struct {
		name      string
		cookieVal string
		setCookie bool
		want      string
	}{
		{name: "no cookie falls back to local", setCookie: false, want: time.Local.String()},
		{name: "empty cookie falls back to local", setCookie: true, cookieVal: "", want: time.Local.String()},
		{name: "valid IANA name resolves", setCookie: true, cookieVal: "America/Los_Angeles", want: "America/Los_Angeles"},
		{name: "valid UTC resolves", setCookie: true, cookieVal: "UTC", want: "UTC"},
		{name: "garbage zone falls back to local", setCookie: true, cookieVal: "Not/A/Real/Zone", want: time.Local.String()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/feed", nil)
			if tc.setCookie {
				r.AddCookie(&http.Cookie{Name: userTZCookie, Value: tc.cookieVal})
			}
			loc := UserLocation(r)
			if loc.String() != tc.want {
				t.Fatalf("UserLocation = %q, want %q", loc.String(), tc.want)
			}
		})
	}
}

func TestUserLocation_NilRequest(t *testing.T) {
	loc := UserLocation(nil)
	if loc != time.Local {
		t.Fatalf("UserLocation(nil) = %v, want time.Local", loc)
	}
}
