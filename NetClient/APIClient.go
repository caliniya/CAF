package netclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// Options 是客户端配置:API 地址、密钥与默认模型。
type Options struct {
	ServerAddress string
	APIKey        string
	Model         string
}

// APIClient 是传输层:负责 HTTP、认证头与 SSE 分帧。
// 本身不持有对话状态——增量通过 onDelta 回调即时上抛,
// 因此同一个客户端可被多个对话并发使用。
type APIClient struct {
	Client         *http.Client
	Config         *Options
	modelInfoReady chan struct{}
	modelInfo      []ModelInfo
	modelInfoErr   error
}

// ModelInfo 描述模型元数据。
type ModelInfo struct {
	ID               string   `json:"id"`
	Object           string   `json:"object"`
	OwnedBy          string   `json:"owned_by"`
	Name             string   `json:"name"`
	ContextWindow    int      `json:"context_window"`
	MaxOutputTokens  int      `json:"max_output_tokens"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

// NewAPIClient 构造客户端并在后台自动尝试获取模型元数据。
// http.Client 不设总超时(长回复会被掐断),超时与取消统一由传入的 context 控制。
func NewAPIClient(opts Options) *APIClient {
	client := &APIClient{
		Client:         &http.Client{},
		Config:         &opts,
		modelInfoReady: make(chan struct{}),
	}
	go client.loadModelInfo()
	return client
}

func (c *APIClient) loadModelInfo() {
	defer close(c.modelInfoReady)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.modelInfo, c.modelInfoErr = c.fetchModelInfo(ctx)
}

func (c *APIClient) fetchModelInfo(ctx context.Context) ([]ModelInfo, error) {
	if c == nil || c.Config == nil {
		return nil, errors.New("客户端未配置")
	}

	endpoint, err := modelEndpointAtRoot(c.Config.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("生成模型信息查询地址失败：%w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.Config.APIKey)

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("获取模型信息失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取模型信息失败：%s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("读取模型信息失败：%w", err)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("解析模型信息失败：%w", err)
	}
	if len(envelope.Data) > 0 {
		var models []ModelInfo
		if err := json.Unmarshal(envelope.Data, &models); err == nil {
			return models, nil
		}
		var model ModelInfo
		if err := json.Unmarshal(envelope.Data, &model); err == nil && model.ID != "" {
			return []ModelInfo{model}, nil
		}
		return nil, errors.New("模型信息的 data 字段格式无效")
	}
	var model ModelInfo
	if err := json.Unmarshal(body, &model); err != nil {
		return nil, fmt.Errorf("解析模型信息失败：%w", err)
	}
	if model.ID == "" {
		return nil, errors.New("模型信息响应未包含模型 ID")
	}
	return []ModelInfo{model}, nil
}

func modelEndpointAtRoot(serverAddress string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverAddress))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("无效服务地址 %q", serverAddress)
	}
	parsed.Path = "/model"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// GetModels 等待初始化时自动发起的模型信息查询，并返回查询结果。
func (c *APIClient) GetModels(ctx context.Context) ([]ModelInfo, error) {
	if c == nil || c.modelInfoReady == nil {
		return nil, errors.New("客户端未初始化")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.modelInfoReady:
	}
	if c.modelInfoErr != nil {
		return nil, c.modelInfoErr
	}
	return append([]ModelInfo(nil), c.modelInfo...), nil
}

// GetModelContextWindow 从自动获取的模型信息中读取上下文窗口容量。
func (c *APIClient) GetModelContextWindow(ctx context.Context, model string) (int, error) {
	models, err := c.GetModels(ctx)
	if err != nil {
		return 0, err
	}
	for _, item := range models {
		if item.ID == model {
			if item.ContextWindow <= 0 {
				return 0, fmt.Errorf("模型 %q 未提供有效的上下文窗口", model)
			}
			return item.ContextWindow, nil
		}
	}
	return 0, fmt.Errorf("模型列表中未找到 %q", model)
}

// StreamChat 发起一次流式对话:编码请求 → POST → 按行分帧 SSE →
// 每行交给协议解码 → 通过 onDelta 上抛内部增量。
// 阻塞直到流结束(ctx 取消或收到 Done)后返回。
func (c *APIClient) StreamChat(
	ctx context.Context,
	req apiImpl.ChatRequest,
	proto apiImpl.Protocol,
	onDelta func(apiImpl.Delta),
) error {
	if c == nil || c.Config == nil {
		return errors.New("客户端未配置")
	}
	body, err := proto.EncodeRequest(req)
	if err != nil {
		return fmt.Errorf("编码请求失败：%w", err)
	}

	url := strings.TrimRight(c.Config.ServerAddress, "/")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.Config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Config.APIKey)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		var apiErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(snippet, &apiErr) == nil && apiErr.Message != "" {
			return fmt.Errorf("模型服务返回 %s：%s", resp.Status, apiErr.Message)
		}
		return fmt.Errorf("模型服务返回 %s：%s", resp.Status, string(snippet))
	}

	scanner := bufio.NewScanner(resp.Body)
	// SSE 单行可能超过默认 64KB(长文本/大参数),放宽到 1MB。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue // 空行分隔符、注释行、event: 行都跳过
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		deltas, err := proto.DecodeEvent(payload)
		if err != nil {
			return fmt.Errorf("解析响应失败：%w", err)
		}
		for _, d := range deltas {
			if onDelta != nil {
				onDelta(d)
			}
			if d.Done {
				return nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("读取响应流失败：%w", err)
	}
	return nil
}
