import { useState } from 'react'
import { App, Badge, Button, Input, Modal, Space, Table, Typography } from 'antd'
import type { TableProps } from 'antd'
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

function ResultText({ result }: { result?: TaskResult }) {
  const r = result ?? {}
  return <span>生成 {r.created || 0} / 复制 {r.copied || 0} / 删除 {r.removed || 0} / 失败 {r.failed || 0}</span>
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
  const [data, setData] = useState<TasksResponse>({ tasks: [], statuses: [] })
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [runTaskId, setRunTaskId] = useState<string | null>(null)
  const [toolsTaskId, setToolsTaskId] = useState<string | null>(null)
  const [previewTaskId, setPreviewTaskId] = useState<string | null>(null)

  const load = async () => {
    try {
      setData(await getJSON<TasksResponse>('/api/tasks'))
    } catch (err) {
      message.error((err as Error).message)
    }
  }
  usePolling(load, 3000)

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
      render: (_, task) => task.schedule || '手动',
    },
    {
      title: '状态',
      key: 'status',
      render: (_, task) => <StatusBadge status={statusOf(data.statuses, task.id)} />,
    },
    {
      title: '结果',
      key: 'result',
      render: (_, task) => <ResultText result={statusOf(data.statuses, task.id)?.result} />,
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, task) => {
        if (statusOf(data.statuses, task.id)?.running) {
          return (
            <Button danger loading={actionLoading === `${task.id}:stop`} onClick={() => void stopTask(task.id)}>
              停止
            </Button>
          )
        }
        return (
          <Space>
            <Button type="primary" onClick={() => setRunTaskId(task.id)}>运行</Button>
            <Button onClick={() => setPreviewTaskId(task.id)}>预览</Button>
            <Button type="text" onClick={() => setToolsTaskId(task.id)}>工具</Button>
          </Space>
        )
      },
    },
  ]

  return (
    <>
      <h2>任务</h2>
      <Typography.Paragraph>
        <Typography.Text type="secondary">
          任务可在“配置”页添加或调整；保存后重启服务生效。此面板用于查看当前运行状态和手动触发。
        </Typography.Text>
      </Typography.Paragraph>
      <Table<TaskConfig> rowKey="id" pagination={false} columns={columns} dataSource={data.tasks} scroll={{ x: 900 }} />
      <RunTaskModal
        key={runTaskId ?? ''}
        taskId={runTaskId}
        onClose={() => setRunTaskId(null)}
        onRan={load}
      />
      <TaskToolsModal
        key={toolsTaskId ?? ''}
        taskId={toolsTaskId ?? ''}
        open={toolsTaskId !== null}
        onClose={() => setToolsTaskId(null)}
        onChanged={() => void load()}
      />
      <StrmPreviewModal
        key={previewTaskId ?? ''}
        taskId={previewTaskId}
        onClose={() => setPreviewTaskId(null)}
      />
    </>
  )
}
