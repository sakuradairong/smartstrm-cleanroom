import { useState } from 'react'
import { App, Button, Input, Modal, Popconfirm, Radio, Space, Switch, Typography } from 'antd'
import { deleteJSON, getJSON, postJSON } from '../api/client'

type Tool = 'replace' | 'overwrite' | 'covers' | 'clear'

interface ReplaceResult {
  scanned: number
  changed: number
}

interface CoverResult {
  scanned: number
  planned: number
  created: number
  skipped: number
  changes: { strm: string; cover: string }[]
}

interface ClearResult {
  readonly scanned: number
  readonly planned: number
  readonly removed: number
  readonly kept: number
}

interface TaskToolsModalProps {
  taskId: string
  open: boolean
  onClose: () => void
  onChanged: () => void
}

export default function TaskToolsModal({ taskId, open, onClose, onChanged }: TaskToolsModalProps) {
  const { message } = App.useApp()
  const [tool, setTool] = useState<Tool>('replace')
  const [loading, setLoading] = useState(false)

  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [replacePreview, setReplacePreview] = useState<ReplaceResult | null>(null)

  const [overwritePath, setOverwritePath] = useState('')

  const [coverPath, setCoverPath] = useState('/')
  const [coverPosition, setCoverPosition] = useState('10s')
  const [coverOverwrite, setCoverOverwrite] = useState(false)
  const [coverPreview, setCoverPreview] = useState<CoverResult | null>(null)
  const [clearPreview, setClearPreview] = useState<ClearResult | null>(null)

  const switchTool = (next: Tool) => {
    setTool(next)
    setReplacePreview(null)
    setCoverPreview(null)
    setClearPreview(null)
  }

  const run = async (action: () => Promise<void>) => {
    if (loading) return
    setLoading(true)
    try {
      await action()
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const previewReplace = () =>
    run(async () => {
      setReplacePreview(
        await postJSON<ReplaceResult>(`/api/tasks/${taskId}/replace-content`, { from, to, preview: true }),
      )
    })

  const executeReplace = () =>
    run(async () => {
      const result = await postJSON<ReplaceResult>(`/api/tasks/${taskId}/replace-content`, {
        from,
        to,
        preview: false,
      })
      message.success(`已修改 ${result.changed} 个 STRM`)
      setReplacePreview(null)
      onChanged()
    })

  const fullOverwrite = () =>
    run(async () => {
      await postJSON(`/api/tasks/${taskId}/full-overwrite`, { path: overwritePath })
      message.success('全量覆写已加入队列')
      onChanged()
    })

  const previewCovers = () =>
    run(async () => {
      setCoverPreview(
        await postJSON<CoverResult>(`/api/tasks/${taskId}/covers`, {
          path: coverPath,
          position: coverPosition,
          overwrite: coverOverwrite,
          preview: true,
        }),
      )
    })

  const executeCovers = () =>
    run(async () => {
      const result = await postJSON<CoverResult>(`/api/tasks/${taskId}/covers`, {
        path: coverPath,
        position: coverPosition,
        overwrite: coverOverwrite,
        preview: false,
      })
      message.success(`已生成 ${result.created} 张封面`)
      setCoverPreview(null)
      onChanged()
    })

  const clearGenerated = () =>
    run(async () => {
      const result = await deleteJSON<ClearResult>(`/api/tasks/${taskId}/generated`)
      message.success(`已清除 ${result.removed} 个 STRM 文件`)
      setClearPreview(null)
      onChanged()
    })

  const previewClear = () =>
    run(async () => {
      setClearPreview(await getJSON<ClearResult>(`/api/tasks/${taskId}/generated`))
    })

  return (
    <Modal title="任务工具" open={open} onCancel={onClose} footer={null} width={560}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Radio.Group value={tool} onChange={(e) => switchTool(e.target.value as Tool)}>
          <Radio.Button value="replace">内容替换</Radio.Button>
          <Radio.Button value="overwrite">全量覆写</Radio.Button>
          <Radio.Button value="covers">提取封面</Radio.Button>
          <Radio.Button value="clear">清除生成文件</Radio.Button>
        </Radio.Group>

        {tool === 'replace' && (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Input
              aria-label="查找内容"
              placeholder="查找内容"
              value={from}
              onChange={(e) => {
                setFrom(e.target.value)
                setReplacePreview(null)
              }}
            />
            <Input
              aria-label="替换内容"
              placeholder="替换为"
              value={to}
              onChange={(e) => {
                setTo(e.target.value)
                setReplacePreview(null)
              }}
            />
            {replacePreview && (
              <Typography.Text>
                扫描 {replacePreview.scanned} 个 STRM，将修改 {replacePreview.changed} 个
              </Typography.Text>
            )}
            <Space>
              <Button loading={loading} disabled={!from} onClick={() => void previewReplace()}>
                预览
              </Button>
              {replacePreview && (
                <Button type="primary" danger loading={loading} onClick={() => void executeReplace()}>
                  确认执行
                </Button>
              )}
            </Space>
          </Space>
        )}

        {tool === 'overwrite' && (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Input
              aria-label="全量覆写路径"
              placeholder="可选源路径，留空覆写整个任务"
              value={overwritePath}
              onChange={(e) => setOverwritePath(e.target.value)}
            />
            <Popconfirm
              title="全量覆写"
              description="将重新生成该任务的全部生成文件（忽略增量），确认执行？"
              okText="覆写"
              cancelText="取消"
              okButtonProps={{ danger: true }}
              onConfirm={() => void fullOverwrite()}
            >
              <Button danger loading={loading}>
                执行覆写
              </Button>
            </Popconfirm>
          </Space>
        )}

        {tool === 'covers' && (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Input
              aria-label="封面提取路径"
              placeholder="任务内子目录（/ 表示全部）"
              value={coverPath}
              onChange={(e) => {
                setCoverPath(e.target.value)
                setCoverPreview(null)
              }}
            />
            <Input
              aria-label="截帧时间"
              placeholder="截帧时间（例如 10s、1m30s）"
              value={coverPosition}
              onChange={(e) => {
                setCoverPosition(e.target.value)
                setCoverPreview(null)
              }}
            />
            <Space>
              <Switch
                aria-label="覆盖已有同名 JPG"
                checked={coverOverwrite}
                onChange={(checked) => {
                  setCoverOverwrite(checked)
                  setCoverPreview(null)
                }}
              />
              <Typography.Text>覆盖已有同名 JPG</Typography.Text>
            </Space>
            {coverPreview && (
              <Typography.Text>
                扫描 {coverPreview.scanned} 个 STRM，计划生成 {coverPreview.planned} 张封面，跳过{' '}
                {coverPreview.skipped} 张
              </Typography.Text>
            )}
            <Space>
              <Button loading={loading} onClick={() => void previewCovers()}>
                预览
              </Button>
              {coverPreview && (
                <Button type="primary" loading={loading} onClick={() => void executeCovers()}>
                  确认执行
                </Button>
              )}
            </Space>
          </Space>
        )}

        {tool === 'clear' && (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Typography.Text type="secondary">
              仅删除当前签名密钥可验证且属于该任务源路径的 STRM。原始媒体、字幕、海报及无法确认归属的文件会保留。
            </Typography.Text>
            {clearPreview && (
              <Typography.Text>
                扫描 {clearPreview.scanned} 个 STRM，可清除 {clearPreview.planned} 个，保留{' '}
                {clearPreview.kept} 个
              </Typography.Text>
            )}
            <Space>
              <Button
                type={clearPreview ? 'default' : 'primary'}
                loading={loading}
                onClick={() => void previewClear()}
              >
                检查可清理文件
              </Button>
              {clearPreview && clearPreview.planned > 0 && (
                <Popconfirm
                  title="确认清除"
                  description={`将删除 ${clearPreview.planned} 个已验证的 STRM，并保留 ${clearPreview.kept} 个无法确认归属的 STRM。`}
                  okText="删除"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                  onConfirm={() => void clearGenerated()}
                >
                  <Button danger loading={loading}>
                    确认清除
                  </Button>
                </Popconfirm>
              )}
            </Space>
          </Space>
        )}
      </Space>
    </Modal>
  )
}
