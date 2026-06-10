package sqlite

import (
	"database/sql"
	"time"
)

func nullDBString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func timeToMs(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UnixMilli()
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
