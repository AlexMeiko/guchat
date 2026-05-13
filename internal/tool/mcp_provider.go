package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
	Transport string
	Command   string
	Args      []string
	Env       []string
}

type MCPProvider struct {
	name   string
	client mcpClient
}

type MCPTools struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpClient interface {
	Request(ctx context.Context, method string, params any) (json.RawMessage, error)
	//Close() error
}

type httpMCPClient struct {
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

type stdioMCPClient struct {
	command string
	args    []string
	env     []string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader // Reader底层可能一次从 stdout 管道里多拿一些字节放进自己的缓冲区，所以不开临时变量。
	// 具体见 https://go.googlesource.com/go/+/refs/tags/go1.24.5/src/bufio/bufio.go， func (b *Reader) fill()部分

	protocolVersion string
	initialized     bool
	nextID          int64
	mu              sync.Mutex
}

func NewMCPProvider(cfg MCPProviderConfig) *MCPProvider {
	var client mcpClient

	switch cfg.Transport {
	case "http":
		client = newHTTPMCPClient(cfg)
	case "stdio":
		client = newStdioMCPClient(cfg)
	default:
		client = newHTTPMCPClient(cfg)
	}
	return &MCPProvider{
		name:   cfg.Name,
		client: client,
	}
}

func newHTTPMCPClient(cfg MCPProviderConfig) *httpMCPClient {
	return &httpMCPClient{
		endpoint:        cfg.URL,
		authType:        cfg.AuthType,
		authField:       cfg.AuthField,
		authKey:         cfg.AuthKey,
		client:          &http.Client{Timeout: 180 * time.Second},
		protocolVersion: MCPProtocolVersion,
	}
}

func (c *httpMCPClient) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.rpc(ctx, method, params)
}

//func (c *httpMCPClient) Close() error {
//	return nil
//}

func (p *MCPProvider) Name() string {
	return p.name
}

func (p *MCPProvider) CallTool(ctx context.Context, user service.UserContext, name string, args json.RawMessage) (service.ToolResult, error) {
	remoteName, ok := strings.CutPrefix(name, p.name+".")
	if !ok || remoteName == "" {
		return service.ToolResult{}, service.ErrToolNotFound
	}

	result, err := p.client.Request(ctx, "tools/call", map[string]any{
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
	result, err := p.client.Request(ctx, "tools/list", map[string]any{})
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

func (c *httpMCPClient) applyAuth(req *http.Request) {
	switch c.authType {
	case MCPAuthHeader:
		if c.authField != "" && c.authKey != "" {
			req.Header.Add(c.authField, "Bearer "+c.authKey)
		}
	case MCPAuthQuery:
		if c.authField != "" && c.authKey != "" {
			q := req.URL.Query()
			q.Set(c.authField, c.authKey)
			req.URL.RawQuery = q.Encode()
		}
	}
}

// ensureInitialized 执行 MCP 初始化握手，并保存服务端返回的 session id
func (c *httpMCPClient) ensureInitialized(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	result, headers, _, err := c.rpcRaw(ctx, "initialize", map[string]any{
		"protocolVersion": c.protocolVersion,
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
		c.protocolVersion = payload.ProtocolVersion
	}

	c.sessionID = headers.Get("MCP-Session-Id")

	if err := c.sendInitializedNotification(ctx); err != nil {
		c.sessionID = ""
		c.initialized = false
		return err
	}

	c.initialized = true
	return nil
}

// sendInitializedNotification 通知 MCP 服务端初始化阶段已经完成
// notification 没有 id，服务端通常只返回空的 2xx 响应
func (c *httpMCPClient) sendInitializedNotification(ctx context.Context) error {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return hideMCPURL(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.protocolVersion)

	if c.sessionID != "" {
		req.Header.Set("MCP-Session-Id", c.sessionID)
	}

	c.applyAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return hideMCPURL(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mcp initialized notification error: status %d: %s", resp.StatusCode, c.hideSensitiveText(string(respBody)))
	}

	return nil
}

// rpc 发送带 session 的 MCP JSON-RPC 请求；如果 session 失效，会重新初始化后重试一次
func (c *httpMCPClient) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := c.ensureInitialized(ctx); err != nil {
		return nil, err
	}

	result, _, statusCode, err := c.rpcRaw(ctx, method, params, true)
	if err == nil {
		return result, nil
	}

	if statusCode == http.StatusBadRequest || statusCode == http.StatusNotFound {
		c.mu.Lock()
		c.initialized = false
		c.sessionID = ""
		c.mu.Unlock()

		if initErr := c.ensureInitialized(ctx); initErr != nil {
			return nil, initErr
		}

		result, _, _, err = c.rpcRaw(ctx, method, params, true)
	}

	return result, err
}

// rpcRaw 发送一次 MCP JSON-RPC 请求，并返回原始 result、响应头和状态码
func (c *httpMCPClient) rpcRaw(ctx context.Context, method string, params any, withSession bool) (json.RawMessage, http.Header, int, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return nil, nil, 0, hideMCPURL(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", c.protocolVersion)

	if withSession && c.sessionID != "" {
		req.Header.Set("MCP-Session-Id", c.sessionID)
	}

	c.applyAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, 0, hideMCPURL(err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, resp.StatusCode, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("mcp rpc error: status %d: %s", resp.StatusCode, c.hideSensitiveText(string(respBody)))
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
		return nil, resp.Header, resp.StatusCode, fmt.Errorf("mcp rpc error: status %d: %s", parsed.Error.Code, c.hideSensitiveText(parsed.Error.Message))
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
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
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

func hideMCPURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (c *httpMCPClient) hideSensitiveText(text string) string {
	if c.authKey == "" {
		return text
	}

	text = strings.ReplaceAll(text, c.authKey, "<redacted>")
	text = strings.ReplaceAll(text, url.QueryEscape(c.authKey), "<redacted>")
	return text
}

func newStdioMCPClient(cfg MCPProviderConfig) *stdioMCPClient {
	return &stdioMCPClient{
		command:         cfg.Command,
		args:            cfg.Args,
		env:             cfg.Env,
		protocolVersion: MCPProtocolVersion,
	}
}

func (c *stdioMCPClient) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sendRequestLocked(method, params)
}

func (c *stdioMCPClient) startLocked() error {
	if c.cmd != nil {
		return nil
	}

	cmd := exec.Command(c.command, c.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	cmd.Stderr = io.Discard
	cmd.Env = append(os.Environ(), c.env...)

	if err := cmd.Start(); err != nil {
		return err
	}

	c.cmd = cmd

	c.stdin = stdin
	c.stdout = stdout
	c.reader = bufio.NewReader(stdout)

	return nil
}

func (c *stdioMCPClient) ensureInitialized() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	if err := c.startLocked(); err != nil {
		return err
	}

	result, err := c.sendRequestLocked("initialize", map[string]any{
		"protocolVersion": c.protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "guchat",
			"version": "0.1.0",
		},
	})
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
		c.protocolVersion = payload.ProtocolVersion
	}

	if err := c.sendNotificationLocked("notifications/initialized", nil); err != nil {
		return err
	}

	c.initialized = true
	return nil
}

func (c *stdioMCPClient) sendRequestLocked(method string, params any) (json.RawMessage, error) {
	c.nextID++
	id := c.nextID

	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	if _, err := c.stdin.Write(append(payload, '\n')); err != nil {
		return nil, err
	}

	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var resp struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(line, &resp); err != nil {
			return nil, err
		}

		if len(resp.ID) == 0 {
			continue
		}

		var respID int64
		if err := json.Unmarshal(resp.ID, &respID); err != nil || respID != id {
			continue
		}

		if resp.Error != nil {
			return nil, fmt.Errorf("mcp rpc error code %d: %s", resp.Error.Code, resp.Error.Message)
		}

		if len(resp.Result) == 0 {
			return json.RawMessage("null"), nil
		}

		return resp.Result, nil
	}
}

func (c *stdioMCPClient) sendNotificationLocked(method string, params any) error {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		body["params"] = params
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	_, err = c.stdin.Write(append(payload, '\n'))
	return err
}

//func (c *stdioMCPClient) Close() error {
//	c.mu.Lock()
//	defer c.mu.Unlock()
//
//	if c.stdin != nil {
//		_ = c.stdin.Close()
//	}
//	if c.stdout != nil {
//		_ = c.stdout.Close()
//	}
//	if c.cmd != nil && c.cmd.Process != nil {
//		_ = c.cmd.Process.Kill()
//	}
//
//	c.cmd = nil
//	c.stdin = nil
//	c.stdout = nil
//	c.reader = nil
//	c.initialized = false
//
//	return nil
//}
