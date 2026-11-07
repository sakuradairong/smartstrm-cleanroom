import {
  CustomerServiceOutlined,
  FileImageOutlined,
  FileOutlined,
  FileTextOutlined,
  FolderOutlined,
  VideoCameraOutlined,
} from '@ant-design/icons'
import type { StorageEntry } from '../api/types'

// 与旧 internal/app/ui/index.html 的 storageFileIcon 完全一致。
const VIDEO = ['mkv', 'mp4', 'avi', 'mov', 'wmv', 'flv', 'ts', 'm2ts', 'iso']
const IMAGE = ['jpg', 'jpeg', 'png', 'webp', 'gif', 'bmp']
const AUDIO = ['mp3', 'flac', 'wav', 'aac', 'm4a', 'ogg']
const SUBTITLE = ['srt', 'ass', 'ssa', 'vtt', 'sub']

export function entryKind(entry: StorageEntry): 'dir' | 'video' | 'image' | 'audio' | 'subtitle' | 'file' {
  if (entry.is_dir) return 'dir'
  const ext = entry.name.split('.').pop()?.toLowerCase() ?? ''
  if (VIDEO.includes(ext)) return 'video'
  if (IMAGE.includes(ext)) return 'image'
  if (AUDIO.includes(ext)) return 'audio'
  if (SUBTITLE.includes(ext)) return 'subtitle'
  return 'file'
}

export function isMediaEntry(entry: StorageEntry): boolean {
  return entryKind(entry) === 'video'
}

const ICONS = {
  dir: FolderOutlined,
  video: VideoCameraOutlined,
  image: FileImageOutlined,
  audio: CustomerServiceOutlined,
  subtitle: FileTextOutlined,
  file: FileOutlined,
} as const

export default function EntryIcon({ entry }: { entry: StorageEntry }) {
  const Icon = ICONS[entryKind(entry)]
  return <Icon aria-hidden="true" />
}
