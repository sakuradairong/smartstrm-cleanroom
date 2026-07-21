import { useCallback, useEffect, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import {
  Alert,
  App,
  Button,
  Card,
  Descriptions,
  Empty,
  Popconfirm,
  Space,
  Spin,
  Table,
  Typography,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { getJSON, postJSON, putJSON } from '../api/client'
import type { ManagedConfig, StorageConfig } from '../api/types'
import type { TaskConfig } from '../api/types'
import StorageFormModal from './config/StorageFormModal'
import TaskFormModal from './config/TaskFormModal'
import JsonField, { validateJSON } from '../components/JsonField'
import SecretInput from '../components/SecretInput'
import './ConfigPage.css'

interface BootstrapConfig {
  listen: string
  public_url: string
  admin_username: string
}

interface ManagedConfigResponse {
  managed: ManagedConfig
  bootstrap: BootstrapConfig
  restart_required: boolean
  backup_available: boolean
}

interface SavedConfigResponse {
  managed: ManagedConfig
  restart_required: boolean
}

interface TokenResponse {
  webhook_token: string
  restart_required: boolean
}

export default function ConfigPage() {
  const { message, modal } = App.useApp()
  const [draft, setDraft] = useState<ManagedConfig | null>(null)
  const [bootstrap, setBootstrap] = useState<BootstrapConfig | null>(null)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [restartNeeded, setRestartNeeded] = useState(false)
  const [backupAvailable, setBackupAvailable] = useState(false)
  const [storageIndex, setStorageIndex] = useState<number | null>(null)
  const [taskIndex, setTaskIndex] = useState<number | null>(null)
  const [error, setError] = useState('')
  const [cloudDriveMappings, setCloudDriveMappings] = useState('{}')
  const [moviePilotMappings, setMoviePilotMappings] = useState('{}')
  const [globalPlugins, setGlobalPlugins] = useState('[]')

  const syncJSONFields = (managed: ManagedConfig) => {
    setCloudDriveMappings(JSON.stringify(managed.integrations.clouddrive2 ?? {}, null, 2))
    setMoviePilotMappings(JSON.stringify(managed.integrations.moviepilot ?? {}, null, 2))
    setGlobalPlugins(JSON.stringify(managed.plugins ?? [], null, 2))
  }

  const load = useCallback(async () => {
    try {
      const response = await getJSON<ManagedConfigResponse>('/api/config/managed')
      setDraft(response.managed)
      syncJSONFields(response.managed)
      setBootstrap(response.bootstrap)
      setBackupAvailable(response.backup_available)
      setDirty(false)
      setError('')
    } catch (loadError) {
      const text = loadError instanceof Error ? loadError.message : '设置加载失败'
      setError(text)
      void message.error(text)
    }
  }, [message])

  useEffect(() => { void load() }, [load])

  const saveStorage = (storage: StorageConfig, index: number) => {
    setDraft(current => {
      if (!current) return current
      const storages = [...current.storages]
      if (index < 0) storages.push(storage)
      else storages[index] = storage
      return { ...current, storages }
    })
    setDirty(true)
    setStorageIndex(null)
  }

  const deleteStorage = (index: number) => {
    if (!draft) return
    const storage = draft.storages[index]
    const used = draft.tasks.filter(task => task.storage_id === storage.id)
    if (used.length) {
      modal.error({
        title: '无法删除存储',
        content: `请先删除或调整引用该存储的任务：${used.map(task => task.id).join(', ')}`,
      })
      return
    }
    setDraft({ ...draft, storages: draft.storages.filter((_, itemIndex) => itemIndex !== index) })
    setDirty(true)
  }

  const saveTask = (task: TaskConfig, index: number) => {
    setDraft(current => {
      if (!current) return current
      const tasks = [...current.tasks]
      if (index < 0) tasks.push(task)
      else tasks[index] = task
      return { ...current, tasks }
    })
    setDirty(true)
    setTaskIndex(null)
  }

  const addTask = () => {
    if (!draft?.storages.length) {
      modal.warning({ title: '无法添加任务', content: '请先添加存储' })
      return
    }
    setTaskIndex(-1)
  }

  const deleteTask = (index: number) => {
    if (!draft) return
    setDraft({ ...draft, tasks: draft.tasks.filter((_, taskItemIndex) => taskItemIndex !== index) })
    setDirty(true)
  }

  const resetToken = async () => {
    try {
      const response = await postJSON<TokenResponse>('/api/config/webhook-token')
      setRestartNeeded(response.restart_required)
      modal.info({
        title: '新 Webhook Token',
        content: (
          <Space direction="vertical">
            <Typography.Text>新 Token 仅显示一次，请立即保存。</Typography.Text>
            <Typography.Text code copyable>{response.webhook_token}</Typography.Text>
          </Space>
        ),
      })
    } catch (tokenError) {
      void message.error(tokenError instanceof Error ? tokenError.message : '重置失败')
    }
  }

  const save = async () => {
    if (!draft) return
    const jsonFields = [cloudDriveMappings, moviePilotMappings, globalPlugins]
    if (jsonFields.some(value => validateJSON(value))) {
      setError('请先修正高级能力中的 JSON 格式错误')
      return
    }
    let clouddrive2: unknown
    let moviepilot: unknown
    let plugins: unknown
    try {
      clouddrive2 = JSON.parse(cloudDriveMappings) as unknown
      moviepilot = JSON.parse(moviePilotMappings) as unknown
      plugins = JSON.parse(globalPlugins) as unknown
    } catch {
      setError('请先修正高级能力中的 JSON 格式错误')
      return
    }
    if (!isStringRecord(clouddrive2) || !isStringRecord(moviepilot) || !Array.isArray(plugins)) {
      setError('路径映射必须是字符串 JSON 对象，全局插件必须是 JSON 数组')
      return
    }
    const payload: ManagedConfig = {
      ...draft,
      integrations: { clouddrive2, moviepilot },
      plugins: plugins as ManagedConfig['plugins'],
    }
    setSaving(true)
    setError('')
    try {
      const response = await putJSON<SavedConfigResponse>('/api/config/managed', payload)
      setDraft(response.managed)
      syncJSONFields(response.managed)
      setDirty(false)
      setRestartNeeded(response.restart_required)
      void message.success('全部业务设置已通过服务端校验并原子写入配置')
    } catch (saveError) {
      const text = saveError instanceof Error ? saveError.message : '保存失败'
      setError(text)
      void message.error(text)
    } finally {
      setSaving(false)
    }
  }

  const restore = async () => {
    try {
      await postJSON('/api/config/restore')
      await load()
      setRestartNeeded(true)
      void message.success('已恢复备份，请重启服务')
    } catch (restoreError) {
      void message.error(restoreError instanceof Error ? restoreError.message : '恢复失败')
    }
  }

  const columns: ColumnsType<StorageConfig> = [
    { title: '标识', dataIndex: 'id' },
    { title: '类型', dataIndex: 'type' },
    {
      title: '根路径 / 服务地址',
      render: (_, storage) => (
        <Space direction="vertical" size={0}>
          {storage.root ? <span>{storage.root}</span> : null}
          {storage.endpoint ? <Typography.Text type="secondary">{storage.endpoint}</Typography.Text> : null}
          {storage.type === '123_open' ? (
            <Typography.Text type="secondary">挂载 File ID：{storage.root_file_id || '0'}</Typography.Text>
          ) : null}
        </Space>
      ),
    },
    {
      title: '操作',
      render: (_, storage, index) => (
        <Space>
          <Button type="link" onClick={() => setStorageIndex(index)}>编辑</Button>
          <Popconfirm
            title={`从配置中删除存储 ${storage.id}？不会删除远端文件。`}
            okText="确定"
            cancelText="取消"
            onConfirm={() => deleteStorage(index)}
          >
            <Button type="link" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const taskColumns: ColumnsType<TaskConfig> = [
    { title: '标识', dataIndex: 'id' },
    { title: '名称', dataIndex: 'name' },
    { title: '存储', dataIndex: 'storage_id' },
    { title: '源路径', dataIndex: 'source' },
    { title: '调度', dataIndex: 'schedule', render: value => value || '手动' },
    {
      title: '操作',
      render: (_, task, index) => (
        <Space>
          <Button type="link" onClick={() => setTaskIndex(index)}>编辑</Button>
          <Popconfirm
            title={`从配置中删除任务 ${task.id}？现有 STRM 文件不会自动删除。`}
            okText="确定"
            cancelText="取消"
            onConfirm={() => deleteTask(index)}
          >
            <Button type="link" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  if (!draft || !bootstrap) {
    return <Spin tip="加载配置中"><div style={{ minHeight: 160 }} /></Spin>
  }

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Typography.Title level={2}>配置</Typography.Title>
      <nav className="config-section-nav" aria-label="配置区块导航">
        {[
          ['config-basic', '基础配置'],
          ['config-storages', '存储'],
          ['config-tasks', '生成任务'],
          ['config-advanced', '高级能力'],
        ].map(([id, label]) => (
          <Button key={id} size="small" onClick={() => document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' })}>
            {label}
          </Button>
        ))}
      </nav>
      {error ? <Alert type="error" showIcon message={error} /> : null}
      <Card id="config-basic" title="基础启动配置" extra={(
        <Popconfirm
          title="生成新 Webhook Token？当前 Token 在重启后失效。"
          okText="确定"
          cancelText="取消"
          onConfirm={() => void resetToken()}
        >
          <Button type="text" danger>重置 Webhook Token</Button>
        </Popconfirm>
      )}>
        <Descriptions column={{ xs: 1, sm: 2, lg: 4 }}>
          <Descriptions.Item label="监听地址"><Typography.Text code>{bootstrap.listen}</Typography.Text></Descriptions.Item>
          <Descriptions.Item label="公开地址"><Typography.Text code>{bootstrap.public_url}</Typography.Text></Descriptions.Item>
          <Descriptions.Item label="管理员"><Typography.Text code>{bootstrap.admin_username || '（未启用）'}</Typography.Text></Descriptions.Item>
          <Descriptions.Item label="Webhook Token"><Typography.Text code>已在基础文件中配置</Typography.Text></Descriptions.Item>
        </Descriptions>
      </Card>

      <Card id="config-storages" title="存储" extra={<Button type="primary" onClick={() => setStorageIndex(-1)}>添加存储</Button>}>
        <Table
          rowKey="id"
          columns={columns}
          dataSource={draft.storages}
          pagination={false}
          locale={{ emptyText: <Empty description="尚未添加存储" /> }}
          scroll={{ x: 700 }}
        />
      </Card>
      <Card id="config-tasks" title="生成任务" extra={<Button type="primary" onClick={addTask}>添加任务</Button>}>
        <Table
          rowKey="id"
          columns={taskColumns}
          dataSource={draft.tasks}
          pagination={false}
          locale={{ emptyText: <Empty description="尚未添加任务" /> }}
          scroll={{ x: 900 }}
        />
      </Card>
      <Card id="config-advanced" title="高级能力">
        <Typography.Paragraph type="secondary">
          TMDB、历史、媒体工具、媒体代理、外部路径映射及全局插件。JSON 区域只覆盖对应区块。
        </Typography.Paragraph>
        <div className="config-grid">
          <label htmlFor="tmdb-api-key">TMDB API Key</label>
          <SecretInput id="tmdb-api-key" value={draft.tmdb.api_key} onChange={value => updateDraft(setDraft, setDirty, current => ({ ...current, tmdb: { ...current.tmdb, api_key: value } }))} />
          <label htmlFor="tmdb-language">TMDB 语言</label>
          <input className="config-control" id="tmdb-language" value={draft.tmdb.language} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, tmdb: { ...current.tmdb, language: event.target.value } }))} />
          <label htmlFor="tmdb-cache">缓存分钟数</label>
          <input className="config-control" id="tmdb-cache" type="number" min={0} value={draft.tmdb.cache_minutes ?? 0} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, tmdb: { ...current.tmdb, cache_minutes: Number(event.target.value) } }))} />
          <label htmlFor="tmdb-base-url">TMDB API 地址</label>
          <input className="config-control" id="tmdb-base-url" value={draft.tmdb.base_url ?? ''} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, tmdb: { ...current.tmdb, base_url: event.target.value } }))} />
          <label htmlFor="tmdb-image-url">图片地址</label>
          <input className="config-control" id="tmdb-image-url" value={draft.tmdb.image_base_url ?? ''} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, tmdb: { ...current.tmdb, image_base_url: event.target.value } }))} />
          <label htmlFor="tmdb-proxy-url">代理 URL</label>
          <input className="config-control" id="tmdb-proxy-url" value={draft.tmdb.proxy_url ?? ''} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, tmdb: { ...current.tmdb, proxy_url: event.target.value } }))} />
          <label htmlFor="history-path">历史文件路径</label>
          <input className="config-control" id="history-path" value={draft.history.path} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, history: { ...current.history, path: event.target.value } }))} />
          <label htmlFor="history-max">历史最大条数</label>
          <input className="config-control" id="history-max" type="number" min={0} max={100000} value={draft.history.max_entries} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, history: { ...current.history, max_entries: Number(event.target.value) } }))} />
          <label htmlFor="ffmpeg-path">FFmpeg 路径</label>
          <input className="config-control" id="ffmpeg-path" value={draft.media_tools.ffmpeg} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, media_tools: { ...current.media_tools, ffmpeg: event.target.value } }))} />
          <label htmlFor="media-timeout">媒体工具超时（秒）</label>
          <input className="config-control" id="media-timeout" type="number" min={0} value={draft.media_tools.timeout_seconds} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, media_tools: { ...current.media_tools, timeout_seconds: Number(event.target.value) } }))} />
          <label htmlFor="media-proxy-enabled">启用媒体代理</label>
          <input id="media-proxy-enabled" type="checkbox" checked={draft.media_proxy.enabled} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, media_proxy: { ...current.media_proxy, enabled: event.target.checked } }))} />
          {draft.media_proxy.enabled ? <>
            <label htmlFor="media-proxy-listen">媒体代理监听</label>
            <input className="config-control" id="media-proxy-listen" value={draft.media_proxy.listen} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, media_proxy: { ...current.media_proxy, listen: event.target.value } }))} />
            <label htmlFor="media-proxy-upstream">媒体代理上游</label>
            <input className="config-control" id="media-proxy-upstream" value={draft.media_proxy.upstream} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, media_proxy: { ...current.media_proxy, upstream: event.target.value } }))} />
            <label htmlFor="media-proxy-type">媒体服务器类型</label>
            <select className="config-control" id="media-proxy-type" value={draft.media_proxy.server_type} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, media_proxy: { ...current.media_proxy, server_type: event.target.value } }))}>
              <option value="">请选择</option><option value="emby">Emby</option><option value="jellyfin">Jellyfin</option>
            </select>
            <label htmlFor="media-proxy-rewrite">改写 PlaybackInfo</label>
            <input id="media-proxy-rewrite" type="checkbox" checked={draft.media_proxy.rewrite_playback_info} onChange={event => updateDraft(setDraft, setDirty, current => ({ ...current, media_proxy: { ...current.media_proxy, rewrite_playback_info: event.target.checked } }))} />
          </> : null}
          <label htmlFor="clouddrive-mappings">CloudDrive2 路径映射</label>
          <JsonField id="clouddrive-mappings" value={cloudDriveMappings} onChange={value => { setCloudDriveMappings(value); setDirty(true) }} schemaHint="JSON 对象：远端路径到本地路径" />
          <label htmlFor="moviepilot-mappings">MoviePilot 路径映射</label>
          <JsonField id="moviepilot-mappings" value={moviePilotMappings} onChange={value => { setMoviePilotMappings(value); setDirty(true) }} schemaHint="JSON 对象：来源路径到目标路径" />
          <label htmlFor="global-plugins">全局插件</label>
          <JsonField id="global-plugins" value={globalPlugins} onChange={value => { setGlobalPlugins(value); setDirty(true) }} schemaHint="JSON 数组" />
        </div>
      </Card>

      <Card className="config-save-bar">
        <Space wrap>
          <Button type="primary" loading={saving} disabled={saving || !dirty} onClick={() => void save()}>
            验证并保存全部设置
          </Button>
          <Button disabled={saving || !dirty} onClick={() => void load()}>放弃修改并重新加载</Button>
          <Popconfirm
            title="用上一次备份覆盖当前全部配置？恢复后必须重启。"
            okText="确定"
            cancelText="取消"
            onConfirm={() => void restore()}
          >
            <Button disabled={saving || !backupAvailable}>恢复上一次备份</Button>
          </Popconfirm>
          <Typography.Text type="secondary" role="status" aria-live="polite">
            {restartNeeded ? '设置已保存，等待重启生效' : dirty ? '配置草稿有未保存修改' : '设置已加载'}
          </Typography.Text>
        </Space>
      </Card>

      {restartNeeded ? (
        <Alert
          type="warning"
          showIcon
          message="配置已写入，请重启服务后生效。当前页面中的运行状态仍来自重启前配置。"
        />
      ) : null}

      <StorageFormModal
        open={storageIndex !== null}
        index={storageIndex ?? -1}
        storages={draft.storages}
        onSave={saveStorage}
        onClose={() => setStorageIndex(null)}
      />
      <TaskFormModal
        open={taskIndex !== null}
        index={taskIndex ?? -1}
        tasks={draft.tasks}
        storages={draft.storages}
        onSave={saveTask}
        onClose={() => setTaskIndex(null)}
      />
    </Space>
  )
}

function isStringRecord(value: unknown): value is Record<string, string> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    && Object.values(value).every(item => typeof item === 'string')
}

function updateDraft(
  setDraft: Dispatch<SetStateAction<ManagedConfig | null>>,
  setDirty: Dispatch<SetStateAction<boolean>>,
  update: (current: ManagedConfig) => ManagedConfig,
) {
  setDraft(current => current ? update(current) : current)
  setDirty(true)
}
