import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import JsonField, { validateJSON } from './JsonField'

test('合法 JSON 通过校验', () => {
  expect(validateJSON('{"a":1}')).toBeNull()
  render(<JsonField value="{}" onChange={() => {}} schemaHint="JSON 对象" />)
  expect(screen.getByRole('textbox')).not.toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByText('JSON 对象')).toBeInTheDocument()
})

test('非法 JSON 显示错误并标记 invalid', async () => {
  const user = userEvent.setup()
  function Example() {
    const [value, setValue] = useState('{}')
    return <JsonField value={value} onChange={setValue} />
  }
  render(<Example />)
  await user.clear(screen.getByRole('textbox'))
  await user.type(screen.getByRole('textbox'), 'bad')
  expect(screen.getByRole('textbox')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByText(/Unexpected token|JSON/)).toBeInTheDocument()
})
