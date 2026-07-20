import { useEffect, useRef, useState } from 'react'
import { Alert, App, Button, Input, InputNumber, Modal, Radio, Space, Table, Typography } from 'antd'
import { postJSON } from '../api/client'

// 模式与默认值逐字移植旧 internal/app/ui/index.html 的 batchRename prompt 序列。
type Mode = 'regex' | 'sequence' | 'magic'
const DEFAULT_PATTERN = 'EP(\\d+)'
const DEFAULT_REPLACEMENT = 'E$1'
const DEFAULT_PREFIX = 'S01E'
const DEFAULT_TEMPLATE = '{title} - S{season}E{episode}{ext}'

interface Change {
  from: string
  to: string
}

interface BatchRenameResult {
  preview: boolean
  changes: Change[]
}

interface BatchRenameModalProps {
  storageId: string
  path: string
  open: boolean
  onClose: () => void
  onDone: () => void
}

export default function BatchRenameModal({ storageId, path, open, onClose, onDone }: BatchRenameModalProps) {
  const { message } = App.useApp()
  const [mode, setMode] = useState<Mode>('regex')
  const [pattern, setPattern] = useState(DEFAULT_PATTERN)
  const [replacement, setReplacement] = useState(DEFAULT_REPLACEMENT)
  const [prefix, setPrefix] = useState(DEFAULT_PREFIX)
  const [start, setStart] = useState(1)
  const [width, setWidth] = useState(2)
  const [suffix, setSuffix] = useState('')
  const [template, setTemplate] = useState(DEFAULT_TEMPLATE)
  const [changes, setChanges] = useState<Change[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  // 关闭时重置内部状态，重新打开回到初始表单。
  const wasOpen = useRef(false)
  useEffect(() => {
    if (open && !wasOpen.current) {
      setMode('regex')
      setPattern(DEFAULT_PATTERN)
      setReplacement(DEFAULT_REPLACEMENT)
      setPrefix(DEFAULT_PREFIX)
      setStart(1)
      setWidth(2)
      setSuffix('')
      setTemplate(DEFAULT_TEMPLATE)
      setChanges(null)
      setError(null)
      setNotice(null)
      setLoading(false)
    }
    wasOpen.current = open
  }, [open])

  // 请求体字段与 app.go 的 batchRename 处理器一致；
  // 与旧 UI 相同，只携带当前模式涉及的字段。
  const payload = (preview: boolean) => {
    const body: Record<string, unknown> = { path, mode, preview }
    if (mode === 'regex') {
      body.pattern = pattern
      body.replacement = replacement
    } else if (mode === 'sequence') {
      body.prefix = prefix
      body.start = start
      body.width = width
      body.suffix = suffix
      body.preserve_extension = true
    } else {
      body.template = template
    }
    return body
  }

  const endpoint = `/api/storages/${encodeURIComponent(storageId)}/batch-rename`

  // 任何输入变化都会使已展示的预览失效，必须重新预览后再确认。
  const invalidate = () => {
    setChanges(null)
    setError(null)
    setNotice(null)
  }

  const preview = async () => {
    if (loading) return
    setLoading(true)
    setError(null)
    setNotice(null)
    try {
      const result = await postJSON<BatchRenameResult>(endpoint, payload(true))
      const list = result.changes ?? []
      if (list.length === 0) {
        setChanges(null)
        setNotice('没有需要重命名的文件')
        return
      }
      setChanges(list)
    } catch (err) {
      // 冲突/校验错误按后端返回文案展示（如 duplicate target name）
      setChanges(null)
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const apply = async () => {
    if (loading || changes === null) return
    setLoading(true)
    setError(null)
    try {
      await postJSON<BatchRenameResult>(endpoint, payload(false))
      message.success('批量命名完成')
      onDone()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const previewDisabled = loading || (mode === 'magic' && !template.trim())

  return (
    <Modal
      title="批量命名"
      open={open}
      onCancel={onClose}
      width={640}
      footer={
        <Space>
          <Button onClick={onClose} disabled={loading}>
            取消
          </Button>
          <Button loading={loading} disabled={previewDisabled} onClick={() => void preview()}>
            预览
          </Button>
          {changes !== null && (
            <Button type="primary" loading={loading} onClick={() => void apply()}>
              确认执行
            </Button>
          )}
        </Space>
      }
    >
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Radio.Group
          value={mode}
          onChange={(e) => {
            setMode(e.target.value as Mode)
            invalidate()
          }}
          options={[
            { value: 'regex', label: 'regex（正则）' },
            { value: 'sequence', label: 'sequence（顺序）' },
            { value: 'magic', label: 'magic（魔法变量）' },
          ]}
        />
        {mode === 'regex' && (
          <>
            <Input
              placeholder="匹配正则"
              value={pattern}
              onChange={(e) => {
                setPattern(e.target.value)
                invalidate()
              }}
            />
            <Input
              placeholder="替换内容"
              value={replacement}
              onChange={(e) => {
                setReplacement(e.target.value)
                invalidate()
              }}
            />
          </>
        )}
        {mode === 'sequence' && (
          <Space wrap>
            <Input
              placeholder="前缀"
              style={{ width: 140 }}
              value={prefix}
              onChange={(e) => {
                setPrefix(e.target.value)
                invalidate()
              }}
            />
            <InputNumber
              placeholder="起始编号"
              style={{ width: 120 }}
              min={0}
              value={start}
              onChange={(v) => {
                if (v !== null) setStart(v)
                invalidate()
              }}
            />
            <InputNumber
              placeholder="补位宽度"
              style={{ width: 120 }}
              min={1}
              max={12}
              value={width}
              onChange={(v) => {
                if (v !== null) setWidth(v)
                invalidate()
              }}
            />
            <Input
              placeholder="后缀"
              style={{ width: 140 }}
              value={suffix}
              onChange={(e) => {
                setSuffix(e.target.value)
                invalidate()
              }}
            />
          </Space>
        )}
        {mode === 'magic' && (
          <Input
            placeholder="模板（支持 {title} {season} {episode} {ext}）"
            value={template}
            onChange={(e) => {
              setTemplate(e.target.value)
              invalidate()
            }}
          />
        )}
        {error && <Alert type="error" showIcon message={error} />}
        {notice && <Alert type="warning" showIcon message={notice} />}
        {changes !== null && (
          <>
            <Typography.Text type="secondary">共 {changes.length} 项，确认后立即执行：</Typography.Text>
            <Table<Change>
              rowKey="from"
              size="small"
              pagination={false}
              scroll={{ y: 320 }}
              dataSource={changes}
              columns={[
                { title: '原名', dataIndex: 'from', key: 'from' },
                { title: '新名', dataIndex: 'to', key: 'to' },
              ]}
            />
          </>
        )}
      </Space>
    </Modal>
  )
}
