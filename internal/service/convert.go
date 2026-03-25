package service

import (
	"fmt"
	"math/big"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := formatUUID(u)
	return &s
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func numericFloat(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	// Construct the float from Int * 10^Exp
	if n.Int == nil {
		return nil
	}
	f := new(big.Float).SetInt(n.Int)
	if n.Exp != 0 {
		exp := new(big.Float).SetFloat64(1)
		base := new(big.Float).SetFloat64(10)
		e := int(n.Exp)
		if e > 0 {
			for i := 0; i < e; i++ {
				exp.Mul(exp, base)
			}
		} else {
			for i := 0; i < -e; i++ {
				exp.Mul(exp, base)
			}
			exp = new(big.Float).Quo(new(big.Float).SetFloat64(1), exp)
		}
		f.Mul(f, exp)
	}
	result, _ := f.Float64()
	return &result
}

func timestampStr(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

func timestampPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time.UTC()
	return &t
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func datePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func nullConnStatusPtr(s db.NullConnectionStatus) *string {
	if !s.Valid {
		return nil
	}
	str := string(s.ConnectionStatus)
	return &str
}

func connStatusPtr(s db.ConnectionStatus) *string {
	str := string(s)
	return &str
}
