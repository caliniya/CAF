package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	identity "github.com/caliniya/caf/utils/id"
)

// askUser 注册后再发事件，保证前端收到问题时已经可以提交答案。
func (c *Core) askUser(ctx context.Context, chatID, nodeID, callID string, question agenttool.UserQuestion) (string, error) {
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return "", serviceErr
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}
	requestID, err := identity.New(identity.Request)
	if err != nil {
		return "", err
	}
	chatServer.mu.Lock()
	serviceContext, err := chatServer.contextLocked()
	if err != nil {
		chatServer.mu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(serviceContext, cancel)
	defer stop()
	defer cancel()
	pending := &pendingQuestion{chatID: chatID, context: ctx, cancel: cancel, answer: make(chan string, 1)}
	chatServer.interactionMu.Lock()
	if chatServer.interactionRequests == nil {
		chatServer.interactionRequests = make(map[string]*pendingQuestion)
	}
	chatServer.interactionRequests[requestID] = pending
	chatServer.interactionMu.Unlock()
	chatServer.mu.Unlock()
	defer func() {
		chatServer.interactionMu.Lock()
		delete(chatServer.interactionRequests, requestID)
		chatServer.interactionMu.Unlock()
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
	chatServer, serviceErr := c.Servers.Get[*ChatServer](ChatServerName)
	if serviceErr != nil {
		return serviceErr
	}

	if strings.TrimSpace(answer) == "" {
		return errors.New("回答不能为空")
	}
	chatServer.interactionMu.Lock()
	defer chatServer.interactionMu.Unlock()
	pending := chatServer.interactionRequests[requestID]
	if pending == nil || pending.chatID != chatID || pending.context.Err() != nil {
		return errors.New("问题已结束或不属于当前对话")
	}
	pending.answer <- answer
	delete(chatServer.interactionRequests, requestID)
	return nil
}
