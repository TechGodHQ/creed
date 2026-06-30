// Package mcp contains the MCP interaction surface for Creed.
//
// The concrete mcp-go server is intentionally deferred until the generator
// slice lands; this stub gives code generation and callers a stable package
// boundary without introducing an unused dependency.
package mcp

import "github.com/techgodhq/creed/internal/service"

// Server is a placeholder MCP server wrapper around the canonical Service API.
type Server struct {
	service service.Service
}

// NewServer creates an MCP server wrapper for the provided Service.
func NewServer(service service.Service) *Server {
	return &Server{service: service}
}

// Service returns the canonical service backing this MCP surface.
func (s *Server) Service() service.Service {
	return s.service
}
