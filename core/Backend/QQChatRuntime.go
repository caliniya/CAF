package Backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	netclient "github.com/caliniya/caf/NetClient"
	api "github.com/caliniya/caf/NetClient/APIImpl"
	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
	chatstruct "github.com/caliniya/caf/core/Chat/ChatStruct"
	qq "github.com/caliniya/caf/core/Extension/Onebot11QQ"
	persona "github.com/caliniya/caf/core/Persona"
	texts "github.com/caliniya/caf/utils/text"
)

type qqRecentMessage struct {
	Message    qq.QQMessage `json:"message"`
	ReceivedAt time.Time    `json:"receivedAt"`
}

type qqChatJob struct {
	key               string
	kind              string
	message           qq.QQMessage
	receivedAt        time.Time
	nearby            []qqRecentMessage
	options           QQChatOptions
	config            ExtensionChatConfig
	commandName       string
	revision          uint64
	epoch             uint64
	proactive         bool
	mentioned         bool
	proactiveMessages []qqRecentMessage
	proactiveDecision *qqProactiveDecision
}

type qqProactiveDecision struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type qqContextWorker struct {
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	jobs      chan qqChatJob
	cancelRun context.CancelFunc // 由 runtime.mu 保护，只取消当前生成，不改变排队顺序。
}

type qqChatRuntime struct {
	core          *Core
	server        *QQServer
	adapterID     string
	clientID      string
	directory     string
	client        qq.Extension
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	wg            sync.WaitGroup
	generations   chan struct{}
	storageMu     sync.Mutex
	storageClosed bool

	mu             sync.Mutex
	closed         bool
	revision       uint64
	changedAt      time.Time
	records        map[string]QQContextInfo
	usage          QQTokenStats
	workers        map[string]*qqContextWorker
	clearing       map[string]bool
	epochs         map[string]uint64
	clearedAt      map[string]time.Time
	recent         map[string][]qqRecentMessage
	senderRecent   map[string][]qqRecentMessage
	batches        map[string][]qqChatJob
	lastSpoke      map[string]time.Time
	decisions      map[string]*qqProactiveDecision
	seen           map[string]bool
	seenOrder      []string
	interactions   map[string]*qqChatInteraction
	messageCache   map[string]qq.QQMessage
	messageOrder   []string
	decisionClient *netclient.APIClient
}

func (c *Core) newQQChatRuntime(parent context.Context, adapterID string, client qq.Extension) (*qqChatRuntime, error) {
	q, err := c.Servers.Get[*QQServer](QQServerName)
	if err != nil {
		return nil, err
	}
	q.chatMu.Lock()
	previous := q.chats[adapterID]
	q.chatMu.Unlock()
	if previous != nil {
		previous.cancel()
		select {
		case <-previous.done:
		case <-parent.Done():
			return nil, parent.Err()
		}
	}
	c.extMu.Lock()
	defer c.extMu.Unlock()
	if err := parent.Err(); err != nil {
		return nil, err
	}
	owner, _ := c.qqCommandOwnerLocked(adapterID)
	if owner == nil {
		return nil, nil
	}
	directory, err := c.qqChatDirectory(owner.ID, adapterID)
	if err != nil {
		return nil, err
	}
	if err := c.FS.CreateDirectory(directory, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	r := &qqChatRuntime{core: c, server: q, adapterID: adapterID, clientID: owner.ID, directory: directory,
		client: client, ctx: ctx, cancel: cancel, done: make(chan struct{}), generations: make(chan struct{}, 4),
		records: map[string]QQContextInfo{}, workers: map[string]*qqContextWorker{}, clearing: map[string]bool{}, epochs: map[string]uint64{}, clearedAt: map[string]time.Time{},
		recent: map[string][]qqRecentMessage{}, senderRecent: map[string][]qqRecentMessage{}, batches: map[string][]qqChatJob{}, lastSpoke: map[string]time.Time{},
		decisions: map[string]*qqProactiveDecision{}, seen: map[string]bool{}, interactions: map[string]*qqChatInteraction{}, messageCache: map[string]qq.QQMessage{}}
	if err := r.loadContextsLocked(); err != nil {
		cancel()
		return nil, err
	}
	if r.usage, err = c.readQQTokenStats(directory); err != nil {
		cancel()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, err
	}
	q.chatMu.Lock()
	if q.chats == nil {
		q.chats = map[string]*qqChatRuntime{}
	}
	q.chats[adapterID] = r
	q.chatMu.Unlock()
	return r, nil
}

// 调用方持有 extMu。归档前关闭写入入口，取消中的旧任务不能重建活跃目录。
func (c *Core) closeQQChatStorageLocked(adapterID string) {
	resume := c.pauseQQChatStorageLocked([]string{adapterID})
	resume(true)
}

// 归档与运行中的保存共用同一写入锁；归档失败时可以恢复原运行状态。
func (c *Core) pauseQQChatStorageLocked(adapterIDs []string) func(bool) {
	q, err := c.Servers.Get[*QQServer](QQServerName)
	if err != nil {
		return func(bool) {}
	}
	q.chatMu.Lock()
	runtimes := []*qqChatRuntime{}
	for _, adapterID := range adapterIDs {
		if runtime := q.chats[adapterID]; runtime != nil {
			runtimes = append(runtimes, runtime)
		}
	}
	q.chatMu.Unlock()
	for _, runtime := range runtimes {
		runtime.mu.Lock()
		runtime.storageMu.Lock()
	}
	return func(closeStorage bool) {
		for index := len(runtimes) - 1; index >= 0; index-- {
			runtime := runtimes[index]
			if closeStorage {
				runtime.storageClosed = true
				runtime.cancel()
			}
			runtime.storageMu.Unlock()
			runtime.mu.Unlock()
		}
	}
}

func (r *qqChatRuntime) shutdown() {
	r.cancel()
	r.mu.Lock()
	r.closed = true
	for _, worker := range r.workers {
		worker.cancel()
	}
	r.cancelDecisionsLocked()
	r.mu.Unlock()
	r.wg.Wait()
	if service, err := r.core.Servers.Get[*ChatServer](ChatServerName); err == nil {
		service.mu.Lock()
		for _, record := range r.records {
			delete(service.ChatList, record.ID)
		}
		service.mu.Unlock()
	}
	r.server.chatMu.Lock()
	if r.server.chats[r.adapterID] == r {
		delete(r.server.chats, r.adapterID)
	}
	r.server.chatMu.Unlock()
	close(r.done)
}

// 配置保存后取消在途生成与排队任务；新消息使用新快照，已保存历史保留。
func (c *Core) cancelQQChatRuns(adapterID string) {
	q, err := c.Servers.Get[*QQServer](QQServerName)
	if err != nil {
		return
	}
	q.chatMu.Lock()
	r := q.chats[adapterID]
	q.chatMu.Unlock()
	if r == nil {
		return
	}
	r.mu.Lock()
	r.revision++
	r.changedAt = time.Now()
	for _, worker := range r.workers {
		if worker.cancelRun != nil {
			worker.cancelRun()
		}
		for {
			select {
			case <-worker.jobs:
				continue
			default:
			}
			break
		}
	}
	r.cancelDecisionsLocked()
	r.recent, r.batches = map[string][]qqRecentMessage{}, map[string][]qqChatJob{}
	r.senderRecent = map[string][]qqRecentMessage{}
	r.decisionClient = nil
	r.mu.Unlock()
}

func (r *qqChatRuntime) cancelDecisionsLocked() {
	for _, decision := range r.decisions {
		decision.cancel()
	}
	r.decisions = map[string]*qqProactiveDecision{}
}

func (r *qqChatRuntime) snapshot() (*ExtensionClient, ClientExtension, QQChatOptions, ExtensionChatConfig, uint64, bool) {
	r.core.extMu.Lock()
	defer r.core.extMu.Unlock()
	owner, index, err := r.core.clientExtensionLocked(r.clientID, r.adapterID)
	if err != nil || !owner.Extensions[index].Enabled {
		return nil, ClientExtension{}, QQChatOptions{}, ExtensionChatConfig{}, 0, false
	}
	binding := owner.Extensions[index]
	r.server.mu.Lock()
	adapterIndex, err := r.core.qqAdapterIndexLocked(r.adapterID)
	if err != nil {
		r.server.mu.Unlock()
		return nil, ClientExtension{}, QQChatOptions{}, ExtensionChatConfig{}, 0, false
	}
	options := copyQQChatOptions(r.server.adapters[adapterIndex].Options)
	r.server.mu.Unlock()
	r.mu.Lock()
	revision := r.revision
	r.mu.Unlock()
	return owner, binding, options, r.core.extensionChatConfig(binding.Chat), revision, true
}

func qqPlainText(message qq.QQMessage) string {
	var result strings.Builder
	for _, segment := range message.Segments {
		if segment.Type == qq.QQSegmentText {
			value, _ := segment.Data["text"].(string)
			result.WriteString(value)
		}
	}
	return result.String()
}

func qqMentioned(message qq.QQMessage) bool {
	if message.SelfID == "" {
		return false
	}
	for _, segment := range message.Segments {
		if segment.Type == qq.QQSegmentAt && fmt.Sprint(segment.Data["qq"]) == message.SelfID {
			return true
		}
	}
	return false
}

func qqRoute(message qq.QQMessage, options QQChatOptions, agent bool) (string, string) {
	kind, target := "person", message.UserID
	if agent {
		kind = "agent"
	} else if !options.ContextIsolation {
		if message.Type == qq.QQMessageGroup {
			kind, target = "group", message.GroupID
		} else {
			kind = "private"
		}
	}
	return qqContextKey(message.SelfID, kind, target), kind
}

func (r *qqChatRuntime) handle(incoming qqCommandJob) {
	message := incoming.Message
	if r.ctx.Err() != nil || message.UserID == "" {
		return
	}
	if message.SelfID == "" {
		message.SelfID = r.client.Info().SelfID
	}
	if message.SelfID == "" || (message.Type != qq.QQMessagePrivate && message.Type != qq.QQMessageGroup) {
		return
	}
	owner, binding, options, config, revision, valid := r.snapshot()
	if !valid {
		return
	}
	if message.Type == qq.QQMessageGroup && (message.GroupID == "" || len(options.GroupWhitelist) > 0 && !slices.Contains(options.GroupWhitelist, message.GroupID)) {
		return
	}
	r.mu.Lock()
	if revision != r.revision || incoming.ReceivedAt.Before(r.changedAt) {
		r.mu.Unlock()
		return
	}
	key := fmt.Sprintf("%s:%s:%s:%s:%s", message.SelfID, message.Type, message.GroupID, message.UserID, message.ID)
	if message.ID != "" {
		if r.seen[key] {
			r.mu.Unlock()
			return
		}
		r.seen[key] = true
		r.seenOrder = append(r.seenOrder, key)
		if len(r.seenOrder) > 1024 {
			delete(r.seen, r.seenOrder[0])
			r.seenOrder = r.seenOrder[1:]
		}
	}
	groupKey := message.SelfID + ":" + message.GroupID
	r.cacheMessageLocked(message)
	nearby := []qqRecentMessage{}
	senderKey := message.SelfID + ":" + message.UserID
	senderHistory := r.senderRecent[senderKey]
	proactiveNearby := []qqRecentMessage{}
	if message.Type == qq.QQMessageGroup {
		recent := r.recent[groupKey]
		proactiveNearby = qqProactiveContext(recent, senderHistory, *options.ProactiveContextLength)
		if options.GroupContext {
			start := max(0, len(recent)-options.GroupContextLength)
			nearby = slices.Clone(recent[start:])
		}
		recent = append(recent, qqRecentMessage{Message: message, ReceivedAt: incoming.ReceivedAt.UTC()})
		if len(recent) > 100 {
			recent = recent[len(recent)-100:]
		}
		r.recent[groupKey] = recent
	}
	if !message.SentBySelf && message.UserID != message.SelfID {
		senderHistory = append(senderHistory, qqRecentMessage{Message: message, ReceivedAt: incoming.ReceivedAt.UTC()})
		r.senderRecent[senderKey] = slices.Clone(senderHistory[max(0, len(senderHistory)-100):])
	}
	r.mu.Unlock()
	if message.SentBySelf || message.UserID == message.SelfID {
		return
	}
	allowed := !options.WhitelistEnabled || slices.Contains(options.Whitelist, message.UserID)
	if !allowed {
		return
	}
	if command, ok := qq.ParseCommand(message, binding.Name); ok {
		r.command(owner, binding, options, message, command)
		return
	}
	mentioned := qqMentioned(message)
	agent := message.Type == qq.QQMessageGroup && options.AgentEnabled && mentioned
	triggered := message.Type == qq.QQMessagePrivate || mentioned
	words := options.TriggerKeywords
	if len(words) == 0 {
		words = []string{binding.Name}
	}
	for _, word := range words {
		if strings.Contains(qqPlainText(message), word) {
			triggered = true
			break
		}
	}
	routeKey, kind := qqRoute(message, options, agent)
	r.mu.Lock()
	if incoming.ReceivedAt.Before(r.clearedAt[routeKey]) || r.clearing[routeKey] {
		r.mu.Unlock()
		return
	}
	epoch := r.epochs[routeKey]
	r.mu.Unlock()
	job := qqChatJob{key: routeKey, kind: kind, message: message, receivedAt: incoming.ReceivedAt, nearby: nearby, options: options, config: config, commandName: binding.Name, revision: revision, epoch: epoch, mentioned: mentioned}
	if triggered {
		r.mu.Lock()
		delete(r.batches, groupKey)
		if decision := r.decisions[groupKey]; decision != nil {
			decision.cancel()
			delete(r.decisions, groupKey)
		}
		r.mu.Unlock()
		r.enqueue(job)
	} else if message.Type == qq.QQMessageGroup && options.ProactiveEnabled {
		job.nearby = proactiveNearby
		r.batchProactive(groupKey, job)
	}
}

func (r *qqChatRuntime) enqueue(job qqChatJob) bool {
	r.mu.Lock()
	if r.ctx.Err() != nil || r.clearing[job.key] || job.revision != r.revision || job.epoch != r.epochs[job.key] {
		r.mu.Unlock()
		return false
	}
	record, err := r.ensureContextLocked(job)
	if err != nil {
		r.mu.Unlock()
		r.report(err)
		return false
	}
	worker := r.workers[job.key]
	if worker == nil {
		ctx, cancel := context.WithCancel(r.ctx)
		worker = &qqContextWorker{ctx: ctx, cancel: cancel, done: make(chan struct{}), jobs: make(chan qqChatJob, 32)}
		r.workers[job.key] = worker
		r.wg.Add(1)
		go r.runWorker(job.key, record, worker)
	}
	if worker.ctx.Err() != nil {
		// 等待旧任务完成后再接受新请求，同一会话文件不会被两个生成同时写入。
		r.wg.Add(1)
		r.mu.Unlock()
		go func() {
			defer r.wg.Done()
			select {
			case <-worker.done:
				if !r.enqueue(job) {
					r.releaseProactive(job)
				}
			case <-r.ctx.Done():
				r.releaseProactive(job)
			}
		}()
		return true
	}
	select {
	case worker.jobs <- job:
	default:
		err = errors.New("QQ 上下文队列已满，当前消息未提交模型")
	}
	r.mu.Unlock()
	if err != nil {
		r.report(err)
	}
	return err == nil
}

func (r *qqChatRuntime) releaseProactive(job qqChatJob) {
	if job.proactiveDecision == nil {
		return
	}
	job.proactiveDecision.cancel()
	r.mu.Lock()
	key := job.message.SelfID + ":" + job.message.GroupID
	if r.decisions[key] == job.proactiveDecision {
		delete(r.decisions, key)
		r.startProactiveLocked(key)
	}
	r.mu.Unlock()
}

func (r *qqChatRuntime) runWorker(key string, record QQContextInfo, worker *qqContextWorker) {
	defer r.wg.Done()
	defer func() {
		r.mu.Lock()
		if r.workers[key] == worker {
			delete(r.workers, key)
		}
		close(worker.done)
		r.mu.Unlock()
	}()
	for {
		select {
		case <-worker.ctx.Done():
			return
		case job := <-worker.jobs:
			r.mu.Lock()
			if worker.ctx.Err() != nil {
				r.mu.Unlock()
				return
			}
			if job.revision != r.revision || job.epoch != r.epochs[key] || r.clearing[key] || job.proactiveDecision != nil && job.proactiveDecision.ctx.Err() != nil {
				r.mu.Unlock()
				r.releaseProactive(job)
				continue
			}
			jobContext, cancel := context.WithCancel(worker.ctx)
			worker.cancelRun = cancel
			r.mu.Unlock()
			stopProactive := func() bool { return true }
			if job.proactiveDecision != nil {
				stopProactive = context.AfterFunc(job.proactiveDecision.ctx, cancel)
			}
			err := r.generate(jobContext, record, job)
			if err != nil && jobContext.Err() == nil {
				r.report(err)
				if !job.proactive {
					_ = r.send(jobContext, job.message, "本次回复失败："+err.Error())
				}
			}
			cancel()
			stopProactive()
			r.releaseProactive(job)
			r.mu.Lock()
			worker.cancelRun = nil
			r.mu.Unlock()
		}
	}
}

func (r *qqChatRuntime) generate(parent context.Context, record QQContextInfo, job qqChatJob) error {
	if job.proactive {
		r.mu.Lock()
		coolingDown := time.Since(r.lastSpoke[job.message.SelfID+":"+job.message.GroupID]) < time.Duration(job.options.ProactiveCooldown)*time.Second
		r.mu.Unlock()
		if coolingDown {
			return nil
		}
	}
	select {
	case r.generations <- struct{}{}:
		defer func() { <-r.generations }()
	case <-parent.Done():
		return parent.Err()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	session, err := r.readSession(record)
	if err != nil {
		return err
	}
	provider, err := r.core.configByID(job.config.ConfigID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(provider.Model, func(model api.Model) bool { return model.ID == job.config.Model }) {
		return errors.New("QQ 所选模型已不存在")
	}
	config := *provider
	session.ConfigID, session.Model, session.ReasoningEffort = job.config.ConfigID, job.config.Model, job.config.ReasoningEffort
	session.Tree.Root.PersonaID = persona.Resolve(job.config.PersonaID).ID
	if record.Kind == "agent" {
		session.Tree.Root.PersonaID = persona.DefaultID
	}
	previousTools := session.ToolConfig.Clone()
	session.ToolConfig = job.config.ToolConfig.Clone()
	if session.ToolConfig == nil {
		session.ToolConfig = &agenttool.ChatToolConfig{}
	}
	session.ToolConfig.Permission = agenttool.PermissionAutoEdit
	session.ToolConfig.AllowedTools = nil
	if previousTools != nil {
		for id, grant := range previousTools.AllowedTools {
			if grant.Scope == agenttool.GrantWorkspace && grant.WorkspaceID == record.Workspace.ID {
				if session.ToolConfig.AllowedTools == nil {
					session.ToolConfig.AllowedTools = map[string]agenttool.ToolGrant{}
				}
				session.ToolConfig.AllowedTools[id] = grant
			}
		}
	}
	session.MemoryLibraryIDs = nil
	if err := r.core.FS.CreateDirectory(record.Workspace.Path, 0o700); err != nil {
		return err
	}
	options := &chatExecutionOptions{workspace: &record.Workspace, save: r.saveSession,
		systemPrompt:          fmt.Sprintf(texts.QQChatSystem, record.Kind),
		recordUsage:           func(usage *api.TokenUsage) { r.recordTokenUsage(job, usage, true) },
		recordCompactionUsage: func(usage *api.TokenUsage) { r.recordTokenUsage(job, usage, false) }}
	options.checkTool = qqToolCheck(job.mentioned)
	options.askUser = func(callCtx context.Context, chatID, nodeID, callID string, question agenttool.UserQuestion) (string, error) {
		return r.askUser(callCtx, job.commandName, job.message, question)
	}
	options.approve = func(callCtx context.Context, chatID, nodeID, callID string, view *api.ToolApproval, apply func(string) (*agenttool.ChatToolConfig, error)) (string, error) {
		return r.approve(callCtx, job.commandName, job.message, view, apply)
	}
	if job.options.LongTermMemory {
		if err := r.bindPersonalMemory(ctx, session, job.message.SelfID, job.message.UserID, options); err != nil {
			return err
		}
	}
	messages := []api.ChatMessage{}
	if len(job.nearby) > 0 {
		body, err := json.Marshal(job.nearby)
		if err != nil {
			return err
		}
		prefix := texts.QQNearbyMessages
		if job.proactive {
			prefix = texts.QQProactiveContext
		}
		messages = append(messages, api.ChatMessage{Role: "system", Content: prefix + string(body)})
	}
	references := r.resolveReferences(ctx, job)
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(references) > 0 {
		body, err := json.Marshal(references)
		if err != nil {
			return err
		}
		messages = append(messages, api.ChatMessage{Role: "system", Content: texts.QQReferencedMessages + string(body)})
	}
	message, err := job.message.ToChatMessage()
	if err != nil {
		return err
	}
	if job.proactive {
		body, err := json.Marshal(job.proactiveMessages)
		if err != nil {
			return err
		}
		message.Content = texts.QQProactiveMessages + string(body)
	}
	mode := texts.QQMemoryOnlyTools
	if job.mentioned {
		mode = texts.QQConfiguredTools
	}
	messages = append(messages, api.ChatMessage{Role: "system", Content: mode})
	messages = append(messages, message)
	path, err := session.Tree.CurrentPath()
	if err != nil {
		return err
	}
	parentID := ""
	if len(path) > 0 {
		parentID = path[len(path)-1].ID
	}
	user := chatstruct.CreateChatNode(messages...)
	if err := session.Tree.AddNext(parentID, user); err != nil {
		return err
	}
	reply := chatstruct.CreateChatNode()
	if err := session.Tree.AddNext(user.ID, reply); err != nil {
		return err
	}
	chatServer, err := r.core.Servers.Get[*ChatServer](ChatServerName)
	if err != nil {
		return err
	}
	chatServer.mu.Lock()
	life, err := chatServer.contextLocked()
	if err != nil {
		chatServer.mu.Unlock()
		return err
	}
	chatServer.initLocked()
	if chatServer.sending[session.ID] != nil {
		chatServer.mu.Unlock()
		return errors.New("QQ 上下文正在生成中")
	}
	stop := context.AfterFunc(life, cancel)
	chatServer.sending[session.ID] = &chatSend{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	chatServer.mu.Unlock()
	defer stop()
	defer r.core.endChatSend(session.ID)
	// 提醒必须在生成任务收尾取消 ctx 之前发送。
	defer func() { r.report(r.warnLongContext(ctx, job)) }()
	if err := r.core.runChatNode(session.ID, session, &config, r.directory, reply, session.ReasoningEffort, options); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	current := r.revision == job.revision && r.epochs[job.key] == job.epoch && !r.clearing[job.key]
	r.mu.Unlock()
	if !current {
		return context.Canceled
	}
	text := ""
	// 工具轮中的中间说明不作为额外 QQ 消息发送，只发送最终自然语言回复。
	for i := len(reply.Child) - 1; i >= 0; i-- {
		if reply.Child[i].Role == "assistant" && len(reply.Child[i].ToolCalls) == 0 {
			text = strings.TrimSpace(reply.Child[i].Content)
			break
		}
	}
	if text != "" {
		if err := r.send(ctx, job.message, text); err != nil {
			return err
		}
	}
	r.mu.Lock()
	// 每轮请求会更新用量，不能用生成开始时的副本覆盖当前记录。
	record, exists := r.records[job.key]
	if !exists || r.epochs[job.key] != job.epoch || r.clearing[job.key] {
		r.mu.Unlock()
		return context.Canceled
	}
	record.UpdatedAt = session.UpdatedAt
	r.records[job.key] = record
	if text != "" {
		r.lastSpoke[job.message.SelfID+":"+job.message.GroupID] = time.Now()
	}
	err = r.saveContextsLocked()
	r.mu.Unlock()
	return err
}

func (r *qqChatRuntime) send(parent context.Context, target qq.QQMessage, text string) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	message := qq.QQMessage{SelfID: r.client.Info().SelfID, Type: target.Type, UserID: target.UserID, GroupID: target.GroupID, Segments: []qq.QQMessageSegment{qq.QQText(text)}}
	result, err := r.client.SendMessage(ctx, message)
	if err == nil {
		message.ID, message.Time, message.SentBySelf = result.MessageID, time.Now().Unix(), true
		message.Sender = &qq.QQSender{UserID: message.SelfID}
		r.mu.Lock()
		r.cacheMessageLocked(message)
		r.mu.Unlock()
	}
	return err
}

func (r *qqChatRuntime) report(err error) {
	if err == nil || r.ctx.Err() != nil {
		return
	}
	r.server.mu.Lock()
	if runtime := r.server.clients[r.adapterID]; runtime != nil && runtime.client == r.client {
		runtime.err = err.Error()
	}
	r.server.mu.Unlock()
}
