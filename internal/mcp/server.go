// Package mcp contains Creed's MCP interaction surface.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	serverlib "github.com/mark3labs/mcp-go/server"

	"github.com/techgodhq/creed/internal/mcp/gen"
	"github.com/techgodhq/creed/internal/service"
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

type registeredTool struct {
	Tool
	mcpTool mcplib.Tool
	handler gen.ToolHandler
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
	for _, tool := range gen.GeneratedTools(s.service) {
		s.register(tool)
	}
}

func (s *Server) register(tool gen.GeneratedTool) {
	registered := registeredTool{
		Tool: Tool{
			Name:        tool.Spec.Name,
			Description: tool.Spec.Description,
			Params:      externalParams(tool.Spec.ParamNames),
		},
		mcpTool: tool.Tool,
		handler: tool.Handler,
	}
	s.tools[registered.Name] = registered
	s.mcpServer.AddTool(registered.mcpTool, s.mcpHandler(registered.Name))
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
	out := make([]rune, 0, len(name))
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, '_')
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}
