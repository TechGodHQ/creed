// Package mcp contains Creed's MCP interaction surface.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	serverlib "github.com/mark3labs/mcp-go/server"

	"github.com/techgodhq/creed/internal/mcp/gen"
	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

// Tool describes a generated MCP tool registered by the Creed server.
type Tool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Params      []string `json:"params"`
}

// CallResult is the structured JSON-compatible response returned by a tool call.
type CallResult struct {
	Tool   string          `json:"tool"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type toolHandler func(context.Context, json.RawMessage) (any, error)

type registeredTool struct {
	Tool
	mcpTool mcplib.Tool
	handler toolHandler
}

// Server exposes generated Service-backed MCP tools.
type Server struct {
	service   service.Service
	mcpServer *serverlib.MCPServer
	tools     map[string]registeredTool
}

// NewServer creates an MCP server wrapper for the provided Service and registers
// generated tools from the Service-derived metadata in internal/mcp/gen.
func NewServer(service service.Service) *Server {
	s := &Server{
		service: service,
		mcpServer: serverlib.NewMCPServer(
			"creed",
			"0.1.0",
			serverlib.WithToolCapabilities(true),
			serverlib.WithStrictInputSchemaDefault(),
			serverlib.WithInputSchemaValidation(),
		),
		tools: map[string]registeredTool{},
	}
	s.registerGeneratedTools()
	return s
}

// Service returns the canonical service backing this MCP surface.
func (s *Server) Service() service.Service {
	return s.service
}

// MCPServer returns the concrete mcp-go server entrypoint.
func (s *Server) MCPServer() *serverlib.MCPServer {
	return s.mcpServer
}

// ServeStdio starts the concrete MCP server over stdio.
func (s *Server) ServeStdio() error {
	return serverlib.ServeStdio(s.mcpServer)
}

// Tools returns all registered tools in deterministic name order.
func (s *Server) Tools() []Tool {
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := make([]Tool, 0, len(names))
	for _, name := range names {
		tools = append(tools, s.tools[name].Tool)
	}
	return tools
}

// ToolNames returns registered tool names in deterministic order.
func (s *Server) ToolNames() []string {
	tools := s.Tools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

// Call invokes a registered MCP tool with a JSON object payload and returns a
// structured response. Service errors are captured in the response instead of
// being returned as transport errors so MCP clients get a stable JSON envelope.
func (s *Server) Call(ctx context.Context, name string, payload json.RawMessage) (CallResult, error) {
	tool, ok := s.tools[name]
	if !ok {
		return CallResult{}, fmt.Errorf("unknown MCP tool %q", name)
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	result, err := tool.handler(ctx, payload)
	if err != nil {
		return CallResult{Tool: name, OK: false, Error: err.Error()}, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return CallResult{}, fmt.Errorf("marshal %s result: %w", name, err)
	}
	return CallResult{Tool: name, OK: true, Result: data}, nil
}

func (s *Server) registerGeneratedTools() {
	for _, spec := range gen.ToolSpecs {
		tool, handler := s.toolForSpec(spec)
		s.register(tool, handler)
	}
}

func (s *Server) toolForSpec(spec gen.ToolSpec) (registeredTool, toolHandler) {
	switch spec.MethodName {
	case "Init":
		return generatedTool(spec, []string{"project_name"}, mcplib.WithString("project_name", mcplib.Required())), s.callInit
	case "Sync":
		return generatedTool(spec, []string{"target", "dry_run", "force"}, mcplib.WithString("target"), mcplib.WithBoolean("dry_run"), mcplib.WithBoolean("force")), s.callSync
	case "AddSkill":
		return generatedTool(spec, []string{"name", "source_path"}, mcplib.WithString("name", mcplib.Required()), mcplib.WithString("source_path")), s.callAddSkill
	case "RemoveSkill":
		return generatedTool(spec, []string{"name"}, mcplib.WithString("name", mcplib.Required())), s.callRemoveSkill
	case "ListSkills":
		return generatedTool(spec, nil), s.callListSkills
	case "ListTargets":
		return generatedTool(spec, nil), s.callListTargets
	case "EnableTarget":
		return generatedTool(spec, []string{"name"}, mcplib.WithString("name", mcplib.Required())), s.callEnableTarget
	case "DisableTarget":
		return generatedTool(spec, []string{"name"}, mcplib.WithString("name", mcplib.Required())), s.callDisableTarget
	case "Pull":
		return generatedTool(spec, []string{"remote_url"}, mcplib.WithString("remote_url")), s.callPull
	case "Push":
		return generatedTool(spec, []string{"remote_url"}, mcplib.WithString("remote_url")), s.callPush
	default:
		return generatedTool(spec, externalParams(spec.ParamNames)), unsupportedGeneratedTool(spec)
	}
}

func (s *Server) register(tool registeredTool, handler toolHandler) {
	tool.handler = handler
	s.tools[tool.Name] = tool
	s.mcpServer.AddTool(tool.mcpTool, s.mcpHandler(tool.Name))
}

func generatedTool(spec gen.ToolSpec, params []string, opts ...mcplib.ToolOption) registeredTool {
	allOpts := []mcplib.ToolOption{mcplib.WithDescription(spec.Description)}
	allOpts = append(allOpts, opts...)
	return registeredTool{
		Tool: Tool{
			Name:        spec.Name,
			Description: spec.Description,
			Params:      params,
		},
		mcpTool: mcplib.NewTool(spec.Name, allOpts...),
	}
}

func unsupportedGeneratedTool(spec gen.ToolSpec) toolHandler {
	return func(ctx context.Context, payload json.RawMessage) (any, error) {
		return nil, fmt.Errorf("generated MCP tool %s for service.Service.%s has no handler", spec.Name, spec.MethodName)
	}
}

func (s *Server) mcpHandler(name string) serverlib.ToolHandlerFunc {
	return func(ctx context.Context, request mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		payload, err := json.Marshal(request.Params.Arguments)
		if err != nil {
			return nil, fmt.Errorf("marshal MCP arguments for %s: %w", name, err)
		}
		result, err := s.Call(ctx, name, payload)
		if err != nil {
			return nil, err
		}
		toolResult := mcplib.NewToolResultStructuredOnly(result)
		if !result.OK {
			toolResult.IsError = true
		}
		return toolResult, nil
	}
}

type initRequest struct {
	ProjectName string `json:"project_name"`
}

type syncRequest struct {
	Target string `json:"target,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

type skillRequest struct {
	Name       string `json:"name"`
	SourcePath string `json:"source_path,omitempty"`
}

type targetRequest struct {
	Name string `json:"name"`
}

type remoteRequest struct {
	RemoteURL string `json:"remote_url,omitempty"`
}

type okResponse struct {
	OK bool `json:"ok"`
}

func (s *Server) callInit(ctx context.Context, payload json.RawMessage) (any, error) {
	var req initRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ProjectName) == "" {
		return nil, fmt.Errorf("project_name is required")
	}
	if err := s.service.Init(ctx, req.ProjectName); err != nil {
		return nil, err
	}
	return okResponse{OK: true}, nil
}

func (s *Server) callSync(ctx context.Context, payload json.RawMessage) (any, error) {
	var req syncRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	return s.service.Sync(ctx, usecase.SyncOptions{
		Target: req.Target,
		DryRun: req.DryRun,
		Force:  req.Force,
	})
}

func (s *Server) callAddSkill(ctx context.Context, payload json.RawMessage) (any, error) {
	var req skillRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.service.AddSkill(ctx, req.Name, req.SourcePath); err != nil {
		return nil, err
	}
	return okResponse{OK: true}, nil
}

func (s *Server) callRemoveSkill(ctx context.Context, payload json.RawMessage) (any, error) {
	var req skillRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.service.RemoveSkill(ctx, req.Name); err != nil {
		return nil, err
	}
	return okResponse{OK: true}, nil
}

func (s *Server) callListSkills(ctx context.Context, payload json.RawMessage) (any, error) {
	if err := decodePayload(payload, &struct{}{}); err != nil {
		return nil, err
	}
	return s.service.ListSkills(ctx)
}

func (s *Server) callListTargets(ctx context.Context, payload json.RawMessage) (any, error) {
	if err := decodePayload(payload, &struct{}{}); err != nil {
		return nil, err
	}
	return s.service.ListTargets(ctx)
}

func (s *Server) callEnableTarget(ctx context.Context, payload json.RawMessage) (any, error) {
	var req targetRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.service.EnableTarget(ctx, req.Name); err != nil {
		return nil, err
	}
	return okResponse{OK: true}, nil
}

func (s *Server) callDisableTarget(ctx context.Context, payload json.RawMessage) (any, error) {
	var req targetRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.service.DisableTarget(ctx, req.Name); err != nil {
		return nil, err
	}
	return okResponse{OK: true}, nil
}

func (s *Server) callPull(ctx context.Context, payload json.RawMessage) (any, error) {
	var req remoteRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	if err := s.service.Pull(ctx, req.RemoteURL); err != nil {
		return nil, err
	}
	return okResponse{OK: true}, nil
}

func (s *Server) callPush(ctx context.Context, payload json.RawMessage) (any, error) {
	var req remoteRequest
	if err := decodePayload(payload, &req); err != nil {
		return nil, err
	}
	if err := s.service.Push(ctx, req.RemoteURL); err != nil {
		return nil, err
	}
	return okResponse{OK: true}, nil
}

func decodePayload(payload json.RawMessage, dst any) error {
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode MCP tool payload: %w", err)
	}
	return nil
}

func externalParams(params []string) []string {
	external := make([]string, 0, len(params))
	for _, param := range params {
		if param == "ctx" {
			continue
		}
		external = append(external, toJSONName(param))
	}
	return external
}

func toJSONName(name string) string {
	var out strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out.WriteByte('_')
		}
		out.WriteRune(r)
	}
	return strings.ToLower(out.String())
}
