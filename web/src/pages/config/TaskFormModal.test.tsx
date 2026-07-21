import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { App as AntApp } from 'antd'
import TaskFormModal, { csv, csvText } from './TaskFormModal'
import type { StorageConfig, TaskConfig } from '../../api/types'

const storage: StorageConfig = { id: 'local', type: 'local', root: '/media', endpoint: '', username: '', password: '', token: '' }
const task: TaskConfig = {
  id: 'movies', name: '电影', storage_id: 'local', source: '/movies', destination: '/strm/movies', schedule: '0 1 * * *',
  media_ext: ['.mkv', '.mp4'], copy_ext: ['.nfo'], min_size: 1, max_size: 99, keep_local: ['poster.jpg'],
  incremental: true, sync_delete: false, watch: true, dir_time_check: false, file_id_mode: true, plugins: [],
}

function renderModal(props: Partial<React.ComponentProps<typeof TaskFormModal>> = {}) {
  const onSave = vi.fn()
  render(<AntApp><TaskFormModal open index={-1} tasks={[]} storages={[storage]} onSave={onSave} onClose={() => {}} {...props} /></AntApp>)
  return onSave
}

test('渲染全部任务字段并回填已有任务', () => {
  renderModal({ index: 0, tasks: [task] })
  for (const label of ['任务 ID', '名称', '存储', '源路径', '目标路径', '调度', '媒体扩展名', '复制扩展名', '最小大小', '最大大小', '保留本地', '增量扫描', '同步删除', '监听变更', '目录时间检查', '文件 ID 模式', '插件 JSON']) {
    expect(screen.getByLabelText(label)).toBeInTheDocument()
  }
  expect(screen.getByLabelText('任务 ID')).toHaveValue('movies')
  expect(screen.getByLabelText('媒体扩展名')).toHaveValue('.mkv, .mp4')
  expect(screen.getByLabelText('监听变更')).toBeChecked()
})

test('扩展名 csv 双向转换', () => {
  expect(csv('.mkv, .mp4')).toEqual(['.mkv', '.mp4'])
  expect(csv('')).toEqual([])
  expect(csvText(['.mkv', '.mp4'])).toBe('.mkv, .mp4')
})

test('勾选 sync_delete 出现二次确认，取消后回滚', async () => {
  const user = userEvent.setup()
  renderModal()
  await user.click(screen.getByLabelText('同步删除'))
  const warning = (await screen.findByText(/同步删除可能永久删除目标文件/)).closest<HTMLElement>('[role="dialog"]')!
  await user.click(within(warning).getByRole('button', { name: /取\s*消/ }))
  expect(screen.getByLabelText('同步删除')).not.toBeChecked()
})

test('确认后启用 sync_delete', async () => {
  const user = userEvent.setup()
  renderModal()
  await user.click(screen.getByLabelText('同步删除'))
  const warning = (await screen.findByText(/同步删除可能永久删除目标文件/)).closest<HTMLElement>('[role="dialog"]')!
  await user.click(within(warning).getByRole('button', { name: /确认启用/ }))
  await waitFor(() => expect(screen.getByLabelText('同步删除')).toBeChecked())
})

test('插件 JSON 非法时阻止保存并提示', async () => {
  const user = userEvent.setup()
  const onSave = renderModal({ index: 0, tasks: [task] })
  const plugins = screen.getByLabelText('插件 JSON')
  fireEvent.change(plugins, { target: { value: '{bad' } })
  await user.click(screen.getByRole('button', { name: /保\s*存/ }))
  expect(await screen.findByText(/插件 JSON 不是合法 JSON/)).toBeInTheDocument()
  expect(onSave).not.toHaveBeenCalled()
})

test('编辑已有任务时 ID 不可改', () => {
  renderModal({ index: 0, tasks: [task] })
  expect(screen.getByLabelText('任务 ID')).toBeDisabled()
})

test('存储为空列表时提示先添加存储并关闭', async () => {
  const onClose = vi.fn()
  render(<AntApp><TaskFormModal open index={-1} tasks={[]} storages={[]} onSave={() => {}} onClose={onClose} /></AntApp>)
  expect(await screen.findByText('请先添加存储')).toBeInTheDocument()
  expect(onClose).toHaveBeenCalled()
  expect(screen.queryByText('添加任务')).not.toBeInTheDocument()
})

test('合法表单保存为 TaskConfig', async () => {
  const user = userEvent.setup()
  const onSave = renderModal({ index: 0, tasks: [task] })
  await user.click(screen.getByRole('button', { name: /保\s*存/ }))
  await waitFor(() => expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ id: 'movies', media_ext: ['.mkv', '.mp4'], plugins: [] }), 0))
})
