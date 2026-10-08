package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatelessDiscovery(t *testing.T) {
	server := httptest.NewServer(Handler(New(t.Context())))
	defer server.Close()
	for range 2 {
		body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`)
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
		request.Header.Set("Mcp-Method", "tools/list")
		// #nosec G704 -- URL belongs to this test's loopback httptest server.
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", response.StatusCode, data)
		}
		if response.Header.Get("Mcp-Session-Id") != "" {
			t.Fatal("stateless endpoint issued a session ID")
		}
		var result struct {
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		err = json.Unmarshal(data, &result)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Result.Tools) != 5 {
			t.Fatalf("expected five tools without an initialization handshake: %s", data)
		}
	}
}
