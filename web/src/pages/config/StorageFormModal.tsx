import { useEffect } from 'react'
import { Alert, Form, Input, Modal, Select } from 'antd'
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
	client_id: '',
	client_secret: '',
	root_file_id: '0',
}

const storageTypes = [
	{ value: 'local', label: 'local' },
	{ value: 'webdav', label: 'webdav' },
	{ value: 'openlist', label: 'openlist' },
	{ value: '123_open', label: '123 云盘官方 API' },
	{ value: 'ani_open', label: 'ani_open' },
]

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
		client_secret: previous?.client_secret ? REDACTED : '',
		root_file_id: value.root_file_id || '0',
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
		client_id: uiType === '123_open' ? values.client_id?.trim() ?? '' : '',
		client_secret: uiType === '123_open' ? resolveSecret(values.client_secret ?? '', previous?.client_secret ?? '') : '',
		root_file_id: uiType === '123_open' ? values.root_file_id?.trim() || '0' : '',
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
          <Select options={storageTypes} />
        </Form.Item>
        {type !== 'ani_open' && type !== '123_open' ? (
          <Form.Item label="Root" name="root" rules={[{ required: true, message: '请输入 Root' }]}>
            <Input />
          </Form.Item>
        ) : null}
        {type !== 'local' && type !== '123_open' ? (
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
		{type === '123_open' ? (
			<>
				<Alert
					type="info"
					showIcon
					message="使用 123 云盘开发者开放平台"
					description="下载和播放通常需要 VIP 权益，并在 123 云盘中为挂载目录启用直链空间。根目录 File ID 为 0，但根目录不能直接提供直链。"
					style={{ marginBottom: 16 }}
				/>
				<Form.Item label="Client ID" name="client_id" rules={[{ required: true, whitespace: true, message: '请输入 Client ID' }]}>
					<Input autoComplete="off" />
				</Form.Item>
				<Form.Item
					label="Client Secret"
					name="client_secret"
					required={!previous?.client_secret}
					getValueFromEvent={(value: string) => value}
					rules={[{
						validator: (_, value: string) => resolveSecret(value ?? '', previous?.client_secret ?? '')
							? Promise.resolve()
							: Promise.reject(new Error('请输入 Client Secret')),
					}]}
				>
					<SecretInput />
				</Form.Item>
				<Form.Item
					label="挂载目录 File ID"
					name="root_file_id"
					rules={[
						{ required: true, message: '请输入挂载目录 File ID' },
						{ pattern: /^\d+$/, message: 'File ID 必须是非负整数' },
					]}
					extra="建议填写已启用直链空间的子目录 File ID；根目录填写 0。"
				>
					<Input inputMode="numeric" />
				</Form.Item>
			</>
		) : null}
      </Form>
    </Modal>
  )
}
