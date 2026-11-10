// Command lightcms-mcp runs the LightCMS MCP server over stdio, exposing content management tools to MCP clients.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/metavert-io/lightcms/v7/internal/apiclient"
	"github.com/metavert-io/lightcms/v7/internal/mcp"
)

func main() {
	// Read configuration from environment variables
	baseURL := strings.TrimSpace(os.Getenv("LIGHTCMS_URL"))
	apiKey := strings.TrimSpace(os.Getenv("LIGHTCMS_API_KEY"))

	if baseURL == "" {
		// Default to localhost
		baseURL = "http://localhost:8082"
	}

	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: LIGHTCMS_API_KEY environment variable is required\n")
		fmt.Fprintf(os.Stderr, "Create an API key in the admin panel at /cm/api-keys\n")
		os.Exit(1)
	}

	// Create API client
	client := apiclient.New(baseURL, apiKey)

	// Every MCP session gets a unique agent-session ID. The server groups
	// this session's changes in the audit log so humans can review or roll
	// them back as a unit.
	client.SetAgentSession(apiclient.NewAgentSessionID())

	// Create and run MCP server
	ctx := context.Background()
	server := mcp.NewServer(client)
	server.AllowLocalFiles()
	if err := server.Run(ctx); err != nil {
		log.Printf("MCP server error: %v", err)
		os.Exit(1)
	}
}
