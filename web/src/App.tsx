import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { Navigate, NavLink, Route, Routes, useLocation } from 'react-router'
import { App as AntApp, Button, ConfigProvider, Drawer, Layout, Menu, Spin, theme as antdTheme, Typography } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { DashboardOutlined, FolderOpenOutlined, GithubOutlined, HistoryOutlined, MenuOutlined, MoonOutlined, PlaySquareOutlined, SettingOutlined, SunOutlined, UnorderedListOutlined } from '@ant-design/icons'
import { useTheme } from './theme'
import './App.css'

const DashboardPage = lazy(() => import('./pages/DashboardPage'))
const TasksPage = lazy(() => import('./pages/TasksPage'))
const StorageBrowserPage = lazy(() => import('./pages/StorageBrowserPage'))
const HistoryPage = lazy(() => import('./pages/HistoryPage'))
const ConfigPage = lazy(() => import('./pages/ConfigPage'))

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
    <nav className="app-navigation" aria-label="主导航">
      <Menu
        theme={mode === 'dark' ? 'dark' : 'light'}
        mode="inline"
        selectedKeys={[selectedKey(location.pathname)]}
        items={NAV_ITEMS}
        onClick={() => setNavigationOpen(false)}
      />
    </nav>
  )

  return (
    <ConfigProvider
      locale={zhCN}
      button={{ autoInsertSpace: false }}
      theme={{
        algorithm,
        token: {
          colorPrimary: '#0d6efd',
          borderRadius: 10,
          borderRadiusLG: 12,
          controlHeight: 40,
          fontFamily: 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Noto Sans SC", sans-serif',
        },
      }}
    >
      <AntApp>
        <Layout className="app-shell" data-theme={mode}>
          <Sider
            className="app-sider"
            theme={mode === 'dark' ? 'dark' : 'light'}
            breakpoint="md"
            collapsedWidth={0}
            trigger={null}
            width={280}
          >
            <div className="app-brand">
              <span className="app-brand-mark" aria-hidden="true"><PlaySquareOutlined /></span>
              <span className="app-brand-name">SmartStrm</span>
            </div>
            {navigation}
            <div className="app-sidebar-meta">
              <span className="app-edition">开源增强版</span>
              <Typography.Link
                className="app-project-link"
                href="https://github.com/sakuradairong/smartstrm-cleanroom"
                target="_blank"
                rel="noopener noreferrer"
              >
                <GithubOutlined /> SmartStrm
              </Typography.Link>
              <span className="app-version">Clean-room</span>
            </div>
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
              <main className="app-page-panel">
                <Suspense fallback={<Spin size="large" tip="正在加载页面…" fullscreen />}>
                  <Routes>
                    <Route path="/" element={<DashboardPage />} />
                    <Route path="/tasks" element={<TasksPage />} />
                    <Route path="/storage" element={<StorageBrowserPage />} />
                    <Route path="/history" element={<HistoryPage />} />
                    <Route path="/config" element={<ConfigPage />} />
                    <Route path="*" element={<Navigate to="/" replace />} />
                  </Routes>
                </Suspense>
              </main>
            </Content>
            <Footer className="app-footer">
              Copyright © 2025 SmartStrm Clean-room contributors ·{' '}
              <Typography.Link href="https://github.com/sakuradairong/smartstrm-cleanroom" target="_blank" rel="noopener noreferrer">
                获取对应源码
              </Typography.Link>{' · '}
              <Typography.Link href="https://github.com/sakuradairong/smartstrm-cleanroom/blob/main/LICENSE" target="_blank" rel="noopener noreferrer">
                AGPL-3.0-only
              </Typography.Link>{' · 本程序不提供任何担保'}
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
