import { useRef, useState } from 'react'
import { PlayCircleOutlined, PlusOutlined } from '@ant-design/icons'
import { App, Button, Space, Typography } from 'antd'
import { useNavigate } from 'react-router'
import { postJSON } from '../api/client'

type TaskPageActionsProps = {
  readonly ready: boolean
  readonly hasTasks: boolean
  readonly onRan: () => Promise<void>
}

type RunAllResponse = {
  readonly status: 'queued'
  readonly tasks: readonly string[]
}

export function TaskPageActions({ ready, hasTasks, onRan }: TaskPageActionsProps) {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const pending = useRef(false)
  const canRun = ready && hasTasks
  const empty = ready && !hasTasks

  const runAll = async () => {
    if (!canRun || pending.current) return
    pending.current = true
    setRunning(true)
    try {
      const result = await postJSON<RunAllResponse>('/api/tasks/run-all', {})
      setError('')
      message.success(`已将 ${result.tasks.length} 个任务加入队列`)
      await onRan()
    } catch (err) {
      if (!(err instanceof Error)) throw err
      setError(err.message)
    } finally {
      pending.current = false
      setRunning(false)
    }
  }

  return (
    <div className="task-page-actions">
      <Space wrap>
        <Button
          type={canRun ? 'primary' : 'default'}
          icon={<PlayCircleOutlined />}
          disabled={!canRun}
          loading={running}
          onClick={() => void runAll()}
        >
          {running ? '正在加入队列…' : '运行全部'}
        </Button>
        <Button
          type={empty ? 'primary' : 'default'}
          icon={<PlusOutlined />}
          onClick={() => navigate('/config?section=tasks&action=add-task')}
        >
          添加任务
        </Button>
      </Space>
      {error ? <Typography.Text type="danger" role="alert">{error}</Typography.Text> : null}
    </div>
  )
}
