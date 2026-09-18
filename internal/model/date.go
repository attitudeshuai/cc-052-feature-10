package model

import (
	"database/sql/driver"
	"errors"
	"strings"
	"time"
)

// Date 是只保留到「日」的日期，API 与 JSON 中统一为 YYYY-MM-DD，
// 数据库中对应 DATE 类型，避免资质到期日被时区带偏。
type Date struct {
	time.Time
}

const dateLayout = "2006-01-02"

func NewDate(t time.Time) Date {
	return Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}
}

func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(s))
	if err != nil {
		return Date{}, err
	}
	return Date{Time: t}, nil
}

func (d Date) Format() string {
	return d.Time.Format(dateLayout)
}

func (d Date) MarshalJSON() ([]byte, error) {
	if d.Time.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Format() + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		d.Time = time.Time{}
		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return errors.New("invalid date, expected YYYY-MM-DD")
	}
	d.Time = t
	return nil
}

// Scan 实现 sql.Scanner（postgres DATE → time.Time → Date）。
func (d *Date) Scan(src interface{}) error {
	if src == nil {
		d.Time = time.Time{}
		return nil
	}
	switch v := src.(type) {
	case time.Time:
		d.Time = v
		return nil
	case string:
		t, err := time.Parse(dateLayout, v)
		if err != nil {
			return err
		}
		d.Time = t
		return nil
	default:
		return errors.New("cannot scan into Date")
	}
}

// Value 实现 driver.Valuer。
func (d Date) Value() (driver.Value, error) {
	if d.Time.IsZero() {
		return nil, nil
	}
	return d.Format(), nil
}
