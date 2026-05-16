package engine

import "testing"

func TestParseLogLineArtemisTimestampWithoutTimezone(t *testing.T) {
	line, err := ParseLogLine("[2026-05-16T10:23:14]: こんにちは")
	if err != nil {
		t.Fatal(err)
	}

	if line.RawTime != "2026-05-16T10:23:14" {
		t.Fatalf("RawTime = %q, want %q", line.RawTime, "2026-05-16T10:23:14")
	}
	if line.Text != "こんにちは" {
		t.Fatalf("Text = %q, want %q", line.Text, "こんにちは")
	}
	if line.Time.Year() != 2026 || line.Time.Month() != 5 || line.Time.Day() != 16 ||
		line.Time.Hour() != 10 || line.Time.Minute() != 23 || line.Time.Second() != 14 {
		t.Fatalf("Time = %s, want 2026-05-16T10:23:14", line.Time.Format("2006-01-02T15:04:05"))
	}
}
