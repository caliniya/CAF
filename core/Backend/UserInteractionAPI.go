package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

// askUser 注册后再发事件，保证前端收到问题时已经可以提交答案。
func (c *Core) askUser(ctx context.Context, chatID, nodeID, callID string, question agenttool.UserQuestion) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	requestID := newID()
	c.Chat.mu.Lock()
	serviceContext, err := c.Chat.contextLocked()
	if err != nil {
		c.Chat.mu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(serviceContext, cancel)
	defer stop()
	defer cancel()
	pending := &pendingQuestion{chatID: chatID, context: ctx, cancel: cancel, answer: make(chan string, 1)}
	c.Chat.interactionMu.Lock()
	if c.Chat.interactionRequests == nil {
		c.Chat.interactionRequests = make(map[string]*pendingQuestion)
	}
	c.Chat.interactionRequests[requestID] = pending
	c.Chat.interactionMu.Unlock()
	c.Chat.mu.Unlock()
	defer func() {
		c.Chat.interactionMu.Lock()
		delete(c.Chat.interactionRequests, requestID)
		c.Chat.interactionMu.Unlock()
	}()
	data, _ := json.Marshal(question)
	EmitJSON(EventChatTool, map[string]string{
		"chatId": chatID, "nodeId": nodeID, "callId": callID, "toolId": "AskUser",
		"name": "向用户提问", "event": "update", "displayType": "question",
		"displayContent": "arguments", "requestId": requestID, "content": string(data),
	})
	select {
	case answer := <-pending.answer:
		return answer, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// SubmitUserAnswer 只接受当前会话中仍在等待的问题，每个请求只能回答一次。
func (c *Core) SubmitUserAnswer(chatID, requestID, answer string) error {
	if strings.TrimSpace(answer) == "" {
		return errors.New("回答不能为空")
	}
	c.Chat.interactionMu.Lock()
	defer c.Chat.interactionMu.Unlock()
	pending := c.Chat.interactionRequests[requestID]
	if pending == nil || pending.chatID != chatID || pending.context.Err() != nil {
		return errors.New("问题已结束或不属于当前对话")
	}
	pending.answer <- answer
	delete(c.Chat.interactionRequests, requestID)
	return nil
}
