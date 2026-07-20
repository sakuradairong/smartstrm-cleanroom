import { useMemo } from 'react'
import { Navigate, NavLink, Route, Routes, useLocation } from 'react-router'
import { App as AntApp, Button, ConfigProvider, Layout, Menu, theme as antdTheme } from 'antd'
import { DashboardOutlined, FolderOpenOutlined, HistoryOutlined, MoonOutlined, SettingOutlined, SunOutlined, UnorderedListOutlined } from '@ant-design/icons'
import { useTheme } from './theme'
import DashboardPage from './pages/DashboardPage'
import TasksPage from './pages/TasksPage'
import StorageBrowserPage from './pages/StorageBrowserPage'
import HistoryPage from './pages/HistoryPage'
import ConfigPage from './pages/ConfigPage'

const { Sider, Content, Footer } = Layout

const NAV_ITEMS = [
  { key: '/', icon: <DashboardOutlined />, label: <NavLink to="/">仪表盘</NavLink> },
  { key: '/tasks', icon: <UnorderedListOutlined />, label: <NavLink to="/tasks">任务</NavLink> },
  { key: '/storage', icon: <FolderOpenOutlined />, label: <NavLink to="/storage">存储浏览</NavLink> },
  { key: '/history', icon: <HistoryOutlined />, label: <NavLink to="/history">运行日志</NavLink> },
  { key: '/config', icon: <SettingOutlined />, label: <NavLink to="/config">配置</NavLink> },
]

function selectedKey(pathname: string): string {
  const match = NAV_ITEMS.map(item => item.key)
    .filter(key => key === '/' ? pathname === '/' : pathname.startsWith(key))
    .sort((a, b) => b.length - a.length)
  return match[0] ?? '/'
}

export default function App() {
  const { mode, toggle } = useTheme()
  const location = useLocation()
  const algorithm = useMemo(
    () => (mode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm),
    [mode],
  )
  return (
    <ConfigProvider theme={{ algorithm }}>
      <AntApp>
        <Layout style={{ minHeight: '100vh' }}>
          <Sider breakpoint="md" collapsedWidth={0} width={200}>
            <div style={{ padding: 16, fontWeight: 600 }}>SmartStrm</div>
            <Menu theme={mode === 'dark' ? 'dark' : 'light'} mode="inline" selectedKeys={[selectedKey(location.pathname)]} items={NAV_ITEMS} />
          </Sider>
          <Layout>
            <Content style={{ padding: 16, maxWidth: 1400, width: '100%', margin: '0 auto' }}>
              <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 8 }}>
                <Button aria-label="切换主题" icon={mode === 'dark' ? <SunOutlined /> : <MoonOutlined />} onClick={toggle} />
              </div>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/tasks" element={<TasksPage />} />
                <Route path="/storage" element={<StorageBrowserPage />} />
                <Route path="/history" element={<HistoryPage />} />
                <Route path="/config" element={<ConfigPage />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Content>
            <Footer style={{ textAlign: 'center' }}>
              Copyright © 2025 SmartStrm Clean-room contributors · <a href="https://github.com/sakuradairong/smartstrm-cleanroom" target="_blank" rel="noopener noreferrer">获取对应源码</a> · <a href="https://github.com/sakuradairong/smartstrm-cleanroom/blob/main/LICENSE" target="_blank" rel="noopener noreferrer">AGPL-3.0-only</a> · 本程序不提供任何担保
            </Footer>
          </Layout>
        </Layout>
      </AntApp>
    </ConfigProvider>
  )
}
