package NetClient

import (
	"net/url"
	"strings"

	api "github.com/caliniya/caf/NetClient/APIImpl"
)

// Explicit catalogue data wins. Compatible gateways can route the same model ID differently.
func ResolveModelCapabilities(address string, m api.Model) api.Model {
	inputs, mode := documentedChatCapabilities(address, m.ID)
	if len(m.InputModalities) == 0 {
		m.InputModalities = inputs
	}
	if len(m.InputModalities) != 0 {
		m.VisionConfirmed = false
	}
	if mode != "" && m.NativeWebSearch == nil {
		m.NativeWebSearch = new(true)
	}
	m.NativeSearchMode = "" // Only the resolved endpoint determines which wire format is safe to send.
	if m.NativeWebSearch != nil && *m.NativeWebSearch {
		m.NativeSearchMode = mode
	}
	return m
}

// An unlisted model is unknown, not text-only. Each rule applies to Chat Completions only.
func documentedChatCapabilities(address, id string) (inputs []string, mode api.NativeSearchMode) {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || u.RawQuery != "" || u.Fragment != "" {
		return nil, ""
	}
	host := strings.ToLower(u.Hostname())
	base := strings.TrimSuffix(strings.TrimRight(u.EscapedPath(), "/"), "/chat/completions")
	workspace, region, _ := strings.Cut(host, ".")
	switch {
	case host == "api.deepseek.com" && (base == "" || base == "/v1"):
		// https://api-docs.deepseek.com/guides/web_tools/ and /api/list-models/
		switch id {
		case "deepseek-flash":
			return []string{"text", "image"}, api.SearchDeepSeek
		case "deepseek-v4-pro":
			return []string{"text"}, ""
		}
	case host == "api.perplexity.ai" && (base == "" || base == "/v1"):
		if id == "sonar" || id == "sonar-pro" {
			return nil, api.SearchSonar
		}
	case host == "openrouter.ai" && base == "/api/v1":
		// These Sonar routes include Perplexity search; the generic :online plugin does not count.
		if id == "perplexity/sonar" || id == "perplexity/sonar-pro" {
			return nil, api.SearchAlwaysOn
		}
	case base == "/compatible-mode/v1" && (host == "dashscope.aliyuncs.com" || (workspace != "" && region == "cn-beijing.maas.aliyuncs.com")):
		// Beijing Qwen models with Chat Completions enable_search: https://help.aliyun.com/zh/model-studio/web-search
		switch id {
		case "qwen-plus", "qwen-plus-latest", "qwen-max", "qwen-flash":
			return nil, api.SearchDashScope
		}
	}
	return nil, ""
}
