package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStartRun_HonorsContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := New(srv.URL).StartRun(ctx, StartRunInput{ProjectID: "p1", ChatMode: "normal_chat"})
	if err == nil {
		t.Fatal("expected timeout/cancel error from hung StartRun")
	}
}
