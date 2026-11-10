package mcp

import (
	"context"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerCard(t *testing.T) {
	b, err := ServerCard("http://localhost:8082")
	if err != nil {
		t.Fatalf("ServerCard: %v", err)
	}
	if len(b) == 0 {
		t.Error("ServerCard returned empty bytes")
	}
}

func TestResourceHandlers(t *testing.T) {
	s := permissiveServer(t)
	ctx := context.Background()
	req := &mcpsdk.ReadResourceRequest{Params: &mcpsdk.ReadResourceParams{}}

	if _, err := s.handleSiteStructure(ctx, req); err != nil {
		t.Errorf("handleSiteStructure: %v", err)
	}
	if _, err := s.handleRecentContent(ctx, req); err != nil {
		t.Errorf("handleRecentContent: %v", err)
	}
	if _, err := s.handleThemeConfig(ctx, req); err != nil {
		t.Errorf("handleThemeConfig: %v", err)
	}
}
