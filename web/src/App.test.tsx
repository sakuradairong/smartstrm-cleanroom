import { render, screen } from '@testing-library/react'
import App from './App'

test('渲染应用标题', () => {
  render(<App />)
  expect(screen.getByRole('heading', { name: 'SmartStrm Clean-room' })).toBeInTheDocument()
})
