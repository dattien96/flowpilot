package runner

import (
	"testing"
	"time"
)

func TestDefaultQuestionTTLIs30Minutes(t *testing.T) {
	svc := NewInteractiveService()
	if svc.questionTTL != 30*time.Minute {
		t.Fatalf("questionTTL = %s, want 30m (operator asked after 10m expiry during CA-758 retest)", svc.questionTTL)
	}
	if svc.approvalTTL != 10*time.Minute {
		t.Fatalf("approvalTTL = %s, want 10m unchanged", svc.approvalTTL)
	}
}
