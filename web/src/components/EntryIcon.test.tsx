import { render } from '@testing-library/react'
import EntryIcon, { entryKind, isMediaEntry } from './EntryIcon'
import type { StorageEntry } from '../api/types'

function makeEntry(overrides: Partial<StorageEntry> = {}): StorageEntry {
  return { path: '/x', name: 'x', size: 0, is_dir: false, modified: '', ...overrides }
}

function iconOf(entry: StorageEntry): string {
  const { container } = render(<EntryIcon entry={entry} />)
  return container.querySelector('svg')?.getAttribute('data-icon') ?? ''
}

test('目录显示文件夹图标', () => {
  const entry = makeEntry({ is_dir: true, name: 'movies' })
  expect(iconOf(entry)).toBe('folder')
  expect(entryKind(entry)).toBe('dir')
  expect(isMediaEntry(entry)).toBe(false)
})

test('视频扩展名 mkv/mp4/iso 显示影片图标且 isMediaEntry 为 true', () => {
  for (const name of ['a.mkv', 'b.mp4', 'c.iso', 'd.MKV', 'e.m2ts', 'f.ts', 'g.avi', 'h.mov', 'i.wmv', 'j.flv']) {
    const entry = makeEntry({ name })
    expect(iconOf(entry)).toBe('video-camera')
    expect(entryKind(entry)).toBe('video')
    expect(isMediaEntry(entry)).toBe(true)
  }
})

test('图片扩展名 jpg 显示图片图标且 isMediaEntry 为 false', () => {
  const entry = makeEntry({ name: 'poster.jpg' })
  expect(iconOf(entry)).toBe('file-image')
  expect(entryKind(entry)).toBe('image')
  expect(isMediaEntry(entry)).toBe(false)
})

test('字幕扩展名 srt 显示字幕图标', () => {
  const entry = makeEntry({ name: 'movie.srt' })
  expect(iconOf(entry)).toBe('file-text')
  expect(entryKind(entry)).toBe('subtitle')
  expect(isMediaEntry(entry)).toBe(false)
})

test('音频扩展名 mp3 显示音频图标', () => {
  const entry = makeEntry({ name: 'song.mp3' })
  expect(iconOf(entry)).toBe('customer-service')
  expect(entryKind(entry)).toBe('audio')
  expect(isMediaEntry(entry)).toBe(false)
})

test('未知扩展名显示通用文件图标', () => {
  const entry = makeEntry({ name: 'notes.txt' })
  expect(iconOf(entry)).toBe('file')
  expect(entryKind(entry)).toBe('file')
  expect(isMediaEntry(entry)).toBe(false)
})

test('无点的整名（如 "mkv"）按 split(".").pop() 语义参与匹配', () => {
  const video = makeEntry({ name: 'mkv' })
  expect(entryKind(video)).toBe('video')
  expect(iconOf(video)).toBe('video-camera')
  const plain = makeEntry({ name: 'README' })
  expect(entryKind(plain)).toBe('file')
  expect(iconOf(plain)).toBe('file')
})
