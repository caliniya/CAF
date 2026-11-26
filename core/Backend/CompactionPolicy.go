package Backend

import (
	"errors"
	"fmt"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	identity "github.com/caliniya/caf/utils/id"
)

type CompactionPrompt struct {
	ChatID        string `json:"chatId"`
	RequestID     string `json:"requestId"`
	Used          int    `json:"used"`
	Threshold     int    `json:"threshold"`
	NextThreshold int    `json:"nextThreshold,omitempty"`
}

type pendingCompaction struct {
	prompt CompactionPrompt
	answer chan bool
}

// 只有本次、这个会话的请求可以被回答一次。等待期间不持有会话锁。
func (c *Core) SubmitCompactionDecision(id, requestID string, compact bool) error {
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	run := server.sending[id]
	if run == nil || run.ctx.Err() != nil || run.compaction == nil || run.compaction.prompt.RequestID != requestID {
		return errors.New("压缩提醒已结束或不属于当前对话")
	}
	run.compaction.answer <- compact
	run.compaction = nil
	return nil
}

// 重新进入对话或刷新页面时仍可应答正在等待的请求。
func (c *Core) GetPendingCompaction(id string) (*CompactionPrompt, error) {
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return nil, err
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if run := server.sending[id]; run != nil && run.ctx.Err() == nil && run.compaction != nil {
		p := run.compaction.prompt
		return &p, nil
	}
	return nil, nil
}

func (c *Core) confirmCompaction(run *toolExecutionRun, prompt CompactionPrompt) (bool, error) {
	ctx := run.context.Context
	if run.options != nil && run.options.confirmCompaction != nil {
		return run.options.confirmCompaction(ctx, prompt)
	}
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return false, err
	}
	prompt.RequestID, err = identity.New(identity.Request)
	if err != nil {
		return false, err
	}
	pending := &pendingCompaction{prompt: prompt, answer: make(chan bool, 1)}
	server.mu.Lock()
	if server.sending[prompt.ChatID] != run.generation || ctx.Err() != nil {
		server.mu.Unlock()
		return false, errors.New("对话生成已结束")
	}
	run.generation.compaction = pending
	server.mu.Unlock()
	defer func() {
		server.mu.Lock()
		if run.generation.compaction == pending {
			run.generation.compaction = nil
		}
		server.mu.Unlock()
	}()
	defer EmitJSON(EventChatCompaction, map[string]any{"chatId": prompt.ChatID, "requestId": prompt.RequestID, "closed": true})
	EmitJSON(EventChatCompaction, prompt)
	select {
	case accept := <-pending.answer:
		return accept, ctx.Err()
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// 窗口未知时只有固定阈值。
func (p CompactionDefaults) thresholds(window int) (low, high int) {
	if window <= 0 {
		return p.TokenThreshold, p.TokenThreshold
	}
	// 向上取整；大窗口也不会因先乘百分比而溢出。
	percent := window/100*p.WindowPercent + (window%100*p.WindowPercent+99)/100
	return min(percent, p.TokenThreshold), max(percent, p.TokenThreshold)
}

func compactionSkippedSinceCheckpoint(tree *chatstruct.ChatTree) (int, error) {
	path, err := tree.CurrentPath()
	if err != nil {
		return 0, err
	}
	for i := len(path) - 1; i >= 0; i-- {
		n := path[i]
		if n.SkippedCompactionAt != 0 {
			return n.SkippedCompactionAt, nil
		}
		if n.Compaction != nil {
			break
		}
	}
	return 0, nil
}

func (c *Core) maybeCompactChat(run *toolExecutionRun, reply *chatstruct.ChatNode, model api.Model, policy CompactionDefaults, keep, used int) (bool, error) {
	if !policy.Enabled {
		return false, nil
	}
	low, high := policy.thresholds(model.MaxContext)
	if used < low {
		return false, nil
	}
	threshold, next := low, high
	if used >= high {
		threshold, next = high, 0
	}
	if !policy.Automatic {
		skipped, err := compactionSkippedSinceCheckpoint(run.session.Tree)
		if err != nil {
			return false, err
		}
		if skipped >= threshold {
			return false, nil
		}
	}
	// 近期原文已覆盖全部内容（例如刚压缩或首条消息特别长）时，无需弹窗或发起总结请求。
	// 自定义阈值较小时，近期原文最多占首个阈值的一半，为摘要与下一轮留出空间。
	keep = min(keep, max(1, low/2))
	plan, err := run.session.Tree.NewCompactionPlan(keep)
	if errors.Is(err, chatstruct.ErrNoCompactionHistory) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !policy.Automatic {
		accepted, err := c.confirmCompaction(run, CompactionPrompt{ChatID: run.session.ID, Used: used, Threshold: threshold, NextThreshold: next})
		if err != nil {
			return false, err
		}
		if !accepted {
			reply.SkippedCompactionAt = threshold
			return false, c.saveReplyLocked(run.directory, run.session, run.options)
		}
	}
	return true, c.compactChatPlan(run, reply, model, keep, plan)
}

func (p CompactionPrompt) question() string {
	text := fmt.Sprintf("上下文约有 %d token，已达到 %d token 的提醒阈值。现在压缩上下文吗？", p.Used, p.Threshold)
	if p.NextThreshold > 0 {
		text += fmt.Sprintf("暂不压缩会在达到 %d token 时再提醒。", p.NextThreshold)
	}
	return text
}
