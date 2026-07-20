import { getJSON } from './client'
import type { Bootstrap, Overview } from './types'

export const getBootstrap = () => getJSON<Bootstrap>('/api/bootstrap')
export const getOverview = () => getJSON<Overview>('/api/overview')
