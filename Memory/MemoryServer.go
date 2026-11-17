package Memory

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	netapi "github.com/caliniya/caf/NetClient/NetAPI"
	chat "github.com/caliniya/caf/core/Chat"
	platform "github.com/caliniya/caf/platform"
	"github.com/caliniya/caf/utils/id"
)

const (
	MemoryPath       = "memory"
	MemoryServerName = "memoryServer"
	MainName         = "Main"
	ScopeGlobal      = "global"
	ScopeWorkspace   = "workspace"
	MainDisplayName  = "全局主记忆"
)

// RecollectionMeta 只保存库身份、介绍、作用域和一个最后修改时间。
type RecollectionMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Date        string `json:"date"`
	Scope       string `json:"scope"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

// 存储依赖由宿主注入。Recollections 是内部缓存，外部通过方法获取深拷贝。
// 同一存储目录只能由一个 MemoryServer 实例写入，使用后不得复制该结构体。
type MemoryServer struct {
	Recollections map[string]Recollection
	root          string
	fs            platform.FilePlatform
	clock         *netapi.TimeClient
	mu            sync.Mutex
}

// dataDir 是宿主的绝对数据目录，实际存储为 dataDir/memory。
// clock 可复用宿主取时客户端；nil 时使用本机 UTC，不隐式联网。
func NewMemoryServer(dataDir string, fileSystem platform.FilePlatform, clock *netapi.TimeClient) *MemoryServer {
	root := ""
	if dataDir != "" {
		root = filepath.Join(dataDir, MemoryPath)
	}
	return &MemoryServer{root: root, fs: fileSystem, clock: clock}
}

func (m *MemoryServer) now(ctx context.Context) string {
	if m.clock != nil {
		return m.clock.Now(ctx).Time.Format(time.RFC3339Nano)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func (m *MemoryServer) Init(ctx context.Context) error {
	if m == nil {
		return errors.New("记忆服务尚未创建")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	if m.fs == nil || !filepath.IsAbs(m.root) {
		return errors.New("记忆存储尚未初始化，请提供绝对数据路径和文件系统")
	}
	if err := m.fs.CreateDirectory(m.root, 0o700); err != nil {
		return err
	}
	if _, err := m.recoverLocked(); err != nil {
		return err
	}
	if err := m.loadLocked(); err != nil {
		return err
	}
	for _, r := range m.Recollections {
		if r.Name == MainName {
			return nil
		}
	}
	_, err := m.createLocked(ctx, MainName, "全局记忆库", ScopeGlobal, "")
	return err
}

// Start 初始化记忆并为已保存的工作区创建局部库，不创建后台任务。
// 新工作区可由宿主后续调用 EnsureWorkspace 或 SyncWorkspaces，不改工作区 API。
func (m *MemoryServer) Start(ctx context.Context) error {
	if err := m.Init(ctx); err != nil {
		return err
	}
	var stored struct {
		Workspaces []chat.WorkSpace `json:"workspaces"`
	}
	if err := m.readJSON(filepath.Join(filepath.Dir(m.root), "workspaces.json"), &stored); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return m.SyncWorkspaces(ctx, stored.Workspaces)
}

// SyncWorkspaces 幂等地补充局部库，不删除列表中未出现的已有记忆库。
func (m *MemoryServer) SyncWorkspaces(ctx context.Context, workspaces []chat.WorkSpace) error {
	for _, workspace := range workspaces {
		if _, err := m.EnsureWorkspace(ctx, workspace.ID, workspace.Name); err != nil {
			return fmt.Errorf("创建工作区 %q 的记忆库：%w", workspace.Name, err)
		}
	}
	return nil
}

// 每次修改即时持久化；Close 恢复未完成提交，不重写历史。
func (m *MemoryServer) Close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fs == nil || m.root == "" {
		return contextError(ctx)
	}
	recovered, err := m.recoverLocked()
	if err == nil && recovered {
		err = m.loadLocked()
	}
	return errors.Join(err, contextError(ctx))
}

func contextError(ctx context.Context) error {
	if ctx != nil {
		return ctx.Err()
	}
	return nil
}

func (m *MemoryServer) readyLocked(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if m.fs == nil || m.Recollections == nil {
		return errors.New("记忆服务尚未初始化")
	}
	recovered, err := m.recoverLocked()
	if err != nil {
		return err
	}
	if recovered {
		return m.loadLocked()
	}
	return nil
}

func (m *MemoryServer) ListRecollections(ctx context.Context) ([]RecollectionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	result := make([]RecollectionMeta, 0, len(m.Recollections))
	keys := make([]string, 0, len(m.Recollections))
	for key := range m.Recollections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		meta, err := m.metaLocked(key)
		if err != nil {
			return nil, err
		}
		result = append(result, meta)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// RebuildIndex 显式从有效的 meta/data 重建索引，损坏的库仍返回错误。
// Init 不自动覆盖已有的错误索引，避免掩盖目录丢失或人为移动。
func (m *MemoryServer) RebuildIndex(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	if m.fs == nil || !filepath.IsAbs(m.root) {
		return errors.New("记忆存储尚未初始化")
	}
	if _, err := m.recoverLocked(); err != nil {
		return err
	}
	return m.loadIndexLocked(true)
}

func (m *MemoryServer) GetRecollection(ctx context.Context, libraryID string) (*Recollection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return nil, err
	}
	copy := cloneRecollection(r)
	return &copy, nil
}

func (m *MemoryServer) GetMeta(ctx context.Context, libraryID string) (*RecollectionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	meta, err := m.metaLocked(libraryID)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func (m *MemoryServer) CreateRecollection(ctx context.Context, name, description, scope, workspaceID string) (*RecollectionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	if strings.EqualFold(name, MainName) {
		return nil, errors.New("Main 是全局记忆库的保留名称")
	}
	return m.createLocked(ctx, name, description, scope, workspaceID)
}

// 相同工作区重复调用返回原库；工作区改名不会改变绑定或覆盖库名称。
// 名称冲突时追加工作区 ID，避免不同工作区的局部库共用目录。
func (m *MemoryServer) EnsureWorkspace(ctx context.Context, workspaceID, name string) (*RecollectionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return nil, err
	}
	if !id.Valid(workspaceID, id.Workspace) {
		return nil, errors.New("无效工作区 ID")
	}
	for key := range m.Recollections {
		meta, err := m.metaLocked(key)
		if err != nil {
			return nil, err
		}
		if meta.WorkspaceID == workspaceID {
			return &meta, nil
		}
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	for _, r := range m.Recollections {
		if strings.EqualFold(r.Name, name) {
			name += "_" + workspaceID
			break
		}
	}
	return m.createLocked(ctx, name, "", ScopeWorkspace, workspaceID)
}

func (m *MemoryServer) createLocked(ctx context.Context, name, description, scope, workspaceID string) (*RecollectionMeta, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateScope(scope, workspaceID); err != nil {
		return nil, err
	}
	for _, r := range m.Recollections {
		if strings.EqualFold(r.Name, name) {
			return nil, errors.New("已存在同名记忆库")
		}
	}
	if _, err := m.fs.Stat(filepath.Join(m.root, name)); err == nil {
		return nil, errors.New("记忆库目录已存在")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	key, err := id.New(id.Recollection)
	if err != nil {
		return nil, err
	}
	meta := RecollectionMeta{ID: key, Name: name, Description: description, Date: m.now(ctx), Scope: scope, WorkspaceID: workspaceID}
	r := Recollection{Name: name, Description: description, Data: map[string]Memory{}}
	index := m.indexLocked()
	index[key] = name
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := m.commitLocked("", "", map[string]any{
		"index.json": index, name + "/meta.json": meta, name + "/data.json": r, name + "/history.json": map[string][]Memory{},
	}); err != nil {
		return nil, err
	}
	m.Recollections[key] = r
	return &meta, nil
}

// ChangeScope 保留 ID、当前数据和历史，只改变可见范围与工作区绑定。
func (m *MemoryServer) ChangeScope(ctx context.Context, libraryID, scope, workspaceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	meta, err := m.metaLocked(libraryID)
	if err != nil {
		return err
	}
	if err := validateScope(scope, workspaceID); err != nil {
		return err
	}
	if meta.Name == MainName && scope != ScopeGlobal {
		return errors.New("全局主记忆不能改为工作区记忆")
	}
	if meta.Scope == scope && meta.WorkspaceID == workspaceID {
		return nil
	}
	meta.Scope, meta.WorkspaceID, meta.Date = scope, workspaceID, m.now(ctx)
	if err := contextError(ctx); err != nil {
		return err
	}
	return m.commitLocked("", "", map[string]any{meta.Name + "/meta.json": meta})
}

func (m *MemoryServer) UpdateRecollection(ctx context.Context, libraryID, name, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	meta, err := m.metaLocked(libraryID)
	if err != nil {
		return err
	}
	if err := validateName(name); err != nil {
		return err
	}
	if (meta.Name == MainName && name != MainName) || (meta.Name != MainName && strings.EqualFold(name, MainName)) {
		return errors.New("不能重命名 Main 或使用其保留名称")
	}
	if meta.Name != name && strings.EqualFold(meta.Name, name) {
		return errors.New("仅修改目录名称大小写不受支持")
	}
	for key, r := range m.Recollections {
		if key != libraryID && strings.EqualFold(r.Name, name) {
			return errors.New("已存在同名记忆库")
		}
	}
	r, err := m.dataLocked(libraryID)
	if err != nil {
		return err
	}
	if meta.Name == name && meta.Description == description {
		return nil
	}
	from, to := "", ""
	if meta.Name != name {
		from, to = meta.Name, name
		if _, err := m.fs.Stat(filepath.Join(m.root, to)); err == nil {
			return errors.New("目标记忆库目录已存在")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	meta.Name, meta.Description, meta.Date = name, description, m.now(ctx)
	r.Name, r.Description = name, description
	index := m.indexLocked()
	index[libraryID] = name
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := m.commitLocked(from, to, map[string]any{"index.json": index, name + "/meta.json": meta, name + "/data.json": r}); err != nil {
		return err
	}
	m.Recollections[libraryID] = r
	return nil
}

// DeleteRecollection 归档整个库，保留 data 和 history；禁止删除 Main。
func (m *MemoryServer) DeleteRecollection(ctx context.Context, libraryID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.readyLocked(ctx); err != nil {
		return err
	}
	meta, err := m.metaLocked(libraryID)
	if err != nil {
		return err
	}
	if meta.Name == MainName {
		return errors.New("不能删除全局记忆库 Main")
	}
	index := m.indexLocked()
	delete(index, libraryID)
	if err := m.commitLocked(meta.Name, ".archive/"+libraryID, map[string]any{"index.json": index}); err != nil {
		return err
	}
	delete(m.Recollections, libraryID)
	return nil
}

func validateScope(scope, workspaceID string) error {
	if scope == ScopeGlobal && workspaceID == "" {
		return nil
	}
	if scope == ScopeWorkspace && id.Valid(workspaceID, id.Workspace) {
		return nil
	}
	return errors.New("作用域必须为 global 或 workspace，局部库需要有效工作区 ID")
}

func (m *MemoryServer) indexLocked() map[string]string {
	index := make(map[string]string, len(m.Recollections))
	for key, r := range m.Recollections {
		index[key] = r.Name
	}
	return index
}

func (m *MemoryServer) metaLocked(libraryID string) (RecollectionMeta, error) {
	r, ok := m.Recollections[libraryID]
	if !ok || !id.Valid(libraryID, id.Recollection) {
		return RecollectionMeta{}, errors.New("记忆库不存在")
	}
	var meta RecollectionMeta
	if err := m.readJSON(filepath.Join(m.root, r.Name, "meta.json"), &meta); err != nil {
		return meta, err
	}
	if err := validateMeta(meta); err != nil {
		return meta, err
	}
	if meta.ID != libraryID || meta.Name != r.Name {
		return meta, errors.New("记忆库元数据与索引不一致")
	}
	return meta, nil
}

func (m *MemoryServer) dataLocked(libraryID string) (Recollection, error) {
	meta, err := m.metaLocked(libraryID)
	if err != nil {
		return Recollection{}, err
	}
	var r Recollection
	if err := m.readJSON(filepath.Join(m.root, meta.Name, "data.json"), &r); err != nil {
		return r, err
	}
	return r, validateData(meta, r)
}

func validateMeta(meta RecollectionMeta) error {
	if !id.Valid(meta.ID, id.Recollection) {
		return fmt.Errorf("记忆库 %q 的 ID 无效", meta.Name)
	}
	if err := validateName(meta.Name); err != nil {
		return err
	}
	if strings.EqualFold(meta.Name, MainName) && (meta.Name != MainName || meta.Scope != ScopeGlobal) {
		return errors.New("Main 必须为全局记忆库，且名称大小写固定")
	}
	if err := validateScope(meta.Scope, meta.WorkspaceID); err != nil {
		return err
	}
	_, err := time.Parse(time.RFC3339Nano, meta.Date)
	return err
}
