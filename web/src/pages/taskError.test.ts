import { describeTaskError } from './taskError'

test.each([
  ['list /movies: open D:\\media\\movies: The system cannot find the file specified.', 'source-unavailable'],
  ['read /a: no such file or directory', 'source-unavailable'],
  ['open /a: permission denied', 'permission-denied'],
  ['context canceled', 'cancelled'],
  ['context deadline exceeded', 'timeout'],
  ['扫描失败', 'execution-failed'],
])('把 %s 归类为 %s 并保留原始诊断', (detail, kind) => {
  const copy = describeTaskError(detail)
  expect(copy.kind).toBe(kind)
  expect(copy.detail).toBe(detail)
  expect(copy.summary).not.toBe(copy.detail)
})

test('不同原因的摘要可区分，未知原因使用通用说明', () => {
  const unavailable = describeTaskError('The system cannot find the file specified.')
  const permission = describeTaskError('permission denied')
  const unknown = describeTaskError('unexpected upstream payload')
  expect(new Set([unavailable.summary, permission.summary, unknown.summary]).size).toBe(3)
  expect(unknown.kind).toBe('execution-failed')
})
