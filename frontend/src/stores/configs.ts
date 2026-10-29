import { defineStore } from 'pinia'
import {
  LoadConfigs,
  SelectConfig,
  SelectModel,
  RenameConfig,
  ProbeModels,
  AddModelConfig,
  RefreshModelConfig,
  type Config,
  type Configs,
  type Model,
} from '../api'

// 全局配置状态：页面切换时保留，避免重复读取配置文件。
export const useConfigsStore = defineStore('configs', {
  state: () => ({
    configs: [] as Config[],
    enables: null as Config | null,
    loaded: false,
    loading: false,
    status: '',
    error: '',
  }),
  getters: {
    isEnabled(state) {
      return (item: Config) =>
        !!state.enables && state.enables.ServerAddress === item.ServerAddress
    },
    // 当前启用的模型，随启用的配置走（每个提供商有自己的记忆）。
    enabledModel(state) {
      return state.enables?.EnabledModel ?? ''
    },
  },
  actions: {
    apply(configs: Configs) {
      this.configs = configs.config ?? []
      this.enables = configs.EnablesConfig
      this.loaded = true
    },
    async load() {
      this.loading = true
      this.error = ''
      this.status = '正在读取配置……'
      try {
        this.apply(await LoadConfigs())
      } catch (err) {
        this.error = `加载失败：${String(err)}`
      } finally {
        this.loading = false
        this.status = ''
      }
    },
    async run(action: () => Promise<Configs>) {
      this.error = ''
      this.status = '正在保存……'
      try {
        this.apply(await action())
      } catch (err) {
        this.error = `操作失败：${String(err)}`
        await this.load()
      } finally {
        this.status = ''
      }
    },
    selectConfig(index: number) {
      return this.run(() => SelectConfig(index))
    },
    selectModel(index: number, model: string) {
      return this.run(async () => {
        // 模型挂在配置下：目标卡片未启用时先切换配置，再选模型。
        if (!this.isEnabled(this.configs[index])) {
          await SelectConfig(index)
        }
        return SelectModel(model)
      })
    },
    rename(index: number, name: string) {
      return this.run(() => RenameConfig(index, name))
    },
    probeModels(serverAddress: string, apiKey: string) {
      return ProbeModels(serverAddress, apiKey)
    },
    async refreshModelConfig(index: number, serverAddress: string, models: Model[]) {
      if (models.length === 0) throw new Error('没有选择模型，原配置未更改')
      this.apply(await RefreshModelConfig(index, serverAddress, models))
    },
    addModelConfig(name: string, serverAddress: string, protocol: Config['Protocol'], apiKey: string, models: Model[]) {
      this.error = ''
      this.status = '正在添加模型服务……'
      return AddModelConfig(name, serverAddress, protocol, apiKey, models)
        .then((configs) => this.apply(configs))
        .catch((err) => {
          this.error = `添加失败：${String(err)}`
        })
        .finally(() => {
          this.status = ''
        })
    },
  },
})
