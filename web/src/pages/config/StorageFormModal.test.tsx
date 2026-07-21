import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App as AntApp } from 'antd'
import StorageFormModal from './StorageFormModal'
import type { StorageConfig } from '../../api/types'

const webdav: StorageConfig = {
  id: 'dav',
  type: 'webdav',
  root: '/media',
  endpoint: 'https://dav.example',
  username: 'alice',
  password: '********',
  token: '',
}

function renderModal(props: Partial<React.ComponentProps<typeof StorageFormModal>> = {}) {
  const onSave = vi.fn()
  render(
    <AntApp>
      <StorageFormModal
        open
        index={-1}
        storages={[]}
        onSave={onSave}
        onClose={() => {}}
        {...props}
      />
    </AntApp>,
  )
  return onSave
}

async function chooseType(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getByLabelText('类型'))
  const matches = await screen.findAllByText(name)
  await user.click(matches[matches.length - 1])
}

test('类型切换显隐字段', async () => {
  const user = userEvent.setup()
  renderModal()
  expect(screen.getByLabelText('Root')).toBeInTheDocument()
  expect(screen.queryByLabelText('Endpoint')).not.toBeInTheDocument()

  await chooseType(user, 'webdav')
  expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
  expect(screen.getByLabelText('Root')).toBeInTheDocument()
  expect(screen.getByLabelText('用户名')).toBeInTheDocument()
  expect(screen.getByLabelText('密码')).toBeInTheDocument()

  await chooseType(user, 'openlist')
  expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
  expect(screen.getByLabelText('Token')).toBeInTheDocument()
  expect(screen.queryByLabelText('用户名')).not.toBeInTheDocument()

  await chooseType(user, 'ani_open')
  expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
  expect(screen.queryByLabelText('Root')).not.toBeInTheDocument()
})

test('编辑已有存储时 ID 不可改', () => {
  renderModal({ index: 0, storages: [webdav] })
  expect(screen.getByLabelText('存储 ID')).toBeDisabled()
})

test('编辑时密码显示 ********，未修改提交保留原值', async () => {
  const user = userEvent.setup()
  const onSave = renderModal({ index: 0, storages: [webdav] })
  const password = screen.getByLabelText('密码')
  expect(password).toHaveValue('********')
  await user.clear(password)
  await user.click(screen.getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ password: '********' }), 0))
})

test('新增 ani_open 时 UI 值映射为后端 ani-open', async () => {
  const user = userEvent.setup()
  const onSave = renderModal()
  await user.type(screen.getByLabelText('存储 ID'), 'ani')
  await chooseType(user, 'ani_open')
  await user.type(screen.getByLabelText('Endpoint'), 'https://ani.example')
  await user.click(screen.getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
    id: 'ani',
    type: 'ani-open',
    root: '',
    endpoint: 'https://ani.example',
  }), -1))
})
