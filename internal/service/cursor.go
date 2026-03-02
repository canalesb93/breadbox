package service

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type cursorPayload struct {
	Date string `json:"d"`
	ID   string `json:"i"`
}

func EncodeCursor(date time.Time, id string) string {
	p := cursorPayload{
		Date: date.Format("2006-01-02"),
		ID:   id,
	}
	b, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeCursor(cursor string) (date time.Time, id string, err error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	date, err = time.Parse("2006-01-02", p.Date)
	if err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	if p.ID == "" {
		return time.Time{}, "", ErrInvalidCursor
	}
	return date, p.ID, nil
}
