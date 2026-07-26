import { useEffect } from 'react'
import { Alert, Collapse, Form, Input, Modal, Select, Typography } from 'antd'
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
  { value: 'local', label: '本机目录（Docker 挂载目录）' },
  { value: '123_open', label: '123 云盘（官方开放平台）' },
  { value: 'webdav', label: 'WebDAV 网络存储' },
  { value: 'openlist', label: 'OpenList / AList' },
  { value: 'ani_open', label: 'Ani API（高级）' },
]

const typeHelp: Record<string, { message: string; description: string }> = {
  local: {
    message: '读取服务器或 Docker 已挂载的目录',
    description: '如果媒体文件已经挂载到 SmartStrm 容器，请填写容器内看到的路径，例如 /media。',
  },
  webdav: {
    message: '连接支持 WebDAV 的网盘或文件服务',
    description: '需要服务地址，部分服务还需要用户名和密码。请填写客户端实际使用的 WebDAV 地址。',
  },
  openlist: {
    message: '通过 OpenList / AList 访问已挂载的网盘',
    description: '需要 OpenList / AList 的服务地址和访问 Token。根目录通常填写 /。',
  },
  ani_open: {
    message: '面向熟悉 Ani API 的高级用户',
    description: '需要 Ani API 服务地址。普通用户请选择其他存储类型。',
  },
}

function nextStorageID(storages: StorageConfig[]): string {
  let index = 1
  while (storages.some(storage => storage.id === `storage-${index}`)) index += 1
  return `storage-${index}`
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
  const help = typeHelp[type]

  useEffect(() => {
    if (!open) return
    const value = previous ?? emptyStorage
    form.setFieldsValue({
      ...value,
      id: previous?.id ?? nextStorageID(storages),
      type: storageTypeUI(value.type),
      password: previous?.password ? REDACTED : '',
      token: previous?.token ? REDACTED : '',
    client_secret: previous?.client_secret ? REDACTED : '',
    root_file_id: value.root_file_id || '0',
    })
  }, [form, open, previous, storages])

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
      okText="加入待应用修改"
      cancelText="取消"
      onOk={() => form.submit()}
      onCancel={onClose}
      destroyOnHidden
    >
      <Alert
        type="info"
        showIcon
        message="先选择媒体文件所在的位置"
        description="这里只建立读取连接，不会移动或删除原有文件。保存后还需要点击页面底部的“应用所有修改”。"
        style={{ marginBottom: 16 }}
      />
      <Form form={form} layout="vertical" onFinish={save}>
        <Form.Item label="存储标识" name="id" extra={index >= 0 ? '创建后不可修改' : '已自动生成，一般无需修改'} rules={[
          { required: true, message: '请输入存储标识' },
          { pattern: /^[A-Za-z0-9][A-Za-z0-9_-]*$/, message: '只能使用字母、数字、短横线和下划线' },
          {
            validator: (_, value: string) => storages.some((item, itemIndex) => itemIndex !== index && item.id === value?.trim())
              ? Promise.reject(new Error('这个存储名称已存在，请换一个'))
              : Promise.resolve(),
          },
        ]}>
          <Input disabled={index >= 0} placeholder="例如 storage-1" />
        </Form.Item>
        <Form.Item label="存储位置" name="type" rules={[{ required: true, message: '请选择存储位置' }]}>
          <Select options={storageTypes} />
        </Form.Item>
        {help ? <Alert type="info" showIcon message={help.message} description={help.description} style={{ marginBottom: 16 }} /> : null}
        {type !== 'ani_open' && type !== '123_open' ? (
          <Form.Item
            label="从哪个目录开始浏览"
            name="root"
            extra={type === 'local' ? '填写 SmartStrm 容器内的路径，不是电脑桌面上看到的路径。' : '填写 / 表示从整个存储的根目录开始。'}
            rules={[{ required: true, message: '请输入起始目录' }]}
          >
            <Input placeholder={type === 'local' ? '/media' : '/'} />
          </Form.Item>
        ) : null}
        {type !== 'local' && type !== '123_open' ? (
          <Form.Item
            label="服务地址"
            name="endpoint"
            extra="包含 http:// 或 https:// 的完整地址。"
            rules={[{ required: true, message: '请输入服务地址' }, { type: 'url', message: '请输入完整地址，例如 https://example.com/dav' }]}
          >
            <Input placeholder="https://example.com" />
          </Form.Item>
        ) : null}
        {type === 'webdav' ? (
          <>
            <Form.Item label="登录用户名（可选）" name="username"><Input autoComplete="off" /></Form.Item>
            <Form.Item label="登录密码（可选）" name="password" extra="只会安全保存到服务端配置中。" getValueFromEvent={(value: string) => value}>
              <SecretInput />
            </Form.Item>
          </>
        ) : null}
        {type === 'openlist' ? (
          <Form.Item label="访问 Token" name="token" extra="请从 OpenList / AList 后台复制访问 Token。" getValueFromEvent={(value: string) => value}>
            <SecretInput />
          </Form.Item>
        ) : null}
    {type === '123_open' ? (
      <>
        <Alert
          type="info"
          showIcon
          message="连接 123 云盘前需要完成两项准备"
          description="先在 123 云盘开放平台创建应用并取得 Client ID 和 Client Secret；再为准备挂载的子目录启用直链空间。下载和播放通常还需要 VIP 权益。"
          style={{ marginBottom: 16 }}
        />
        <Form.Item label="应用 Client ID" name="client_id" extra="在 123 云盘开放平台的应用详情中复制。" rules={[{ required: true, whitespace: true, message: '请输入 Client ID' }]}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item
          label="应用 Client Secret"
          name="client_secret"
          extra="属于敏感信息，请勿分享给他人。"
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
          extra="建议填写已启用直链空间的子目录 File ID。只有浏览整个云盘时才填写 0。"
        >
          <Input inputMode="numeric" />
        </Form.Item>
      </>
    ) : null}
        <Collapse
          ghost
          items={[{
            key: 'help',
            label: '不知道这些信息在哪里？',
            children: (
              <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                本机目录来自 Docker 的目录挂载；WebDAV 地址和账号由网盘服务商提供；OpenList Token 在其后台生成；123 云盘凭据来自开放平台。保存失败时，页面会保留当前填写内容，请按错误提示修正后重试。
              </Typography.Paragraph>
            ),
          }]}
        />
      </Form>
    </Modal>
  )
}
