import { useEffect, useRef, useState } from 'react'
import { Alert, App, Button, Input, Modal, Radio, Space, Steps, Table, Typography } from 'antd'
import { getJSON, postJSON } from '../api/client'

// 变量说明逐字移植旧 internal/app/ui/index.html 的 tmdbDialog。
const TEMPLATE_HELP =
  '命名模板变量：{title} {title_original} {year} {tmdbid} {season} {episode} {episode_name} {quality} {ext}'
// 默认模板与旧 selectTMDB 一致。
const TV_TEMPLATE = '{title} - S{season:2}E{episode:2}{.,episode_name}{ext}'
const MOVIE_TEMPLATE = '{title} ({year}) {tmdbid}{ext}'

// 字段与 internal/tmdb/client.go 的 SearchResult 一致。
interface TmdbResult {
  id: number
  media_type: string
  title: string
  original_title: string
  release_date: string
  poster_path: string
  overview: string
  vote_average: number
}

interface Change {
  from: string
  to: string
}

interface TmdbRenameResult {
  preview: boolean
  changes: Change[]
}

interface TmdbModalProps {
  storageId: string
  path: string
  open: boolean
  onClose: () => void
  onDone: () => void
}

export default function TmdbModal({ storageId, path, open, onClose, onDone }: TmdbModalProps) {
  const { message } = App.useApp()
  const [step, setStep] = useState(0)
  const [query, setQuery] = useState('')
  const [mediaType, setMediaType] = useState<'movie' | 'tv'>('tv')
  const [year, setYear] = useState('')
  const [results, setResults] = useState<TmdbResult[]>([])
  const [selected, setSelected] = useState<TmdbResult | null>(null)
  const [template, setTemplate] = useState(TV_TEMPLATE)
  const [changes, setChanges] = useState<Change[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  // 关闭时重置内部步骤状态，重新打开回到搜索步。
  const wasOpen = useRef(false)
  useEffect(() => {
    if (open && !wasOpen.current) {
      setStep(0)
      setQuery('')
      setMediaType('tv')
      setYear('')
      setResults([])
      setSelected(null)
      setTemplate(TV_TEMPLATE)
      setChanges(null)
      setError(null)
      setNotice(null)
      setLoading(false)
    }
    wasOpen.current = open
  }, [open])

  const search = async () => {
    if (loading || !query.trim()) return
    setLoading(true)
    setError(null)
    setNotice(null)
    try {
      // 参数拼接与旧 tmdbRecognize 一致：query/type/year 全部携带。
      const found = await getJSON<TmdbResult[]>(
        `/api/tmdb/search?query=${encodeURIComponent(query.trim())}&type=${mediaType}&year=${encodeURIComponent(year.trim())}`,
      )
      if (!found.length) {
        setNotice('未找到结果')
        return
      }
      // 旧 UI results.slice(0,12)：最多展示 12 张卡片
      setResults(found.slice(0, 12))
      setStep(1)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const select = (item: TmdbResult) => {
    setSelected(item)
    setTemplate(mediaType === 'tv' ? TV_TEMPLATE : MOVIE_TEMPLATE)
    setChanges(null)
    setError(null)
    setNotice(null)
    setStep(2)
  }

  const endpoint = `/api/storages/${encodeURIComponent(storageId)}/tmdb-rename`

  // 请求体字段与 app.go 的 tmdbRename 处理器一致。
  const rename = async (preview: boolean) => {
    if (loading || !selected || !template.trim()) return
    setLoading(true)
    setError(null)
    setNotice(null)
    try {
      const result = await postJSON<TmdbRenameResult>(endpoint, {
        path,
        media_type: mediaType,
        tmdb_id: selected.id,
        template,
        preview,
      })
      if (preview) {
        const list = result.changes ?? []
        if (list.length === 0) {
          setChanges(null)
          setNotice('没有可识别并重命名的文件')
          return
        }
        setChanges(list)
      } else {
        message.success('影视识别重命名完成')
        onDone()
      }
    } catch (err) {
      if (preview) setChanges(null)
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const footer = (
    <Space>
      {step > 0 && (
        <Button
          disabled={loading}
          onClick={() => {
            setStep(step - 1)
            setChanges(null)
            setError(null)
            setNotice(null)
          }}
        >
          上一步
        </Button>
      )}
      <Button onClick={onClose} disabled={loading}>
        取消
      </Button>
      {step === 0 && (
        <Button type="primary" loading={loading} disabled={!query.trim()} onClick={() => void search()}>
          搜索
        </Button>
      )}
      {step === 2 && (
        <>
          <Button loading={loading} disabled={!template.trim()} onClick={() => void rename(true)}>
            预览
          </Button>
          {changes !== null && (
            <Button type="primary" loading={loading} onClick={() => void rename(false)}>
              确认执行
            </Button>
          )}
        </>
      )}
    </Space>
  )

  return (
    <Modal title="影视识别" open={open} onCancel={onClose} width={720} footer={footer}>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Steps
          size="small"
          current={step}
          items={[{ title: '搜索' }, { title: '选择结果' }, { title: '命名模板' }]}
        />
        {step === 0 && (
          <Space wrap>
            <Input
              placeholder="影视名称"
              style={{ width: 220 }}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onPressEnter={() => void search()}
            />
            <Radio.Group
              value={mediaType}
              onChange={(e) => setMediaType(e.target.value as 'movie' | 'tv')}
              options={[
                { value: 'movie', label: 'movie（电影）' },
                { value: 'tv', label: 'tv（剧集）' },
              ]}
            />
            <Input
              placeholder="年份（可留空）"
              style={{ width: 140 }}
              value={year}
              onChange={(e) => setYear(e.target.value)}
              onPressEnter={() => void search()}
            />
          </Space>
        )}
        {step === 1 && (
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))',
              gap: 12,
              maxHeight: '50vh',
              overflow: 'auto',
            }}
          >
            {results.map((item) => (
              <button
                key={item.id}
                type="button"
                data-testid="tmdb-card"
                onClick={() => select(item)}
                style={{
                  padding: 8,
                  background: 'transparent',
                  border: '1px solid rgba(255,255,255,0.2)',
                  borderRadius: 8,
                  cursor: 'pointer',
                  color: 'inherit',
                  textAlign: 'left',
                }}
              >
                {item.poster_path ? (
                  // 海报地址拼接与旧 UI 一致：/api/tmdb/poster?path=<encodeURIComponent(poster_path)>
                  <img
                    alt=""
                    src={`/api/tmdb/poster?path=${encodeURIComponent(item.poster_path)}`}
                    style={{ width: '100%', aspectRatio: '2 / 3', objectFit: 'cover', borderRadius: 4 }}
                  />
                ) : (
                  <Typography.Text type="secondary">无海报</Typography.Text>
                )}
                <div>
                  <b>{item.title}</b>
                  <br />
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {item.release_date || '未知'} · {(item.vote_average || 0).toFixed(1)}
                  </Typography.Text>
                </div>
              </button>
            ))}
          </div>
        )}
        {step === 2 && (
          <>
            <Input
              placeholder="命名模板"
              value={template}
              onChange={(e) => {
                setTemplate(e.target.value)
                setChanges(null)
              }}
            />
            <Typography.Text type="secondary">{TEMPLATE_HELP}</Typography.Text>
            {changes !== null && (
              <>
                <Typography.Text type="secondary">
                  {selected?.title}（评分 {(selected?.vote_average || 0).toFixed(1)}）共 {changes.length}{' '}
                  项，确认后立即执行：
                </Typography.Text>
                <Table<Change>
                  rowKey="from"
                  size="small"
                  pagination={false}
                  scroll={{ y: 280 }}
                  dataSource={changes}
                  columns={[
                    { title: '原名', dataIndex: 'from', key: 'from' },
                    { title: '新名', dataIndex: 'to', key: 'to' },
                  ]}
                />
              </>
            )}
          </>
        )}
        {error && <Alert type="error" showIcon message={error} />}
        {notice && <Alert type="warning" showIcon message={notice} />}
      </Space>
    </Modal>
  )
}
