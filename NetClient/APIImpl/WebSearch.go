package APIImpl

import (
	"encoding/json"
	"net/url"
	"strings"
)

// NativeSearchMode identifies a verified Chat Completions wire format, not a tool for CAF to execute.
type NativeSearchMode string

const (
	SearchDeepSeek  NativeSearchMode = "deepseek"
	SearchDashScope NativeSearchMode = "dashscope"
	SearchSonar     NativeSearchMode = "sonar"
	SearchAlwaysOn  NativeSearchMode = "always" // OpenRouter Sonar has no documented off switch
)

type SearchSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// A non-nil receipt with no URLs means the service confirmed a completed search without returning sources.
type SearchReceipt struct {
	Sources []SearchSource `json:"sources,omitempty"`
}

func (r *SearchReceipt) add(source SearchSource) {
	source.URL = strings.TrimSpace(source.URL)
	u, err := url.Parse(source.URL)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return
	}
	for i := range r.Sources {
		if r.Sources[i].URL == source.URL {
			if r.Sources[i].Title == "" {
				r.Sources[i].Title = source.Title
			}
			return
		}
	}
	if len(r.Sources) < 64 {
		r.Sources = append(r.Sources, source)
	}
}

func (r *SearchReceipt) Merge(next *SearchReceipt) *SearchReceipt {
	if next == nil {
		return r
	}
	if r == nil {
		r = &SearchReceipt{}
	}
	for _, source := range next.Sources {
		r.add(source)
	}
	return r
}

func (api *ChatCompletionAPI) configureSearch(body *ChatCompletionsRequest, req ChatRequest) {
	api.searchMode = ""
	if req.WebSearch || req.SearchMode == SearchAlwaysOn {
		api.searchMode = req.SearchMode
	}
	switch req.SearchMode {
	case SearchDeepSeek:
		if req.WebSearch {
			body.Tools = append(body.Tools, CCChatTool{Type: "deepseek_web_search"})
		}
	case SearchDashScope:
		body.EnableSearch = new(req.WebSearch)
	case SearchSonar:
		body.ReasoningEffort = "" // these are the non-reasoning Sonar models
		body.DisableSearch = new(!req.WebSearch)
		body.EnableSearchClassifier = req.WebSearch // let Sonar decide whether the question needs retrieval
	case SearchAlwaysOn:
		body.ReasoningEffort = ""
		body.Tools = nil // these native-only Sonar routes do not accept function tools
	}
}

type searchCall struct {
	Type    string         `json:"type"`
	Status  string         `json:"status"`
	Sources []SearchSource `json:"sources"`
}
type searchStep struct {
	Reasoning string       `json:"reasoning_content"`
	Calls     []searchCall `json:"calls"`
}

// Optional response extensions must not prevent delivery of ordinary text from a compatible gateway.
func searchEvidence(mode NativeSearchMode, chunk *ccStreamChunk) (delta Delta) {
	if mode == "" {
		return
	}
	r := &SearchReceipt{}
	completed := false
	if mode == SearchSonar || mode == SearchAlwaysOn || mode == SearchDashScope {
		var results []SearchSource
		var citations []string
		var info struct {
			Results []SearchSource `json:"search_results"`
		}
		if len(chunk.SearchResults) != 0 {
			_ = json.Unmarshal(chunk.SearchResults, &results)
		}
		if len(chunk.Citations) != 0 {
			_ = json.Unmarshal(chunk.Citations, &citations)
		}
		if len(chunk.SearchInfo) != 0 {
			_ = json.Unmarshal(chunk.SearchInfo, &info)
		}
		// Sonar's numeric references follow citations order; results can supply titles and additional sources.
		for _, link := range citations {
			r.add(SearchSource{URL: link})
		}
		for _, source := range append(results, info.Results...) {
			r.add(source)
		}
	}
	for _, choice := range chunk.Choices {
		if len(choice.Delta.Annotations) != 0 {
			var items []struct {
				Type     string       `json:"type"`
				Citation SearchSource `json:"url_citation"`
			}
			if json.Unmarshal(choice.Delta.Annotations, &items) == nil {
				for _, item := range items {
					if item.Type == "url_citation" {
						r.add(item.Citation)
					}
				}
			}
		}
		if mode == SearchDeepSeek && len(choice.Delta.ServerToolCalls) != 0 {
			var steps []searchStep
			if json.Unmarshal(choice.Delta.ServerToolCalls, &steps) == nil && len(steps) != 0 {
				delta.ServerToolCalls = choice.Delta.ServerToolCalls
				for _, step := range steps {
					for _, call := range step.Calls {
						if call.Type == "deepseek_web_search" && call.Status == "completed" {
							completed = true
							for _, source := range call.Sources {
								r.add(source)
							}
						}
					}
				}
			}
		}
	}
	if completed || len(r.Sources) > 0 {
		delta.WebSearch = r
	}
	return
}
