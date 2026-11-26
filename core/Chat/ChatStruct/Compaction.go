package ChatStruct

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	texts "github.com/caliniya/caf/utils/text"
)

const CompactionKeepTokens = 16 * 1024

var ErrNoCompactionHistory = errors.New("近期上下文已覆盖全部历史，没有可压缩的较早消息")

// MessagePosition 指向保留原文的起点；这个位置可以在压缩标记节点之前。
type MessagePosition struct {
	NodeID string `json:"nodeId"`
	Index  int    `json:"index"`
}

type CompactionRestoreCall struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Purpose   string          `json:"purpose"`
}

// CompactionData 只在检查通过后保存。Results 是宿主执行只读恢复的结果，
// 作为数据提供给模型，不伪造模型实际发起的工具调用。
type CompactionData struct {
	Start   MessagePosition         `json:"start"`
	Summary string                  `json:"summary"`
	Restore []CompactionRestoreCall `json:"restore"`
	Results []api.ChatMessage       `json:"results,omitempty"`
}

type CompactionTurn struct {
	NodeID   string            `json:"nodeId"`
	Messages []api.ChatMessage `json:"messages"`
}

// CompactionPlan 是一次压缩的固定快照，不写入会话。Recent 只用于理解当前
// 任务及规划恢复读取，仍会以原文保留；History 才是本次被替换的历史。
type CompactionPlan struct {
	Start           MessagePosition   `json:"start"`
	PreviousSummary string            `json:"previousSummary,omitempty"`
	History         []CompactionTurn  `json:"history"`
	Recent          []api.ChatMessage `json:"recent"`
}

func activeCompaction(path []*ChatNode) (*ChatNode, int, *CompactionData, error) {
	for marker := len(path) - 1; marker >= 0; marker-- {
		data := path[marker].Compaction
		if data == nil {
			continue
		}
		if strings.TrimSpace(data.Summary) == "" {
			return nil, 0, nil, errors.New("压缩记录的摘要为空")
		}
		for start := 0; start <= marker; start++ {
			if path[start].ID != data.Start.NodeID {
				continue
			}
			if data.Start.Index < 0 || data.Start.Index > len(path[start].Child) {
				return nil, 0, nil, errors.New("压缩记录的原文起点越界")
			}
			return path[marker], start, data, nil
		}
		return nil, 0, nil, errors.New("压缩记录的原文起点不在当前分支中")
	}
	return nil, 0, nil, nil
}

func (tree *ChatTree) CurrentCompaction() (*ChatNode, *CompactionData, error) {
	path, err := tree.CurrentPath()
	if err != nil {
		return nil, nil, err
	}
	node, _, data, err := activeCompaction(path)
	return node, data, err
}

// ContextMessages 只改变模型可见历史。Each、CurrentPath 和 GetMessages
// 继续返回原始对话，供界面、分支恢复和追溯使用。
func (tree *ChatTree) ContextMessages() ([]api.ChatMessage, error) {
	path, err := tree.CurrentPath()
	if err != nil {
		return nil, err
	}
	_, start, data, err := activeCompaction(path)
	if err != nil {
		return nil, err
	}
	messages := make([]api.ChatMessage, 0)
	offset := 0
	if data != nil {
		messages = append(messages, api.ChatMessage{Role: "system", Content: texts.CompactionSummary + data.Summary})
		offset = data.Start.Index
	}
	for index := start; index < len(path); index++ {
		if index != start {
			offset = 0
		}
		messages = append(messages, path[index].Child[offset:]...)
	}
	if data != nil && len(data.Results) > 0 {
		body, err := json.Marshal(data.Results)
		if err != nil {
			return nil, fmt.Errorf("编码压缩恢复结果：%w", err)
		}
		messages = append(messages, api.ChatMessage{Role: "system", Content: texts.CompactionRestored + string(body)})
	}
	return messages, nil
}

// NewCompactionPlan 按单条消息新增用量选取约 keepTokens 的近期原文。
// 优先在完整轮次之间切分；超长轮次可以在完整工具调用组之后切分。
// 待执行调用及其后的消息始终保留。
func (tree *ChatTree) NewCompactionPlan(keepTokens int) (*CompactionPlan, error) {
	if keepTokens <= 0 {
		return nil, errors.New("保留上下文的 token 目标必须为正数")
	}
	path, err := tree.CurrentPath()
	if err != nil {
		return nil, err
	}
	_, startNode, previous, err := activeCompaction(path)
	if err != nil {
		return nil, err
	}
	type sourceMessage struct {
		position MessagePosition
		message  api.ChatMessage
	}
	source := make([]sourceMessage, 0)
	for nodeIndex := startNode; nodeIndex < len(path); nodeIndex++ {
		first := 0
		if nodeIndex == startNode && previous != nil {
			first = previous.Start.Index
		}
		for messageIndex := first; messageIndex < len(path[nodeIndex].Child); messageIndex++ {
			message := path[nodeIndex].Child[messageIndex]
			if message.Role == "" {
				continue
			}
			source = append(source, sourceMessage{
				position: MessagePosition{NodeID: path[nodeIndex].ID, Index: messageIndex}, message: message,
			})
		}
	}
	if len(source) < 2 {
		return nil, ErrNoCompactionHistory
	}
	target, tokens := len(source), 0
	for target > 0 && tokens < keepTokens {
		target--
		tokens += api.MessageTokens(source[target].message)
	}
	// tool 消息不能脱离调用；正常轮次保留它的 user 消息作为起点。
	safe, pending := make([]bool, len(source)+1), make(map[string]bool)
	for index, entry := range source {
		safe[index] = len(pending) == 0
		message := entry.message
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				if call.ID == "" || pending[call.ID] {
					return nil, errors.New("历史工具调用 ID 为空或重复，无法确定压缩边界")
				}
				pending[call.ID] = true
			}
		}
		if message.Role == "tool" {
			if !pending[message.ToolCallID] {
				return nil, errors.New("历史工具结果缺少对应调用，无法确定压缩边界")
			}
			delete(pending, message.ToolCallID)
		}
	}
	safe[len(source)] = len(pending) == 0
	cut := target
	// 尽量向前移动到本轮 user 所在的节点开头，包括 QQ 附近消息等系统数据。
	for index := target; index >= 0; index-- {
		if source[index].message.Role != "user" {
			continue
		}
		begin := index
		for begin > 0 && source[begin-1].position.NodeID == source[index].position.NodeID {
			begin--
		}
		// 超长单轮超过目标的两倍时，改用轮次内的安全消息边界。
		recent := 0
		for _, entry := range source[begin:] {
			recent += api.MessageTokens(entry.message)
		}
		if recent <= keepTokens*2 {
			cut = begin
		}
		break
	}
	for cut > 0 && !safe[cut] {
		cut--
	}
	if cut == 0 {
		return nil, ErrNoCompactionHistory
	}
	plan := &CompactionPlan{Start: source[cut].position, History: []CompactionTurn{}, Recent: []api.ChatMessage{}}
	if previous != nil {
		plan.PreviousSummary = previous.Summary
	}
	for index, entry := range source {
		message := entry.message
		message.Tokens, message.Reasoning = 0, ""
		message.ToolCalls = append([]api.ToolCall(nil), message.ToolCalls...)
		for callIndex := range message.ToolCalls {
			call := &message.ToolCalls[callIndex]
			call.DisplayName, call.DisplayType, call.DisplayContent, call.Approval = "", "", "", nil
		}
		if index >= cut {
			plan.Recent = append(plan.Recent, message)
			continue
		}
		if len(plan.History) == 0 || plan.History[len(plan.History)-1].NodeID != entry.position.NodeID {
			plan.History = append(plan.History, CompactionTurn{NodeID: entry.position.NodeID, Messages: []api.ChatMessage{}})
		}
		last := &plan.History[len(plan.History)-1]
		last.Messages = append(last.Messages, message)
	}
	return plan, nil
}
