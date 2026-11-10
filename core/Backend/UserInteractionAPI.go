package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

type pendingQuestion struct {
	chatID  string
	context context.Context
	answer  chan string
}

// askUser 注册后再发事件，保证前端收到问题时已经可以提交答案。
func (c *Core) askUser(ctx context.Context, chatID, nodeID, callID string, question agenttool.UserQuestion) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	requestID := newID()
	pending := &pendingQuestion{chatID: chatID, context: ctx, answer: make(chan string, 1)}
	c.interactionMu.Lock()
	if c.interactionRequests == nil {
		c.interactionRequests = make(map[string]*pendingQuestion)
	}
	c.interactionRequests[requestID] = pending
	c.interactionMu.Unlock()
	defer func() {
		c.interactionMu.Lock()
		delete(c.interactionRequests, requestID)
		c.interactionMu.Unlock()
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
	c.interactionMu.Lock()
	defer c.interactionMu.Unlock()
	pending := c.interactionRequests[requestID]
	if pending == nil || pending.chatID != chatID || pending.context.Err() != nil {
		return errors.New("问题已结束或不属于当前对话")
	}
	pending.answer <- answer
	delete(c.interactionRequests, requestID)
	return nil
}
