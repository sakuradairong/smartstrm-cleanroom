import { useState } from 'react'
import { Button, Input, Space, Table, Tag, Typography } from 'antd'
import type { TableProps } from 'antd'
import type { HistoryEvent } from '../api/types'
import { useHistoryStream } from '../hooks/useHistoryStream'

// 与旧 UI renderHistory 一致：有错误只显示错误，否则显示计数
function resultText(event: HistoryEvent): string {
  if (event.error) return event.error
  return `生成 ${event.created || 0} / 复制 ${event.copied || 0} / 删除 ${event.removed || 0} / 跳过 ${event.skipped || 0}`
}

export default function HistoryPage() {
  const [keyword, setKeyword] = useState('')
  const [filter, setFilter] = useState('')
  const { events, connected } = useHistoryStream(filter)

  const applyFilter = () => setFilter(keyword.trim())

  const columns: TableProps<HistoryEvent>['columns'] = [
    {
      title: '时间',
      key: 'timestamp',
      render: (_, event) => new Date(event.timestamp).toLocaleString(),
    },
    { title: '任务', dataIndex: 'task_id', key: 'task_id' },
    {
      title: '事件',
      key: 'type',
      render: (_, event) => (
        <Tag color={event.type === 'failed' ? 'red' : event.type === 'completed' ? 'green' : undefined}>
          {event.type}
        </Tag>
      ),
    },
    { title: '路径', key: 'path', render: (_, event) => event.path || '' },
    { title: '结果·错误', key: 'result', render: (_, event) => resultText(event) },
  ]

  return (
    <>
      <h2>运行日志</h2>
      <Space style={{ marginBottom: 16 }} wrap>
        <Input
          placeholder="任务 ID（留空显示全部）"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          onPressEnter={applyFilter}
          style={{ width: 240 }}
          allowClear
        />
        <Button type="primary" onClick={applyFilter}>
          筛选
        </Button>
        <Typography.Text type="secondary">
          {connected ? '实时连接已建立' : '实时连接重试中…'}
        </Typography.Text>
      </Space>
      <Table<HistoryEvent> rowKey="id" pagination={false} columns={columns} dataSource={events} />
    </>
  )
}
