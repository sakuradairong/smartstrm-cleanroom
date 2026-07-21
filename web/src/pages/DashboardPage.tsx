import { useCallback, useState } from 'react'
import { NavLink } from 'react-router'
import { Alert, App, Button, Card, Col, Empty, Row, Space, Spin, Statistic, Table, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import dayjs from 'dayjs'
import { getOverview } from '../api/bootstrap'
import type { HistoryEvent, Overview } from '../api/types'
import { usePolling } from '../hooks/usePolling'

const historyColumns: ColumnsType<HistoryEvent> = [
  { title: '时间', dataIndex: 'timestamp', render: value => dayjs(value).format('YYYY-MM-DD HH:mm:ss') },
  { title: '任务', dataIndex: 'task_id' },
  { title: '事件', dataIndex: 'type' },
  {
    title: '结果',
    render: (_, event) => event.error || [
      `新增 ${event.created ?? 0}`,
      `复制 ${event.copied ?? 0}`,
      `删除 ${event.removed ?? 0}`,
      `跳过 ${event.skipped ?? 0}`,
      `失败 ${event.failed ?? 0}`,
    ].join(' · '),
  },
]

export default function DashboardPage() {
  const { message } = App.useApp()
  const [overview, setOverview] = useState<Overview | null>(null)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      setOverview(await getOverview())
      setError('')
    } catch (loadError) {
      const text = loadError instanceof Error ? loadError.message : '仪表盘加载失败'
      setError(text)
      void message.error(text)
    }
  }, [message])

  usePolling(refresh, 5000)

  if (!overview && !error) return <Spin tip="加载仪表盘"><div style={{ minHeight: 160 }} /></Spin>

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Typography.Title level={2}>仪表盘</Typography.Title>
      {error ? <Alert type="error" showIcon message={error} /> : null}
      {overview ? <>
        <Row gutter={[16, 16]}>
          <Col xs={12} md={6}><Card><Statistic title="任务总数" value={overview.tasks.total} /></Card></Col>
          <Col xs={12} md={6}><Card><Statistic title="运行中" value={overview.tasks.running} valueStyle={{ color: '#3f8600' }} /></Card></Col>
          <Col xs={12} md={6}><Card><Statistic title="失败" value={overview.tasks.failed} valueStyle={{ color: '#cf1322' }} /></Card></Col>
          <Col xs={12} md={6}><Card><Statistic title="队列" value={overview.tasks.queued} /></Card></Col>
          <Col xs={12} md={6}><Card><Statistic title="存储总数" value={overview.storages} /></Card></Col>
        </Row>
        <Card title="最近运行">
          <Table
            rowKey="id"
            columns={historyColumns}
            dataSource={(overview.history ?? []).slice(0, 10)}
            pagination={false}
            locale={{ emptyText: <Empty description="暂无运行记录" /> }}
            scroll={{ x: 700 }}
          />
        </Card>
        <Card title="快捷操作">
          <Space wrap>
            <Button type="primary"><NavLink to="/tasks">运行任务</NavLink></Button>
            <Button><NavLink to="/storage">浏览存储</NavLink></Button>
            <Button><NavLink to="/config">修改配置</NavLink></Button>
          </Space>
        </Card>
      </> : null}
    </Space>
  )
}
