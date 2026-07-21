import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

const pan123: StorageConfig = {
  id: 'pan123',
  type: '123_open',
  root: '',
  endpoint: '',
  username: '',
  password: '',
  token: '',
  client_id: 'client-id',
  client_secret: '********',
  root_file_id: '777',
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

async function chooseType(name: string) {
  fireEvent.mouseDown(screen.getByLabelText('类型'))
  const matches = await screen.findAllByText(name)
  fireEvent.click(matches[matches.length - 1])
}

test('webdav 类型显示端点和账号字段', async () => {
  renderModal()
  expect(screen.getByLabelText('Root')).toBeInTheDocument()
  expect(screen.queryByLabelText('Endpoint')).not.toBeInTheDocument()

  await chooseType('webdav')
  expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
  expect(screen.getByLabelText('Root')).toBeInTheDocument()
  expect(screen.getByLabelText('用户名')).toBeInTheDocument()
  expect(screen.getByLabelText('密码')).toBeInTheDocument()
})

test('openlist 类型显示端点和 Token 字段', async () => {
  renderModal()
  await chooseType('openlist')
  expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
  expect(screen.getByLabelText('Token')).toBeInTheDocument()
  expect(screen.queryByLabelText('用户名')).not.toBeInTheDocument()
})

test('ani_open 类型隐藏 Root 字段', async () => {
  renderModal()
  await chooseType('ani_open')
  expect(screen.getByLabelText('Endpoint')).toBeInTheDocument()
  expect(screen.queryByLabelText('Root')).not.toBeInTheDocument()
})

test('123 官方 API 类型显示开发者凭据和挂载目录，不要求 Endpoint 或 Root', async () => {
  renderModal()
  await chooseType('123 云盘官方 API')
  expect(screen.getByLabelText('Client ID')).toBeInTheDocument()
  expect(screen.getByLabelText('Client Secret')).toBeInTheDocument()
  expect(screen.getByLabelText('挂载目录 File ID')).toHaveValue('0')
  expect(screen.queryByLabelText('Endpoint')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Root')).not.toBeInTheDocument()
  expect(screen.getAllByText(/直链空间/).length).toBeGreaterThan(0)
})

test('123 官方 API 保存凭据并保留编辑时的 Secret', async () => {
  const user = userEvent.setup()
  const onSave = renderModal({ index: 0, storages: [pan123] })
  expect(screen.getByLabelText('Client Secret')).toHaveValue('********')
  await user.clear(screen.getByLabelText('Client Secret'))
  await user.click(screen.getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
    id: 'pan123',
    type: '123_open',
    root: '',
    endpoint: '',
    client_id: 'client-id',
    client_secret: '********',
    root_file_id: '777',
  }), 0))
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
  const onSave = renderModal()
  fireEvent.change(screen.getByLabelText('存储 ID'), { target: { value: 'ani' } })
  await chooseType('ani_open')
  fireEvent.change(screen.getByLabelText('Endpoint'), { target: { value: 'https://ani.example' } })
  fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
    id: 'ani',
    type: 'ani-open',
    root: '',
    endpoint: 'https://ani.example',
  }), -1))
})
