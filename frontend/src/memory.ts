import type { MemoryLibrary, Workspace } from './api'

export function memoryScopeLabel(library: MemoryLibrary, workspaces: Workspace[] = []): string {
  if (library.scope === 'global') return '全局记忆'
  const name = workspaces.find(workspace => workspace.id === library.workspaceId)?.name ?? '已删除的'
  return `${name}工作区记忆`
}
