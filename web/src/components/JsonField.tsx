import { Input, Typography } from 'antd'

interface JsonFieldProps {
  id?: string
  value: string
  onChange: (value: string) => void
  rows?: number
  schemaHint?: string
}

export function validateJSON(value: string): string | null {
  try {
    JSON.parse(value)
    return null
  } catch (error) {
    const detail = error instanceof Error ? error.message : '无法解析'
    return `JSON 格式错误：${detail}`
  }
}

export default function JsonField({ id, value, onChange, rows = 5, schemaHint }: JsonFieldProps) {
  const error = validateJSON(value)
  const errorID = id ? `${id}-error` : undefined

  return (
    <div>
      <Input.TextArea
        id={id}
        value={value}
        rows={rows}
        spellCheck={false}
        aria-invalid={error ? 'true' : undefined}
        aria-describedby={error ? errorID : undefined}
        onChange={event => onChange(event.target.value)}
      />
      {error ? (
        <Typography.Text id={errorID} type="danger">{error}</Typography.Text>
      ) : schemaHint ? (
        <Typography.Text type="secondary">{schemaHint}</Typography.Text>
      ) : null}
    </div>
  )
}
