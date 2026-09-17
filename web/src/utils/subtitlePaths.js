function joinMediaPath(root, relativePath) {
  const base = String(root || '').trim()
  const relative = String(relativePath || '').trim()
  if (!base) return relative
  if (!relative) return base
  const separator = base.includes('\\') ? '\\' : '/'
  const normalizedRelative = relative.replace(/[\\/]+/g, separator).replace(/^[\\/]+/, '')
  return `${base.replace(/[\\/]+$/, '')}${separator}${normalizedRelative}`
}

export function collectExternalSubtitlePaths(videos) {
  const entries = []
  const seen = new Set()
  for (const video of Array.isArray(videos) ? videos : []) {
    const videoID = Number(video?.id)
    if (!videoID) continue
    const locations =
      Array.isArray(video?.locations) && video.locations.length
        ? video.locations
        : [
            {
              id: video?.location_id,
              filename: video?.filename,
              relative_path: video?.path,
              directory: video?.directory,
              subtitles: video?.subtitles,
            },
          ]
    for (const location of locations) {
      const locationID = Number(location?.id || video?.location_id)
      if (!locationID) continue
      const subtitles = Array.isArray(location?.subtitles)
        ? location.subtitles
        : Number(video?.location_id) === locationID && Array.isArray(video?.subtitles)
          ? video.subtitles
          : []
      const directory = String(
        location?.directory?.path || video?.directory?.path || video?.directory_path || ''
      ).trim()
      const videoLabel = String(
        location?.filename || video?.filename || location?.relative_path || video?.path || videoID
      ).trim()
      for (const subtitle of subtitles) {
        if (subtitle?.kind !== 'external') continue
        const subtitleID = Number(subtitle?.id)
        const relativePath = String(subtitle?.relative_path || '').trim()
        if (!subtitleID || !relativePath) continue
        const key = `${videoID}:${locationID}:${subtitleID}`
        if (seen.has(key)) continue
        seen.add(key)
        entries.push({
          key,
          videoID,
          locationID,
          subtitleID,
          videoLabel,
          language: String(subtitle?.language || '').trim(),
          relativePath,
          absolutePath: joinMediaPath(directory, relativePath),
        })
      }
    }
  }
  return entries
}

export { joinMediaPath }
