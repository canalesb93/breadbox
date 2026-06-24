//go:build !lite

package service

import (
	"context"
	"testing"
)

// flag / unflag take no parameters and validate cleanly; they also compose with
// other actions. actionAuditFields carries their semantic intent.
func TestValidateActions_Flag(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	cases := []struct {
		name    string
		actions []RuleAction
		wantErr bool
	}{
		{"flag", []RuleAction{{Type: "flag"}}, false},
		{"unflag", []RuleAction{{Type: "unflag"}}, false},
		{"flag + add_tag composes", []RuleAction{{Type: "flag"}, {Type: "add_tag", TagSlug: "needs-review"}}, false},
		{"unknown still rejected", []RuleAction{{Type: "frobnicate"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.ValidateActions(ctx, tc.actions); (err != nil) != tc.wantErr {
				t.Errorf("ValidateActions err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestActionAuditFields_Flag(t *testing.T) {
	if f, v := actionAuditFields(RuleAction{Type: "flag"}); f != "flag" || v != "" {
		t.Errorf("flag audit fields = (%q,%q), want (flag,\"\")", f, v)
	}
	if f, v := actionAuditFields(RuleAction{Type: "unflag"}); f != "unflag" || v != "" {
		t.Errorf("unflag audit fields = (%q,%q), want (unflag,\"\")", f, v)
	}
}
