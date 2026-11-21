package Backend

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
)

type qqMessageReference struct {
	ID      string        `json:"id"`
	Message *qq.QQMessage `json:"message,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// 缓存按机器人与消息 ID 保存原文，白名单和聊天路由只控制是否触发模型。
func (r *qqChatRuntime) cacheMessageLocked(message qq.QQMessage) {
	if message.ID == "" {
		return
	}
	key := message.SelfID + ":" + message.ID
	if _, exists := r.messageCache[key]; !exists {
		r.messageOrder = append(r.messageOrder, key)
	}
	r.messageCache[key] = message
	if len(r.messageOrder) > 1024 {
		delete(r.messageCache, r.messageOrder[0])
		r.messageOrder = r.messageOrder[1:]
	}
}

func (r *qqChatRuntime) resolveReferences(parent context.Context, job qqChatJob) []qqMessageReference {
	// 引用查询有独立的短超时；失败仅标记原文不可用，不阻断聊天。
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	result := []qqMessageReference{}
	seen := map[string]bool{}
	messages := []qq.QQMessage{job.message}
	for _, nearby := range job.nearby {
		messages = append(messages, nearby.Message)
	}
	for _, message := range job.proactiveMessages {
		messages = append(messages, message.Message)
	}
	for _, source := range messages {
		for _, segment := range source.Segments {
			if segment.Type != qq.QQSegmentReply {
				continue
			}
			id, err := strconv.ParseInt(fmt.Sprint(segment.Data["id"]), 10, 64)
			if err != nil {
				continue
			}
			text := strconv.FormatInt(id, 10)
			key := source.SelfID + ":" + text
			if seen[key] || len(result) >= 8 {
				continue
			}
			seen[key] = true
			reference := qqMessageReference{ID: text}
			r.mu.Lock()
			message, found := r.messageCache[key]
			r.mu.Unlock()
			if !found {
				if reader, ok := r.client.(qq.MessageReader); ok {
					message, err = reader.GetMessage(ctx, text)
				} else {
					err = errors.New("当前 QQ 适配器不支持读取引用消息")
				}
			}
			if err == nil && (message.SelfID != source.SelfID || message.Type != source.Type ||
				source.Type == qq.QQMessageGroup && message.GroupID != "" && message.GroupID != source.GroupID ||
				source.Type == qq.QQMessagePrivate && message.UserID != source.UserID && !message.SentBySelf) {
				err = errors.New("引用消息不属于当前聊天")
			}
			if err != nil {
				reference.Error = err.Error()
			} else {
				// 标准 get_msg 的群号是可选字段，引用所在消息提供缺失的群号。
				if source.Type == qq.QQMessageGroup && message.GroupID == "" {
					message.GroupID = source.GroupID
				}
				reference.Message = &message
				r.mu.Lock()
				r.cacheMessageLocked(message)
				r.mu.Unlock()
			}
			result = append(result, reference)
		}
	}
	return result
}

// 两部分按各自长度截取，合并去重后按消息时间排列；快照中没有当前触发消息或未来消息。
func qqProactiveContext(nearby, sender []qqRecentMessage, lengths [2]int) []qqRecentMessage {
	result := make([]qqRecentMessage, 0, lengths[0]+lengths[1])
	seen := map[string]bool{}
	for index, items := range [][]qqRecentMessage{nearby, sender} {
		for _, item := range items[max(0, len(items)-lengths[index]):] {
			key := item.Message.SelfID + ":" + item.Message.ID
			if item.Message.ID == "" {
				key = fmt.Sprintf("%s:%s:%s:%d:%s", item.Message.SelfID, item.Message.GroupID, item.Message.UserID, item.Message.Time, qqPlainText(item.Message))
			}
			if !seen[key] {
				seen[key] = true
				result = append(result, item)
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		leftTime, rightTime := left.ReceivedAt, right.ReceivedAt
		if left.Message.Time > 0 {
			leftTime = time.Unix(left.Message.Time, 0)
		}
		if right.Message.Time > 0 {
			rightTime = time.Unix(right.Message.Time, 0)
		}
		if !leftTime.Equal(rightTime) {
			return leftTime.Before(rightTime)
		}
		return left.ReceivedAt.Before(right.ReceivedAt)
	})
	return result
}
