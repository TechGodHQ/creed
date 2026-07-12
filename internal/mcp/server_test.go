package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

func TestNewServerRegistersGeneratedTools(t *testing.T) {
	server := NewServer(&fakeService{})

	names := server.ToolNames()
	for _, want := range []string{"add_skill", "list_targets", "sync"} {
		if !contains(names, want) {
			t.Fatalf("expected tool %q in %v", want, names)
		}
	}

	var syncTool Tool
	for _, tool := range server.Tools() {
		if tool.Name == "sync" {
			syncTool = tool
		}
	}
	if !reflect.DeepEqual(syncTool.Params, []string{"target", "dry_run", "force"}) {
		t.Fatalf("sync params = %#v, want callable JSON params", syncTool.Params)
	}
	if server.MCPServer() == nil {
		t.Fatal("MCPServer() returned nil")
	}
}

func TestInProcessMCPListsGeneratedToolsAndCallsSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	svc := &fakeService{
		syncResult: &usecase.SyncResult{Targets: []usecase.TargetResult{{Target: "claude", FilesWritten: 1}}},
	}
	server := NewServer(svc)
	client, err := mcpclient.NewInProcessClient(server.MCPServer())
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	defer client.Close()

	initRequest := mcplib.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcplib.Implementation{Name: "creed-test", Version: "0.1.0"}
	if _, err := client.Initialize(ctx, initRequest); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	toolsResult, err := client.ListTools(ctx, mcplib.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	toolNames := make([]string, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	for _, want := range []string{"add_skill", "list_targets", "sync"} {
		if !contains(toolNames, want) {
			t.Fatalf("expected MCP tool %q in %v", want, toolNames)
		}
	}

	callRequest := mcplib.CallToolRequest{}
	callRequest.Params.Name = "sync"
	callRequest.Params.Arguments = map[string]any{"target": "claude", "dry_run": true}
	callResult, err := client.CallTool(ctx, callRequest)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if callResult.IsError {
		t.Fatalf("CallTool returned error result: %#v", callResult.StructuredContent)
	}
	structured, ok := callResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %#v, want map", callResult.StructuredContent)
	}
	if structured["ok"] != true || structured["tool"] != "sync" || structured["operation"] != "sync" {
		t.Fatalf("structured content = %#v", structured)
	}
	if svc.syncOptions.Target != "claude" || !svc.syncOptions.DryRun {
		t.Fatalf("sync options = %#v", svc.syncOptions)
	}
}

func TestCallListTargetsReturnsStructuredJSON(t *testing.T) {
	svc := &fakeService{
		listTargetsResult: []domain.TargetInfo{{Name: "claude", DisplayName: "Claude", Enabled: true, OutputDir: "."}},
	}
	server := NewServer(svc)

	result, err := server.Call(context.Background(), "list_targets", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Call returned transport error: %v", err)
	}
	if !result.OK || result.Tool != "list_targets" || result.Operation != "list_targets" {
		t.Fatalf("result = %#v, want ok list_targets response", result)
	}

	var targets []domain.TargetInfo
	if err := json.Unmarshal(result.Result, &targets); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "claude" {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestCallSyncDelegatesOptionsAndReturnsStructuredResult(t *testing.T) {
	svc := &fakeService{
		syncResult: &usecase.SyncResult{Targets: []usecase.TargetResult{{Target: "claude", FilesWritten: 1}}},
	}
	server := NewServer(svc)

	result, err := server.Call(context.Background(), "sync", json.RawMessage(`{"target":"claude","dry_run":true,"force":true}`))
	if err != nil {
		t.Fatalf("Call returned transport error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %#v, want ok", result)
	}
	if svc.syncOptions.Target != "claude" || !svc.syncOptions.DryRun || !svc.syncOptions.Force {
		t.Fatalf("sync options = %#v", svc.syncOptions)
	}

	var syncResult usecase.SyncResult
	if err := json.Unmarshal(result.Result, &syncResult); err != nil {
		t.Fatalf("unmarshal sync result: %v", err)
	}
	if len(syncResult.Targets) != 1 || syncResult.Targets[0].FilesWritten != 1 {
		t.Fatalf("sync result = %#v", syncResult)
	}
}

func TestCallAddSkillDelegatesToService(t *testing.T) {
	svc := &fakeService{}
	server := NewServer(svc)

	result, err := server.Call(context.Background(), "add_skill", json.RawMessage(`{"name":"review","source_path":"skills/review.md"}`))
	if err != nil {
		t.Fatalf("Call returned transport error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %#v, want ok", result)
	}
	if svc.addSkillName != "review" || svc.addSkillPath != "skills/review.md" {
		t.Fatalf("add skill call = (%q, %q)", svc.addSkillName, svc.addSkillPath)
	}
}

func TestCallReturnsStructuredServiceError(t *testing.T) {
	server := NewServer(&fakeService{syncErr: errors.New("fixture missing manifest")})

	result, err := server.Call(context.Background(), "sync", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Call returned transport error: %v", err)
	}
	if result.OK || result.Operation != "sync" || result.Error != "fixture missing manifest" {
		t.Fatalf("result = %#v, want structured service error", result)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type fakeService struct {
	service.Service

	syncOptions usecase.SyncOptions
	syncResult  *usecase.SyncResult
	syncErr     error

	addSkillName string
	addSkillPath string

	listTargetsResult []domain.TargetInfo
}

func (f *fakeService) Init(ctx context.Context, projectName string) error { return nil }

func (f *fakeService) Sync(ctx context.Context, opts usecase.SyncOptions) (*usecase.SyncResult, error) {
	f.syncOptions = opts
	if f.syncErr != nil {
		return nil, f.syncErr
	}
	if f.syncResult != nil {
		return f.syncResult, nil
	}
	return &usecase.SyncResult{}, nil
}

func (f *fakeService) AddSkill(ctx context.Context, name, sourcePath string) error {
	f.addSkillName = name
	f.addSkillPath = sourcePath
	return nil
}

func (f *fakeService) RemoveSkill(ctx context.Context, name string) error { return nil }

func (f *fakeService) ListSkills(ctx context.Context) ([]domain.SkillInfo, error) { return nil, nil }

func (f *fakeService) ListTargets(ctx context.Context) ([]domain.TargetInfo, error) {
	return f.listTargetsResult, nil
}

func (f *fakeService) EnableTarget(ctx context.Context, name string) error { return nil }

func (f *fakeService) DisableTarget(ctx context.Context, name string) error { return nil }

func (f *fakeService) Pull(ctx context.Context, remoteURL string) error { return nil }

func (f *fakeService) Push(ctx context.Context, remoteURL string) error { return nil }
