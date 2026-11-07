import { useMemo, useState } from 'react'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { App, Badge, Button, Empty, Input, Modal, Space, Table, Typography } from 'antd'
import type { TableProps } from 'antd'
import { useNavigate } from 'react-router'
import { getJSON, postJSON } from '../api/client'
import type { TaskConfig, TaskResult, TaskStatus, TasksResponse } from '../api/types'
import { usePolling } from '../hooks/usePolling'
import TaskToolsModal from './TaskToolsModal'

function statusOf(statuses: TaskStatus[], taskId: string): TaskStatus | undefined {
  return statuses.find((s) => s.task_id === taskId)
}

function StatusBadge({ status }: { status?: TaskStatus }) {
  const badge = status?.running
    ? { status: 'processing' as const, text: '运行中' }
    : status?.stopped
      ? { status: 'default' as const, text: '已停止' }
      : status?.error
        ? { status: 'error' as const, text: '失败' }
        : { status: 'success' as const, text: '空闲' }
  return (
    <>
      <Badge status={badge.status} text={badge.text} />
      {status && status.queued > 0 ? <Typography.Text>队列 {status.queued}</Typography.Text> : null}
    </>
  )
}

function ResultText({ result, error }: { result?: TaskResult; error?: string }) {
  const r = result ?? {}
  return (
    <Space direction="vertical" size={0}>
      {error ? <Typography.Text type="danger">{error}</Typography.Text> : null}
      <span>
        <span className="management-nowrap">生成 {r.created || 0}</span>
        {' / '}
        <span className="management-nowrap">复制 {r.copied || 0}</span>
        {' / '}
        <span className="management-nowrap">跳过 {r.skipped || 0}</span>
        {' / '}
        <span className="management-nowrap">删除 {r.removed || 0}</span>
        {' / '}
        <span className="management-nowrap">失败 {r.failed || 0}</span>
      </span>
    </Space>
  )
}

function RunTaskModal({ taskId, onClose, onRan }: { taskId: string | null; onClose: () => void; onRan: () => Promise<void> }) {
  const { message } = App.useApp()
  const [path, setPath] = useState('')
  const [running, setRunning] = useState(false)

  const run = async () => {
    if (!taskId || running) return
    setRunning(true)
    try {
      await postJSON(`/api/tasks/${taskId}/run`, path ? { path } : {})
      message.success('已加入队列')
      onClose()
      await onRan()
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setRunning(false)
    }
  }

  return (
    <Modal
      title="运行任务"
      open={taskId !== null}
      onOk={() => void run()}
      onCancel={onClose}
      confirmLoading={running}
      okText="确定"
      cancelText="取消"
    >
      <Input
        aria-label="运行路径"
        placeholder="可选源路径，留空运行整个任务"
        value={path}
        onChange={(e) => setPath(e.target.value)}
      />
    </Modal>
  )
}

function StrmPreviewModal({ taskId, onClose }: { taskId: string | null; onClose: () => void }) {
  const { message } = App.useApp()
  const [path, setPath] = useState('')
  const [content, setContent] = useState<unknown>(null)
  const [loading, setLoading] = useState(false)

  const generate = async () => {
    const sourcePath = path.trim()
    if (!taskId || !sourcePath || loading) return
    setLoading(true)
    try {
      const query = new URLSearchParams({ path: sourcePath })
      setContent(await getJSON(`/api/tasks/${encodeURIComponent(taskId)}/preview?${query.toString()}`))
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title="STRM 生成预览"
      open={taskId !== null}
      onCancel={onClose}
      width={720}
      footer={(
        <Space>
          <Button onClick={onClose} disabled={loading}>取消</Button>
          <Button type="primary" loading={loading} disabled={!path.trim()} onClick={() => void generate()}>
            生成预览
          </Button>
        </Space>
      )}
    >
      <Typography.Paragraph type="secondary">
        输入任务存储中的具体媒体文件路径；预览不会写入文件或修改远端。
      </Typography.Paragraph>
      <Input
        aria-label="源文件路径"
        placeholder="例如 /movies/example.mkv"
        value={path}
        onChange={(event) => setPath(event.target.value)}
      />
      {content !== null ? <pre>{JSON.stringify(content, null, 2)}</pre> : null}
    </Modal>
  )
}

export default function TasksPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const [data, setData] = useState<TasksResponse>({ tasks: [], statuses: [] })
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [runTaskId, setRunTaskId] = useState<string | null>(null)
  const [toolsTaskId, setToolsTaskId] = useState<string | null>(null)
  const [previewTaskId, setPreviewTaskId] = useState<string | null>(null)
  const [filter, setFilter] = useState('')
  const [refreshing, setRefreshing] = useState(false)

  const load = async () => {
    try {
      setData(await getJSON<TasksResponse>('/api/tasks'))
    } catch (err) {
      message.error((err as Error).message)
    }
  }
  usePolling(load, 3000)

  const visibleTasks = useMemo(() => {
    const keyword = filter.trim().toLocaleLowerCase()
    if (!keyword) return data.tasks
    return data.tasks.filter((task) => [task.name, task.id, task.storage_id, task.source]
      .some((value) => value?.toLocaleLowerCase().includes(keyword)))
  }, [data.tasks, filter])

  const refresh = async () => {
    if (refreshing) return
    setRefreshing(true)
    try {
      await load()
    } finally {
      setRefreshing(false)
    }
  }

  const stopTask = async (taskId: string) => {
    setActionLoading(`${taskId}:stop`)
    try {
      await postJSON(`/api/tasks/${taskId}/stop`, {})
      await load()
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setActionLoading(null)
    }
  }

  const columns: TableProps<TaskConfig>['columns'] = [
    {
      title: '任务',
      key: 'name',
      width: 140,
      render: (_, task) => (
        <>
          <div>{task.name || task.id}</div>
          <Typography.Text type="secondary">{task.id}</Typography.Text>
        </>
      ),
    },
    {
      title: '存储·源路径',
      key: 'storage',
      width: 180,
      render: (_, task) => (
        <>
          <div>{task.storage_id}</div>
          <Typography.Text type="secondary">{task.source || '/'}</Typography.Text>
        </>
      ),
    },
    {
      title: '调度',
      key: 'schedule',
      width: 130,
      className: 'management-nowrap',
      render: (_, task) => task.schedule || '手动',
    },
    {
      title: '状态',
      key: 'status',
      width: 110,
      className: 'management-nowrap',
      render: (_, task) => <StatusBadge status={statusOf(data.statuses, task.id)} />,
    },
    {
      title: '结果',
      key: 'result',
      width: 280,
      render: (_, task) => {
        const status = statusOf(data.statuses, task.id)
        return <ResultText result={status?.result} error={status?.error} />
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      className: 'management-nowrap',
      render: (_, task) => {
        if (statusOf(data.statuses, task.id)?.running) {
          return (
            <Button danger loading={actionLoading === `${task.id}:stop`} onClick={() => void stopTask(task.id)}>
              停止
            </Button>
          )
        }
        return (
          <Space size={4} wrap>
            <Button onClick={() => setRunTaskId(task.id)}>运行</Button>
            <Button onClick={() => setPreviewTaskId(task.id)}>预览</Button>
            {statusOf(data.statuses, task.id)?.error ? (
              <Button onClick={() => navigate(`/history?task_id=${encodeURIComponent(task.id)}`)}>查看日志</Button>
            ) : null}
            <Button type="text" onClick={() => setToolsTaskId(task.id)}>工具</Button>
          </Space>
        )
      },
    },
  ]

  return (
    <section className="management-page tasks-page">
      <header className="management-heading">
        <div>
          <h2>任务</h2>
          <Typography.Text type="secondary">
            在这里查看运行状态并手动触发；添加或调整任务后会立即动态生效。
          </Typography.Text>
        </div>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => navigate('/config?section=tasks&action=add-task')}
        >
          添加任务
        </Button>
      </header>
      <Space className="management-toolbar" wrap>
        <Input.Search
          className="management-search"
          aria-label="筛选任务"
          allowClear
          placeholder="筛选任务"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
        <Button icon={<ReloadOutlined />} loading={refreshing} onClick={() => void refresh()}>
          刷新
        </Button>
        <Typography.Text type="secondary" aria-live="polite">
          显示 {visibleTasks.length} / {data.tasks.length} 个任务
        </Typography.Text>
      </Space>
      <Table<TaskConfig>
        rowKey="id"
        pagination={false}
        columns={columns}
        dataSource={visibleTasks}
        locale={{
          emptyText: filter.trim()
            ? <Empty description="没有匹配的任务" />
            : (
              <Empty description="还没有任务。任务用于扫描媒体并生成 STRM 文件。">
                <Button type="primary" onClick={() => navigate('/config?section=tasks&action=add-task')}>创建第一个任务</Button>
              </Empty>
            ),
        }}
      />
      <RunTaskModal
        key={`run:${runTaskId ?? 'closed'}`}
        taskId={runTaskId}
        onClose={() => setRunTaskId(null)}
        onRan={load}
      />
      <TaskToolsModal
        key={`tools:${toolsTaskId ?? 'closed'}`}
        taskId={toolsTaskId ?? ''}
        open={toolsTaskId !== null}
        onClose={() => setToolsTaskId(null)}
        onChanged={() => void load()}
      />
      <StrmPreviewModal
        key={`preview:${previewTaskId ?? 'closed'}`}
        taskId={previewTaskId}
        onClose={() => setPreviewTaskId(null)}
      />
    </section>
  )
}
