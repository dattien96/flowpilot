# Task-336: Thăng cấp bài học thành Skill và Đồng bộ Skillpack

## Metadata

- Document ID: `Task-336`
- Title: `Thăng cấp bài học thành Skill và Đồng bộ Skillpack`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-23: Bộ trí tuệ vận hành tích hợp](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md)
- Child Documents: `None`
- Related Documents: [Task-335: Bộ phát hiện lệch hướng Drift Detector](../done/Task-335-Drift-Wrong-Way-Detector-And-Correction-Ladder.md), [Task-334: Context Resolver và Budget Packer](../done/Task-334-Context-Resolver-And-Budget-Packer.md), `skillpack` package
- Replaces: `None`
- Tags: `runtime-intelligence, auto-skill, lesson-candidates, skillpack, target-project-sync`

## AI Quick View

### Summary

- Hiện thực hóa **Phase 3 của CP-23**: Xây dựng cơ chế tự học động giúp chuyển đổi các sai lầm lặp lại trong quá trình vận hành thành tri thức kỹ thuật lâu dài dưới dạng **Skill**.
- **Thang thăng cấp bài học (Promotion Ladder)**:
  - Sự cố đơn lẻ $\rightarrow$ Lưu bản ghi `DriftEvent` (từ [Task-335](../done/Task-335-Drift-Wrong-Way-Detector-And-Correction-Ladder.md)).
  - Lỗi có cùng mẫu hình (pattern) lặp lại $\ge 2$ lần $\rightarrow$ Tự động đúc kết thành **Lesson Candidate** (gồm: Title, Trigger pattern, Anti-pattern, Preferred behavior).
  - Người dùng xem xét và phê duyệt $\rightarrow$ Xuất bản thành **Skill** chính thức.
- **Phân tách lưu trữ và đồng bộ thông minh**:
  - **Cấp độ Dự án (Target Project)**: Ghi trực tiếp vào thư mục `.agents/skills/<group>/<skill-name>/SKILL.md` (hoặc `.claude/`, `.grok/`) của dự án đích để AI ngay lập tức nhận diện và tuân thủ trong các phiên làm việc tiếp theo.
  - **Cấp độ Nền tảng (Platform Core)**: Tùy chọn đóng góp ngược vào kho `apps/local-runner/internal/skillpack/flow-pack/<group>/` để mọi dự án mới sau này khi chạy `skillpack.Install()` đều được kế thừa bài học này.

### Current Ask

- Xây dựng module quản lý `internal/skilllearn/` gồm `candidate.go`, `promoter.go`, `exporter.go` và bộ kiểm thử `skilllearn_test.go`.

### Key Decisions

- `T-1` **Không bao giờ tự động tạo file Skill mà không có con người duyệt**: Tránh tình trạng sinh ra các rule rác hoặc overfit vào một lỗi ngẫu nhiên. Mọi bài học đề xuất đều dừng lại ở trạng thái `candidate` chờ duyệt.
- `T-2` **Tương thích 100% với kiến trúc `skillpack` hiện có**: Tuân thủ cấu trúc phân nhóm nền tảng đã có trong FlowPilot (`common`, `android`, `golang`, `reactjs`, `flutter`, `ios`, `kmm`,...).
- `T-3` **Định dạng 2 đầu ra**:
  - *Dạng thẻ rút gọn (Compact Rule Card)*: Nạp ngay vào bộ `BudgetPacker` của [Task-334](../done/Task-334-Context-Resolver-And-Budget-Packer.md) để tiết kiệm token.
  - *Dạng tài liệu hoàn chỉnh (Markdown Skill File)*: Lưu theo format chuẩn `SKILL.md` kèm YAML frontmatter để các AI tool khác đọc được.

### Constraints

- Không tạo skill mâu thuẫn với các thiết luật cốt lõi (như `safe-fix-contract` hay `additive-tests-only`).
- Quá trình xuất file skill không làm hỏng các skill có sẵn trong thư mục.

### Source Refs

- `requirements/07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md` (Phase 3).
- `apps/local-runner/internal/skillpack/install.go` (Hạ tầng cài đặt skillpack).
- `apps/local-runner/internal/skillpack/flow-pack/` (Cây thư mục skillpack nền tảng).

### Open Questions

- Đã giải quyết: Skill trùng tên sẽ tạo version mới, không ghi đè file cũ.
- Đã giải quyết: AI skill phải được copy qua target project lúc init thì mới được AI detect.

---

## 1. Goal

Biến FlowPilot thành một trợ lý AI có khả năng tự tích lũy kinh nghiệm và tự học hỏi từ các lỗi kỹ thuật lặp lại, biến mỗi bài học thành một Skill chuẩn hóa có thể tái sử dụng lâu dài cho dự án và toàn bộ nền tảng.

---

## 2. Parent Links

- Coding Plan: [CP-23 Phase 3](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md).
- Upstream Tasks: [Task-334](../done/Task-334-Context-Resolver-And-Budget-Packer.md), [Task-335](../done/Task-335-Drift-Wrong-Way-Detector-And-Correction-Ladder.md).

---

## 3. Trigger

Hiện tại, nếu AI mắc một lỗi đặc thù trong framework (ví dụ cách gọi goroutine trong Go hoặc lifecycle Compose trong Android), lập trình viên phải nhắc đi nhắc lại trong prompt ở mỗi phiên làm việc mới vì AI không có bộ nhớ kỹ năng dài hạn.

---

## 4. Exact Change

- `T-1` Tạo package `internal/skilllearn/` định nghĩa struct `LessonCandidate` và `SkillExportSpec`.
- `T-2` Triển khai `CandidateAggregator`: Nhận các `DriftEvent`, gom nhóm theo pattern, nếu xuất hiện $\ge 2$ lần thì tạo `LessonCandidate`.
- `T-3` Triển khai API phê duyệt / từ chối bài học từ phía người dùng.
- `T-4` Triển khai `SkillExporter`: Xuất bản thành file `SKILL.md` chuẩn, hỗ trợ ghi vào thư mục `.agents/skills/` của target project hoặc vào `skillpack/flow-pack/`.

---

## 5. Touched Areas

- `apps/local-runner/internal/skilllearn/candidate.go` (Mới)
- `apps/local-runner/internal/skilllearn/promoter.go` (Mới)
- `apps/local-runner/internal/skilllearn/exporter.go` (Mới)
- `apps/local-runner/internal/skilllearn/skilllearn_test.go` (Mới)
- `apps/local-runner/internal/skillpack/install.go` (Cung cấp hàm xuất bản vào skillpack)

---

## 6. Acceptance Check

- Chạy `go test ./internal/skilllearn/ -run TestSkillPromotion` pass 100%.
- Khi có 2 event drift cùng pattern $\rightarrow$ Hệ thống tự động tạo 1 Lesson Candidate.
- Khi người dùng bấm phê duyệt $\rightarrow$ File `SKILL.md` được sinh ra đúng cú pháp YAML frontmatter và nằm đúng thư mục `.agents/skills/<group>/<name>/SKILL.md`.
- Lựa chọn xuất bản Core $\rightarrow$ File được copy vào `internal/skillpack/flow-pack/` thành công.

---

## 7. Out of Scope

- Tự động sửa code mã nguồn của dự án.
- Can thiệp vào các cài đặt AI provider cá nhân.

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-11).
- Triển khai: package `internal/skilllearn/` — candidate.go (LessonCandidate/SkillExportOptions/AggregateDriftEvents: dedupe (RunID,TurnID) keep-first theo ghi nhận review CA-838, pattern key = TriggeredSignals chuẩn hoá, threshold clamp ≥2, deterministic; PromoteCandidateToSkill), promoter.go (PromoterService: List/Upsert/UpdateCandidate — Edit trước duyệt /ApproveCandidateAndExport/RejectCandidate; store JSON atomic tmp+rename tại `.flowpilot/workflow_lesson_candidates.json`; corrupt file → error không panic), exporter.go (RenderSkillMarkdownWithSlug: YAML frontmatter name+description + # Title + Core Rules/Anti-Pattern/Preferred Behavior/Example; ghi `.agents/.claude/.grok/skills/<group>/<slug>/`; core flow-pack; duplicate slug → -v2/-v3 không ghi đè; DetectCoreSkillConflict + ConflictError đến khi Force=true — file không bao giờ được ghi khi có conflict chưa force; CompactRuleCard 3-5 dòng cho Budget Packer).
- skillpack/install.go: thêm `InstallFromRoot(targetRepoDir, platform, flowPackRoot)` (+72/−0 thuần additive) — tái dùng platformGroups/installRoots của Install, không bao giờ ghi đè skill đích có sẵn; là đường gọi thật cho test discoverability (embedded go:embed Install không thấy temp root).
- T-1 được enforced: không có đường đi nào export mà không qua Approve/Force; transition candidate→approved→promoted; export fail giữ `approved` (không mất dữ liệu); rejected/promoted là trạng thái kết thúc.
- Tests: 9/9 test signature §10 + 9 test bổ sung + TestSkillPromotion_EndToEnd_CandidateToSkill = 18/18 pass; skillpack/driftdetect/promptpacker vẫn xanh.
- GitNexus: Install không bị sửa (additive 72/0), callers của Install = 9 test nội bộ → LOW.
- Review PASS, 7 non-blocking hardening cho tương lai (khi có caller thật): frontmatter slug lệch giữa các provider dir khi mixed state; Group chưa slug-validate (chống `..` traversal — local tool nên thấp); upsert key chưa gồm ProjectID (store per-workspace nên vô hại, gộp key khi có multi-project); store single-writer assumption (cần lock khi wire TUI); conflict heuristic over-warn các lesson củng cố (fail-safe cố ý); SKILL.md chưa có `version:` frontmatter (không ảnh hưởng InstallFromRoot stat-based).
- Provider parity: provider-agnostic (deterministic Go, 0 LLM; double-call equality test pin determinism).
- Prior CA claims giữ nguyên: CA-837 (CompactRuleCard tương thích `## Core Rules` heading mà CompactSkillCard nhận), CA-838 (dedupe note đã implement + test), CA-833..CA-836.

---

## 9. Definition of Done

- [x] Struct `LessonCandidate` lưu đầy đủ thông tin: `Title`, `Group`, `TriggerPattern`, `AntiPattern`, `PreferredBehavior`, `RepeatCount`, `Status`.
- [x] Logic gom nhóm phát hiện chính xác mẫu lỗi lặp lại $\ge 2$ lần để kích hoạt đề xuất candidate.
- [x] Người dùng có quyền: Chấp thuận (`Approve`), Chỉnh sửa nội dung (`Edit`), hoặc Từ chối (`Reject`).
- [x] File `SKILL.md` sinh ra tuân thủ nghiêm ngặt định dạng chuẩn: Có YAML frontmatter (`name`, `description`), tiêu đề `#`, phần quy tắc và ví dụ.
- [x] Hỗ trợ ghi đúng vào thư mục `.agents/skills/`, `.claude/skills/`, `.grok/skills/` của target project.
- [x] Hỗ trợ ghi vào kho `internal/skillpack/flow-pack/<group>/` khi chọn chế độ Core Platform.
- [x] Chạy `go test ./internal/skilllearn/...` pass 100%. (18/18.)
- [x] API phê duyệt (`ApproveCandidateAndExport`) và từ chối (`RejectCandidate`) hoạt động chính xác.
- [x] Skill sau khi xuất bản và chạy `skillpack.Install()` thực sự xuất hiện trong thư mục `.agents/skills/` của target project. (Qua `InstallFromRoot` — đường gọi thật, không mock; embedded Install không đọc được flow-pack root ngoài build.)
- [x] Kiểm tra conflict: Không cho phép tạo skill mâu thuẫn với `safe-fix-contract` hay `additive-tests-only` mà không có cảnh báo. (DetectCoreSkillConflict + ConflictError đến khi Force.)

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/skilllearn/skilllearn_test.go`

```go
package skilllearn

import "testing"

// Scenario: Sự cố xảy ra lần đầu -> Chưa đủ điều kiện tạo candidate
// Input: 1 DriftEvent duy nhất với pattern "goroutine_leak"
// Expect: Không tạo LessonCandidate mới
func TestCandidateAggregator_SingleEvent_NoCandidate(t *testing.T) {}

// Scenario: Sự cố lặp lại lần 2 -> Tự động sinh LessonCandidate
// Input: 2 DriftEvent có cùng pattern "goroutine_leak" trên cùng project
// Expect: Sinh ra 1 LessonCandidate với RepeatCount=2, Status="candidate"
func TestCandidateAggregator_RepeatedEvent_GeneratesCandidate(t *testing.T) {}

// Scenario: Người dùng phê duyệt candidate -> Xuất bản file SKILL.md vào target project
// Input: Candidate đã duyệt, TargetDir="/path/to/project/.agents/skills", Group="golang"
// Expect: File /path/to/project/.agents/skills/golang/goroutine-safety/SKILL.md được tạo với đúng format
func TestSkillExporter_ApproveToTargetProject_WritesValidSkill(t *testing.T) {}

// Scenario: Xuất bản vào kho Core Flow-Pack
// Input: Candidate đã duyệt, ExportToCore=true, Group="common"
// Expect: File được ghi vào internal/skillpack/flow-pack/common/ với đúng cấu trúc
func TestSkillExporter_ExportToCoreFlowPack_Success(t *testing.T) {}

// [Edge] Scenario: Hai pattern khác nhau, mỗi pattern chỉ xuất hiện 1 lần
// Input: DriftEvent pattern "goroutine_leak" x1, DriftEvent pattern "nil_pointer" x1
// Expect: Không tạo LessonCandidate nào (mỗi pattern chưa đủ ngưỡng 2)
func TestCandidateAggregator_DifferentPatterns_NoCandidates(t *testing.T) {}

// [Edge] Scenario: Skill trùng tên đã tồn tại trong target project
// Input: File .agents/skills/golang/goroutine-safety/SKILL.md đã có
// Expect: Tạo version mới (goroutine-safety-v2) thay vì ghi đè
func TestSkillExporter_DuplicateName_CreatesNewVersion(t *testing.T) {}

// [Error] Scenario: WorkspaceRoot không tồn tại hoặc readonly
// Input: WorkspaceRoot="/nonexistent/path"
// Expect: Trả về error rõ ràng, không panic
func TestSkillExporter_InvalidWorkspace_ReturnsError(t *testing.T) {}

// [Error] Scenario: Skill mới mâu thuẫn với skill cốt lõi (safe-fix-contract)
// Input: Candidate có AntiPattern trùng với rule trong safe-fix-contract
// Expect: Cảnh báo conflict, yêu cầu user xác nhận thêm
func TestSkillExporter_ConflictWithCoreSkill_WarnsUser(t *testing.T) {}

// [Edge] Scenario: Kiểm tra skill sau install có thể được AI phát hiện
// Input: Xuất bản skill vào skillpack, chạy skillpack.Install()
// Expect: File SKILL.md xuất hiện đúng vị trí trong target project
func TestSkillExporter_InstalledSkillIsDiscoverable(t *testing.T) {}
```

---

## 11. Code Guide

Chữ ký và cấu trúc trong `apps/local-runner/internal/skilllearn/candidate.go`:

```go
package skilllearn

type CandidateStatus string

const (
	StatusCandidate CandidateStatus = "candidate"
	StatusApproved  CandidateStatus = "approved"
	StatusRejected  CandidateStatus = "rejected"
	StatusPromoted  CandidateStatus = "promoted"
)

type LessonCandidate struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	Title             string          `json:"title"`
	Group             string          `json:"group"` // "common", "android", "golang",...
	TriggerPattern    string          `json:"trigger_pattern"`
	AntiPattern       string          `json:"anti_pattern"`
	PreferredBehavior string          `json:"preferred_behavior"`
	RepeatCount       int             `json:"repeat_count"`
	SupportingRunIDs  []string        `json:"supporting_run_ids"`
	Status            CandidateStatus `json:"status"`
}

type SkillExportOptions struct {
	WorkspaceRoot string `json:"workspace_root"`
	ExportToCore  bool   `json:"export_to_core"`
}

// PromoteCandidateToSkill chuyển đổi candidate thành file skill chuẩn trên đĩa.
func PromoteCandidateToSkill(candidate LessonCandidate, opts SkillExportOptions) error {
	// 1. Sinh nội dung Markdown chuẩn:
	//    ---
	//    name: <slug>
	//    description: <mô tả>
	//    ---
	//    # <Title>
	//    ...
	// 2. Nếu ghi vào target project:
	//    - Xác định thư mục .agents/skills/<group>/<slug>/
	//    - Ghi file SKILL.md
	// 3. Nếu ghi vào Core FlowPack:
	//    - Xác định thư mục internal/skillpack/flow-pack/<group>/<slug>/
	//    - Ghi file SKILL.md
	// 4. Cập nhật trạng thái candidate thành StatusPromoted
	return nil
}
```

Chữ ký trong `apps/local-runner/internal/skilllearn/candidate.go` (bổ sung):

```go
// AggregateDriftEvents nhận danh sách DriftEvent và gom nhóm theo pattern.
// Nếu cùng pattern xuất hiện >= threshold lần, tạo LessonCandidate.
func AggregateDriftEvents(events []driftdetect.DriftEvent, threshold int) []LessonCandidate {
	// 1. Nhóm events theo TriggerPattern (dựa trên TriggeredSignals)
	// 2. Với mỗi nhóm có len >= threshold:
	//    - Tạo LessonCandidate với RepeatCount = len(group)
	//    - Trích xuất AntiPattern và PreferredBehavior từ dữ liệu event
	// 3. Trả về danh sách candidates mới
	return nil
}
```

Chữ ký trong `apps/local-runner/internal/skilllearn/promoter.go`:

```go
package skilllearn

import "context"

// PromoterService cung cấp API quản lý và phê duyệt LessonCandidate.
type PromoterService struct {
	StorePath string // Đường dẫn file lưu trữ candidates JSON
}

// ListCandidates trả về danh sách tất cả candidates hiện tại.
func (p *PromoterService) ListCandidates(ctx context.Context) ([]LessonCandidate, error) {
	// Đọc file JSON chứa candidates
	return nil, nil
}

// ApproveCandidateAndExport phê duyệt candidate và xuất bản thành Skill.
func (p *PromoterService) ApproveCandidateAndExport(ctx context.Context, candidateID string, opts SkillExportOptions) error {
	// 1. Tìm candidate theo ID, kiểm tra status == StatusCandidate
	// 2. Kiểm tra conflict với core skills
	// 3. Gọi PromoteCandidateToSkill
	// 4. Cập nhật status thành StatusPromoted
	return nil
}

// RejectCandidate từ chối candidate.
func (p *PromoterService) RejectCandidate(ctx context.Context, candidateID string) error {
	// Cập nhật status thành StatusRejected
	return nil
}
```
```
