package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	texts "github.com/caliniya/caf/utils/text"
)

const qqLongContextTokens int64 = 300_000

// QQTokenStats 是扩展的累计用量，清除或删除单个上下文不会重置。
type QQTokenStats struct {
	Requests           int64 `json:"requests"`
	UsageRequests      int64 `json:"usageRequests"`
	PromptTokens       int64 `json:"promptTokens"`
	CompletionTokens   int64 `json:"completionTokens"`
	TotalTokens        int64 `json:"totalTokens"`
	CachedPromptTokens int64 `json:"cachedPromptTokens"`
	// 缓存命中率的分母，只计入提供了缓存统计的请求的输入 token。
	CachePromptTokens int64 `json:"cachePromptTokens"`
}

func (c *Core) readQQTokenStats(directory string) (QQTokenStats, error) {
	var stats QQTokenStats
	data, err := c.FS.ReadFile(filepath.Join(directory, "usage.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return stats, nil
	}
	if err != nil {
		return stats, err
	}
	if err := json.Unmarshal(data, &stats); err != nil {
		return stats, fmt.Errorf("解析 QQ 用量统计：%w", err)
	}
	return stats, nil
}

// 调用方持有 extMu，读取已启动实例的快照，或停用实例的持久化统计。
func (c *Core) qqTokenStatsLocked(clientID, adapterID string) (*QQTokenStats, error) {
	q, err := c.Servers.Get[*QQServer](QQServerName)
	if err != nil {
		return nil, err
	}
	q.chatMu.Lock()
	r := q.chats[adapterID]
	q.chatMu.Unlock()
	if r != nil && r.clientID == clientID {
		r.mu.Lock()
		stats := r.usage
		r.mu.Unlock()
		return &stats, nil
	}
	directory, err := c.qqChatDirectory(clientID, adapterID)
	if err != nil {
		return nil, err
	}
	stats, err := c.readQQTokenStats(directory)
	return &stats, err
}

func qqUsageTotal(usage *api.TokenUsage) int64 {
	if usage.TotalTokens > 0 {
		return int64(usage.TotalTokens)
	}
	return int64(max(0, usage.PromptTokens)) + int64(max(0, usage.CompletionTokens))
}

func qqCacheUsage(usage *api.TokenUsage) (hit, input int64) {
	input = int64(max(0, usage.PromptTokens))
	if usage.PromptCacheHitTokens != nil {
		hit = int64(max(0, *usage.PromptCacheHitTokens))
		if usage.PromptCacheMissTokens != nil {
			input = hit + int64(max(0, *usage.PromptCacheMissTokens))
		}
	} else if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens != nil {
		hit = int64(max(0, *usage.PromptTokensDetails.CachedTokens))
	} else {
		return 0, 0
	}
	return min(hit, input), input
}

// 由每轮请求的完成回调调用；判断请求也计费，但不改变对话的上下文长度。
func (r *qqChatRuntime) recordTokenUsage(job qqChatJob, usage *api.TokenUsage, contextUsage bool) {
	r.mu.Lock()
	r.usage.Requests++
	if usage != nil {
		r.usage.UsageRequests++
		r.usage.PromptTokens += int64(max(0, usage.PromptTokens))
		r.usage.CompletionTokens += int64(max(0, usage.CompletionTokens))
		r.usage.TotalTokens += qqUsageTotal(usage)
		hit, input := qqCacheUsage(usage)
		r.usage.CachedPromptTokens += hit
		r.usage.CachePromptTokens += input
	}
	r.storageMu.Lock()
	err := context.Canceled
	if !r.storageClosed {
		err = r.core.writeJSONLocked(filepath.Join(r.directory, "usage.json"), r.usage)
	}
	r.storageMu.Unlock()
	if record, exists := r.records[job.key]; contextUsage && exists && r.epochs[job.key] == job.epoch && !r.clearing[job.key] {
		record.ContextTokens = nil
		if usage != nil {
			tokens := qqUsageTotal(usage)
			record.ContextTokens = &tokens
		}
		r.records[job.key] = record
		err = errors.Join(err, r.saveContextsLocked())
	}
	r.mu.Unlock()
	if err != nil && !errors.Is(err, context.Canceled) {
		r.report(fmt.Errorf("保存 QQ 用量统计：%w", err))
	}
}

// 宿主直接发送通知，不消耗模型请求；同一上下文清除前最多提醒一次。
func (r *qqChatRuntime) warnLongContext(ctx context.Context, job qqChatJob) error {
	r.mu.Lock()
	record, exists := r.records[job.key]
	shouldWarn := exists && record.ContextTokens != nil && *record.ContextTokens >= qqLongContextTokens && !record.ContextWarningSent &&
		r.revision == job.revision && r.epochs[job.key] == job.epoch && !r.clearing[job.key] && ctx.Err() == nil
	r.mu.Unlock()
	if !shouldWarn {
		return nil
	}
	suffix := ""
	if record.Kind == "agent" {
		suffix = " agent"
	}
	if err := r.send(ctx, job.message, fmt.Sprintf(texts.QQLongContextNotice, *record.ContextTokens, job.commandName, suffix)); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists = r.records[job.key]
	if !exists || r.revision != job.revision || r.epochs[job.key] != job.epoch || r.clearing[job.key] || ctx.Err() != nil {
		return nil
	}
	record.ContextWarningSent = true
	r.records[job.key] = record
	return r.saveContextsLocked()
}
