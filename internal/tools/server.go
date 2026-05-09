package tools

import (
	"context"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Server wraps an MCP server and an in-process client connected to it.
// The anthropic client is held for tools that make their own LLM calls (analyze_js).
type Server struct {
	mcpServer      *mcpserver.MCPServer
	Client         *mcpclient.Client
	anthropicClient *anthropic.Client
}

// New creates an MCP server, registers all enrichment tools, and connects an
// in-process client to it. The client is initialized and ready to use on return.
func New(ctx context.Context, anthropicClient *anthropic.Client) (*Server, error) {
	s := &Server{anthropicClient: anthropicClient}

	s.mcpServer = mcpserver.NewMCPServer("go-phish-tools", "1.0.0")
	registerTools(s.mcpServer, anthropicClient)

	var err error
	s.Client, err = mcpclient.NewInProcessClient(s.mcpServer)
	if err != nil {
		return nil, fmt.Errorf("create in-process MCP client: %w", err)
	}

	if err := s.Client.Start(ctx); err != nil {
		return nil, fmt.Errorf("start MCP client: %w", err)
	}

	_, err = s.Client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "go-phish",
				Version: "1.0.0",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize MCP client: %w", err)
	}

	return s, nil
}

// Stop closes the in-process client connection.
func (s *Server) Stop() error {
	return s.Client.Close()
}

// registerTools registers all enrichment tool handlers on the MCP server.
// Phase B agents add their tool registration here.
//
// Registration pattern:
//
//	s.AddTool(
//	    mcp.NewTool("<name>",
//	        mcp.WithDescription("..."),
//	        mcp.WithString("<param>", mcp.Required(), mcp.Description("...")),
//	    ),
//	    handlerFunc,
//	)
//
// Handler signature: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
// Return tool errors as JSON text, not Go errors:
//
//	mcp.NewToolResultText(`{"error":"..."}`)   — tool-level error (model sees it)
//	return nil, err                            — MCP protocol error (loop aborts)
func registerTools(s *mcpserver.MCPServer, _ *anthropic.Client) {
	// Phase B agents register tools here.
	s.AddTool(
		mcp.NewTool("cert_transparency",
			mcp.WithDescription("Look up certificate transparency logs for a domain via crt.sh. Returns recently issued certificates including SANs."),
			mcp.WithString("domain", mcp.Required(), mcp.Description("Domain to search (e.g. example.com)")),
		),
		certTransparencyHandler,
	)
	s.AddTool(
		mcp.NewTool("urlscan_lookup",
			mcp.WithDescription("Search urlscan.io for prior scans of a URL. Returns verdicts, tags, and scan dates."),
			mcp.WithString("url", mcp.Required(), mcp.Description("URL to search for (e.g. https://example.com/path)")),
		),
		urlscanHandler,
	)
}
