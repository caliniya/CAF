package APIImpl

import (
	"encoding/json"
	"unicode/utf8"
)

// EstimateTextTokens 不依赖模型分词器，只用于预留空间和选择大致的历史边界。
// ASCII 按约四个字符一个 token，其余字符按约两个 token 估算。
func EstimateTextTokens(text string) int {
	tokens, ascii := 0, 0
	for _, value := range text {
		if value < utf8.RuneSelf {
			ascii++
			continue
		}
		tokens += (ascii+3)/4 + 2
		ascii = 0
	}
	return tokens + (ascii+3)/4
}

// MessageTokens 使用单条消息的新增用量，绝不累加重复发送的 prompt_tokens。
func MessageTokens(message ChatMessage) int {
	if message.Tokens > 0 {
		return message.Tokens
	}
	tokens := 4 + EstimateTextTokens(message.Content) + EstimateTextTokens(message.Reasoning)
	if message.ToolCallID != "" {
		tokens += EstimateTextTokens(message.ToolCallID)
	}
	for _, call := range message.ToolCalls {
		tokens += 8 + EstimateTextTokens(call.ID) + EstimateTextTokens(call.Name) + EstimateTextTokens(call.Arguments)
	}
	return tokens
}

func MessagesTokens(messages []ChatMessage) int {
	tokens := 0
	for _, message := range messages {
		tokens += MessageTokens(message)
	}
	return tokens
}

func RequestTokens(request ChatRequest) int {
	tokens := MessagesTokens(request.Messages)
	if len(request.Tools) > 0 {
		body, _ := json.Marshal(request.Tools)
		tokens += EstimateTextTokens(string(body))
	}
	return tokens
}
