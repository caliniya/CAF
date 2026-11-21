package Backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	identity "github.com/caliniya/caf/utils/id"
)

type qqChatInteraction struct {
	ctx      context.Context
	target   qq.QQMessage
	approval bool
	answer   chan string
}

func (r *qqChatRuntime) interaction(ctx context.Context, commandName string, target qq.QQMessage, approval bool, prompt func(string, string) string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id, err := identity.New(identity.Request)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	pending := &qqChatInteraction{ctx: ctx, target: target, approval: approval, answer: make(chan string, 1)}
	r.mu.Lock()
	r.interactions[id] = pending
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.interactions, id); r.mu.Unlock() }()
	if err := r.send(ctx, target, prompt(commandName, id)); err != nil {
		return "", err
	}
	select {
	case answer := <-pending.answer:
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return answer, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (r *qqChatRuntime) askUser(ctx context.Context, commandName string, target qq.QQMessage, question agenttool.UserQuestion) (string, error) {
	return r.interaction(ctx, commandName, target, false, func(name, id string) string {
		text := "向 QQ " + target.UserID + " 提问：\n" + question.Question
		for index, option := range question.Options {
			text += fmt.Sprintf("\n%d. %s", index+1, option)
		}
		return text + "\n请在 3 分钟内回复：/" + name + " answer " + id + " 你的回答"
	})
}

func (r *qqChatRuntime) approve(ctx context.Context, commandName string, target qq.QQMessage, view *api.ToolApproval, apply func(string) (*agenttool.ChatToolConfig, error)) (string, error) {
	// QQ 入口使用自动编辑权限。QQ 成员不能通过聊天授予宿主其他目录或系统权限。
	if !view.WorkspaceAllowed {
		return "", errors.New("QQ 对话只允许在自己的内部工作区自动编辑；此调用需要在本机处理")
	}
	choice, err := r.interaction(ctx, commandName, target, true, func(name, id string) string {
		return "向 QQ " + target.UserID + " 申请执行工具：" + view.Name + "\n" + view.Reason + "\n参数：" + view.Arguments +
			"\n请在 3 分钟内回复：/" + name + " approve " + id + " once（允许本次）、workspace（允许此工作区）或 reject（拒绝）"
	})
	if err != nil {
		view.Decision = "canceled"
		return "", err
	}
	if _, err := apply(choice); err != nil {
		return "", err
	}
	view.Decision = choice
	return choice, nil
}

func (r *qqChatRuntime) answerInteraction(message qq.QQMessage, command qq.Command) (string, error) {
	if len(command.Args) < 2 {
		return "", errors.New("请提供请求 ID 和回答")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	pending := r.interactions[command.Args[0]]
	if pending == nil || pending.ctx.Err() != nil {
		return "", errors.New("请求已经结束或不存在")
	}
	if pending.target.SelfID != message.SelfID || pending.target.UserID != message.UserID || pending.target.Type != message.Type || pending.target.GroupID != message.GroupID {
		return "", errors.New("只能由原请求的用户在原聊天中回答")
	}
	if (command.Name == "approve") != pending.approval {
		return "", errors.New("请求类型不匹配")
	}
	answer := strings.Join(command.Args[1:], " ")
	if pending.approval && answer != ApprovalOnce && answer != ApprovalWorkspace && answer != ApprovalReject {
		return "", errors.New("QQ 审批支持 once、workspace 或 reject")
	}
	delete(r.interactions, command.Args[0])
	pending.answer <- answer
	return "已提交。", nil
}

func (r *qqChatRuntime) command(owner *ExtensionClient, binding ClientExtension, options QQChatOptions, message qq.QQMessage, command qq.Command) {
	text := fmt.Sprintf("可用指令：\n/%s help — 查看帮助\n/%s status — 查看客户端状态\n/%s clear — 清除当前上下文\n/%s clear agent — 清除自己的 agent 上下文\n/%s answer 请求ID 回答 — 回答模型提问\n/%s approve 请求ID once|workspace|reject — 处理内部工作区工具申请", binding.Name, binding.Name, binding.Name, binding.Name, binding.Name, binding.Name)
	switch command.Name {
	case "help":
		if len(command.Args) > 0 {
			text = "无法识别该指令。\n" + text
		}
	case "status":
		if len(command.Args) == 0 {
			text = fmt.Sprintf("客户端：%s\nID：%s\n扩展：%s\nQQ：%s", owner.Name, owner.ID, binding.Name, r.client.Info().SelfID)
		} else {
			text = "无法识别该指令。\n" + text
		}
	case "answer", "approve":
		answer, err := r.answerInteraction(message, command)
		if err != nil {
			text = err.Error()
		} else {
			text = answer
		}
	case "clear":
		agent := qqMentioned(message) && options.AgentEnabled
		if len(command.Args) == 1 && command.Args[0] == "agent" {
			agent = true
		} else if len(command.Args) != 0 {
			text = "清除指令支持 clear 或 clear agent"
			break
		}
		key, _ := qqRoute(message, options, agent)
		id := r.contextIDForKey(key)
		if id == "" {
			text = "当前上下文为空。"
			break
		}
		// 等待旧模型退出放在独立任务中，不阻塞消息接收或其他用户的回复。
		r.mu.Lock()
		if r.ctx.Err() != nil {
			r.mu.Unlock()
			return
		}
		r.wg.Add(1)
		r.mu.Unlock()
		go func() {
			defer r.wg.Done()
			err := r.core.ClearQQContext(r.clientID, r.adapterID, id)
			result := "已清除当前上下文，工作区文件和长期记忆保留。"
			if err != nil {
				result = "清除失败：" + err.Error()
			}
			if err := r.send(r.ctx, message, result); err != nil {
				r.report(err)
			}
		}()
		return
	default:
		text = "无法识别该指令。\n" + text
	}
	if err := r.send(r.ctx, message, text); err != nil {
		r.report(err)
	}
}
