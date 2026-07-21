import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SecretInput, { REDACTED, resolveSecret } from './SecretInput'

test('初始值 ******** 时保持不变视为保留', () => {
  expect(resolveSecret(REDACTED, 'old-secret')).toBe('old-secret')
})

test('清空输入视为保留原密钥', () => {
  expect(resolveSecret('', 'old-secret')).toBe('old-secret')
})

test('输入新值则使用新值', async () => {
  const onChange = vi.fn()
  const user = userEvent.setup()
  render(<SecretInput value="" onChange={onChange} placeholder="密钥" />)
  await user.type(screen.getByPlaceholderText('密钥'), 'new-secret')
  expect(onChange).toHaveBeenLastCalledWith('t')
  expect(resolveSecret('new-secret', 'old-secret')).toBe('new-secret')
})
