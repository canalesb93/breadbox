package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"strings"
	"testing"

	"breadbox/internal/client"
)

// TestBuildCSVMultipart asserts the multipart body the client produces
// carries the file under the `file` field name, the form values the
// REST handler reads (positive_is_debit, column_mapping, etc.), and uses
// the correct Content-Type. The CSV endpoint is the only place the CLI
// ships multipart, so a regression here would silently break import.
func TestBuildCSVMultipart(t *testing.T) {
	data := []byte("date,amount,description\n2025-01-01,1.00,Test\n")
	mapping := map[string]int{"date": 0, "amount": 1, "description": 2}
	buf, contentType, err := client.BuildCSVMultipart("sample.csv", data, client.CSVOptions{
		UserID:          "user-1",
		AccountName:     "Checking",
		ColumnMapping:   mapping,
		PositiveIsDebit: true,
		Limit:           5,
	})
	if err != nil {
		t.Fatalf("BuildCSVMultipart: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("contentType = %q, want multipart/form-data; ...", contentType)
	}

	// Parse the body back out and assert each field landed.
	_, params, err := mimeParse(contentType)
	if err != nil {
		t.Fatalf("parse Content-Type: %v", err)
	}
	mr := multipart.NewReader(buf, params["boundary"])
	got := map[string]string{}
	var file []byte
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		body, err := io.ReadAll(p)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		if p.FormName() == "file" {
			file = body
			continue
		}
		got[p.FormName()] = string(body)
	}

	if string(file) != string(data) {
		t.Errorf("file body mismatch:\ngot  %q\nwant %q", file, data)
	}
	if got["user_id"] != "user-1" {
		t.Errorf("user_id = %q, want user-1", got["user_id"])
	}
	if got["account_name"] != "Checking" {
		t.Errorf("account_name = %q, want Checking", got["account_name"])
	}
	if got["positive_is_debit"] != "true" {
		t.Errorf("positive_is_debit = %q, want true", got["positive_is_debit"])
	}
	if got["limit"] != "5" {
		t.Errorf("limit = %q, want 5", got["limit"])
	}
	var mapBack map[string]int
	if err := json.Unmarshal([]byte(got["column_mapping"]), &mapBack); err != nil {
		t.Fatalf("decode column_mapping: %v", err)
	}
	if mapBack["amount"] != 1 {
		t.Errorf("column_mapping[amount] = %d, want 1", mapBack["amount"])
	}
}

// mimeParse is a tiny shim around mime.ParseMediaType so the test reads
// without an extra import shuffle.
func mimeParse(v string) (string, map[string]string, error) {
	// inline minimal split — Content-Type is "multipart/form-data; boundary=xxx"
	parts := strings.SplitN(v, ";", 2)
	out := map[string]string{}
	if len(parts) == 2 {
		for _, kv := range strings.Split(parts[1], ";") {
			kv = strings.TrimSpace(kv)
			if i := strings.Index(kv, "="); i > 0 {
				out[kv[:i]] = strings.Trim(kv[i+1:], `"`)
			}
		}
	}
	return parts[0], out, nil
}

// _ ensures the bytes import survives if pruned by formatter.
var _ = bytes.NewBuffer
