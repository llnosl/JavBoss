const LANGUAGE_LABELS = {
  zh: ['中文', 'Chinese'],
  'zh-cn': ['简中', 'Simplified Chinese'],
  'zh-tw': ['繁中', 'Traditional Chinese'],
  ja: ['日文', 'Japanese'],
  en: ['英文', 'English'],
  ko: ['韩文', 'Korean'],
}

export function subtitleLanguageLabels(code) {
  const normalized = String(code || '')
    .trim()
    .toLowerCase()
  if (!normalized || normalized === 'und') return null
  return LANGUAGE_LABELS[normalized] || [normalized.toUpperCase(), normalized.toUpperCase()]
}

export function summarizeVideoSubtitles(video) {
  const items = Array.isArray(video?.subtitles) ? video.subtitles : []
  const languages = []
  const seen = new Set()
  let embeddedCount = 0
  let externalCount = 0
  for (const item of items) {
    if (item?.kind === 'embedded') embeddedCount += 1
    if (item?.kind === 'external') externalCount += 1
    const language = String(item?.language || '')
      .trim()
      .toLowerCase()
    if (!language || language === 'und' || seen.has(language)) continue
    seen.add(language)
    languages.push(language)
  }
  return {
    scanned: Boolean(video?.subtitles_scanned_at),
    hasSubtitles: items.length > 0,
    embeddedCount,
    externalCount,
    languages,
  }
}

export function summarizeJavSubtitles(jav) {
  const videos = Array.isArray(jav?.videos) ? jav.videos : []
  const items = videos.flatMap((video) => (Array.isArray(video?.subtitles) ? video.subtitles : []))
  const languages = []
  const seen = new Set()
  let embeddedCount = 0
  let externalCount = 0
  let scannedVideoCount = 0

  for (const video of videos) {
    if (video?.subtitles_scanned_at) scannedVideoCount += 1
  }
  for (const item of items) {
    if (item?.kind === 'embedded') embeddedCount += 1
    if (item?.kind === 'external') externalCount += 1
    const language = String(item?.language || '')
      .trim()
      .toLowerCase()
    if (!language || language === 'und' || seen.has(language)) continue
    seen.add(language)
    languages.push(language)
  }

  return {
    videoCount: videos.length,
    scannedVideoCount,
    allScanned: videos.length > 0 && scannedVideoCount === videos.length,
    partiallyScanned: scannedVideoCount > 0 && scannedVideoCount < videos.length,
    hasSubtitles: items.length > 0,
    embeddedCount,
    externalCount,
    languages,
  }
}
