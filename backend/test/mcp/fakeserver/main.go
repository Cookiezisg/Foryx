// Command fakeserver is a minimal stdio MCP server for pipeline tests; exposes echo/fail/crash tools.
//
// Command fakeserver 是 pipeline 测试用的最小 stdio MCP server，暴露 echo/fail/crash 三 tool。
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "forgify-fake-mcp",
		Version: "0.0.1",
	}, nil)

	type echoArgs struct {
		Text string `json:"text" jsonschema:"text to echo back"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echo the input back verbatim.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: args.Text}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fail",
		Description: "Always return isError:true (failure-counter tests).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "intentional failure"}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crash",
		Description: "Exit the subprocess immediately (crash-detection tests).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		fmt.Fprintln(os.Stderr, "fakeserver: crash tool invoked, exiting")
		os.Exit(1)
		return nil, nil, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Printf("fakeserver: %v", err)
		os.Exit(1)
	}
}
