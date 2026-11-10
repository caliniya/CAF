package NetClient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	apiImpl "github.com/caliniya/caf/NetClient/APIImpl"
)

// FetchModels 显式查询模型元数据(仅配置页面调用,对话链路不做任何探测)。
func (c *APIClient) FetchModels(ctx context.Context) ([]apiImpl.Model, error) {
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

func parseModelsResponse(body []byte) ([]apiImpl.Model, error) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("解析模型信息失败：%w", err)
	}
	if len(envelope.Data) > 0 {
		var models []apiImpl.Model
		if err := json.Unmarshal(envelope.Data, &models); err == nil {
			return models, nil
		}
		var model apiImpl.Model
		if err := json.Unmarshal(envelope.Data, &model); err == nil && model.ID != "" {
			return []apiImpl.Model{model}, nil
		}
		return nil, errors.New("模型信息的 data 字段格式无效")
	}
	var model apiImpl.Model
	if err := json.Unmarshal(body, &model); err != nil {
		return nil, fmt.Errorf("解析模型信息失败：%w", err)
	}
	if model.ID == "" {
		return nil, errors.New("模型信息响应未包含模型 ID")
	}
	return []apiImpl.Model{model}, nil
}
