import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HashRouter } from 'react-router'
import App from './App'

test('布局壳渲染导航与页脚', () => {
  render(<HashRouter><App /></HashRouter>)
  expect(screen.getByRole('link', { name: '仪表盘' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '任务' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '存储浏览' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '运行日志' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: '配置' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /切换主题/ })).toBeInTheDocument()
  expect(screen.getByText(/AGPL-3\.0-only/)).toBeInTheDocument()
})

test('移动导航按钮在页面流内打开抽屉，不使用覆盖标题的零宽触发器', async () => {
  const user = userEvent.setup()
  const { container } = render(<HashRouter><App /></HashRouter>)

  expect(container.querySelector('.ant-layout-sider-zero-width-trigger')).not.toBeInTheDocument()
  const menuButton = screen.getByRole('button', { name: '打开导航菜单' })
  expect(menuButton.closest('header')).toHaveClass('app-toolbar')

  await user.click(menuButton)
  expect(await screen.findByRole('dialog', { name: 'SmartStrm 导航' })).toBeInTheDocument()
})
