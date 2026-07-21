import { Input } from 'antd'

export const REDACTED = '********'

interface SecretInputProps {
  id?: string
  value?: string
  onChange?: (value: string) => void
  placeholder?: string
}

export default function SecretInput({ id, value = '', onChange = () => {}, placeholder }: SecretInputProps) {
  return (
    <Input.Password
      value={value}
      id={id}
      placeholder={placeholder}
      autoComplete="new-password"
      onChange={event => onChange(event.target.value)}
    />
  )
}

export function resolveSecret(input: string, previous: string): string {
  return input === REDACTED || input === '' ? previous : input
}
