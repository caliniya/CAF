package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	netclient "github.com/caliniya/caf/NetClient"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	persona "github.com/caliniya/caf/core/Persona"
	texts "github.com/caliniya/caf/utils/text"
)

func (r *qqChatRuntime) batchProactive(groupKey string, job qqChatJob) {
	r.mu.Lock()
	if r.revision != job.revision || r.ctx.Err() != nil || r.clearing[job.key] || r.epochs[job.key] != job.epoch {
		r.mu.Unlock()
		return
	}
	if time.Since(r.lastSpoke[groupKey]) < time.Duration(job.options.ProactiveCooldown)*time.Second {
		delete(r.batches, groupKey)
		r.mu.Unlock()
		return
	}
	if len(r.batches[groupKey]) >= 256 {
		r.mu.Unlock()
		r.report(errors.New("QQ 主动消息队列已满，当前消息未提交判断"))
		return
	}
	r.batches[groupKey] = append(r.batches[groupKey], job)
	r.startProactiveLocked(groupKey)
	r.mu.Unlock()
}

// 每群同时保留一个判断或回复，每条触发消息独立判断，期间的消息逐条排队。
func (r *qqChatRuntime) startProactiveLocked(groupKey string) {
	if r.ctx.Err() != nil || r.decisions[groupKey] != nil {
		return
	}
	jobs := slices.DeleteFunc(r.batches[groupKey], func(job qqChatJob) bool {
		return job.revision != r.revision || job.epoch != r.epochs[job.key] || r.clearing[job.key]
	})
	if len(jobs) == 0 {
		delete(r.batches, groupKey)
		return
	}
	if time.Since(r.lastSpoke[groupKey]) < time.Duration(jobs[0].options.ProactiveCooldown)*time.Second {
		delete(r.batches, groupKey)
		return
	}
	job := jobs[0]
	r.batches[groupKey] = slices.Clone(jobs[1:])
	job.proactiveMessages = []qqRecentMessage{{Message: job.message, ReceivedAt: job.receivedAt.UTC()}}
	ctx, cancel := context.WithCancel(r.ctx)
	decision := &qqProactiveDecision{ctx: ctx, cancel: cancel}
	r.decisions[groupKey] = decision
	r.wg.Add(1)
	go r.judgeProactive(groupKey, job, decision)
}

// 同一扩展复用传输客户端；协议解码器持有流状态，仍须每个请求独立创建。
func (r *qqChatRuntime) proactiveClient(provider *Config, model string) *netclient.APIClient {
	options := netclient.Options{ServerAddress: provider.ServerAddress, APIKey: provider.APIKey, Model: model}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.decisionClient == nil || *r.decisionClient.Config != options {
		r.decisionClient = netclient.NewAPIClient(options)
	}
	return r.decisionClient
}

// 判断请求只携带聊天相关字段，省去原始事件、重复正文和接收时间等冗余数据。
func qqDecisionMessages(items []qqRecentMessage) []qq.QQMessage {
	result := make([]qq.QQMessage, 0, len(items))
	for _, item := range items {
		message := item.Message
		message.Extra, message.RawMessage, message.Anonymous = nil, "", nil
		message.SubType, message.Font = "", 0
		if message.Sender != nil {
			message.Sender = &qq.QQSender{UserID: message.Sender.UserID, Nickname: message.Sender.Nickname, Card: message.Sender.Card}
		}
		result = append(result, message)
	}
	return result
}

func (r *qqChatRuntime) judgeProactive(groupKey string, job qqChatJob, decisionRun *qqProactiveDecision) {
	defer r.wg.Done()
	ctx, cancel := context.WithTimeout(decisionRun.ctx, 30*time.Second)
	defer cancel()
	queued := false
	defer func() {
		if !queued {
			job.proactiveDecision = decisionRun
			r.releaseProactive(job)
		}
	}()
	select {
	case r.generations <- struct{}{}:
		defer func() { <-r.generations }()
	case <-ctx.Done():
		return
	}
	provider, err := r.core.configByID(job.config.ConfigID)
	if err != nil {
		r.report(err)
		return
	}
	references := r.resolveReferences(ctx, job)
	if ctx.Err() != nil {
		return
	}
	body, err := json.Marshal(struct {
		Nearby     []qq.QQMessage       `json:"nearby,omitempty"`
		Messages   []qq.QQMessage       `json:"messages"`
		References []qqMessageReference `json:"references,omitempty"`
	}{qqDecisionMessages(job.nearby), qqDecisionMessages(job.proactiveMessages), references})
	if err != nil {
		r.report(err)
		return
	}
	prompt, err := persona.Resolve(job.config.PersonaID).Prompt()
	if err != nil {
		r.report(err)
		return
	}
	request := api.ChatRequest{Model: job.config.Model, MaxTokens: 256, Thinking: "low", Messages: []api.ChatMessage{
		{Role: "system", Content: prompt + texts.QQProactiveDecision}, {Role: "user", Content: string(body)},
	}}
	var result strings.Builder
	var requestUsage *api.TokenUsage
	err = r.proactiveClient(provider, job.config.Model).StreamChat(ctx, request, &api.ChatCompletionAPI{}, func(delta api.Delta) {
		result.WriteString(delta.Content)
		if delta.Usage != nil {
			usage := *delta.Usage
			requestUsage = &usage
		}
	})
	r.recordTokenUsage(job, requestUsage, false)
	if err != nil {
		if ctx.Err() == nil {
			r.report(err)
		}
		return
	}
	reply, err := ParseQQProactiveDecision(result.String())
	if err != nil {
		r.report(fmt.Errorf("主动发言判断未返回有效 JSON 或布尔值：%v。模型原始输出：%q", err, result.String()))
		return
	}
	if !reply || ctx.Err() != nil {
		return
	}
	job.proactive = true
	job.proactiveDecision = decisionRun
	queued = r.enqueue(job)
}
