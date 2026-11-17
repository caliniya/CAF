# 记忆存储

宿主注入文件系统和已有取时客户端，并将 `*MemoryServer` 注册到 `Servers`；记忆包不依赖 `Backend`。对话设置和创建记忆工具通过宿主提供的方法访问此服务。

```go
server := memory.NewMemoryServer(paths.Data, fileSystem, timeClient)
err := server.Start(ctx)
```

`Init` 创建 Main，校验目录与索引并恢复未完成写入。`Start` 还会读取宿主已有的 `workspaces.json`，为每个工作区补充局部库；后续工作区变化由调用方执行 `EnsureWorkspace` 或 `SyncWorkspaces`。不会后台轮询或修改工作区服务。

桌面宿主启动时使用 `Init`，只在新建工作区时创建默认局部记忆。删除或提升默认记忆后，切换工作区和重启不会重新补建；`Start` / `SyncWorkspaces` 留给需要主动补建的调用方。

目录名称使用记忆库名称：

```text
data/memory/
  index.json
  Main/
    meta.json
    data.json
    history.json
  项目/
    meta.json
    data.json
    history.json
  .archive/                 # 删除的整个记忆库
```

`index.json` 直接保存 `{ "recollection-ID": "名称" }`。`meta.json` 只有 `id`、`name`、`description`、`date`、`scope`，局部库另有 `workspaceId`。作用域为 `global` 或 `workspace`，绑定使用工作区 ID；默认局部库重名时自动追加工作区 ID。`Main`、`main`、`MAIN` 等保留名称禁止普通库使用，正式 Main 不可删除、重命名或改为工作区作用域；界面显示名称为“全局主记忆”，存储名称仍为 `Main`。

`data.json` 直接序列化完整 Recollection，无额外包装：

```json
{
  "name": "Main",
  "description": "全局记忆库",
  "data": {
    "memory-ID": {
      "data": "记忆正文",
      "date": "2026-11-17T08:00:00Z",
      "description": ["标签"]
    }
  }
}
```

`history.json` 直接保存 `{ "memory-ID": [旧Memory副本, ...] }`。副本保持原来的正文、标签和时间，可以重复。新增不记历史；修改、删除先归档旧副本；相同内容的保存不新增历史。历史只在修改旧记忆、恢复或明确查询历史时读取，正常启动及当前检索不加载旧历史。没有版本 ID、修改原因、额外创建/更新时间字段。

方法：

- 库：`ListRecollections`、`GetMeta`、`GetRecollection`、`CreateRecollection`、`UpdateRecollection`、`DeleteRecollection`、`ChangeScope`；作用域切换保留 ID、正文和历史，只更新元数据中的作用域、工作区绑定和时间。
- 工作区：`EnsureWorkspace(ctx, workspaceID, name)`、`SyncWorkspaces`；重复调用在已有局部库中返回一个，不重复创建。用户可以为同一工作区创建多个局部库。
- 单元：`AddMemory`、`UpdateMemory`、`DeleteMemory`，或通过 `SaveRecollection` 保存整个库，自动归档变更的单元。
- 历史：`History(ctx, libraryID, memoryID)`，`RestoreMemory(ctx, libraryID, memoryID, index)` 按数组下标恢复。
- 检索：`Search(ctx, enabledLibraryIDs, queries...)` 只查询明确启用的库，任意作用域均可选择；空库列表表示全部禁用，Main 不会强制加入。按正文子串或标签匹配，历史不参与检索。
- 校验：`Init` 对照索引与元数据；索引缺失时重建，已有错误索引返回错误。`RebuildIndex` 可显式从有效库重建索引，损坏库不会被忽略。

宿主只向对话提供全局记忆和其当前工作区的记忆，并按对话保存明确启用的 ID；不存在或已变为其他工作区作用域的 ID 在读取时自动清理。草稿同样按照已选工作区筛选。模型上下文和创建记忆工具再次校验工作区范围，全局写入必须是完全访问权限。记忆设置支持创建、重命名、删除和范围变更，删除及范围变更需要界面确认。

已有消息的对话切换工作区时，界面先提示对缓存命中率和模型工作能力的影响。后端保留全局记忆的启用记录，移除原工作区记忆，并在当前对话分支追加一条 `system` 消息记录切换前后的工作区名称；空对话和重复选择同一工作区不追加消息。

记忆只有 `date` 一个时间字段，meta 的 `date` 表示该库最近发生变更的时间；一次保存共用一个时间。时间以 RFC3339Nano 保存，有注入的取时客户端就复用它，未注入时使用本机 UTC。独立的 `CreateMemory` 构造函数使用本机时间；正式保存时由服务重新赋值。返回内容和历史均为深拷贝，修改返回的标签不会污染存储。

写入前生成临时 `.pending.json`，保存待发布的完整文件内容；每个文件先写 `.tmp` 再替换，索引最后发布。中断后下次初始化或读取会完成同一次写入，避免重复追加历史。成功后删除临时记录，业务 JSON 不增加事务或版本字段。失败发生在提交后时，方法可能返回错误，而恢复最终仍会完成该修改；调用方应先重新读取结果再决定是否重试。

同一记忆目录由一个服务实例写入，内部锁串行处理操作；不支持多进程同时写。临时文件替换支持进程中断恢复，未承诺硬件断电后的持久化事务保证。索引用于结构一致性校验，不能证明正文未被改成另一份合法 JSON。公开缓存 `Recollections` 只供运行时持有，调用方应通过方法读写。
