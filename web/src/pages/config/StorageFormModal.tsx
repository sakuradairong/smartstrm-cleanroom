import { useEffect } from 'react'
import { Form, Input, Modal, Select } from 'antd'
import type { StorageConfig } from '../../api/types'
import SecretInput, { REDACTED, resolveSecret } from '../../components/SecretInput'

export interface StorageFormModalProps {
  open: boolean
  index: number
  storages: StorageConfig[]
  onSave: (storage: StorageConfig, index: number) => void
  onClose: () => void
}

type StorageFields = StorageConfig

const emptyStorage: StorageFields = {
  id: '',
  type: 'local',
  root: '/media',
  endpoint: '',
  username: '',
  password: '',
  token: '',
}

export function storageTypeBackend(type: string): string {
  return type === 'ani_open' ? 'ani-open' : type
}

export function storageTypeUI(type: string): string {
  return type === 'ani-open' ? 'ani_open' : type
}

export default function StorageFormModal({ open, index, storages, onSave, onClose }: StorageFormModalProps) {
  const [form] = Form.useForm<StorageFields>()
  const type = Form.useWatch('type', form) ?? 'local'
  const previous = index >= 0 ? storages[index] : undefined

  useEffect(() => {
    if (!open) return
    const value = previous ?? emptyStorage
    form.setFieldsValue({
      ...value,
      type: storageTypeUI(value.type),
      password: previous?.password ? REDACTED : '',
      token: previous?.token ? REDACTED : '',
    })
  }, [form, open, previous])

  const save = (values: StorageFields) => {
    const uiType = storageTypeUI(values.type)
    const item: StorageConfig = {
      id: values.id.trim(),
      type: storageTypeBackend(uiType),
      root: uiType === 'ani_open' ? '' : values.root?.trim() ?? '',
      endpoint: uiType === 'local' ? '' : values.endpoint?.trim() ?? '',
      username: uiType === 'webdav' ? values.username ?? '' : '',
      password: uiType === 'webdav' ? resolveSecret(values.password ?? '', previous?.password ?? '') : '',
      token: uiType === 'openlist' ? resolveSecret(values.token ?? '', previous?.token ?? '') : '',
    }
    onSave(item, index)
  }

  return (
    <Modal
      title={index >= 0 ? '编辑存储' : '添加存储'}
      open={open}
      okText="保存"
      cancelText="取消"
      onOk={() => form.submit()}
      onCancel={onClose}
      destroyOnHidden
    >
      <Form form={form} layout="vertical" onFinish={save}>
        <Form.Item label="存储 ID" name="id" rules={[
          { required: true, message: '请输入存储 ID' },
          {
            validator: (_, value: string) => storages.some((item, itemIndex) => itemIndex !== index && item.id === value?.trim())
              ? Promise.reject(new Error('存储 ID 已存在'))
              : Promise.resolve(),
          },
        ]}>
          <Input disabled={index >= 0} />
        </Form.Item>
        <Form.Item label="类型" name="type" rules={[{ required: true }]}>
          <Select options={['local', 'webdav', 'openlist', 'ani_open'].map(value => ({ value, label: value }))} />
        </Form.Item>
        {type !== 'ani_open' ? (
          <Form.Item label="Root" name="root" rules={[{ required: true, message: '请输入 Root' }]}>
            <Input />
          </Form.Item>
        ) : null}
        {type !== 'local' ? (
          <Form.Item label="Endpoint" name="endpoint" rules={[{ required: true, message: '请输入 Endpoint' }]}>
            <Input />
          </Form.Item>
        ) : null}
        {type === 'webdav' ? (
          <>
            <Form.Item label="用户名" name="username"><Input autoComplete="off" /></Form.Item>
            <Form.Item label="密码" name="password" getValueFromEvent={(value: string) => value}>
              <SecretInput />
            </Form.Item>
          </>
        ) : null}
        {type === 'openlist' ? (
          <Form.Item label="Token" name="token" getValueFromEvent={(value: string) => value}>
            <SecretInput />
          </Form.Item>
        ) : null}
      </Form>
    </Modal>
  )
}
