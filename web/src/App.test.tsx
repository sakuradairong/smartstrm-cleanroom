import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import App from './App'

beforeEach(() => localStorage.clear())

test('布局壳渲染导航与页脚', async () => {
  render(<MemoryRouter initialEntries={['/']}><App /></MemoryRouter>)
  expect(await screen.findByRole('heading', { name: '仪表盘' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '仪表盘' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '任务' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '存储浏览' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '运行日志' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '配置' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /切换主题/ })).toBeInTheDocument()
  expect(screen.getByText(/AGPL-3\.0-only/)).toBeInTheDocument()
})

test('点击导航菜单切换页面', async () => {
  const user = userEvent.setup()
  render(<MemoryRouter initialEntries={['/']}><App /></MemoryRouter>)
  expect(await screen.findByRole('heading', { name: '仪表盘' })).toBeInTheDocument()
  await user.click(screen.getByRole('link', { name: '任务' }))
  expect(await screen.findByRole('heading', { name: '任务' }, { timeout: 10_000 })).toBeInTheDocument()
})

test('移动导航按钮在页面流内打开抽屉，不使用覆盖标题的零宽触发器', async () => {
  const user = userEvent.setup()
  const { container } = render(<MemoryRouter initialEntries={['/']}><App /></MemoryRouter>)
  expect(await screen.findByRole('heading', { name: '仪表盘' })).toBeInTheDocument()

  expect(container.querySelector('.ant-layout-sider-zero-width-trigger')).not.toBeInTheDocument()
  const menuButton = screen.getByRole('button', { name: '打开导航菜单' })
  expect(menuButton.closest('header')).toHaveClass('app-toolbar')

  await user.click(menuButton)
  expect(await screen.findByRole('dialog', { name: 'SmartStrm 导航' })).toBeInTheDocument()
})

test('主题菜单提供三个带选择状态的模式，并持久化选择', async () => {
  const user = userEvent.setup()
  const { container } = render(<MemoryRouter initialEntries={['/']}><App /></MemoryRouter>)
  const button = screen.getByRole('button', { name: '切换主题，当前：跟随系统' })
  expect(button.closest('header')).toHaveClass('app-toolbar')
  expect(button).toHaveAttribute('aria-haspopup', 'menu')
  await user.click(button)
  expect(button).toHaveAttribute('aria-expanded', 'true')
  expect(await screen.findByRole('menu', { name: '主题偏好' })).toBeInTheDocument()
  expect(screen.getByRole('menuitemradio', { name: /跟随系统/ })).toHaveAttribute('aria-checked', 'true')
  expect(screen.getByRole('menuitemradio', { name: /浅色/ })).toHaveAttribute('aria-checked', 'false')
  await user.click(screen.getByRole('menuitemradio', { name: /深色/ }))
  expect(container.querySelector('.app-shell')).toHaveAttribute('data-theme', 'dark')
  expect(localStorage.getItem('smartstrm.theme')).toBe('dark')

  await user.click(screen.getByRole('button', { name: '切换主题，当前：深色' }))
  await user.click(await screen.findByRole('menuitemradio', { name: /浅色/ }))
  expect(container.querySelector('.app-shell')).toHaveAttribute('data-theme', 'light')
  expect(localStorage.getItem('smartstrm.theme')).toBe('light')

  await user.click(screen.getByRole('button', { name: '切换主题，当前：浅色' }))
  await user.click(await screen.findByRole('menuitemradio', { name: /跟随系统/ }))
  expect(screen.getByRole('button', { name: '切换主题，当前：跟随系统' })).toBeInTheDocument()
  expect(localStorage.getItem('smartstrm.theme')).toBe('system')
})
