package netclient

import (
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

// ModelInfo 是模型元数据的兼容名称，实际结构统一使用 apiImpl.Model。
type ModelInfo = apiImpl.Model

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

	endpoints, err := modelEndpoints(c.Config.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("生成模型信息查询地址失败：%w", err)
	}

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	var lastErr error
	for _, endpoint := range endpoints {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+c.Config.APIKey)

		response, err := client.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s 返回 %s", endpoint, response.Status)
			_ = response.Body.Close()
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
		_ = response.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("读取模型信息失败：%w", readErr)
			continue
		}
		models, parseErr := parseModelsResponse(body)
		if parseErr != nil {
			lastErr = parseErr
			continue
		}
		return models, nil
	}
	return nil, fmt.Errorf("获取模型信息失败：%w", lastErr)
}

func modelEndpoints(serverAddress string) ([]string, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverAddress))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("无效服务地址 %q", serverAddress)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if before, ok := strings.CutSuffix(path, "/chat/completions"); ok {
		path = before
	}
	path = strings.TrimRight(path, "/")
	paths := []string{path + "/models"}
	if strings.HasSuffix(path, "/v1") {
		paths = append(paths, strings.TrimSuffix(path, "/v1")+"/models")
	} else {
		paths = append(paths, path+"/v1/models")
	}

	endpoints := make([]string, 0, len(paths))
	for _, candidatePath := range paths {
		candidate := *parsed
		candidate.Path = candidatePath
		candidate.RawPath = ""
		candidate.RawQuery = ""
		candidate.Fragment = ""
		endpoints = append(endpoints, candidate.String())
	}
	return endpoints, nil
}

func parseModelsResponse(body []byte) ([]ModelInfo, error) {
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
			if item.MaxContext <= 0 {
				return 0, fmt.Errorf("模型 %q 未提供有效的上下文窗口", model)
			}
			return item.MaxContext, nil
		}
	}
	return 0, fmt.Errorf("模型列表中未找到 %q", model)
}
