package netclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	Client *http.Client
	Config *Options
}

// NewAPIClient 构造客户端。http.Client 不设总超时(长回复会被掐断),
// 超时与取消统一由传入的 context 控制。
func NewAPIClient(opts Options) *APIClient {
	return &APIClient{Client: &http.Client{}, Config: &opts}
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

	url := strings.TrimRight(c.Config.ServerAddress, "/") + "/chat/completions"
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
