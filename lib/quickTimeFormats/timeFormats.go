package quickTimeFormats

import "time"

func TimeFormats() []string {
	return []string{
		"1/2/2006",
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04:05.999999999",
		time.DateOnly,
		time.Layout,
		time.UnixDate,
		time.Stamp,
		time.StampMilli,
		time.StampMicro,
		time.StampNano,
		"01/02",
		"01/02-01/02",
	}
}
