package Tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	netclient "github.com/caliniya/caf/NetClient"
	api "github.com/caliniya/caf/NetClient/APIImpl"
)

func TestModelDiscoveryDistinguishesInputAndNativeSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected capability probe: %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `{"data":[
			{"id":"vision","architecture":{"input_modalities":[" TEXT ","IMAGE"],"output_modalities":["text"]},"native_web_search":true},
			{"id":"text","input_modalities":["text"],"architecture":{"input_modalities":["image"]},"native_web_search":false,"vision_confirmed":true},
			{"id":"search-vision:online","supported_parameters":["tools","web_search_options"],"architecture":{"output_modalities":["image"]},"vision_confirmed":true}
		]}`)
	}))
	defer server.Close()
	models, err := netclient.NewAPIClient(netclient.Options{ServerAddress: server.URL + "/v1"}).FetchModels(context.Background())
	if err != nil || len(models) != 3 {
		t.Fatalf("models: %+v, %v", models, err)
	}
	if m := models[0]; !m.AllowsImages() || m.NativeWebSearch == nil || !*m.NativeWebSearch || m.InputModalities[1] != "image" {
		t.Fatalf("lost declared capabilities: %+v", m)
	}
	if m := models[1]; m.AllowsImages() || m.NativeWebSearch == nil || *m.NativeWebSearch || m.VisionConfirmed {
		t.Fatalf("overrode a negative declaration: %+v", m)
	}
	if m := models[2]; m.AllowsImages() || m.NativeWebSearch != nil || len(m.InputModalities) != 0 || m.VisionConfirmed {
		t.Fatalf("guessed capabilities from output, name, gateway tools or remote opt-in: %+v", m)
	}
}

func TestNativeSearchRulesMatchProviderAndChatRoute(t *testing.T) {
	for _, tc := range []struct {
		address, id string
		search      bool
	}{
		{"https://api.deepseek.com/v1/chat/completions", "deepseek-flash", true},
		{"https://API.PERPLEXITY.AI:443/chat/completions", "sonar-pro", true},
		{"https://openrouter.ai/api/v1", "perplexity/sonar", true},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen-flash", true},
		{"https://workspace-a.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/chat/completions", "qwen-plus", true},
		{"https://workspace-a.cn-beijing.maas.aliyuncs.com.example/compatible-mode/v1", "qwen-plus", false},
		{"https://workspace-a.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/responses", "qwen-plus", false},
		{"https://third-party.example/compatible-mode/v1", "qwen-plus", false},
		{"https://api.deepseek.com.example/v1", "deepseek-flash", false},
		{"http://api.deepseek.com/v1", "deepseek-flash", false},
		{"https://api.deepseek.com:8443/v1", "deepseek-flash", false},
		{"https://api.deepseek.com/other/chat/completions", "deepseek-flash", false},
		{"https://openrouter.ai/api/v1", "perplexity/sonar:online", false},
		{"https://api.perplexity.ai", "local-sonar-pro", false},
	} {
		m := netclient.ResolveModelCapabilities(tc.address, api.Model{ID: tc.id})
		if (m.NativeWebSearch != nil) != tc.search || (tc.search && !*m.NativeWebSearch) {
			t.Errorf("%s %s: %+v", tc.address, tc.id, m)
		}
	}
	m := netclient.ResolveModelCapabilities("https://api.deepseek.com", api.Model{ID: "deepseek-flash", NativeWebSearch: new(false), InputModalities: []string{"text"}, VisionConfirmed: true})
	if m.AllowsImages() || m.NativeWebSearch == nil || *m.NativeWebSearch || m.VisionConfirmed {
		t.Fatalf("official fallback overrode declaration: %+v", m)
	}
}
