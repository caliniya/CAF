package Backend

import (
	"context"
	"fmt"

	netclient "github.com/caliniya/caf/NetClient"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chat "github.com/caliniya/caf/core/Chat"
	persona "github.com/caliniya/caf/core/Persona"
	texts "github.com/caliniya/caf/utils/text"
)

// 内部客户端复用同一生成流程，由宿主提供存储、工作区和交互入口。
// 普通桌面对话不传入此配置，继续使用原有 API。
type chatExecutionOptions struct {
	workspace     *chat.WorkSpace
	systemPrompt  string
	memoryContext string
	memoryScope   func(context.Context, string) (bool, error)
	createMemory  func(*agenttool.ToolContext, string, string, []string) (string, error)
	readMemory    func(context.Context, string, string) (string, error)
	searchMemory  func(context.Context, string, []string) (string, error)
	listMemories  func(context.Context, string) (string, error)
	// 声明保持稳定，宿主可以按当前入口另外限制工具执行。
	checkTool func(string) error
	save      func(*chat.ChatTask) error
	// 每次模型请求完成后调用一次；nil 表示该请求没有上报用量。
	recordUsage func(*api.TokenUsage)
	// 压缩和检查也计入累计用量，但不覆盖正常对话的上下文长度。
	recordCompactionUsage func(*api.TokenUsage)
	confirmCompaction     func(context.Context, CompactionPrompt) (bool, error)
	askUser               func(context.Context, string, string, string, agenttool.UserQuestion) (string, error)
	approve               func(context.Context, string, string, string, *api.ToolApproval, func(string) (*agenttool.ChatToolConfig, error)) (string, error)
}

func (c *Core) saveExecutionChatLocked(directory string, session *chat.ChatTask, options *chatExecutionOptions) error {
	if options != nil && options.save != nil {
		return options.save(session)
	}
	return c.saveChatLocked(directory, session)
}

func firstExecutionOptions(options []*chatExecutionOptions) *chatExecutionOptions {
	if len(options) > 0 {
		return options[0]
	}
	return nil
}

func (c *Core) chatExecutionContext(ctx context.Context, session *chat.ChatTask, options *chatExecutionOptions) (*agenttool.ToolContext, string, error) {
	var toolContext *agenttool.ToolContext
	var err error
	if options != nil && options.workspace != nil {
		var path string
		path, err = c.validateWorkspacePath(options.workspace.Path)
		toolContext = agenttool.CreateContext(path, c.toolPermission(session.ToolConfig))
	} else {
		toolContext, err = c.toolContextFor(session)
	}
	if err != nil {
		return nil, "", err
	}
	toolContext.Context = ctx
	toolContext.WorkspaceID, toolContext.FileSystem = session.WorkspaceID, c.FS
	enabledMemoryIDs := append([]string(nil), session.MemoryLibraryIDs...)
	toolContext.MemoryScope = func(callCtx context.Context, libraryID string) (bool, error) {
		return c.chatMemoryScope(callCtx, session.WorkspaceID, enabledMemoryIDs, libraryID)
	}
	toolContext.CreateMemory = func(callCtx *agenttool.ToolContext, libraryID, data string, tags []string) (string, error) {
		return c.createChatMemory(callCtx.Context, session.WorkspaceID, enabledMemoryIDs, callCtx.PermissionLevel, libraryID, data, tags, callCtx.WorkspaceOnly)
	}
	c.bindChatMemoryRead(toolContext, session)
	if options != nil {
		toolContext.MemoryScope, toolContext.CreateMemory = options.memoryScope, options.createMemory
		toolContext.ReadMemory, toolContext.SearchMemory, toolContext.ListMemories = options.readMemory, options.searchMemory, options.listMemories
		return toolContext, options.memoryContext, nil
	}
	memoryContext, err := c.chatMemoryContext(ctx, session)
	return toolContext, memoryContext, err
}

func (c *Core) chatExecutionClient(session *chat.ChatTask, config *Config) error {
	server, err := c.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	server.initLocked()
	entry := server.ChatList[session.ID]
	if entry.Client == nil || entry.Client.Config == nil || entry.ConfigID != session.ConfigID || entry.Model != session.Model ||
		entry.Client.Config.ServerAddress != config.ServerAddress || entry.Client.Config.APIKey != config.APIKey {
		entry = chat.ChatTask{ConfigID: session.ConfigID, Model: session.Model,
			Client: netclient.NewAPIClient(netclient.Options{ServerAddress: config.ServerAddress, APIKey: config.APIKey, Model: session.Model})}
		server.ChatList[session.ID] = entry
	}
	session.Client = entry.Client
	return nil
}

func chatExecutionPrefix(session *chat.ChatTask, ctx *agenttool.ToolContext, memoryContext string, options *chatExecutionOptions, strict bool) []api.ChatMessage {
	prefix := make([]api.ChatMessage, 0, 5)
	if session.Tree != nil && session.Tree.Root != nil {
		if prompt, err := persona.Resolve(session.Tree.Root.PersonaID).Prompt(); err == nil {
			prefix = append(prefix, api.ChatMessage{Role: "system", Content: prompt})
		}
	}
	workspace := texts.ChatWithoutWorkspace
	if ctx.WorkSpace != "" {
		workspace = fmt.Sprintf(texts.ChatWorkspace, ctx.WorkSpace)
	}
	prefix = append(prefix, api.ChatMessage{Role: "system", Content: workspace})
	if memoryContext != "" {
		prefix = append(prefix, api.ChatMessage{Role: "system", Content: memoryContext})
	}
	if options != nil && options.systemPrompt != "" {
		prefix = append(prefix, api.ChatMessage{Role: "system", Content: options.systemPrompt})
	}
	return append(prefix, api.ChatMessage{Role: "system", Content: toolPolicyPrompt(strict, ctx, session.ToolConfig)})
}

func chatExecutionHistory(session *chat.ChatTask, model api.Model, prefix []api.ChatMessage) ([]api.ChatMessage, error) {
	request, err := session.Tree.ToChatRequest(model)
	if err != nil {
		return nil, err
	}
	history := append(append([]api.ChatMessage(nil), prefix...), request.Messages...)
	notice, err := toolChangesNotice(session)
	if err != nil {
		return nil, err
	}
	if notice != "" {
		history = append(history, api.ChatMessage{Role: "system", Content: notice})
	}
	if _, data, err := session.Tree.CurrentCompaction(); err != nil {
		return nil, err
	} else if data != nil {
		if mode, err := chatCurrentToolMode(session); err != nil {
			return nil, err
		} else if mode != "" {
			history = append(history, api.ChatMessage{Role: "system", Content: mode})
		}
		history = append(history, api.ChatMessage{Role: "system", Content: texts.CompactionContinue})
	}
	return history, nil
}

// QQ 的入口限制属于宿主状态；即使本轮的早期系统消息被压缩，也重新提供
// 当前入口的规则。放在历史之后，使稳定的请求前缀仍可复用。
func chatCurrentToolMode(session *chat.ChatTask) (string, error) {
	path, err := session.Tree.CurrentPath()
	if err != nil {
		return "", err
	}
	for node := len(path) - 1; node >= 0; node-- {
		for message := len(path[node].Child) - 1; message >= 0; message-- {
			entry := path[node].Child[message]
			if entry.Role == "system" && (entry.Content == texts.QQMemoryOnlyTools || entry.Content == texts.QQConfiguredTools) {
				return entry.Content, nil
			}
		}
	}
	return "", nil
}
