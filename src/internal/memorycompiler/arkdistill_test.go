package memorycompiler

import (
	"strings"
	"testing"
)

func TestDetectOutputType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  OutputType
	}{
		{"HTML", "<!DOCTYPE html><html><body>hello</body></html>", OutputHTML},
		{"JSON", `{"key": "value", "arr": [1,2,3]}`, OutputJSON},
		{"JSONArray", `[{"id":1},{"id":2}]`, OutputJSON},
		{"Log", "[2024-01-01 12:00:00] INFO: started", OutputLog},
		{"Text", "hello world", OutputText},
		{"Table", "| A | B |\n|---+---|\n| 1 | 2 |", OutputTable},
		{"Empty", "", OutputUnknown},
		{"Trace", "panic at main.go:42", OutputTrace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectOutputType(tt.input)
			if got != tt.want {
				t.Errorf("DetectOutputType(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestCompactToolOutput(t *testing.T) {
	profiles := DefaultProfiles()
	htmlProfile := profiles[OutputHTML]

	// Test HTML stripping
	longHTML := "<!DOCTYPE html>\n<html>\n<body>\n<p>line1</p>\n<p>line2</p>\n<p>line3</p>\n</body>\n</html>"
	compacted, truncated := CompactToolOutput(longHTML, OutputHTML, &htmlProfile)
	if truncated {
		t.Error("unexpected truncation for short HTML")
	}
	if strings.Contains(compacted, "<p>") {
		t.Error("HTML tags not stripped")
	}

	// Test deduplication
	logProfile := profiles[OutputLog]
	dupedLog := "[INFO] start\n[INFO] start\n[INFO] start\n[INFO] end"
	result, _ := CompactToolOutput(dupedLog, OutputLog, &logProfile)
	lines := strings.Split(result, "\n")
	if len(lines) < 2 || len(lines) > 4 {
		t.Errorf("expected 2-3 lines after dedup, got %d", len(lines))
	}

	// Test line limit truncation
	smallProfile := CompactionProfile{
		Type: OutputText, MaxLines: 4, MaxChars: 1000,
		CompressionPct: 0.5,
	}
	manyLines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"
	result2, truncated2 := CompactToolOutput(manyLines, OutputText, &smallProfile)
	if !truncated2 {
		t.Error("expected truncation for 8 lines with MaxLines=4")
	}
	if !strings.Contains(result2, "lines suppressed") {
		t.Error("expected suppression message in output")
	}
}

func TestProfileLearner(t *testing.T) {
	pl := NewProfileLearner()

	// Record corrections
	pl.RecordCorrection("webfetch", OutputHTML, true, "too_verbose")
	pl.RecordCorrection("webfetch", OutputHTML, true, "too_verbose")
	pl.RecordCorrection("webfetch", OutputHTML, true, "too_verbose")
	pl.RecordCorrection("bash", OutputLog, true, "too_verbose")
	pl.RecordCorrection("bash", OutputLog, true, "too_verbose")

	// Should suggest tightening HTML
	adjustments := pl.SuggestAdjustments()
	found := false
	for _, a := range adjustments {
		if a.Type == OutputHTML && a.Field == "MaxLines" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected HTML profile adjustment")
	}
}
