import assert from 'node:assert/strict'
import test from 'node:test'

import { subtitleLanguageLabels, summarizeVideoSubtitles } from '../src/utils/subtitles.js'

test('summarizeVideoSubtitles reports status, kinds, and unique languages', () => {
  const summary = summarizeVideoSubtitles({
    subtitles_scanned_at: '2026-09-15T00:00:00Z',
    subtitles: [
      { kind: 'embedded', language: 'zh-cn' },
      { kind: 'external', language: 'zh-cn' },
      { kind: 'external', language: 'en' },
    ],
  })

  assert.equal(summary.scanned, true)
  assert.equal(summary.hasSubtitles, true)
  assert.equal(summary.embeddedCount, 1)
  assert.equal(summary.externalCount, 2)
  assert.deepEqual(summary.languages, ['zh-cn', 'en'])
})

test('summarizeVideoSubtitles distinguishes scanned empty videos from pending ones', () => {
  assert.deepEqual(summarizeVideoSubtitles({ subtitles: [] }), {
    scanned: false,
    hasSubtitles: false,
    embeddedCount: 0,
    externalCount: 0,
    languages: [],
  })
  assert.equal(
    summarizeVideoSubtitles({ subtitles_scanned_at: '2026-09-15T00:00:00Z', subtitles: [] })
      .scanned,
    true
  )
})

test('subtitleLanguageLabels translates known codes and preserves unknown codes', () => {
  assert.deepEqual(subtitleLanguageLabels('zh-cn'), ['简中', 'Simplified Chinese'])
  assert.deepEqual(subtitleLanguageLabels('FRA'), ['FRA', 'FRA'])
  assert.equal(subtitleLanguageLabels('und'), null)
})
