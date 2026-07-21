import { useEffect, useState } from 'react'
import {
  App,
  Checkbox,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
} from 'antd'
import type { PluginConfig, StorageConfig, TaskConfig } from '../../api/types'

export interface TaskFormModalProps {
  open: boolean
  index: number
  tasks: TaskConfig[]
  storages: StorageConfig[]
  onSave: (task: TaskConfig, index: number) => void
  onClose: () => void
}

interface TaskFields {
  id: string
  name: string
  storage_id: string
  source: string
  destination: string
  schedule: string
  media_ext: string
  copy_ext: string
  min_size: number
  max_size: number
  keep_local: string
  incremental: boolean
  sync_delete: boolean
  watch: boolean
  dir_time_check: boolean
  file_id_mode: boolean
  plugins: string
}

export function csv(value: string): string[] {
  return String(value || '').split(',').map(item => item.trim()).filter(Boolean)
}

export function csvText(value: string[] | undefined): string {
  return (value ?? []).join(', ')
}

const newTask = (storageID: string): TaskFields => ({
  id: '',
  name: '',
  storage_id: storageID,
  source: '/',
  destination: '/strm/',
  schedule: '',
  media_ext: '.mkv, .mp4, .iso',
  copy_ext: '.jpg, .png, .nfo, .srt',
  min_size: 0,
  max_size: 0,
  keep_local: '',
  incremental: true,
  sync_delete: false,
  watch: false,
  dir_time_check: false,
  file_id_mode: false,
  plugins: '[]',
})

function fieldsFromTask(task: TaskConfig): TaskFields {
  return {
    ...task,
    media_ext: csvText(task.media_ext),
    copy_ext: csvText(task.copy_ext),
    keep_local: csvText(task.keep_local),
    plugins: JSON.stringify(task.plugins ?? [], null, 2),
  }
}

export default function TaskFormModal({ open, index, tasks, storages, onSave, onClose }: TaskFormModalProps) {
  const { modal } = App.useApp()
  const [form] = Form.useForm<TaskFields>()
  const [pluginError, setPluginError] = useState('')
  const previous = index >= 0 ? tasks[index] : undefined

  useEffect(() => {
    if (!open) return
    if (!storages.length) {
      modal.warning({ title: '无法添加任务', content: '请先添加存储' })
      onClose()
      return
    }
    form.setFieldsValue(previous ? fieldsFromTask(previous) : newTask(storages[0].id))
    setPluginError('')
  }, [form, modal, onClose, open, previous, storages])

  const confirmSyncDelete = (checked: boolean) => {
    if (!checked) {
      form.setFieldValue('sync_delete', false)
      return
    }
    form.setFieldValue('sync_delete', false)
    modal.confirm({
      title: '确认启用同步删除？',
      content: '同步删除可能永久删除目标文件，包括目标目录中源端已不存在的文件。确认在该任务中启用？',
      okText: '确认启用',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: () => form.setFieldValue('sync_delete', true),
    })
  }

  const save = (values: TaskFields) => {
    let plugins: PluginConfig[]
    try {
      const parsed: unknown = JSON.parse(values.plugins || '[]')
      if (!Array.isArray(parsed)) throw new Error('必须是 JSON 数组')
      plugins = parsed as PluginConfig[]
      setPluginError('')
    } catch (error) {
      const detail = error instanceof Error ? error.message : '格式错误'
      setPluginError(`插件 JSON 不是合法 JSON：${detail}`)
      return
    }

    onSave({
      id: values.id.trim(),
      name: values.name.trim(),
      storage_id: values.storage_id,
      source: values.source.trim(),
      destination: values.destination.trim(),
      schedule: values.schedule.trim(),
      media_ext: csv(values.media_ext),
      copy_ext: csv(values.copy_ext),
      min_size: Number(values.min_size || 0),
      max_size: Number(values.max_size || 0),
      keep_local: csv(values.keep_local),
      incremental: Boolean(values.incremental),
      sync_delete: Boolean(values.sync_delete),
      watch: Boolean(values.watch),
      dir_time_check: Boolean(values.dir_time_check),
      file_id_mode: Boolean(values.file_id_mode),
      plugins,
    }, index)
  }

  const pathRule = { pattern: /^\//, message: '路径必须以 / 开头' }
  const scheduleValidator = (_: unknown, value: string) => {
    if (!value?.trim() || value.trim().split(/\s+/).length === 5) return Promise.resolve()
    return Promise.reject(new Error('调度必须是五段 cron 表达式，留空表示手动运行'))
  }

  return (
    <Modal
      title={index >= 0 ? '编辑任务' : '添加任务'}
      open={open && storages.length > 0}
      width={760}
      okText="保存"
      cancelText="取消"
      onOk={() => form.submit()}
      onCancel={onClose}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" onFinish={save}>
        <Space align="start" wrap style={{ width: '100%' }}>
          <Form.Item label="任务 ID" name="id" rules={[
            { required: true, message: '请输入任务 ID' },
            {
              validator: (_, value: string) => tasks.some((task, taskIndex) => taskIndex !== index && task.id === value?.trim())
                ? Promise.reject(new Error('任务 ID 已存在'))
                : Promise.resolve(),
            },
          ]}>
            <Input disabled={index >= 0} />
          </Form.Item>
          <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}><Input /></Form.Item>
          <Form.Item label="存储" name="storage_id" rules={[{ required: true, message: '请选择存储' }]}>
            <Select style={{ minWidth: 180 }} options={storages.map(storage => ({ value: storage.id, label: `${storage.id} (${storage.type})` }))} />
          </Form.Item>
        </Space>
        <Form.Item label="源路径" name="source" rules={[{ required: true, message: '请输入源路径' }, pathRule]}><Input /></Form.Item>
        <Form.Item label="目标路径" name="destination" rules={[{ required: true, message: '请输入目标路径' }, pathRule]}><Input /></Form.Item>
        <Form.Item label="调度" name="schedule" rules={[{ validator: scheduleValidator }]} extra="五段 cron 表达式；留空表示手动运行"><Input /></Form.Item>
        <Form.Item label="媒体扩展名" name="media_ext"><Input placeholder=".mkv, .mp4, .iso" /></Form.Item>
        <Form.Item label="复制扩展名" name="copy_ext"><Input placeholder=".jpg, .png, .nfo, .srt" /></Form.Item>
        <Space align="start" wrap>
          <Form.Item label="最小大小" name="min_size"><InputNumber min={0} /></Form.Item>
          <Form.Item label="最大大小" name="max_size"><InputNumber min={0} /></Form.Item>
        </Space>
        <Form.Item label="保留本地" name="keep_local"><Input placeholder="以逗号分隔" /></Form.Item>
        <Space wrap>
          <Form.Item name="incremental" valuePropName="checked"><Checkbox>增量扫描</Checkbox></Form.Item>
          <Form.Item name="sync_delete" valuePropName="checked">
            <Checkbox onChange={event => confirmSyncDelete(event.target.checked)}>同步删除</Checkbox>
          </Form.Item>
          <Form.Item name="watch" valuePropName="checked"><Checkbox>监听变更</Checkbox></Form.Item>
          <Form.Item name="dir_time_check" valuePropName="checked"><Checkbox>目录时间检查</Checkbox></Form.Item>
          <Form.Item name="file_id_mode" valuePropName="checked"><Checkbox>文件 ID 模式</Checkbox></Form.Item>
        </Space>
        <Form.Item
          label="插件 JSON"
          name="plugins"
          validateStatus={pluginError ? 'error' : undefined}
          help={pluginError || undefined}
        >
          <Input.TextArea rows={5} onChange={() => setPluginError('')} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
