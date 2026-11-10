package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP server and API client
type Server struct {
	mcpServer *mcp.Server
	client    *apiclient.Client
	// localFiles lets upload_asset read file_path from this machine's disk.
	localFiles bool
}

// AllowLocalFiles lets upload_asset read a file_path from the local disk. Only
// the stdio server calls it: it runs on the user's own machine, on the user's
// behalf. On the hosted endpoint the local disk is the server's.
func (s *Server) AllowLocalFiles() {
	s.localFiles = true
}

// NewServer creates a new MCP server with all tools registered
func NewServer(client *apiclient.Client) *Server {
	// Create MCP server
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "lightcms",
			Version: "7.0.0",
		},
		nil,
	)

	s := &Server{
		mcpServer: mcpServer,
		client:    client,
	}

	// Register all tools
	s.registerContentTools()
	s.registerTemplateTools()
	s.registerAssetTools()
	s.registerSettingsTools()
	s.registerSearchTools()
	s.registerSnippetTools()
	s.registerForkTools()
	s.registerImportTools()
	s.registerWebhookTools()
	s.registerLockTools()
	s.registerScheduleTools()
	s.registerAuditTools()
	s.registerLinkCheckTools()
	s.registerCommentTools()
	s.registerApprovalTools()
	s.registerSandboxTools()
	s.registerGovernanceTools()
	s.registerMaintenanceTools()
	s.registerIndexNowTools()
	s.registerSEOTools()

	// Register resources
	s.registerResources()

	return s
}

// Run starts the MCP server using stdio transport
func (s *Server) Run(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// MCPServer returns the underlying SDK server for use with HTTP transport
func (s *Server) MCPServer() *mcp.Server {
	return s.mcpServer
}

// boolPtr returns a pointer to a bool value.
// Needed for ToolAnnotations fields where nil means "use spec default" (true).
func boolPtr(b bool) *bool {
	return &b
}

// Helper to create a text result
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// Helper to create an error result
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)},
		},
		IsError: true,
	}
}

// ServerCard returns the MCP server card JSON with full tool schemas.
// It creates a temporary in-memory client-server pair to extract the
// tool definitions (with inputSchema) from the registered tools.
func ServerCard(baseURL string) ([]byte, error) {
	// Create a dummy server with a nil-client — tools are registered at
	// creation time and schemas are derived from Go struct types, so no
	// API calls are needed.
	client := apiclient.New("http://localhost:0", "dummy")
	server := NewServer(client)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Run the server in the background
	go server.MCPServer().Run(ctx, serverTransport)

	// Connect a client and list tools
	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "server-card-generator",
		Version: "1.0.0",
	}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	card := map[string]interface{}{
		"serverInfo": map[string]string{
			"name":    "LightCMS",
			"version": "4.5.0",
		},
		"endpoint":  "/mcp",
		"transport": []string{"streamable-http", "stdio"},
		"authentication": map[string]interface{}{
			"required":             true,
			"schemes":              []string{"bearer"},
			"authorization_server": baseURL,
			"resource_metadata":    baseURL + "/.well-known/oauth-protected-resource",
		},
		"tools":     res.Tools,
		"resources": []interface{}{},
		"prompts":   []interface{}{},
	}

	return json.MarshalIndent(card, "", "  ")
}

// Helper to create a JSON result
func jsonResult(data interface{}) *mcp.CallToolResult {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errorResult(err)
	}
	log.Printf("[mcp] response size: %d bytes", len(jsonBytes))
	return textResult(string(jsonBytes))
}
