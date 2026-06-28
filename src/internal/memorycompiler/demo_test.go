package memorycompiler

import (
	"fmt"
	"testing"
)

// TestArkDistillDemo demonstrates the actual compression ratios for different output types.
func TestArkDistillDemo(t *testing.T) {
	profiles := DefaultProfiles()
	demo := func(name, output string, outType OutputType) {
		profile := profiles[outType]
		detected := DetectOutputType(output)
		compacted, _ := CompactToolOutput(output, outType, &profile)
		originalLen := len(output)
		compactedLen := len(compacted)
		ratio := 1.0 - float64(compactedLen)/float64(originalLen)

		fmt.Printf("\n=== %s ===\n", name)
		fmt.Printf("  检测类型: %s (期望: %s)\n", detected, outType)
		fmt.Printf("  原始大小: %d chars\n", originalLen)
		fmt.Printf("  压缩大小: %d chars\n", compactedLen)
		fmt.Printf("  压缩率:   %.1f%%\n", ratio*100)
		if originalLen > 0 {
			fmt.Printf("  原始预览: %q\n", output[:min(80, len(output))])
			fmt.Printf("  压缩预览: %q\n", compacted[:min(80, len(compacted))])
		}
	}

	// Test HTML output (like a web fetch)
	html := `<!DOCTYPE html>
<html lang="zh">
<head><meta charset="UTF-8"><title>Test Page</title></head>
<body>
<nav><a href="/">Home</a><a href="/about">About</a></nav>
<main>
<article><h1>Article Title</h1>
<p>This is a long paragraph with lots of <strong>formatting</strong> and <em>emphasis</em> that we want to strip out.</p>
<p>Another paragraph with <a href="http://example.com">a link</a> and more content.</p>
</article>
<aside><p>Sidebar content that might be noise.</p></aside>
</main>
<footer><p>Footer &copy; 2024</p></footer>
</body>
</html>`
	demo("HTML 网页", html, OutputHTML)

	// Test log output (like bash command output)
	log := `[2024-06-27 10:00:01] INFO  Starting build process
[2024-06-27 10:00:02] DEBUG Loading config file: /etc/app/config.yaml
[2024-06-27 10:00:02] DEBUG Config loaded successfully
[2024-06-27 10:00:03] INFO  Compiling module A... done (1.2s)
[2024-06-27 10:00:04] INFO  Compiling module B... done (0.8s)
[2024-06-27 10:00:04] DEBUG Module B dependencies resolved
[2024-06-27 10:00:05] INFO  Compiling module C... done (2.1s)
[2024-06-27 10:00:05] DEBUG Module C dependencies resolved
[2024-06-27 10:00:06] INFO  Running tests...
[2024-06-27 10:00:06] DEBUG Test configuration loaded
[2024-06-27 10:00:07] INFO  Test Suite A: 42 passed, 0 failed
[2024-06-27 10:00:08] INFO  Test Suite B: 18 passed, 0 failed
[2024-06-27 10:00:09] INFO  Test Suite C: 7 passed, 0 failed
[2024-06-27 10:00:10] INFO  Build completed successfully in 9.2s`
	demo("构建日志", log, OutputLog)

	// Test with dedup
	logDuped := `[INFO] scanning directory...
[INFO] scanning directory...
[INFO] scanning directory...
[INFO] scanning directory...
[INFO] scanning directory...
[INFO] scanning complete
[INFO] found 3 files
[INFO] found 3 files
[INFO] found 3 files
[INFO] processing complete`
	demo("重复日志(去重)", logDuped, OutputLog)

	// Test JSON output
	json := `{
	"status": "ok",
	"data": {
		"users": [
			{"id": 1, "name": "Alice", "email": "alice@example.com", "role": "admin"},
			{"id": 2, "name": "Bob", "email": "bob@example.com", "role": "user"},
			{"id": 3, "name": "Charlie", "email": "charlie@example.com", "role": "user"}
		],
		"total": 3,
		"page": 1,
		"per_page": 100
	},
	"meta": {
		"request_id": "abc-123-def-456",
		"timestamp": "2024-06-27T10:00:00Z",
		"version": "2.1.0"
	}
}`
	demo("JSON 数据", json, OutputJSON)

	// Test text with line limit (simulating long output)
	longText := ""
	for i := 1; i <= 50; i++ {
		longText += fmt.Sprintf("line %d: This is a sample line of output that might appear in a tool result. It has some useful information but there are many lines like it.\n", i)
	}
	// Use a stricter profile to force truncation
	strictProfile := profiles[OutputText]
	strictProfile.MaxLines = 20
	profiles[OutputText] = strictProfile
	demo("长文本(50行→压缩)", longText, OutputText)
	// Reset
	profiles[OutputText] = DefaultProfiles()[OutputText]
}

func TestCanaryGate(t *testing.T) {
	// Demonstrate the full learning cycle
	pl := NewProfileLearner()
	cg := NewCanaryGate(pl)

	fmt.Println("\n=== Canary Gate Demo ===")
	fmt.Println("Phase 1: Recording user corrections...")
	corrections := []string{"too_verbose", "too_verbose", "too_verbose", "missing_info", "too_verbose"}
	for _, c := range corrections {
		pl.RecordCorrection("webfetch", OutputHTML, true, c)
	}
	fmt.Printf("  Recorded %d corrections for HTML output\n", len(corrections))

	// Phase 2: Get adjustments and propose canary
	fmt.Println("\nPhase 2: Proposing canary trial...")
	adjustments := pl.SuggestAdjustments()
	for _, a := range adjustments {
		trialID := cg.Propose(a.Type, a.Field, a.OldValue, a.NewValue, a.Reason)
		fmt.Printf("  [CANARY] %s → trial=%s\n", trialID, a.Reason)
	}

	// Phase 3: Simulate feedback cycle
	fmt.Println("\nPhase 3: Simulating feedback...")
	feedbacks := []struct {
		correction string
		expected   string
	}{
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"},
		{"too_verbose", "monitoring"}, // 10th → promote
	}
	for _, fb := range feedbacks {
		result := cg.RecordFeedback(OutputHTML, fb.correction)
		fmt.Printf("  feedback=%s → status=%s\n", fb.correction, result)
	}

	// Phase 4: Check final state
	fmt.Println("\nPhase 4: Trial results...")
	for _, t := range cg.TrialHistory() {
		fmt.Printf("  trial=%s status=%s positive=%d negative=%d\n",
			t.ID, t.Status, t.PositiveCount, t.NegativeCount)
	}

	// Phase 5: Test rollback scenario
	fmt.Println("\nPhase 5: Testing rollback...")
	pl2 := NewProfileLearner()
	cg2 := NewCanaryGate(pl2)
	// Record corrections suggesting change
	for i := 0; i < 5; i++ {
		pl2.RecordCorrection("bash", OutputLog, true, "too_verbose")
	}
	adj := pl2.SuggestAdjustments()
	for _, a := range adj {
		cg2.Propose(a.Type, a.Field, a.OldValue, a.NewValue, a.Reason)
		fmt.Printf("  [CANARY] %s proposed\n", a.Reason)
	}
	// Now send mostly negative feedback (should trigger rollback)
	negativeFeedbacks := []string{
		"missing_info", "missing_info", "missing_info",
		"missing_info", "too_verbose", "missing_info",
		"missing_info", "missing_info", "missing_info", "too_verbose",
	}
	for i, fb := range negativeFeedbacks {
		result := cg2.RecordFeedback(OutputLog, fb)
		if result == "rolled_back" {
			fmt.Printf("  [%d] feedback=%s → ROLLED BACK at step %d\n", i+1, fb, i+1)
			break
		}
	}
	for _, t := range cg2.TrialHistory() {
		fmt.Printf("  trial=%s status=%s positive=%d negative=%d\n",
			t.ID, t.Status, t.PositiveCount, t.NegativeCount)
	}
}
