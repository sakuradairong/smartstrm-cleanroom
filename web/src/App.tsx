import { useEffect, useMemo, useState } from 'react'
import { Navigate, NavLink, Route, Routes, useLocation } from 'react-router'
import { App as AntApp, Button, ConfigProvider, Drawer, Layout, Menu, theme as antdTheme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { DashboardOutlined, FolderOpenOutlined, HistoryOutlined, MenuOutlined, MoonOutlined, SettingOutlined, SunOutlined, UnorderedListOutlined } from '@ant-design/icons'
import { useTheme } from './theme'
import DashboardPage from './pages/DashboardPage'
import TasksPage from './pages/TasksPage'
import StorageBrowserPage from './pages/StorageBrowserPage'
import HistoryPage from './pages/HistoryPage'
import ConfigPage from './pages/ConfigPage'
import './App.css'

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
  const [navigationOpen, setNavigationOpen] = useState(false)
  const algorithm = useMemo(
    () => (mode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm),
    [mode],
  )
  useEffect(() => setNavigationOpen(false), [location.pathname])

  const navigation = (
    <Menu
      theme={mode === 'dark' ? 'dark' : 'light'}
      mode="inline"
      selectedKeys={[selectedKey(location.pathname)]}
      items={NAV_ITEMS}
      onClick={() => setNavigationOpen(false)}
    />
  )

  return (
    <ConfigProvider locale={zhCN} button={{ autoInsertSpace: false }} theme={{ algorithm }}>
      <AntApp>
        <Layout className="app-shell">
          <Sider className="app-sider" breakpoint="md" collapsedWidth={0} trigger={null} width={200}>
            <div className="app-brand">SmartStrm</div>
            {navigation}
          </Sider>
          <Layout className="app-main-layout">
            <Content className="app-content">
              <header className="app-toolbar">
                <Button
                  className="app-menu-button"
                  aria-label="打开导航菜单"
                  icon={<MenuOutlined />}
                  onClick={() => setNavigationOpen(true)}
                />
                <Button aria-label="切换主题" icon={mode === 'dark' ? <SunOutlined /> : <MoonOutlined />} onClick={toggle} />
              </header>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/tasks" element={<TasksPage />} />
                <Route path="/storage" element={<StorageBrowserPage />} />
                <Route path="/history" element={<HistoryPage />} />
                <Route path="/config" element={<ConfigPage />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Content>
            <Footer className="app-footer">
              Copyright © 2025 SmartStrm Clean-room contributors · <a href="https://github.com/sakuradairong/smartstrm-cleanroom" target="_blank" rel="noopener noreferrer">获取对应源码</a> · <a href="https://github.com/sakuradairong/smartstrm-cleanroom/blob/main/LICENSE" target="_blank" rel="noopener noreferrer">AGPL-3.0-only</a> · 本程序不提供任何担保
            </Footer>
          </Layout>
          <Drawer
            className="app-navigation-drawer"
            title="SmartStrm 导航"
            placement="left"
            width={240}
            open={navigationOpen}
            onClose={() => setNavigationOpen(false)}
            destroyOnHidden
          >
            {navigation}
          </Drawer>
        </Layout>
      </AntApp>
    </ConfigProvider>
  )
}
