import assert from 'node:assert/strict'
import test from 'node:test'

import { collectExternalSubtitlePaths, joinMediaPath } from '../src/utils/subtitlePaths.js'

test('joinMediaPath keeps the media root path style', () => {
  assert.equal(
    joinMediaPath('F:\\media', 'folder/video.zh-cn.srt'),
    'F:\\media\\folder\\video.zh-cn.srt'
  )
  assert.equal(joinMediaPath('/media', 'folder\\video.zh-cn.srt'), '/media/folder/video.zh-cn.srt')
})

test('collectExternalSubtitlePaths lists editable external tracks without embedded tracks', () => {
  const entries = collectExternalSubtitlePaths([
    {
      id: 9,
      location_id: 12,
      filename: 'ABC-001.mp4',
      directory: { path: 'F:\\media' },
      subtitles: [
        { id: 20, kind: 'embedded', stream_index: 2 },
        {
          id: 21,
          kind: 'external',
          language: 'zh-cn',
          relative_path: 'ABC-001.zh-cn.manual.srt',
        },
      ],
    },
  ])

  assert.deepEqual(entries, [
    {
      key: '9:12:21',
      videoID: 9,
      locationID: 12,
      subtitleID: 21,
      videoLabel: 'ABC-001.mp4',
      language: 'zh-cn',
      relativePath: 'ABC-001.zh-cn.manual.srt',
      absolutePath: 'F:\\media\\ABC-001.zh-cn.manual.srt',
    },
  ])
})
