// Package memorycompiler — ArkDistill-style tool output compression.
// Adds deterministic, profile-driven compaction of tool outputs before
// they enter the LLM context, plus self-learning from user corrections.
package memorycompiler

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// --- ArkDistill-style compression profiles ---

// OutputType classifies tool output for profile-based compression.
type OutputType string

const (
	OutputHTML    OutputType = "html"
	OutputLog     OutputType = "log"
	OutputTrace   OutputType = "trace"
	OutputJSON    OutputType = "json"
	OutputText    OutputType = "text"
	OutputTable   OutputType = "table"
	OutputUnknown OutputType = "unknown"
)

// CompactionProfile defines how aggressively to compact a given output type.
type CompactionProfile struct {
	Type           OutputType `json:"type"`
	MaxLines       int        `json:"max_lines"`               // 0 = no limit
	MaxChars       int        `json:"max_chars"`               // 0 = no limit
	Deduplicate    bool       `json:"deduplicate"`             // remove consecutive duplicate lines
	StripTags      bool       `json:"strip_tags"`              // for HTML: remove tags
	KeepFields     []string   `json:"keep_fields,omitempty"`   // for JSON: only keep these fields
	DropPatterns   []string   `json:"drop_patterns,omitempty"` // lines matching any pattern are dropped
	CompressionPct float64    `json:"compression_pct"`         // target compression ratio (0.0-1.0)
}

// DefaultProfiles returns the default compaction profiles.
func DefaultProfiles() map[OutputType]CompactionProfile {
	return map[OutputType]CompactionProfile{
		OutputHTML: {
			Type: OutputHTML, MaxLines: 200, MaxChars: 8000,
			StripTags: true, CompressionPct: 0.75,
		},
		OutputLog: {
			Type: OutputLog, MaxLines: 300, MaxChars: 12000,
			Deduplicate: true, CompressionPct: 0.70,
		},
		OutputTrace: {
			Type: OutputTrace, MaxLines: 100, MaxChars: 6000,
			Deduplicate: true, CompressionPct: 0.65,
		},
		OutputJSON: {
			Type: OutputJSON, MaxChars: 15000,
			CompressionPct: 0.30, // JSON is already compact
		},
		OutputText: {
			Type: OutputText, MaxLines: 500, MaxChars: 20000,
			CompressionPct: 0.40,
		},
		OutputTable: {
			Type: OutputTable, MaxLines: 200, MaxChars: 10000,
			CompressionPct: 0.50,
		},
		OutputUnknown: {
			Type: OutputUnknown, MaxLines: 400, MaxChars: 16000,
			CompressionPct: 0.30,
		},
	}
}

// DetectOutputType heuristically classifies tool output.
func DetectOutputType(output string) OutputType {
	output = strings.TrimSpace(output)
	if len(output) == 0 {
		return OutputUnknown
	}

	// HTML detection
	if strings.HasPrefix(output, "<!DOCTYPE") || strings.HasPrefix(output, "<html") ||
		strings.HasPrefix(output, "<div") || strings.HasPrefix(output, "<table") {
		return OutputHTML
	}

	// JSON detection
	first := output[0]
	if first == '{' || first == '[' {
		var js json.RawMessage
		if json.Unmarshal([]byte(output), &js) == nil {
			return OutputJSON
		}
	}

	// Log detection (timestamp prefix patterns)
	lines := strings.Split(output, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		if strings.HasPrefix(firstLine, "[") && (strings.Contains(firstLine, "]") || len(firstLine) > 15) {
			return OutputLog
		}
		// Detect trace (stack traces, file:line patterns)
		for _, line := range lines[:min(5, len(lines))] {
			if strings.Contains(line, ".go:") || (strings.Contains(line, " at ") && (strings.Contains(line, ":line") || strings.Contains(line, ".go:"))) {
				return OutputTrace
			}
		}
	}

	// Table detection (pipe/plus separated)
	if len(lines) > 2 {
		hasPipes := false
		hasPluses := false
		for _, line := range lines[:min(5, len(lines))] {
			if strings.Contains(line, "|") {
				hasPipes = true
			}
			if strings.Contains(line, "+-") || strings.Contains(line, "-+-") {
				hasPluses = true
			}
		}
		if hasPipes && hasPluses {
			return OutputTable
		}
	}

	return OutputText
}

// CompactToolOutput applies the profile to compress a tool's output string.
func CompactToolOutput(output string, outType OutputType, profile *CompactionProfile) (string, bool) {
	if output == "" || profile == nil {
		return output, false
	}

	lines := strings.Split(output, "\n")
	wasTruncated := false

	// Step 1: Strip HTML tags
	if profile.StripTags && outType == OutputHTML {
		output = stripHTMLTags(output)
		lines = strings.Split(output, "\n")
	}

	// Step 2: Deduplicate consecutive identical lines
	if profile.Deduplicate {
		var deduped []string
		var prev string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != prev {
				deduped = append(deduped, line)
			}
			prev = trimmed
		}
		lines = deduped
	}

	// Step 3: Drop lines matching patterns
	if len(profile.DropPatterns) > 0 {
		var filtered []string
		for _, line := range lines {
			drop := false
			for _, pat := range profile.DropPatterns {
				if strings.Contains(line, pat) {
					drop = true
					break
				}
			}
			if !drop {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}

	// Step 4: Apply line limit
	if profile.MaxLines > 0 && len(lines) > profile.MaxLines {
		half := profile.MaxLines / 2
		truncated := make([]string, 0, profile.MaxLines+1)
		truncated = append(truncated, lines[:half]...)
		truncated = append(truncated, "... ["+itoa(len(lines)-profile.MaxLines)+" lines suppressed] ...")
		truncated = append(truncated, lines[len(lines)-half:]...)
		lines = truncated
		wasTruncated = true
	}

	result := strings.Join(lines, "\n")

	// Step 5: Apply char limit
	if profile.MaxChars > 0 && len(result) > profile.MaxChars {
		result = result[:profile.MaxChars] + "\n... [truncated at " + itoa(profile.MaxChars) + " chars] ..."
		wasTruncated = true
	}

	return result, wasTruncated
}

// --- Self-learning: correction-driven profile tuning ---

// CorrectionEvent records when a user corrects a tool output.
type CorrectionEvent struct {
	ToolName   string     `json:"tool_name"`
	OutputType OutputType `json:"output_type"`
	WasCompact bool       `json:"was_compact"`
	Correction string     `json:"correction"` // "too_verbose","missing_info","wrong_format"
	Timestamp  time.Time  `json:"timestamp"`
}

// ProfileLearner tracks correction patterns and suggests profile adjustments.
type ProfileLearner struct {
	mu            sync.Mutex
	corrections   []CorrectionEvent
	profiles      map[OutputType]CompactionProfile
	adjustmentLog []ProfileAdjustment
}

// ProfileAdjustment records a learned profile change.
type ProfileAdjustment struct {
	Type      OutputType  `json:"type"`
	Field     string      `json:"field"`
	OldValue  interface{} `json:"old_value"`
	NewValue  interface{} `json:"new_value"`
	Reason    string      `json:"reason"`
	AppliedAt time.Time   `json:"applied_at"`
}

// NewProfileLearner creates a learner with default profiles.
func NewProfileLearner() *ProfileLearner {
	return &ProfileLearner{
		profiles: DefaultProfiles(),
	}
}

// RecordCorrection logs a user correction for later analysis.
func (pl *ProfileLearner) RecordCorrection(toolName string, outType OutputType, wasCompact bool, correction string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.corrections = append(pl.corrections, CorrectionEvent{
		ToolName:   toolName,
		OutputType: outType,
		WasCompact: wasCompact,
		Correction: correction,
		Timestamp:  time.Now(),
	})
}

// SuggestAdjustments analyzes correction history and returns suggested profile tweaks.
func (pl *ProfileLearner) SuggestAdjustments() []ProfileAdjustment {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	if len(pl.corrections) < 3 {
		return nil // not enough data
	}

	var suggestions []ProfileAdjustment

	// Group corrections by output type
	typeCorrections := map[OutputType]struct {
		verbose int
		missing int
		total   int
	}{}
	for _, c := range pl.corrections {
		entry := typeCorrections[c.OutputType]
		entry.total++
		switch c.Correction {
		case "too_verbose":
			entry.verbose++
		case "missing_info":
			entry.missing++
		}
		typeCorrections[c.OutputType] = entry
	}

	for outType, stats := range typeCorrections {
		profile, ok := pl.profiles[outType]
		if !ok {
			continue
		}

		// If >40% corrections say "too verbose", tighten compression
		if stats.total >= 3 && float64(stats.verbose)/float64(stats.total) > 0.4 {
			oldLines := profile.MaxLines
			newLines := int(math.Max(50, float64(oldLines)*0.7))
			if newLines < oldLines {
				suggestions = append(suggestions, ProfileAdjustment{
					Type: outType, Field: "MaxLines",
					OldValue: oldLines, NewValue: newLines,
					Reason:    "users frequently found this output type too verbose",
					AppliedAt: time.Now(),
				})
				profile.MaxLines = newLines
			}
		}

		// If >30% corrections say "missing info", loosen compression
		if stats.total >= 3 && float64(stats.missing)/float64(stats.total) > 0.3 {
			oldLines := profile.MaxLines
			newLines := int(math.Min(1000, float64(oldLines)*1.5))
			if newLines > oldLines {
				suggestions = append(suggestions, ProfileAdjustment{
					Type: outType, Field: "MaxLines",
					OldValue: oldLines, NewValue: newLines,
					Reason:    "users frequently found this output type missing information",
					AppliedAt: time.Now(),
				})
				profile.MaxLines = newLines
			}
		}

		pl.profiles[outType] = profile
	}

	// Trim old corrections (keep last 100)
	if len(pl.corrections) > 100 {
		pl.corrections = pl.corrections[len(pl.corrections)-100:]
	}

	pl.adjustmentLog = append(pl.adjustmentLog, suggestions...)
	return suggestions
}

// GetProfile returns the current profile for an output type.
func (pl *ProfileLearner) GetProfile(outType OutputType) *CompactionProfile {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if p, ok := pl.profiles[outType]; ok {
		cp := p // copy
		return &cp
	}
	return nil
}

// SetProfile updates a profile.
func (pl *ProfileLearner) SetProfile(outType OutputType, profile CompactionProfile) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.profiles[outType] = profile
}

// AdjustmentLog returns the history of profile adjustments.
func (pl *ProfileLearner) AdjustmentLog() []ProfileAdjustment {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	out := make([]ProfileAdjustment, len(pl.adjustmentLog))
	copy(out, pl.adjustmentLog)
	return out
}

// --- Canary Gate: safe staged rollout of profile changes ---

// CanaryStatus represents the current state of a canary trial.
type CanaryStatus string

const (
	CanaryPending    CanaryStatus = "pending"     // proposed, awaiting trial
	CanaryActive     CanaryStatus = "active"      // limited rollout in progress
	CanaryPromoted   CanaryStatus = "promoted"    // rolled out fully
	CanaryRolledBack CanaryStatus = "rolled_back" // reverted due to negative feedback
)

// CanaryTrial tracks one proposed profile change through its lifecycle.
type CanaryTrial struct {
	ID            string       `json:"id"`
	Type          OutputType   `json:"type"`
	Field         string       `json:"field"`
	OldValue      interface{}  `json:"old_value"`
	NewValue      interface{}  `json:"new_value"`
	Reason        string       `json:"reason"`
	Status        CanaryStatus `json:"status"`
	CreatedAt     time.Time    `json:"created_at"`
	PromotedAt    *time.Time   `json:"promoted_at,omitempty"`
	EvaluationWin int          `json:"evaluation_window"` // how many corrections to evaluate
	PositiveCount int          `json:"positive_count"`    // "too_verbose" resolved
	NegativeCount int          `json:"negative_count"`    // "missing_info" new complaints
}

// CanaryGate manages staged rollouts of profile adjustments.
type CanaryGate struct {
	mu         sync.Mutex
	trials     []*CanaryTrial
	learner    *ProfileLearner
	windowSize int // default evaluation window in corrections
}

// NewCanaryGate creates a gate wired to a profile learner.
func NewCanaryGate(learner *ProfileLearner) *CanaryGate {
	return &CanaryGate{
		learner:    learner,
		windowSize: 10, // evaluate after 10 corrections
	}
}

// Propose creates a canary trial and starts the staged rollout.
// Returns the trial ID for later status checks.
func (cg *CanaryGate) Propose(outType OutputType, field string, oldVal, newVal interface{}, reason string) string {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	id := fmt.Sprintf("canary-%s-%s-%d", outType, field, len(cg.trials))
	trial := &CanaryTrial{
		ID:            id,
		Type:          outType,
		Field:         field,
		OldValue:      oldVal,
		NewValue:      newVal,
		Reason:        reason,
		Status:        CanaryActive,
		CreatedAt:     time.Now(),
		EvaluationWin: cg.windowSize,
	}

	// Apply the change immediately (canary = limited rollout by sample rate,
	// but for simplicity we apply and monitor; rollback reverts).
	cg.applyTrial(trial)
	cg.trials = append(cg.trials, trial)
	return id
}

// RecordFeedback feeds a correction into the canary evaluation.
// If the correction matches an active canary's type/field, it counts toward promotion or rollback.
func (cg *CanaryGate) RecordFeedback(outType OutputType, correction string) string {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	for _, trial := range cg.trials {
		if trial.Status != CanaryActive {
			continue
		}
		if trial.Type != outType {
			continue
		}

		// "missing_info" complaints during canary → negative signal
		if correction == "missing_info" {
			trial.NegativeCount++
		}
		// "too_verbose" still appearing → positive that compression is working,
		// but if it goes up → might be too aggressive
		if correction == "too_verbose" {
			trial.PositiveCount++
		}

		// Check if evaluation window is complete
		total := trial.PositiveCount + trial.NegativeCount
		if total >= trial.EvaluationWin {
			negativeRatio := float64(trial.NegativeCount) / float64(total)
			if negativeRatio > 0.3 {
				// Too much negative feedback → rollback
				trial.Status = CanaryRolledBack
				cg.rollbackTrial(trial)
				return "rolled_back"
			}
			// Acceptable → promote
			now := time.Now()
			trial.Status = CanaryPromoted
			trial.PromotedAt = &now
			return "promoted"
		}
	}
	return "monitoring"
}

// ActiveTrials returns all canary trials currently in active status.
func (cg *CanaryGate) ActiveTrials() []*CanaryTrial {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	var out []*CanaryTrial
	for _, t := range cg.trials {
		if t.Status == CanaryActive {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out
}

// TrialHistory returns all completed trials.
func (cg *CanaryGate) TrialHistory() []*CanaryTrial {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	out := make([]*CanaryTrial, len(cg.trials))
	for i, t := range cg.trials {
		cp := *t
		out[i] = &cp
	}
	return out
}

func (cg *CanaryGate) applyTrial(trial *CanaryTrial) {
	profile := cg.learner.GetProfile(trial.Type)
	if profile == nil {
		return
	}
	switch trial.Field {
	case "MaxLines":
		if v, ok := trial.NewValue.(int); ok {
			profile.MaxLines = v
		}
	case "MaxChars":
		if v, ok := trial.NewValue.(int); ok {
			profile.MaxChars = v
		}
	}
	cg.learner.SetProfile(trial.Type, *profile)
}

func (cg *CanaryGate) rollbackTrial(trial *CanaryTrial) {
	profile := cg.learner.GetProfile(trial.Type)
	if profile == nil {
		return
	}
	switch trial.Field {
	case "MaxLines":
		if v, ok := trial.OldValue.(int); ok {
			profile.MaxLines = v
		}
	case "MaxChars":
		if v, ok := trial.OldValue.(int); ok {
			profile.MaxChars = v
		}
	}
	cg.learner.SetProfile(trial.Type, *profile)
}

// --- helpers ---

func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			b.WriteRune(' ')
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	sort.Slice(digits, func(i, j int) bool { return i > j })
	return string(digits)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
