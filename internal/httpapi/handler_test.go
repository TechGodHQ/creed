package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

func TestListOperationsReturnsGeneratedCatalog(t *testing.T) {
	server := httptest.NewServer(NewHandler(&fakeService{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/operations")
	if err != nil {
		t.Fatalf("GET operations: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Operations []struct {
			OperationName string `json:"OperationName"`
			HTTPRoute     string `json:"HTTPRoute"`
		} `json:"operations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Operations) == 0 {
		t.Fatal("expected generated operations")
	}
	assertOperationRoute(t, body.Operations, "sync", "/v1/operations/sync")
	assertOperationRoute(t, body.Operations, "list_targets", "/v1/operations/list_targets")
}

func TestCallSyncDelegatesOptionsAndReturnsStructuredResult(t *testing.T) {
	svc := &fakeService{syncResult: &usecase.SyncResult{Targets: []usecase.TargetResult{{Target: "claude", FilesWritten: 1}}}}
	server := httptest.NewServer(NewHandler(svc))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/operations/sync", "application/json", bytes.NewBufferString(`{"target":"claude","dry_run":true,"force":true}`))
	if err != nil {
		t.Fatalf("POST sync: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if svc.syncOptions.Target != "claude" || !svc.syncOptions.DryRun || !svc.syncOptions.Force {
		t.Fatalf("sync options = %+v", svc.syncOptions)
	}

	var body struct {
		OK     bool                `json:"ok"`
		Result *usecase.SyncResult `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || body.Result == nil || len(body.Result.Targets) != 1 || body.Result.Targets[0].Target != "claude" {
		t.Fatalf("body = %+v", body)
	}
}

func TestCallListTargetsDelegatesAndReturnsTargets(t *testing.T) {
	svc := &fakeService{listTargetsResult: []domain.TargetInfo{{Name: "claude", DisplayName: "Claude", Enabled: true, OutputDir: "."}}}
	server := httptest.NewServer(NewHandler(svc))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/operations/list_targets", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("POST list_targets: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		OK     bool                `json:"ok"`
		Result []domain.TargetInfo `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || len(body.Result) != 1 || body.Result[0].Name != "claude" {
		t.Fatalf("body = %+v", body)
	}
}

func TestCallListTargetsAcceptsEmptyBody(t *testing.T) {
	svc := &fakeService{listTargetsResult: []domain.TargetInfo{{Name: "claude", DisplayName: "Claude", Enabled: true, OutputDir: "."}}}
	server := httptest.NewServer(NewHandler(svc))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/operations/list_targets", "application/json", nil)
	if err != nil {
		t.Fatalf("POST list_targets empty body: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestUnsupportedOperationReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(NewHandler(&fakeService{}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/operations/nope", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("POST unsupported operation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func assertOperationRoute(t *testing.T, operations []struct {
	OperationName string `json:"OperationName"`
	HTTPRoute     string `json:"HTTPRoute"`
}, name, route string) {
	t.Helper()
	for _, operation := range operations {
		if operation.OperationName == name {
			if operation.HTTPRoute != route {
				t.Fatalf("%s route = %q, want %q", name, operation.HTTPRoute, route)
			}
			return
		}
	}
	t.Fatalf("operation %s not found in catalog", name)
}

type fakeService struct {
	service.Service

	syncOptions usecase.SyncOptions
	syncResult  *usecase.SyncResult

	listTargetsResult []domain.TargetInfo
}

func (f *fakeService) Init(ctx context.Context, projectName string) error { return nil }

func (f *fakeService) Sync(ctx context.Context, opts usecase.SyncOptions) (*usecase.SyncResult, error) {
	f.syncOptions = opts
	if f.syncResult != nil {
		return f.syncResult, nil
	}
	return &usecase.SyncResult{}, nil
}

func (f *fakeService) AddSkill(ctx context.Context, name, sourcePath string) error { return nil }

func (f *fakeService) RemoveSkill(ctx context.Context, name string) error { return nil }

func (f *fakeService) ListSkills(ctx context.Context) ([]domain.SkillInfo, error) { return nil, nil }

func (f *fakeService) ListTargets(ctx context.Context) ([]domain.TargetInfo, error) {
	return f.listTargetsResult, nil
}

func (f *fakeService) EnableTarget(ctx context.Context, name string) error { return nil }

func (f *fakeService) DisableTarget(ctx context.Context, name string) error { return nil }

func (f *fakeService) Pull(ctx context.Context, remoteURL string) error { return nil }

func (f *fakeService) Push(ctx context.Context, remoteURL string) error { return nil }
