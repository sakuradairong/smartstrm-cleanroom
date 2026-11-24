export type TaskErrorKind =
  | 'source-unavailable'
  | 'permission-denied'
  | 'cancelled'
  | 'timeout'
  | 'execution-failed'

export type TaskErrorCopy = {
  readonly kind: TaskErrorKind
  readonly summary: string
  readonly detail: string
}

const CAUSES = [
  {
    kind: 'source-unavailable',
    matches: ['cannot find the file', 'no such file', 'path does not exist', 'not found'],
    summary: '源目录不可用，请检查路径。',
  },
  {
    kind: 'permission-denied',
    matches: ['permission denied', 'access is denied', 'operation not permitted'],
    summary: '无权访问该路径，请检查存储权限。',
  },
  {
    kind: 'cancelled',
    matches: ['context canceled'],
    summary: '任务已取消。',
  },
  {
    kind: 'timeout',
    matches: ['context deadline exceeded', 'timed out', 'timeout'],
    summary: '操作超时，请重试或查看日志。',
  },
] as const

export function describeTaskError(error: string): TaskErrorCopy {
  const lowered = error.toLocaleLowerCase()
  const cause = CAUSES.find((entry) => entry.matches.some((needle) => lowered.includes(needle)))
  return {
    kind: cause?.kind ?? 'execution-failed',
    summary: cause?.summary ?? '任务执行失败，请查看日志。',
    detail: error,
  }
}
