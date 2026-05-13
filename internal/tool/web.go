package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/service"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	defaultMaxChars     = 10000
	maxReadWebPageBytes = 2 << 20 // 2MB
	readWebPageTimeout  = 8 * time.Second
)

type readWebPageArgs struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars"`
}

type readWebPageResult struct {
	URL         string `json:"url"`
	FinalURL    string `json:"final_url"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type"`
	Text        string `json:"text"`
	Truncated   bool   `json:"truncated"`
}

func (p *BuiltinProvider) readWebPage(ctx context.Context, args json.RawMessage) (service.ToolResult, error) {
	var input readWebPageArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}

	normalizedURL, err := normalizeURL(input.URL)
	if err != nil {
		return service.ToolResult{}, err
	}

	if input.MaxChars <= 0 {
		input.MaxChars = defaultMaxChars
	}

	client := &http.Client{
		Timeout: readWebPageTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			_, err := normalizeURL(req.URL.String())
			return err
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizedURL, nil)
	if err != nil {
		return service.ToolResult{}, err
	}
	req.Header.Set("User-Agent", "guchat-web-reader/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return service.ToolResult{}, err
	}
	defer resp.Body.Close()

	limitedBody := io.LimitReader(resp.Body, maxReadWebPageBytes+1)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return service.ToolResult{}, err
	}

	truncated := int64(len(body)) > maxReadWebPageBytes
	if truncated {
		body = body[:maxReadWebPageBytes]
	}

	reader, err := charset.NewReader(bytes.NewReader(body), resp.Header.Get("Content-Type"))
	if err == nil {
		body, err = io.ReadAll(reader)
		if err != nil {
			return service.ToolResult{}, err
		}
	}

	text, err := extractHTMLText(body)
	if err != nil {
		return service.ToolResult{}, err
	}

	runes := []rune(text)
	if len(runes) > input.MaxChars {
		text = string(runes[:input.MaxChars])
		truncated = true
	}

	payload, err := json.Marshal(readWebPageResult{
		URL:         normalizedURL,
		FinalURL:    resp.Request.URL.String(),
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Text:        text,
		Truncated:   truncated,
	})
	if err != nil {
		return service.ToolResult{}, err
	}

	return service.ToolResult{
		Name:   ToolReadWebPage,
		Result: payload,
	}, nil
}

// 基础文本提取和空白清理
func extractHTMLText(body []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	var parts []string
	walkHTML(doc, &parts)

	return normalizeWebText(strings.Join(parts, "\n")), nil
}

// 去除连续空白和换行
func normalizeWebText(text string) string {
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n")
}

// 递归遍历HTML节点，并去除部分不必要节点
func walkHTML(node *html.Node, parts *[]string) {
	if node.Type == html.ElementNode {
		switch node.Data {
		case "script", "style", "noscript", "svg":
			return
		}
	}

	if node.Type == html.TextNode {
		text := strings.TrimSpace(node.Data)
		if len(text) > 0 {
			*parts = append(*parts, text)
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, parts)
	}
}

// 分析URL是否合法
func normalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("url is required")
	}

	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	pageURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	if pageURL.Host == "" {
		return "", errors.New("url must be absolute")
	}

	// 一定要去除结尾 ".", 防止使用"localhost."绕过规则
	host := strings.ToLower(strings.TrimSuffix(pageURL.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", errors.New("localhost url is not allowed")
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return "", errors.New("private ip url is not allowed")
		}
	}

	if pageURL.Scheme != "http" && pageURL.Scheme != "https" {
		return "", errors.New("url scheme must be http or https")
	}

	return pageURL.String(), nil
}
