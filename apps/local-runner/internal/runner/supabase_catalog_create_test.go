package runner

import (
	"bytes"
	"context"
	"log"
	"regexp"
	"strings"
	"testing"
)

type scriptedResponse struct {
	status int
	body   []byte
	err    error
}

// withScriptedHTTP swaps the global httpRequestFn, returning scripted
// responses per call in order (F-3/F-5 create-path tests need different
// responses for the projects insert vs the binding insert).
func withScriptedHTTP(t *testing.T, responses []scriptedResponse) *[]capturedRequest {
	t.Helper()
	orig := httpRequestFn
	captured := &[]capturedRequest{}
	idx := 0
	httpRequestFn = func(_ context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		*captured = append(*captured, capturedRequest{method, endpoint, headers, body})
		if idx >= len(responses) {
			return 500, []byte("unexpected extra http call"), nil
		}
		r := responses[idx]
		idx++
		return r.status, r.body, r.err
	}
	t.Cleanup(func() { httpRequestFn = orig })
	return captured
}

func newCreateStore() *SupabaseCatalogStore {
	return NewSupabaseCatalogStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co"}, "k")
}

func TestCreateProjectSuccessAndBindingRequest(t *testing.T) {
	store := newCreateStore()
	captured := withScriptedHTTP(t, []scriptedResponse{
		{status: 201, body: []byte(`[{"id":"proj-1","name":"Acme","directory_path":"/tmp/acme","default_model":null,"platform":null}]`)},
		{status: 201, body: []byte(`[]`)},
	})

	proj, err := store.CreateProject(context.Background(), CreateProjectInput{
		Name:          "Acme",
		DirectoryPath: "/tmp/acme",
		Platform:      "golang",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if proj.ID != "proj-1" || proj.Name != "Acme" || proj.Path != "/tmp/acme" || proj.Platform != "golang" {
		t.Fatalf("project = %+v", proj)
	}
	if len(*captured) != 2 {
		t.Fatalf("expected 2 http calls (projects + binding), got %d", len(*captured))
	}
	if (*captured)[0].method != "POST" || !strings.HasSuffix((*captured)[0].endpoint, "/projects") {
		t.Fatalf("first call = %s %s", (*captured)[0].method, (*captured)[0].endpoint)
	}
	bind := (*captured)[1]
	if !strings.HasSuffix(bind.endpoint, "/project_workspace_bindings") {
		t.Fatalf("binding endpoint = %s", bind.endpoint)
	}
	if !strings.Contains(string(bind.body), `"project_id":"proj-1"`) || !strings.Contains(string(bind.body), `"local_path":"/tmp/acme"`) {
		t.Fatalf("binding body = %s", string(bind.body))
	}
}

// The projects table keeps legacy_id + created_by NOT NULL with no default
// (BUG-135/BUG-136 fixed the TS insert paths); the Go store must supply the
// same fields or PostgREST rejects the insert with 23502.
func TestCreateProjectSendsLegacyIDAndCreatedBy(t *testing.T) {
	store := newCreateStore()
	captured := withScriptedHTTP(t, []scriptedResponse{
		{status: 201, body: []byte(`[{"id":"proj-1","name":"Acme","directory_path":"/tmp/acme","default_model":null,"platform":null}]`)},
		{status: 201, body: []byte(`[]`)},
	})

	_, err := store.CreateProject(context.Background(), CreateProjectInput{
		Name:          "Acme",
		DirectoryPath: "/tmp/acme",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	body := string((*captured)[0].body)
	legacyRe := regexp.MustCompile(`"legacy_id":"project_[0-9a-f]{18}"`)
	if !legacyRe.MatchString(body) {
		t.Fatalf("projects insert missing generated legacy_id, body = %s", body)
	}
	if !strings.Contains(body, `"created_by":"supabase-admin"`) {
		t.Fatalf("projects insert missing created_by, body = %s", body)
	}
	// description / repository_url are NOT NULL without defaults — absent
	// input must still send them as empty strings, never omit them.
	if !strings.Contains(body, `"description":""`) {
		t.Fatalf("projects insert missing description, body = %s", body)
	}
	if !strings.Contains(body, `"repository_url":""`) {
		t.Fatalf("projects insert missing repository_url, body = %s", body)
	}
}

// F-3: a failed workspace binding insert must not fail project creation and
// must surface a warning log (never silent).
func TestCreateProjectBindingFailureReturnsProjectAndLogs(t *testing.T) {
	store := newCreateStore()
	withScriptedHTTP(t, []scriptedResponse{
		{status: 201, body: []byte(`[{"id":"proj-1","name":"Acme","directory_path":"/tmp/acme","default_model":null,"platform":null}]`)},
		{status: 500, body: []byte(`binding exploded`)},
	})

	var logBuf bytes.Buffer
	origWriter := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(origWriter) })

	proj, err := store.CreateProject(context.Background(), CreateProjectInput{
		Name:          "Acme",
		DirectoryPath: "/tmp/acme",
	})
	if err != nil {
		t.Fatalf("binding failure must not fail CreateProject, got: %v", err)
	}
	if proj.ID != "proj-1" {
		t.Fatalf("project = %+v", proj)
	}
	if !strings.Contains(logBuf.String(), "workspace binding failed") || !strings.Contains(logBuf.String(), "proj-1") {
		t.Fatalf("expected binding failure warning log, got: %q", logBuf.String())
	}
}

// F-5: an empty (but well-formed) insert response must produce a clean,
// descriptive error — never a "%!w(<nil>)" wrapped message.
func TestCreateProjectEmptyResponseReturnsCleanError(t *testing.T) {
	store := newCreateStore()
	withScriptedHTTP(t, []scriptedResponse{
		{status: 201, body: []byte(`[]`)},
	})

	_, err := store.CreateProject(context.Background(), CreateProjectInput{
		Name:          "Acme",
		DirectoryPath: "/tmp/acme",
	})
	if err == nil {
		t.Fatal("expected error for empty created-project response")
	}
	if strings.Contains(err.Error(), "%!w") {
		t.Fatalf("error must not wrap a nil error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "no project row") {
		t.Fatalf("error should describe the empty response: %q", err.Error())
	}
}

func TestCreateProjectMalformedResponseReturnsParseError(t *testing.T) {
	store := newCreateStore()
	withScriptedHTTP(t, []scriptedResponse{
		{status: 201, body: []byte(`{not-json`)},
	})

	_, err := store.CreateProject(context.Background(), CreateProjectInput{
		Name:          "Acme",
		DirectoryPath: "/tmp/acme",
	})
	if err == nil || !strings.Contains(err.Error(), "parse created project response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}
