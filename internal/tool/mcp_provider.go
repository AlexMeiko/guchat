package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AlexMeiko/guchat/internal/service"
)

const (
	MCPAuthNone   string = "none"
	MCPAuthQuery  string = "query"
	MCPAuthHeader string = "header"
)

const MCPProtocolVersion = "2025-11-25"

type MCPProviderConfig struct {
	Name      string
	URL       string
	AuthType  string
	AuthField string
	AuthKey   string
}

type MCPProvider struct {
	name      string
	endpoint  string
	authType  string
	authField string
	authKey   string
	client    *http.Client

	protocolVersion string
	sessionID       string
	initialized     bool
	mu              sync.Mutex
}

type MCPTools struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func NewMCPProvider(cfg MCPProviderConfig) *MCPProvider {
	return &MCPProvider{
		name:      cfg.Name,
		endpoint:  cfg.URL,
		authType:  cfg.AuthType,
		authField: cfg.AuthField,
		authKey:   cfg.AuthKey,
		client:    &http.Client{Timeout: 180 * time.Second},

		protocolVersion: MCPProtocolVersion,
	}
}

func (p *MCPProvider) Name() string {
	return p.name
}

func (p *MCPProvider) CallTool(ctx context.Context, user service.UserContext, name string, args json.RawMessage) (service.ToolResult, error) {
	prefix := p.name + "."

	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return service.ToolResult{}, service.ErrToolNotFound
	}

	remoteName := name[len(prefix):]

	result, err := p.rpc(ctx, "tools/call", map[string]any{
		"name":      remoteName,
		"arguments": json.RawMessage(args),
	})
	if err != nil {
		return service.ToolResult{}, err
	}

	return service.ToolResult{
		Name:   name,
		Result: result,
	}, nil
}

func (p *MCPProvider) ListTools(ctx context.Context, user service.UserContext) ([]service.ToolDefinition, error) {
	result, err := p.rpc(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Tools []MCPTools `json:"tools"`
	}

	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, err
	}

	tools := make([]service.ToolDefinition, 0, len(payload.Tools))
	for _, tool := range payload.Tools {
		if tool.Name == "" {
			continue
		}

		tools = append(tools, service.ToolDefinition{
			Name:        p.name + "." + tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}

	return tools, nil
}

func (p *MCPProvider) applyAuth(req *http.Request) {
	switch p.authType {
	case MCPAuthHeader:
		if p.authField != "" && p.authKey != "" {
			req.Header.Add(p.authField, "Bearer "+p.authKey)
		}
	case MCPAuthQuery:
		if p.authField != "" && p.authKey != "" {
			q := req.URL.Query()
			q.Set(p.authField, p.authKey)
			req.URL.RawQuery = q.Encode()
		}
	}
}

// ensureInitialized 执行 MCP 初始化握手，并保存服务端返回的 session id
func (p *MCPProvider) ensureInitialized(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	result, headers, _, err := p.rpcRaw(ctx, "initialize", map[string]any{
		"protocolVersion": p.protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "guchat",
			"version": "0.1.0",
		},
	}, false)
	if err != nil {
		return err
	}

	var payload struct {
		ProtocolVersion string `json:"protocolVersion"`
	}

	if err := json.Unmarshal(result, &payload); err != nil {
		return err
	}

	if payload.ProtocolVersion != "" {
		p.protocolVersion = payload.ProtocolVersion
	}

	p.sessionID = headers.Get("MCP-Session-Id")

	if err := p.sendInitializedNotification(ctx); err != nil {
		p.sessionID = ""
		p.initialized = false
		return err
	}

	p.initialized = true
	return nil
}

// sendInitializedNotification 通知 MCP 服务端初始化阶段已经完成
// notification 没有 id，服务端通常只返回空的 2xx 响应
func (p *MCPProvider) sendInitializedNotification(ctx context.Context) error {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", p.protocolVersion)

	if p.sessionID != "" {
		req.Header.Set("MCP-Session-Id", p.sessionID)
	}

	p.applyAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mcp initialized notification error: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// rpc 发送带 session 的 MCP JSON-RPC 请求；如果 session 失效，会重新初始化后重试一次
func (p *MCPProvider) rpc(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if err := p.ensureInitialized(ctx); err != nil {
		return nil, err
	}

	result, _, statusCode, err := p.rpcRaw(ctx, method, params, true)
	if err == nil {
		return result, nil
	}

	if statusCode == http.StatusBadRequest || statusCode == http.StatusNotFound {
		p.mu.Lock()
		p.initialized = false
		p.sessionID = ""
		p.mu.Unlock()

		if initErr := p.ensureInitialized(ctx); initErr != nil {
			return nil, initErr
		}

		result, _, _, err = p.rpcRaw(ctx, method, params, true)
	}

	return result, err
}

// rpcRaw 发送一次 MCP JSON-RPC 请求，并返回原始 result、响应头和状态码
func (p *MCPProvider) rpcRaw(ctx context.Context, method string, params any, withSession bool) (json.RawMessage, http.Header, int, error) {
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return nil, nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", p.protocolVersion)

	if withSession && p.sessionID != "" {
		req.Header.Set("MCP-Session-Id", p.sessionID)
	}

	p.applyAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, resp.StatusCode, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("mcp rpc error: status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	rpcPayload, err := decodeMCPRPCPayload(resp, respBody)
	if err != nil {
		return nil, resp.Header, resp.StatusCode, err
	}

	if err := json.Unmarshal(rpcPayload, &parsed); err != nil {
		return nil, resp.Header, resp.StatusCode, err
	}

	if parsed.Error != nil {
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("mcp rpc error: status %d: %s", parsed.Error.Code, parsed.Error.Message)
	}

	if len(parsed.Result) == 0 {
		return json.RawMessage("null"), resp.Header, resp.StatusCode, nil
	}

	return parsed.Result, resp.Header, resp.StatusCode, nil
}

// decodeMCPRPCPayload 负责从 text/event-stream 响应中提取 JSON-RPC payload，同时不会影响 application/json
func decodeMCPRPCPayload(resp *http.Response, body []byte) ([]byte, error) {
	contentType := resp.Header.Get("Content-Type")
	trimmed := strings.TrimSpace(string(body))

	if !strings.HasPrefix(contentType, "text/event-stream") &&
		!strings.HasPrefix(trimmed, "event:") &&
		!strings.HasPrefix(trimmed, "data:") {
		return body, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	dataLines := make([]string, 0, 4)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(dataLines) == 0 {
		return nil, fmt.Errorf("mcp sse response missing data")
	}

	return []byte(strings.Join(dataLines, "\n")), nil
}
