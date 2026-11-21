package Tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	backend "github.com/caliniya/caf/core/Backend"
	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
)

func TestQQProactiveDecisionAcceptsModelFormatting(t *testing.T) {
	for _, test := range []struct {
		output string
		reply  bool
	}{
		{"true", true}, {"FALSE", false}, {" TuRe ", true}, {"FLASE", false},
		{`"True"`, true}, {"yes", true}, {"NO", false}, {"1", true}, {"0", false},
		{`{"RePlY":true}`, true}, {`{"REPLY":"FALSE"}`, false}, {`{"reply":1}`, true},
		{`{"reply":false,"reason":"mentions true and false"}`, false},
		{"```JSON\n{\"REPLY\": TRUE,}\n```", true}, {"```\nFALSE\n```", false},
		{`{'reply':'ture'}`, true}, {"{reply: FALSE,}", false}, {"reply = TURE", true},
		{"判断结果：TRUE。", true}, {"**false**", false}, {"true — 可以回答这个问题", true},
	} {
		t.Run(test.output, func(t *testing.T) {
			reply, err := backend.ParseQQProactiveDecision(test.output)
			if err != nil || reply != test.reply {
				t.Fatalf("valid model judgment rejected: %q => %v / %v", test.output, reply, err)
			}
		})
	}
	for _, output := range []string{"", "{}", `{"message":"true"}`, `{"reply":null}`, `{"reply":"maybe"}`, "true or false", `{"reply":true}, {"reply":false}`, `{"reply":10}`, "未来再讨论", "notreply = true", "untrue"} {
		t.Run("invalid:"+output, func(t *testing.T) {
			if reply, err := backend.ParseQQProactiveDecision(output); err == nil {
				t.Fatalf("unrecognized or ambiguous judgment accepted: %q => %v", output, reply)
			}
		})
	}
}

func TestQQProactiveParseErrorShowsModelOutputAndQueueContinues(t *testing.T) {
	var judgments atomic.Int32
	const output = "模型具体回复：我还没有决定，请再等等"
	f := useQQChat(t, map[string]string{"proactiveEnabled": "true", "proactiveCooldown": "0"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if strings.Contains(qqRequestText(request), "判断是否有必要主动参与") {
			if judgments.Add(1) == 1 {
				qqSSE(w, output)
			} else {
				qqSSE(w, "TuRe")
			}
			return
		}
		qqSSE(w, "兼容格式判断后正常回复")
	})
	f.message(t, "group", 30003, 23456, "first-judgment", false)
	f.request(t)
	deadline := time.Now().Add(3 * time.Second)
	for {
		options, err := f.core.ListExtensions()
		if err != nil {
			t.Fatal(err)
		}
		if len(options) == 1 && strings.Contains(options[0].LastError, output) && strings.Contains(options[0].LastError, "主动发言判断未返回") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("parse error omitted model output: %+v", options)
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.message(t, "group", 30003, 23456, "next-judgment", false)
	f.request(t)
	f.request(t)
	if reply := f.reply(t); qqPlainReply(reply) != "兼容格式判断后正常回复" {
		t.Fatal("parse error stopped the proactive queue")
	}
}

func TestQQProactiveContextUsesNearbyAndCrossChannelSenderHistory(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "false", "proactiveEnabled": "true", "proactiveContextLength": "3,3", "proactiveCooldown": "0", "whitelistEnabled": "true", "whitelist": "30003"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		if strings.Contains(qqRequestText(request), "判断是否有必要主动参与") {
			var input struct {
				Messages []qq.QQMessage `json:"messages"`
			}
			json.Unmarshal([]byte(fmt.Sprint(request.Messages[1].Content)), &input)
			qqSSE(w, fmt.Sprintf(`{"reply":%t}`, len(input.Messages) == 1 && qqPlainReply(input.Messages[0]) == "current-trigger"))
			return
		}
		qqSSE(w, "正式回复")
	})
	messageAt := func(kind string, user, group int, text string, timestamp int64) {
		t.Helper()
		f.sequence++
		packet := map[string]any{"post_type": "message", "message_type": kind, "sub_type": "normal", "message_id": f.sequence,
			"self_id": 10001, "user_id": user, "time": timestamp, "message": []qq.QQMessageSegment{qq.QQText(text)},
			"sender": map[string]any{"user_id": user, "nickname": fmt.Sprint(user)}}
		if group > 0 {
			packet["group_id"] = group
		}
		writeOneBotPacket(t, f.connection, packet)
	}
	// 到达顺序故意与 QQ 时间不同，验证合并后按消息时间排列。
	messageAt("group", 30003, 34567, "sender-other-group", 1795200003)
	f.request(t)
	f.waitTokenStats(t, 1)
	messageAt("private", 30003, 0, "sender-private", 1795200001)
	f.request(t)
	f.reply(t)
	messageAt("group", 30003, 23456, "sender-overlap", 1795200004)
	f.request(t)
	f.waitTokenStats(t, 3)
	messageAt("group", 50005, 23456, "crowd-near-one", 1795200002)
	messageAt("group", 60006, 23456, "crowd-near-two", 1795200005)
	messageAt("group", 30003, 23456, "current-trigger", 1795200006)
	request := f.request(t)
	var input struct {
		Nearby   []qq.QQMessage `json:"nearby"`
		Messages []qq.QQMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(fmt.Sprint(request.Messages[1].Content)), &input); err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, message := range input.Nearby {
		texts = append(texts, qqPlainReply(message))
	}
	expected := []string{"sender-private", "crowd-near-one", "sender-other-group", "sender-overlap", "crowd-near-two"}
	if !reflect.DeepEqual(texts, expected) || len(input.Messages) != 1 || qqPlainReply(input.Messages[0]) != "current-trigger" {
		t.Fatalf("wrong context lengths, ordering, deduplication or trigger separation: %+v / %+v", texts, input.Messages)
	}
	if input.Nearby[2].GroupID != "34567" || input.Nearby[0].Type != qq.QQMessagePrivate {
		t.Fatal("sender history excluded other groups or private messages")
	}
	request = f.request(t)
	f.reply(t)
	for _, marker := range expected {
		if !strings.Contains(qqRequestText(request), marker) {
			t.Fatal("formal proactive reply lost selected context", marker)
		}
	}
	if len(f.contexts(t)) != 1 {
		t.Fatal("sender contexts unexpectedly separated across private and group")
	}
	f.options(t, map[string]string{"proactiveContextLength": "0,0"})
	f.message(t, "group", 50005, 23456, "zero-nearby", false)
	f.message(t, "group", 30003, 23456, "zero-trigger", false)
	request = f.request(t)
	input.Nearby = nil
	json.Unmarshal([]byte(fmt.Sprint(request.Messages[1].Content)), &input)
	if len(input.Nearby) != 0 || len(input.Messages) != 1 {
		t.Fatal("zero context lengths attached historical messages", input)
	}
	for _, bad := range []string{"1", "1,", ",1", "1,2,3", "101,0", "0,-1", "x,2", "1.5,2"} {
		values, err := f.core.GetClientExtensionParameters(f.clientID, f.extensionID)
		if err != nil {
			t.Fatal(err)
		}
		values["proactiveContextLength"] = bad
		if err := f.core.UpdateClientExtensionParameters(f.clientID, f.extensionID, values); err == nil {
			t.Fatal("invalid context length pair accepted", bad)
		}
	}
}

func latestQQToolMode(request api.ChatCompletionsRequest) string {
	for index := len(request.Messages) - 1; index >= 0; index-- {
		message := request.Messages[index]
		if message.Role == "system" && strings.Contains(fmt.Sprint(message.Content), "当前工具使用模式") {
			return fmt.Sprint(message.Content)
		}
	}
	return ""
}

func latestQQToolResult(request api.ChatCompletionsRequest) string {
	for index := len(request.Messages) - 1; index >= 0; index-- {
		if message := request.Messages[index]; message.Role == "tool" {
			return fmt.Sprint(message.Content)
		}
	}
	return ""
}

func TestQQToolExecutionRequiresDirectMentionAndDeclarationsStayStable(t *testing.T) {
	f := useQQChat(t, map[string]string{"groupContext": "true", "agentEnabled": "false", "longTermMemory": "true"}, func(w http.ResponseWriter, r *http.Request, request api.ChatCompletionsRequest) {
		last := request.Messages[len(request.Messages)-1]
		if last.Role == "tool" {
			qqSSE(w, "完成")
			return
		}
		if strings.Contains(fmt.Sprint(last.Content), "memory-only") {
			var library struct {
				ID string `json:"id"`
			}
			for _, message := range request.Messages {
				if message.Role == "system" && strings.Contains(fmt.Sprint(message.Content), "内部个人长期记忆如下") {
					parts := strings.SplitN(fmt.Sprint(message.Content), "\n", 2)
					json.Unmarshal([]byte(parts[1]), &library)
				}
			}
			qqSSE(w, "", approvalCall("memory", "CreateMemory", map[string]any{"recollection_id": library.ID, "data": "ordinary-memory", "tags": []string{"qq"}}))
			return
		}
		if strings.Contains(fmt.Sprint(last.Content), "request-blocked") {
			qqSSE(w, "", approvalCall("apply", agenttool.RequestToolExecutionID, map[string]any{"call_id": "old-call"}))
			return
		}
		qqSSE(w, "", approvalCall("write", "WriteFile", map[string]any{"path": []string{"mention.txt"}, "content": "allowed"}))
	})
	strict := true
	config, err := f.core.GetClientExtensionChatConfig(f.clientID, f.extensionID)
	if err != nil {
		t.Fatal(err)
	}
	config.ToolConfig.StrictMode = &strict
	if _, err := f.core.UpdateClientExtensionChatConfig(f.clientID, f.extensionID, *config); err != nil {
		t.Fatal(err)
	}
	exchange := func(text string, mentioned bool) (api.ChatCompletionsRequest, api.ChatCompletionsRequest) {
		f.message(t, "group", 30003, 23456, text, mentioned)
		first, second := f.request(t), f.request(t)
		f.reply(t)
		return first, second
	}
	ordinary, blocked := exchange("助手 ordinary-write", false)
	if !strings.Contains(latestQQToolMode(ordinary), "只允许调用记忆工具") || !strings.Contains(latestQQToolResult(blocked), "其他工具需要用户直接 @") {
		t.Fatal("ordinary message lacked enforced memory-only policy")
	}
	record := f.contexts(t)[0]
	file := filepath.Join(record.Workspace.Path, "mention.txt")
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("ordinary message executed file tool", err)
	}
	mentioned, result := exchange("mention-authorized", true)
	if !strings.Contains(latestQQToolMode(mentioned), "直接 @ 机器人触发") || strings.Contains(latestQQToolResult(result), "工具执行失败") {
		t.Fatal("direct @ did not allow configured tool", qqRequestText(result))
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "allowed" {
		t.Fatal("direct @ file write failed", err)
	}
	if len(f.contexts(t)) != 1 || f.contexts(t)[0].ID != record.ID {
		t.Fatal("disabled agent option still created an independent context")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	_, blocked = exchange("助手 ordinary-after-mention @10001", false)
	if _, err := os.Stat(file); !os.IsNotExist(err) || !strings.Contains(latestQQToolResult(blocked), "其他工具需要用户直接 @") {
		t.Fatal("ordinary message inherited a previous @ authorization", err)
	}
	_, result = exchange("助手 memory-only", false)
	if !strings.Contains(latestQQToolResult(result), "已创建记忆") {
		t.Fatal("ordinary memory tool was blocked")
	}
	_, blocked = exchange("助手 request-blocked", false)
	if !strings.Contains(latestQQToolResult(blocked), "其他工具需要用户直接 @") {
		t.Fatal("strict execution request bypassed mention check")
	}
	if !reflect.DeepEqual(ordinary.Tools, mentioned.Tools) || len(ordinary.Tools) == 0 {
		t.Fatal("trigger type changed tool declarations")
	}
	for index := 1; index < len(ordinary.Tools); index++ {
		if ordinary.Tools[index-1].Function.Name >= ordinary.Tools[index].Function.Name {
			t.Fatal("tool declaration order is unstable")
		}
	}
	for _, segment := range []qq.QQMessageSegment{
		{Type: qq.QQSegmentReply, Data: map[string]any{"id": "2"}},
		{Type: qq.QQSegmentAt, Data: map[string]any{"qq": "all"}},
		{Type: qq.QQSegmentAt, Data: map[string]any{"qq": "40004"}},
	} {
		f.messageSegments(t, "group", 30003, 23456, segment, qq.QQText("助手 indirect-mention"))
		f.request(t)
		blocked = f.request(t)
		f.reply(t)
		if !strings.Contains(latestQQToolResult(blocked), "其他工具需要用户直接 @") {
			t.Fatal("an indirect or unrelated mention granted tool access")
		}
	}
	f.options(t, map[string]string{"agentEnabled": "true"})
	_, result = exchange("agent-authorized", true)
	if strings.Contains(latestQQToolResult(result), "工具执行失败") {
		t.Fatal("independent agent could not execute configured tools")
	}
	var agentPath string
	for _, item := range f.contexts(t) {
		if item.Kind == "agent" {
			agentPath = item.Workspace.Path
		}
	}
	if agentPath == "" {
		t.Fatal("agent context not created")
	}
	if _, err := os.Stat(filepath.Join(agentPath, "mention.txt")); err != nil {
		t.Fatal("agent file tool failed", err)
	}
}
