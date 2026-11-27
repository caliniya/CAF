package Backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	netclient "github.com/caliniya/caf/NetClient"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	identity "github.com/caliniya/caf/utils/id"
	texts "github.com/caliniya/caf/utils/text"
)

const (
	compactionAttempts     = 3
	compactionRestoreCalls = 8
	compactionTimeout      = 5 * time.Minute
)

type compactionCandidate struct {
	Summary string                             `json:"summary"`
	Restore []chatstruct.CompactionRestoreCall `json:"restore"`
}

type compactionSource struct {
	*chatstruct.CompactionPlan
	State        []string   `json:"state"`
	RestoreTools []api.Tool `json:"restoreTools"`
}

func compactionSummaryBudget(model api.Model) int {
	budget := 4096
	if model.MaxContext > 0 {
		budget = min(budget, max(128, model.MaxContext/8))
	}
	if model.MaxOutputTokens > 0 {
		budget = min(budget, model.MaxOutputTokens)
	}
	return budget
}

func compactionReviewBudget(model api.Model) int {
	return min(1024, compactionSummaryBudget(model))
}

func compactionRestoreBudget(model api.Model) int {
	if model.MaxContext > 0 {
		return min(4096, max(128, model.MaxContext/8))
	}
	return 4096
}

func compactionOutputReserve(model api.Model) int {
	if model.MaxOutputTokens > 0 {
		return model.MaxOutputTokens
	}
	if model.MaxContext > 0 {
		return min(4096, max(128, model.MaxContext/8))
	}
	return 4096
}

func compactionKeepBudget(model api.Model, prefix []api.ChatMessage, tools []api.Tool) int {
	if model.MaxContext <= 0 {
		return chatstruct.CompactionKeepTokens
	}
	fixed := api.RequestTokens(api.ChatRequest{Messages: prefix, Tools: tools})
	available := model.MaxContext - fixed - compactionSummaryBudget(model) - compactionRestoreBudget(model) - compactionOutputReserve(model) - 256
	return min(chatstruct.CompactionKeepTokens, max(1, available))
}

func compactionReadTool(name string) bool {
	switch name {
	case "ReadFile", "ListDirectory", "SearchFiles", "ReadMemory", "SearchMemory", "ListMemories":
		return true
	default:
		return false
	}
}

func compactionTools(run *toolExecutionRun) []api.Tool {
	tools := make([]api.Tool, 0, 6)
	for _, declaration := range run.enabled {
		if !compactionReadTool(declaration.Name) {
			continue
		}
		tool := agenttool.Tools[declaration.Name]
		if tool == nil || tool.Meta() == nil || tool.Meta().Permission != agenttool.PermissionReadOnly {
			continue
		}
		if _, reversible := tool.(agenttool.ReversibleTool); reversible {
			continue
		}
		if run.options != nil && run.options.checkTool != nil && run.options.checkTool(declaration.Name) != nil {
			continue
		}
		tools = append(tools, declaration)
	}
	return tools
}

func compactionJSON(text string, result any) error {
	raw := strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if strings.HasPrefix(raw, "```") {
		first, last := strings.IndexByte(raw, '\n'), strings.LastIndex(raw, "```")
		if first < 0 || last <= first || strings.TrimSpace(raw[last+3:]) != "" {
			return errors.New("JSON 代码块不完整")
		}
		raw = strings.TrimSpace(raw[first+1 : last])
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("JSON 之后包含额外内容")
	}
	return nil
}

func compactionRequest(ctx context.Context, client *netclient.APIClient, request api.ChatRequest, options *chatExecutionOptions) (string, error) {
	var output strings.Builder
	var usage *api.TokenUsage
	called, oversized := false, false
	err := client.StreamChat(ctx, request, &api.ChatCompletionAPI{}, func(delta api.Delta) {
		if output.Len()+len(delta.Content) > 1024*1024 {
			oversized = true
		} else {
			output.WriteString(delta.Content)
		}
		called = called || len(delta.ToolCalls) > 0
		if delta.Usage != nil {
			copy := *delta.Usage
			usage = &copy
		}
	})
	if options != nil && options.recordCompactionUsage != nil {
		options.recordCompactionUsage(usage)
	}
	if err != nil {
		return output.String(), err
	}
	if called {
		return output.String(), errors.New("压缩请求只能返回文本，不能执行工具调用")
	}
	if oversized {
		return "", errors.New("压缩响应超过大小限制")
	}
	return output.String(), ctx.Err()
}

func validateCompactionCandidate(candidate *compactionCandidate, allowed []api.Tool, run *toolExecutionRun, model api.Model) error {
	candidate.Summary = strings.TrimSpace(candidate.Summary)
	if candidate.Summary == "" {
		return errors.New("压缩摘要为空")
	}
	if api.EstimateTextTokens(candidate.Summary) > compactionSummaryBudget(model) {
		return errors.New("压缩摘要超过输出预算")
	}
	if len(candidate.Restore) > compactionRestoreCalls {
		return fmt.Errorf("恢复读取不能超过 %d 项", compactionRestoreCalls)
	}
	if candidate.Restore == nil {
		candidate.Restore = []chatstruct.CompactionRestoreCall{}
	}
	seen := make(map[string]bool)
	for index := range candidate.Restore {
		call := &candidate.Restore[index]
		if !slices.ContainsFunc(allowed, func(tool api.Tool) bool { return tool.Name == call.Tool }) {
			return fmt.Errorf("恢复清单包含不可用或非只读工具 %q", call.Tool)
		}
		var args map[string]any
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args == nil {
			return fmt.Errorf("恢复工具 %q 的 arguments 必须为 JSON 对象", call.Tool)
		}
		canonical, _ := json.Marshal(args)
		call.Arguments = canonical
		prepared, err := run.prepare(api.ToolCall{Name: call.Tool, Arguments: string(canonical)})
		if err != nil {
			return fmt.Errorf("恢复工具 %q：%w", call.Tool, err)
		}
		if prepared.permission.Level != agenttool.PermissionReadOnly {
			return errors.New("恢复读取不能请求更高权限")
		}
		key := call.Tool + string(canonical)
		if seen[key] {
			return errors.New("恢复清单包含重复读取")
		}
		seen[key] = true
	}
	body, _ := json.Marshal(candidate.Restore)
	if api.EstimateTextTokens(string(body)) > compactionRestoreBudget(model)/2 {
		return errors.New("恢复清单的参数超过上下文预算")
	}
	return nil
}

func checkCompactionRequestBudget(request api.ChatRequest, model api.Model) error {
	if model.MaxContext > 0 && api.RequestTokens(request)+request.MaxTokens+128 > model.MaxContext {
		return errors.New("压缩请求及输出预留超过模型上下文窗口，请检查模型窗口配置或缩短过大的单条消息")
	}
	return nil
}

func compactionReviewPassed(raw string) (bool, string, error) {
	var review struct {
		Passed *bool    `json:"passed"`
		Issues []string `json:"issues"`
	}
	if err := compactionJSON(raw, &review); err != nil {
		return false, "", fmt.Errorf("检查结果解析失败：%w", err)
	}
	if review.Passed == nil {
		return false, "", errors.New("检查结果缺少 passed")
	}
	return *review.Passed && len(review.Issues) == 0, strings.Join(review.Issues, "；"), nil
}

func compactionReducesHistory(plan *chatstruct.CompactionPlan, candidate *compactionCandidate) bool {
	before := api.EstimateTextTokens(texts.CompactionSummary + plan.PreviousSummary)
	for _, turn := range plan.History {
		before += api.MessagesTokens(turn.Messages)
	}
	return api.EstimateTextTokens(texts.CompactionSummary+candidate.Summary) < before
}

// summarizeChat 重试始终使用同一份 source 和相同的总结请求，不把检查意见
// 加入重试，也不沿用前一次模型对话。每次检查都是独立的新请求。
func summarizeChat(ctx context.Context, client *netclient.APIClient, source []byte, plan *chatstruct.CompactionPlan, allowed []api.Tool, run *toolExecutionRun, model api.Model) (*compactionCandidate, error) {
	request := api.ChatRequest{Model: model.ID, Thinking: run.session.ReasoningEffort, MaxTokens: compactionSummaryBudget(model),
		Messages: []api.ChatMessage{{Role: "system", Content: texts.CompactionSummarize}, {Role: "user", Content: string(source)}}}
	if err := checkCompactionRequestBudget(request, model); err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < compactionAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := compactionRequest(ctx, client, request, run.options)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			continue
		}
		var candidate compactionCandidate
		if err := compactionJSON(raw, &candidate); err != nil {
			lastErr = fmt.Errorf("摘要解析失败：%w；模型输出：%q", err, shortCompactionOutput(raw))
			continue
		}
		if err := validateCompactionCandidate(&candidate, allowed, run, model); err != nil {
			lastErr = err
			continue
		}
		if !compactionReducesHistory(plan, &candidate) {
			lastErr = errors.New("摘要未减少上下文用量")
			continue
		}
		reviewInput, _ := json.Marshal(struct {
			Source    json.RawMessage     `json:"source"`
			Candidate compactionCandidate `json:"candidate"`
		}{Source: source, Candidate: candidate})
		review := api.ChatRequest{Model: model.ID, Thinking: request.Thinking, MaxTokens: compactionReviewBudget(model),
			Messages: []api.ChatMessage{{Role: "system", Content: texts.CompactionReview}, {Role: "user", Content: string(reviewInput)}}}
		if err := checkCompactionRequestBudget(review, model); err != nil {
			return nil, err
		}
		rawReview, err := compactionRequest(ctx, client, review, run.options)
		if err != nil {
			lastErr = err
			continue
		}
		passed, issues, err := compactionReviewPassed(rawReview)
		if err != nil {
			lastErr = fmt.Errorf("%w；模型输出：%q", err, shortCompactionOutput(rawReview))
			continue
		}
		if !passed {
			lastErr = fmt.Errorf("压缩检查未通过：%s", issues)
			continue
		}
		return &candidate, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("对话压缩尝试 %d 次仍未通过，原始历史已保留：%w", compactionAttempts, lastErr)
}

func shortCompactionOutput(value string) string {
	if len(value) <= 2000 {
		return value
	}
	end := 2000
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "…"
}

func (c *Core) compactChatSnapshot(run *toolExecutionRun, marker *chatstruct.ChatNode, model api.Model, prefix []api.ChatMessage) error {
	low, _ := c.chatDefaults().Compaction.thresholds(model.MaxContext)
	keep := min(compactionKeepBudget(model, prefix, run.enabled), max(1, low/2))
	plan, err := run.session.Tree.NewCompactionPlan(keep)
	if err != nil {
		return err
	}
	return c.compactChatPlan(run, marker, model, keep, plan)
}

func (c *Core) compactChatPlan(run *toolExecutionRun, marker *chatstruct.ChatNode, model api.Model, keep int, plan *chatstruct.CompactionPlan) error {
	allowed := compactionTools(run)
	workspace := fmt.Sprintf(texts.ChatWorkspace, run.context.WorkSpace)
	state := []string{workspace, toolPolicyPrompt(run.strict, run.context, run.session.ToolConfig)}
	if notice, err := toolChangesNotice(run.session); err != nil {
		return err
	} else if notice != "" {
		state = append(state, notice)
	}
	if run.options != nil && run.options.systemPrompt != "" {
		state = append(state, run.options.systemPrompt)
	}
	if mode, err := chatCurrentToolMode(run.session); err != nil {
		return err
	} else if mode != "" {
		state = append(state, mode)
	}
	source, err := json.Marshal(compactionSource{CompactionPlan: plan, State: state, RestoreTools: allowed})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(run.context.Context, compactionTimeout)
	defer cancel()
	candidate, err := summarizeChat(ctx, run.session.Client, source, plan, allowed, run, model)
	if err != nil {
		return err
	}
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	server.mu.Lock()
	currentPlan, planErr := run.session.Tree.NewCompactionPlan(keep)
	beforePlan, _ := json.Marshal(plan)
	afterPlan, _ := json.Marshal(currentPlan)
	if ctx.Err() != nil || server.sending[run.session.ID] != run.generation || planErr != nil || !bytes.Equal(beforePlan, afterPlan) {
		server.mu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("压缩期间对话来源发生变化，候选摘要未保存")
	}
	path, err := run.session.Tree.CurrentPath()
	if err != nil || !slices.Contains(path, marker) {
		server.mu.Unlock()
		return errors.New("压缩标记节点不在当前分支中")
	}
	previous, skipped, updated := marker.Compaction, marker.SkippedCompactionAt, run.session.UpdatedAt
	marker.SkippedCompactionAt = 0
	marker.Compaction = &chatstruct.CompactionData{Start: plan.Start, Summary: candidate.Summary, Restore: candidate.Restore}
	run.session.UpdatedAt = time.Now().UTC()
	err = c.saveExecutionChatLocked(run.directory, run.session, run.options)
	if err != nil {
		marker.Compaction, marker.SkippedCompactionAt, run.session.UpdatedAt = previous, skipped, updated
		err = errors.Join(err, c.saveExecutionChatLocked(run.directory, run.session, run.options))
	}
	server.mu.Unlock()
	if err != nil {
		return fmt.Errorf("保存压缩记录失败：%w", err)
	}
	restoreContext := *run.context
	restoreContext.Context = ctx
	restoreRun := *run
	restoreRun.context = &restoreContext
	return c.restoreCompactionReads(&restoreRun, model)
}

func truncateCompactionResult(content string, budget int) string {
	if api.EstimateTextTokens(content) <= budget {
		return content
	}
	budget -= api.EstimateTextTokens(texts.CompactionTruncated)
	if budget <= 0 {
		return texts.CompactionTruncated
	}
	tokens, ascii, end := 0, 0, 0
	for offset, value := range content {
		cost := 2
		if value < utf8.RuneSelf {
			cost = 0
			if ascii%4 == 0 {
				cost = 1
			}
			ascii++
		} else {
			ascii = 0
		}
		if tokens+cost > budget {
			break
		}
		tokens += cost
		_, width := utf8.DecodeRuneInString(content[offset:])
		end = offset + width
	}
	return content[:end] + texts.CompactionTruncated
}

// boundCompactionResults 检查实际发送的 JSON 和宿主说明，而不是只估算原文。
// HTML 字符、控制字符和错误元数据经 JSON 转义后也必须落在恢复预算内。
func boundCompactionResults(results []api.ChatMessage, budget int) error {
	if len(results) == 0 {
		return nil
	}
	contents, limits := make([]string, len(results)), make([]int, len(results))
	for index, result := range results {
		if result.Role == "tool" {
			contents[index] = result.Content
			limits[index] = api.EstimateTextTokens(result.Content)
		}
	}
	for {
		body, err := json.Marshal(results)
		if err != nil {
			return fmt.Errorf("编码压缩恢复结果：%w", err)
		}
		if api.MessageTokens(api.ChatMessage{Role: "system", Content: texts.CompactionRestored + string(body)}) <= budget {
			return nil
		}
		largest, largestTokens := -1, 0
		for index, result := range results {
			if limits[index] <= 0 {
				continue
			}
			encoded, _ := json.Marshal(result.Content)
			if tokens := api.EstimateTextTokens(string(encoded)); tokens > largestTokens {
				largest, largestTokens = index, tokens
			}
		}
		if largest < 0 {
			return errors.New("恢复读取的调用元数据超过上下文预算，请减少恢复项或选择更大的上下文窗口")
		}
		limits[largest] /= 2
		results[largest].Content = truncateCompactionResult(contents[largest], limits[largest])
		if results[largest].Content == texts.CompactionTruncated {
			limits[largest] = 0
		}
	}
}

// restoreCompactionReads 不走审批或普通工具轮，不允许编辑及交互；结果按
// 独立预算截断。每次新生成会重新读取，避免切换分支、撤销或重开后沿用旧文件。
func (c *Core) restoreCompactionReads(run *toolExecutionRun, model api.Model) error {
	marker, data, err := run.session.Tree.CurrentCompaction()
	if err != nil || data == nil {
		return err
	}
	if len(data.Restore) > compactionRestoreCalls {
		return errors.New("保存的恢复读取清单超过数量限制")
	}
	if len(data.Restore) == 0 && len(data.Results) == 0 {
		return nil
	}
	results := make([]api.ChatMessage, 0, len(data.Restore)*2)
	budget := compactionRestoreBudget(model)
	for _, read := range data.Restore {
		if err := run.context.Context.Err(); err != nil {
			return err
		}
		callID, err := identity.New(identity.Request)
		if err != nil {
			return err
		}
		call := api.ToolCall{ID: callID, Name: read.Tool, Arguments: string(read.Arguments), DisplayName: agenttool.DisplayName(read.Tool)}
		assistant := api.ChatMessage{Role: "assistant", ToolCalls: []api.ToolCall{call}}
		var output string
		if !compactionReadTool(read.Tool) {
			err = errors.New("压缩恢复只允许读取工具")
		} else {
			var prepared *preparedToolCall
			prepared, err = run.prepare(call)
			if err == nil {
				if _, reversible := prepared.tool.(agenttool.ReversibleTool); reversible || prepared.permission.Level != agenttool.PermissionReadOnly {
					err = errors.New("压缩恢复不能修改状态或提升权限")
				} else if ctx, allowed := run.effectiveContext(call.Name, prepared.permission); !allowed {
					err = errors.New("当前权限不允许此恢复读取")
				} else {
					if c.profileRoot != "" {
						ctx.CheckPath = c.checkProfilePath
					}
					ctx.CallID, ctx.RecoveryFile, ctx.FileSystem = call.ID, "", c.FS
					prepared.timeout = min(prepared.timeout, agenttool.DefaultToolTimeout)
					output, err = invokeTool(prepared, ctx)
				}
			}
		}
		if run.context.Context.Err() != nil {
			return run.context.Context.Err()
		}
		result := api.ChatMessage{Role: "tool", ToolCallID: call.ID}
		if err != nil {
			result.ToolError = truncateCompactionResult(err.Error(), 128)
			output = "恢复读取失败：" + result.ToolError
		}
		remainingCalls := max(1, len(data.Restore)-len(results)/2)
		// 结果会被包装为 JSON，给转义和元数据留出约一半预算。
		allowance := max(64, budget/remainingCalls-api.MessageTokens(assistant)-api.MessageTokens(result))
		allowance /= 2
		result.Content = truncateCompactionResult(output, allowance)
		pair, _ := json.Marshal([]api.ChatMessage{assistant, result})
		budget -= api.EstimateTextTokens(string(pair))
		results = append(results, assistant, result)
	}
	if err := boundCompactionResults(results, compactionRestoreBudget(model)); err != nil {
		return err
	}
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if run.context.Context.Err() != nil || server.sending[run.session.ID] != run.generation || marker.Compaction != data {
		return context.Canceled
	}
	previous := data.Results
	data.Results = results
	if err := c.saveExecutionChatLocked(run.directory, run.session, run.options); err != nil {
		data.Results = previous
		return errors.Join(err, c.saveExecutionChatLocked(run.directory, run.session, run.options))
	}
	return nil
}

// CompactChat 显式压缩当前分支，不生成新的助手回复；GetChat 返回的节点包含
// 压缩摘要、原文起点和恢复结果。与普通生成共用取消、存储和并发控制。
func (c *Core) CompactChat(id string) (*chat.ChatTask, error) {
	session, config, directory, err := c.beginChatSend(id)
	if err != nil {
		return nil, err
	}
	defer c.endChatSend(id)
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	server.mu.Lock()
	generation := server.sending[id]
	server.mu.Unlock()
	path, err := session.Tree.CurrentPath()
	if err != nil {
		return nil, err
	}
	if len(path) == 0 {
		return nil, errors.New("空对话无法压缩")
	}
	modelIndex := slices.IndexFunc(config.Model, func(model api.Model) bool { return model.ID == session.Model })
	if modelIndex < 0 {
		return nil, errors.New("压缩模型不存在")
	}
	toolContext, memoryContext, err := c.chatExecutionContext(generation.ctx, session, nil)
	if err != nil {
		return nil, err
	}
	if err := c.chatExecutionClient(session, config); err != nil {
		return nil, err
	}
	tools := c.enabledTools(session)
	run := &toolExecutionRun{core: c, session: session, generation: generation, directory: directory,
		nodeID: path[len(path)-1].ID, context: toolContext, enabled: tools, strict: c.strictToolMode(session)}
	prefix := chatExecutionPrefix(session, toolContext, memoryContext, nil, run.strict)
	if err := c.compactChatSnapshot(run, path[len(path)-1], config.Model[modelIndex], prefix); err != nil {
		return nil, err
	}
	return session, nil
}
