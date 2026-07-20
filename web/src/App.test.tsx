import { render, screen } from '@testing-library/react'
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
