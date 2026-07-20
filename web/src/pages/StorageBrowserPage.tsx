import { useEffect, useMemo, useState } from 'react'
import { App, Button, Input, Modal, Popconfirm, Select, Space, Table, Typography } from 'antd'
import type { TableProps } from 'antd'
import { deleteJSON, getJSON, postJSON } from '../api/client'
import type { StorageEntry, StorageSummary } from '../api/types'
import EntryIcon from '../components/EntryIcon'

export const collator = new Intl.Collator('zh', { numeric: true, sensitivity: 'base' })

type SortKey = 'name' | 'size' | 'modified'
type SortState = { key: SortKey; direction: 'asc' | 'desc' }

export function filterEntries(entries: StorageEntry[], filter: string): StorageEntry[] {
  const q = filter.trim().toLowerCase()
  return q ? entries.filter((e) => e.name.toLowerCase().includes(q)) : entries
}

export function sortEntries(entries: StorageEntry[], sort: SortState): StorageEntry[] {
  const factor = sort.direction === 'asc' ? 1 : -1
  return [...entries].sort((a, b) => {
    if (sort.key === 'name') return factor * collator.compare(a.name, b.name)
    if (sort.key === 'size') return factor * (a.size - b.size)
    return factor * (Date.parse(a.modified || '0') - Date.parse(b.modified || '0'))
  })
}

// 与旧 internal/app/ui/index.html 的 formatSize 完全一致。
export function formatSize(n: number): string {
  if (!n) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return v.toFixed(i ? 1 : 0) + ' ' + units[i]
}

const NAME_WIDTH_KEY = 'smartstrm.storageNameWidth'
const NAME_WIDTHS = [220, 320, 420]

function loadNameWidth(): number {
  try {
    const v = Number(localStorage.getItem(NAME_WIDTH_KEY))
    return NAME_WIDTHS.includes(v) ? v : 320
  } catch {
    return 320
  }
}

function parentPath(path: string): string {
  // 与旧 UI 的 up() 一致：去尾段，根停住。
  const parts = path.replace(/\/$/, '').split('/')
  parts.pop()
  return parts.join('/') || '/'
}

function RenameModal({
  entry,
  loading,
  onCancel,
  onOk,
}: {
  entry: StorageEntry | null
  loading: boolean
  onCancel: () => void
  onOk: (name: string) => void
}) {
  const [name, setName] = useState(entry?.name ?? '')
  return (
    <Modal
      title="重命名"
      open={entry !== null}
      onCancel={onCancel}
      onOk={() => {
        if (name.trim()) onOk(name.trim())
      }}
      confirmLoading={loading}
      okText="确定"
      cancelText="取消"
    >
      <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="新名称" />
    </Modal>
  )
}

function MoveModal({
  entry,
  loading,
  onCancel,
  onOk,
}: {
  entry: StorageEntry | null
  loading: boolean
  onCancel: () => void
  onOk: (destination: string) => void
}) {
  const [destination, setDestination] = useState('/')
  return (
    <Modal
      title={entry ? `移动 ${entry.name}` : '移动'}
      open={entry !== null}
      onCancel={onCancel}
      onOk={() => {
        if (destination.trim()) onOk(destination.trim())
      }}
      confirmLoading={loading}
      okText="确定"
      cancelText="取消"
    >
      <Input value={destination} onChange={(e) => setDestination(e.target.value)} placeholder="目标目录" />
    </Modal>
  )
}

function MkdirModal({
  open,
  loading,
  onCancel,
  onOk,
}: {
  open: boolean
  loading: boolean
  onCancel: () => void
  onOk: (name: string) => void
}) {
  const [name, setName] = useState('')
  return (
    <Modal
      title="新建目录"
      open={open}
      onCancel={onCancel}
      onOk={() => {
        if (name.trim()) onOk(name.trim())
      }}
      confirmLoading={loading}
      okText="确定"
      cancelText="取消"
    >
      <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="目录名" />
    </Modal>
  )
}

export default function StorageBrowserPage() {
  const { message } = App.useApp()
  const [storages, setStorages] = useState<StorageSummary[]>([])
  const [storageId, setStorageId] = useState('')
  const [path, setPath] = useState('/')
  const [pathInput, setPathInput] = useState('/')
  const [entries, setEntries] = useState<StorageEntry[]>([])
  const [filter, setFilter] = useState('')
  const [sort, setSort] = useState<SortState>({ key: 'name', direction: 'asc' })
  const [nameWidth, setNameWidth] = useState(loadNameWidth)
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [renameTarget, setRenameTarget] = useState<StorageEntry | null>(null)
  const [moveTarget, setMoveTarget] = useState<StorageEntry | null>(null)
  const [mkdirOpen, setMkdirOpen] = useState(false)

  const browse = async (nextPath: string, id = storageId) => {
    if (!id) return
    try {
      const list = await getJSON<StorageEntry[]>(
        `/api/storages/${encodeURIComponent(id)}/entries?path=${encodeURIComponent(nextPath)}`,
      )
      setEntries(list)
      setPath(nextPath)
      setPathInput(nextPath)
    } catch (err) {
      message.error((err as Error).message)
    }
  }

  useEffect(() => {
    void (async () => {
      try {
        const list = await getJSON<StorageSummary[]>('/api/storages')
        setStorages(list)
        if (list.length > 0) {
          setStorageId(list[0].id)
          await browse('/', list[0].id)
        }
      } catch (err) {
        message.error((err as Error).message)
      }
    })()
    // 仅挂载时执行一次
  }, [])

  const filtered = useMemo(() => filterEntries(entries, filter), [entries, filter])
  const sorted = useMemo(() => sortEntries(filtered, sort), [filtered, sort])

  const onTableChange: TableProps<StorageEntry>['onChange'] = (_pagination, _filters, sorter) => {
    const s = Array.isArray(sorter) ? sorter[0] : sorter
    if (s.columnKey === 'name' && s.order) {
      setSort({ key: 'name', direction: s.order === 'descend' ? 'desc' : 'asc' })
    }
  }

  const changeNameWidth = (value: number) => {
    setNameWidth(value)
    try {
      localStorage.setItem(NAME_WIDTH_KEY, String(value))
    } catch {
      // localStorage 不可用时忽略
    }
  }

  const submitRename = async (name: string) => {
    const entry = renameTarget
    if (!entry || actionLoading) return
    setActionLoading(`${entry.path}:rename`)
    try {
      await postJSON(`/api/storages/${encodeURIComponent(storageId)}/rename`, { path: entry.path, new_name: name })
      message.success('已重命名')
      setRenameTarget(null)
      await browse(path)
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setActionLoading(null)
    }
  }

  const submitMove = async (destination: string) => {
    const entry = moveTarget
    if (!entry || actionLoading) return
    setActionLoading(`${entry.path}:move`)
    try {
      await postJSON(`/api/storages/${encodeURIComponent(storageId)}/move`, { path: entry.path, destination })
      message.success('已移动')
      setMoveTarget(null)
      await browse(path)
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setActionLoading(null)
    }
  }

  const submitDelete = async (entry: StorageEntry) => {
    if (actionLoading) return
    setActionLoading(`${entry.path}:delete`)
    try {
      await deleteJSON(`/api/storages/${encodeURIComponent(storageId)}/entry?path=${encodeURIComponent(entry.path)}`)
      message.success('已删除')
      await browse(path)
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setActionLoading(null)
    }
  }

  const copyURL = async (entry: StorageEntry) => {
    if (actionLoading) return
    setActionLoading(`${entry.path}:copy`)
    try {
      // 与旧 UI 的 copyURL 一致：直接写入后端返回的 url。
      const d = await getJSON<{ url: string }>(
        `/api/storages/${encodeURIComponent(storageId)}/stream-url?path=${encodeURIComponent(entry.path)}`,
      )
      await navigator.clipboard.writeText(d.url)
      message.success('STRM 地址已复制')
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setActionLoading(null)
    }
  }

  const submitMkdir = async (name: string) => {
    if (actionLoading) return
    setActionLoading(`${path}:mkdir`)
    try {
      // mkdir 处理器只读 path 字段，与旧 UI 一样发送拼接后的完整路径。
      await postJSON(`/api/storages/${encodeURIComponent(storageId)}/mkdir`, {
        path: path.replace(/\/$/, '') + '/' + name,
      })
      message.success('已创建')
      setMkdirOpen(false)
      await browse(path)
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setActionLoading(null)
    }
  }

  const columns: TableProps<StorageEntry>['columns'] = [
    {
      title: '名称',
      key: 'name',
      width: nameWidth,
      ellipsis: true,
      sorter: true,
      sortOrder: sort.key === 'name' ? (sort.direction === 'asc' ? 'ascend' : 'descend') : null,
      render: (_, entry) => (
        <Space size={4}>
          <EntryIcon entry={entry} />
          {entry.is_dir ? (
            <Typography.Link strong onClick={() => void browse(entry.path)}>
              {entry.name}
            </Typography.Link>
          ) : (
            entry.name
          )}
        </Space>
      ),
    },
    {
      title: '类型',
      key: 'type',
      render: (_, entry) => (entry.is_dir ? '目录' : '文件'),
    },
    {
      title: '大小',
      key: 'size',
      render: (_, entry) => formatSize(entry.size),
    },
    {
      title: '修改时间',
      key: 'modified',
      render: (_, entry) => entry.modified || '',
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, entry) => (
        <Space wrap>
          <Button loading={actionLoading === `${entry.path}:rename`} onClick={() => setRenameTarget(entry)}>
            重命名
          </Button>
          <Button loading={actionLoading === `${entry.path}:move`} onClick={() => setMoveTarget(entry)}>
            移动
          </Button>
          {!entry.is_dir && (
            <Button loading={actionLoading === `${entry.path}:copy`} onClick={() => void copyURL(entry)}>
              复制STRM地址
            </Button>
          )}
          <Popconfirm
            title={`确定删除 ${entry.name}？此操作不可恢复。`}
            okText="确定"
            cancelText="取消"
            onConfirm={() => void submitDelete(entry)}
          >
            <Button danger loading={actionLoading === `${entry.path}:delete`}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <h2>存储浏览</h2>
      <Space wrap style={{ marginBottom: 8 }}>
        <Select
          aria-label="存储"
          placeholder="存储"
          style={{ minWidth: 160 }}
          value={storageId || undefined}
          options={storages.map((s) => ({ value: s.id, label: `${s.id} (${s.type})` }))}
          onChange={(id) => {
            setStorageId(id)
            void browse('/', id)
          }}
        />
        <Input
          aria-label="路径"
          style={{ width: 260 }}
          value={pathInput}
          onChange={(e) => setPathInput(e.target.value)}
          onPressEnter={() => void browse(pathInput || '/')}
        />
        <Button onClick={() => void browse(pathInput || '/')}>前往</Button>
        <Button onClick={() => void browse(parentPath(path))}>上一级</Button>
        <Input
          placeholder="筛选"
          allowClear
          style={{ width: 180 }}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <Select
          aria-label="名称列宽"
          style={{ width: 130 }}
          value={nameWidth}
          options={[
            { value: 220, label: '紧凑 220' },
            { value: 320, label: '标准 320' },
            { value: 420, label: '加宽 420' },
          ]}
          onChange={changeNameWidth}
        />
        <Button onClick={() => setMkdirOpen(true)}>新建目录</Button>
        <span role="status" aria-live="polite">
          显示 {filtered.length} / 共 {entries.length} 项
        </span>
      </Space>
      <Table<StorageEntry>
        rowKey="path"
        pagination={false}
        columns={columns}
        dataSource={sorted}
        onChange={onTableChange}
      />
      <RenameModal
        key={'rename:' + (renameTarget?.path ?? '')}
        entry={renameTarget}
        loading={renameTarget !== null && actionLoading === `${renameTarget.path}:rename`}
        onCancel={() => setRenameTarget(null)}
        onOk={(name) => void submitRename(name)}
      />
      <MoveModal
        key={'move:' + (moveTarget?.path ?? '')}
        entry={moveTarget}
        loading={moveTarget !== null && actionLoading === `${moveTarget.path}:move`}
        onCancel={() => setMoveTarget(null)}
        onOk={(destination) => void submitMove(destination)}
      />
      <MkdirModal
        key={String(mkdirOpen)}
        open={mkdirOpen}
        loading={actionLoading === `${path}:mkdir`}
        onCancel={() => setMkdirOpen(false)}
        onOk={(name) => void submitMkdir(name)}
      />
    </>
  )
}
