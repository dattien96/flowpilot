package promptpacker

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// sectionBody builds a section content of exactly totalChars bytes whose
// distinctive marker survives assertions (pad after the marker).
func sectionBody(marker string, totalChars int) string {
	if len(marker) > totalChars {
		marker = marker[:totalChars]
	}
	return marker + strings.Repeat("x", totalChars-len(marker))
}

// Scenario: Đóng gói các section nằm trong ngân sách cho phép
// Input: 3 section với tổng 2,000 tokens, ngân sách 4,000 tokens
// Expect: Toàn bộ 3 section được giữ nguyên vẹn, 0 item bị drop
func TestBudgetPacker_WithinBudget_RetainsAll(t *testing.T) {
	sections := []PromptSection{
		{Kind: SectionCurrentTask, Title: "Task", Content: sectionBody("TASK-SELECTED", 4000), Priority: 2},       // 1000 tokens
		{Kind: SectionMandatoryDoc, Title: "Contract", Content: sectionBody("DOC-SELECTED", 2000), Priority: 3},   // 500 tokens
		{Kind: SectionMemorySummary, Title: "Memory", Content: sectionBody("MEMORY-SELECTED", 2000), Priority: 4}, // 500 tokens
	}
	budget := SectionBudget{TotalMaxTokens: 4000, MaxExcerptTokens: 1600, MaxMemoryTokens: 2400, MaxSkillTokens: 800}

	packed, report, err := PackPrompt(sections, budget)
	if err != nil {
		t.Fatalf("PackPrompt returned error: %v", err)
	}
	if report.SelectedTokens != 2000 {
		t.Fatalf("SelectedTokens = %d, want 2000", report.SelectedTokens)
	}
	if report.DroppedTokens != 0 {
		t.Fatalf("DroppedTokens = %d, want 0", report.DroppedTokens)
	}
	if len(report.DroppedItems) != 0 {
		t.Fatalf("DroppedItems = %v, want none", report.DroppedItems)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", report.Warnings)
	}
	for _, marker := range []string{"TASK-SELECTED", "DOC-SELECTED", "MEMORY-SELECTED"} {
		if !strings.Contains(packed, marker) {
			t.Fatalf("packed prompt must retain section content %q", marker)
		}
	}
	// Sections are assembled in priority order (1 highest first).
	taskIdx := strings.Index(packed, "TASK-SELECTED")
	docIdx := strings.Index(packed, "DOC-SELECTED")
	memIdx := strings.Index(packed, "MEMORY-SELECTED")
	if !(taskIdx < docIdx && docIdx < memIdx) {
		t.Fatalf("packed sections must be in priority order: task=%d doc=%d memory=%d", taskIdx, docIdx, memIdx)
	}
	if report.SectionUsage["current_task"] != 1000 || report.SectionUsage["memory_summary"] != 500 {
		t.Fatalf("SectionUsage = %v, want current_task=1000 memory_summary=500", report.SectionUsage)
	}
}

// Scenario: Ngữ cảnh vượt quá ngân sách -> Cắt tỉa raw excerpts trước, bảo vệ core task
// Input: Raw excerpt chiếm 5,000 tokens, ngân sách cho phép 3,000 tokens
// Expect: Core task được bảo toàn, raw excerpt bị cắt gọn hoặc drop, tổng token <= 3,000
func TestBudgetPacker_OverBudget_PrunesLowestPriority(t *testing.T) {
	sections := []PromptSection{
		{Kind: SectionCurrentTask, Title: "Task", Content: sectionBody("CORE-TASK-KEPT", 2000), Priority: 2},         // 500 tokens
		{Kind: SectionRawExcerpt, Title: "Excerpt A", Content: sectionBody("EXCERPT-A-KEPT", 10000), Priority: 5},    // 2500 tokens
		{Kind: SectionRawExcerpt, Title: "Excerpt B", Content: sectionBody("EXCERPT-B-DROPPED", 12000), Priority: 5}, // 3000 tokens
	}
	// Per-kind caps left at 0 (uncapped) so the global budget drives the prune.
	budget := SectionBudget{TotalMaxTokens: 3000}

	packed, report, err := PackPrompt(sections, budget)
	if err != nil {
		t.Fatalf("PackPrompt returned error: %v", err)
	}
	if report.SelectedTokens > 3000 {
		t.Fatalf("SelectedTokens = %d, must be <= 3000", report.SelectedTokens)
	}
	if report.SelectedTokens != 3000 {
		t.Fatalf("SelectedTokens = %d, want 3000 (task 500 + excerpt A 2500)", report.SelectedTokens)
	}
	if !strings.Contains(packed, "CORE-TASK-KEPT") {
		t.Fatalf("core task must be protected from pruning")
	}
	if !strings.Contains(packed, "EXCERPT-A-KEPT") {
		t.Fatalf("smaller same-priority excerpt should be retained once the budget is met")
	}
	if strings.Contains(packed, "EXCERPT-B-DROPPED") {
		t.Fatalf("lowest-priority oversized excerpt must be dropped")
	}
	if report.DroppedTokens != 3000 {
		t.Fatalf("DroppedTokens = %d, want 3000", report.DroppedTokens)
	}
	if len(report.DroppedItems) != 1 || !strings.Contains(report.DroppedItems[0], "exceeded_section_budget") {
		t.Fatalf("DroppedItems = %v, want one entry with reason exceeded_section_budget", report.DroppedItems)
	}
	if report.SectionUsage["raw_excerpt"] != 2500 {
		t.Fatalf("SectionUsage[raw_excerpt] = %d, want 2500", report.SectionUsage["raw_excerpt"])
	}
}

// Scenario: Lọc trùng lặp giữa chat history và memory summary
// Input: Đoạn văn bản X xuất hiện ở cả Chat History và Working Memory
// Expect: Đoạn văn bản X chỉ xuất hiện một lần trong prompt ghép cuối cùng
func TestContextDeduplicator_RemovesDuplicateText(t *testing.T) {
	const duplicatedBlock = "func add(a int, b int) int {\n\treturn a + b\n}"
	sections := []PromptSection{
		{Kind: SectionMemorySummary, Title: "Chat History", Content: "Earlier discussion of the add helper.\n\n" + duplicatedBlock, Priority: 4},
		{Kind: SectionMemorySummary, Title: "Working Memory", Content: duplicatedBlock + "\n\nWorking set for the add helper.", Priority: 4},
	}

	deduped := DeduplicateContext(sections)
	if len(deduped) != 2 {
		t.Fatalf("DeduplicateContext returned %d sections, want 2", len(deduped))
	}
	if !strings.Contains(deduped[0].Content, duplicatedBlock) {
		t.Fatalf("higher-priority (first) copy must keep the duplicated text")
	}
	if strings.Contains(deduped[1].Content, duplicatedBlock) {
		t.Fatalf("lower-priority copy must have the duplicated text removed")
	}
	if !strings.Contains(deduped[1].Content, "Working set for the add helper.") {
		t.Fatalf("non-duplicated content of the second section must survive")
	}

	// PackPrompt deduplicates internally, so the original sections are packed.
	packed, report, err := PackPrompt(sections, SectionBudget{TotalMaxTokens: 4000, MaxMemoryTokens: 2400})
	if err != nil {
		t.Fatalf("PackPrompt returned error: %v", err)
	}
	if got := strings.Count(packed, duplicatedBlock); got != 1 {
		t.Fatalf("duplicated text must appear exactly once in the packed prompt, got %d times", got)
	}
	if report.SelectedTokens <= 0 || report.DroppedTokens <= 0 {
		t.Fatalf("audit must record dedup drop: selected=%d dropped=%d", report.SelectedTokens, report.DroppedTokens)
	}
}

// Scenario: Sinh audit record đầy đủ sau khi đóng gói
// Input: Quá trình đóng gói có 1 item bị drop do hết ngân sách
// Expect: Audit log ghi nhận DroppedCount=1 kèm lý do "exceeded_section_budget"
func TestPromptContextAudit_RecordsDroppedItems(t *testing.T) {
	sections := []PromptSection{
		{Kind: SectionCurrentTask, Title: "Task", Content: sectionBody("AUDIT-TASK", 2000), Priority: 2},            // 500 tokens
		{Kind: SectionRawExcerpt, Title: "Audit Excerpt", Content: sectionBody("AUDIT-EXCERPT", 4000), Priority: 5}, // 1000 tokens
	}
	budget := SectionBudget{TotalMaxTokens: 1200}

	_, report, err := PackPrompt(sections, budget)
	if err != nil {
		t.Fatalf("PackPrompt returned error: %v", err)
	}
	if len(report.DroppedItems) != 1 {
		t.Fatalf("DroppedItems = %v, want exactly 1", report.DroppedItems)
	}
	if !strings.Contains(report.DroppedItems[0], "exceeded_section_budget") {
		t.Fatalf("dropped item must carry reason exceeded_section_budget, got %q", report.DroppedItems[0])
	}
	if report.DroppedTokens != 1000 || report.SelectedTokens != 500 {
		t.Fatalf("audit tokens: selected=%d dropped=%d, want 500/1000", report.SelectedTokens, report.DroppedTokens)
	}

	var buf bytes.Buffer
	if err := WriteAuditLog(&buf, report); err != nil {
		t.Fatalf("WriteAuditLog returned error: %v", err)
	}
	var record struct {
		SelectedTokens int            `json:"selected_tokens"`
		DroppedTokens  int            `json:"dropped_tokens"`
		DroppedItems   []string       `json:"dropped_items"`
		SectionUsage   map[string]int `json:"section_usage"`
	}
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("audit log must be valid JSON: %v\n%s", err, buf.String())
	}
	if record.SelectedTokens != 500 || record.DroppedTokens != 1000 {
		t.Fatalf("JSON audit tokens: selected=%d dropped=%d, want 500/1000", record.SelectedTokens, record.DroppedTokens)
	}
	if len(record.DroppedItems) != 1 || !strings.Contains(record.DroppedItems[0], "exceeded_section_budget") {
		t.Fatalf("JSON dropped_items = %v, want one reason exceeded_section_budget", record.DroppedItems)
	}
	if record.SectionUsage["current_task"] != 500 || record.SectionUsage["raw_excerpt"] != 0 {
		t.Fatalf("JSON section_usage = %v, want current_task=500 and no raw_excerpt", record.SectionUsage)
	}
}

// [Edge] Scenario: Danh sách section rỗng
// Input: sections=[], budget mặc định
// Expect: Prompt rỗng, audit log ghi 0 selected 0 dropped
func TestBudgetPacker_EmptySections_ReturnsEmpty(t *testing.T) {
	packed, report, err := PackPrompt(nil, SectionBudget{TotalMaxTokens: 4000, MaxExcerptTokens: 1600, MaxMemoryTokens: 2400, MaxSkillTokens: 800})
	if err != nil {
		t.Fatalf("PackPrompt returned error: %v", err)
	}
	if packed != "" {
		t.Fatalf("packed prompt = %q, want empty", packed)
	}
	if report.SelectedTokens != 0 || report.DroppedTokens != 0 {
		t.Fatalf("audit must record 0 selected / 0 dropped, got selected=%d dropped=%d", report.SelectedTokens, report.DroppedTokens)
	}
	if len(report.DroppedItems) != 0 {
		t.Fatalf("DroppedItems = %v, want none", report.DroppedItems)
	}

	var buf bytes.Buffer
	if err := WriteAuditLog(&buf, report); err != nil {
		t.Fatalf("WriteAuditLog returned error: %v", err)
	}
	var record struct {
		SelectedTokens int `json:"selected_tokens"`
		DroppedTokens  int `json:"dropped_tokens"`
	}
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("audit log must be valid JSON: %v\n%s", err, buf.String())
	}
	if record.SelectedTokens != 0 || record.DroppedTokens != 0 {
		t.Fatalf("empty-pack audit must be 0/0, got %+v", record)
	}
}

// [Edge] Scenario: Một section bắt buộc đơn lẻ vượt quá toàn bộ ngân sách
// Input: 1 section SectionSystemContract chiếm 10,000 tokens, ngân sách 4,000 tokens
// Expect: Section bắt buộc được giữ nguyên (không cắt), audit ghi cảnh báo vượt ngân sách
func TestBudgetPacker_MandatorySectionExceedsBudget_Retained(t *testing.T) {
	sections := []PromptSection{
		{Kind: SectionSystemContract, Title: "System Contract", Content: sectionBody("SYSTEM-CONTRACT-RETAINED", 40000), Priority: 1}, // 10000 tokens
	}
	budget := SectionBudget{TotalMaxTokens: 4000}

	packed, report, err := PackPrompt(sections, budget)
	if err != nil {
		t.Fatalf("PackPrompt returned error: %v", err)
	}
	if !strings.Contains(packed, "SYSTEM-CONTRACT-RETAINED") {
		t.Fatalf("mandatory section must be retained whole (not cut)")
	}
	if report.SelectedTokens != 10000 {
		t.Fatalf("SelectedTokens = %d, want 10000 (kept whole)", report.SelectedTokens)
	}
	if report.DroppedTokens != 0 || len(report.DroppedItems) != 0 {
		t.Fatalf("nothing may be dropped for a lone mandatory section: dropped=%d items=%v", report.DroppedTokens, report.DroppedItems)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "exceed") {
		t.Fatalf("audit must record the over-budget warning, got %v", report.Warnings)
	}
	if report.SectionUsage["system_contract"] != 10000 {
		t.Fatalf("SectionUsage[system_contract] = %d, want 10000", report.SectionUsage["system_contract"])
	}
}

// [Error] Scenario: Ngân sách âm hoặc bằng 0
// Input: budget.TotalMaxTokens = 0
// Expect: Trả về error rõ ràng "invalid budget: total must be positive"
func TestBudgetPacker_ZeroBudget_ReturnsError(t *testing.T) {
	sections := []PromptSection{
		{Kind: SectionCurrentTask, Title: "Task", Content: "hello task", Priority: 2},
	}
	for _, total := range []int{0, -5} {
		packed, report, err := PackPrompt(sections, SectionBudget{TotalMaxTokens: total})
		if err == nil {
			t.Fatalf("TotalMaxTokens=%d: expected an error", total)
		}
		if err.Error() != "invalid budget: total must be positive" {
			t.Fatalf("TotalMaxTokens=%d: error = %q, want %q", total, err.Error(), "invalid budget: total must be positive")
		}
		if packed != "" {
			t.Fatalf("TotalMaxTokens=%d: packed prompt = %q, want empty", total, packed)
		}
		if report.SelectedTokens != 0 || report.DroppedTokens != 0 || len(report.DroppedItems) != 0 {
			t.Fatalf("TotalMaxTokens=%d: report must be zero-valued, got %+v", total, report)
		}
	}
}

// [Edge] Scenario: Chuyển đổi Full Skill thành Compact Skill Card
// Input: SKILL.md đầy đủ ~3000 tokens
// Expect: Compact card chỉ còn 3-5 dòng Core Rule, token < 200
func TestCompactSkillCard_ConvertsFromFullSkill(t *testing.T) {
	alwaysDo := "- MUST run impact analysis before editing any symbol\n" +
		"- NEVER edit a pre-existing test file\n" +
		"- MUST keep the provider adapter parity green\n" +
		"- MUST record every dropped context item in the audit log\n" +
		"- ALWAYS report deviations at the end of the task\n"
	fullSkill := "# Big Skill\n\n" +
		strings.Repeat("Long explanation prose that inflates the skill file well past three thousand tokens. ", 210) + "\n\n" +
		"## Always Do\n\n" + alwaysDo +
		"\n## Never Do\n\n- Never commit secrets\n"
	if EstimateTokens(fullSkill) < 2500 {
		t.Fatalf("test premise: full skill must be ~3000 tokens, got %d", EstimateTokens(fullSkill))
	}

	card := CompactSkillCard(fullSkill)
	if card == "" {
		t.Fatalf("compact card must not be empty")
	}
	if EstimateTokens(card) >= 200 {
		t.Fatalf("compact card must stay under 200 tokens, got %d (len=%d)", EstimateTokens(card), len(card))
	}
	if len(card) > maxCompactSkillChars {
		t.Fatalf("compact card must be capped at %d chars, got %d", maxCompactSkillChars, len(card))
	}
	for _, rule := range strings.Split(strings.TrimRight(alwaysDo, "\n"), "\n") {
		if !strings.Contains(card, rule) {
			t.Fatalf("compact card must contain core rule %q, got:\n%s", rule, card)
		}
	}
	if strings.Contains(card, "Never commit secrets") {
		t.Fatalf("compact card must not pull rules from unrelated sections")
	}

	// "## Core Rules" heading is accepted as an alternative to "## Always Do".
	coreRulesSkill := "# Other Skill\n\n## Core Rules\n\n- MUST pack prompts under the token budget\n- NEVER drop the current task\n\n## Notes\n\nbody"
	card2 := CompactSkillCard(coreRulesSkill)
	if !strings.Contains(card2, "MUST pack prompts under the token budget") ||
		!strings.Contains(card2, "NEVER drop the current task") {
		t.Fatalf("## Core Rules heading must be accepted, got:\n%s", card2)
	}

	// Fallback: no Core Rules heading -> first 3-5 bullet lines of the document.
	fallbackSkill := "# Plain Skill\n\nSome intro prose.\n\n- first rule\n- second rule\n- third rule\n- fourth rule\n- fifth rule\n- sixth rule\n- seventh rule\n"
	card3 := CompactSkillCard(fallbackSkill)
	lines := strings.Split(card3, "\n")
	if len(lines) != maxCompactSkillLines {
		t.Fatalf("fallback card must keep the first %d bullets, got %d lines:\n%s", maxCompactSkillLines, len(lines), card3)
	}
	if !strings.Contains(card3, "first rule") || strings.Contains(card3, "seventh rule") {
		t.Fatalf("fallback card must keep only the first bullets, got:\n%s", card3)
	}
	if EstimateTokens(card3) >= 200 {
		t.Fatalf("fallback card must stay under 200 tokens, got %d", EstimateTokens(card3))
	}
}

// Scenario: Default packer options expose the CP-23 Phase 1 slice budget
// Input: DefaultPackerOptions()
// Expect: positive total (8000) with per-kind caps at the documented slice
// percentages — memory 30%, excerpts 20%, skills 10% (Task-334 T-1 / DOD).
func TestDefaultPackerOptions_CPSliceBudget(t *testing.T) {
	opts := DefaultPackerOptions()
	b := opts.Budget
	if b.TotalMaxTokens != 8000 {
		t.Fatalf("total = %d, want 8000", b.TotalMaxTokens)
	}
	if b.MaxMemoryTokens != 2400 || b.MaxExcerptTokens != 1600 || b.MaxSkillTokens != 800 {
		t.Fatalf("per-kind caps = memory %d / excerpt %d / skill %d, want 2400/1600/800",
			b.MaxMemoryTokens, b.MaxExcerptTokens, b.MaxSkillTokens)
	}
	if _, _, err := PackPrompt(nil, b); err != nil {
		t.Fatalf("default budget must be valid, got error: %v", err)
	}
}

// Review round-3 hardening (discriminating input chosen per review: the
// previous "Tiếng Việt có dấu: ..." input cut on a rune LEADING byte, so the
// old byte-slice code passed it too). 7×"ế" is 21 bytes with a 20-byte cap —
// the cut lands INSIDE the final rune, where the old content[:maxChars]
// produced invalid UTF-8 ("ếếếếếế\xe1\xba"); clipRunes must drop the partial rune.
func TestTruncateToTokens_MultibyteRuneSafe(t *testing.T) {
	content := strings.Repeat("ế", 7) // 21 bytes; cap 20 bytes cuts mid-rune
	out := truncateToTokens(content, 5)
	if !utf8.ValidString(out) {
		t.Fatalf("clipped output must stay valid UTF-8, got %q", out)
	}
	if EstimateTokens(out) > 5 {
		t.Fatalf("clipped output must stay within the token budget, got %d tokens", EstimateTokens(out))
	}
	if out != strings.Repeat("ế", 6) {
		t.Fatalf("clip must keep exactly the whole runes that fit, got %q", out)
	}
	exact := truncateToTokens("abcdefghij", 2) // 10 bytes vs 8-byte cap
	if exact != "abcdefgh" {
		t.Fatalf("ascii boundary clip must be exact, got %q", exact)
	}
}
