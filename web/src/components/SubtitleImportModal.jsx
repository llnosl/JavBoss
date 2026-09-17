import { useEffect, useMemo, useState } from 'react'
import CloseIcon from '@mui/icons-material/Close'
import UploadFileOutlinedIcon from '@mui/icons-material/UploadFileOutlined'

import { importVideoSubtitle } from '@/api'
import AppModal from '@/components/AppModal'
import { getErrorMessage } from '@/utils/errors'
import { zh } from '@/utils/i18n'

const languageOptions = [
  { value: 'zh-cn', zh: '简体中文', en: 'Simplified Chinese' },
  { value: 'zh-tw', zh: '繁体中文', en: 'Traditional Chinese' },
  { value: 'zh', zh: '中文（未区分）', en: 'Chinese (unspecified)' },
  { value: 'ja', zh: '日语', en: 'Japanese' },
  { value: 'en', zh: '英语', en: 'English' },
  { value: '', zh: '未知语言', en: 'Unknown language' },
]

function subtitleTargets(item) {
  const seen = new Set()
  const targets = []
  for (const video of Array.isArray(item?.videos) ? item.videos : []) {
    const locations =
      Array.isArray(video?.locations) && video.locations.length
        ? video.locations
        : [
            {
              id: video?.location_id,
              filename: video?.filename,
              relative_path: video?.path,
              directory: video?.directory,
            },
          ]
    for (const location of locations) {
      const videoID = Number(video?.id)
      const locationID = Number(location?.id || video?.location_id)
      if (!videoID || !locationID) continue
      const key = `${videoID}:${locationID}`
      if (seen.has(key)) continue
      seen.add(key)
      targets.push({
        key,
        videoID,
        locationID,
        filename: String(
          location?.filename || video?.filename || location?.relative_path || video?.path || key
        ),
        directory: String(location?.directory?.path || video?.directory?.path || ''),
      })
    }
  }
  return targets
}

export default function SubtitleImportModal({ item, onClose, onImported }) {
  const targets = useMemo(() => subtitleTargets(item), [item])
  const [targetKey, setTargetKey] = useState('')
  const [language, setLanguage] = useState('zh-cn')
  const [file, setFile] = useState(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    setTargetKey(targets[0]?.key || '')
    setLanguage('zh-cn')
    setFile(null)
    setBusy(false)
    setError('')
  }, [item, targets])

  const selectedTarget = targets.find((target) => target.key === targetKey) || targets[0]

  const handleSubmit = async (event) => {
    event.preventDefault()
    if (!selectedTarget || !file || busy) return
    setBusy(true)
    setError('')
    try {
      const result = await importVideoSubtitle(selectedTarget.videoID, {
        locationId: selectedTarget.locationID,
        language,
        file,
      })
      await onImported?.(result, item)
      onClose?.()
    } catch (submitError) {
      setError(getErrorMessage(submitError))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AppModal
      open={Boolean(item)}
      onClose={busy ? undefined : onClose}
      closeDisabled={busy}
      ariaLabel={zh('导入字幕', 'Import subtitles')}
      contentClassName="mx-4 w-full max-w-lg rounded-2xl bg-white p-5 shadow-2xl"
    >
      <form onSubmit={handleSubmit}>
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              {zh('导入字幕', 'Import subtitles')}
            </h2>
            <p className="mt-1 text-sm text-gray-500">{String(item?.code || '').trim()}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="rounded-full p-1.5 text-gray-500 hover:bg-gray-100 disabled:opacity-50"
            aria-label={zh('关闭', 'Close')}
          >
            <CloseIcon fontSize="small" />
          </button>
        </div>

        {targets.length ? (
          <div className="mt-5 space-y-4">
            <label className="block text-sm font-medium text-gray-700">
              {zh('目标视频', 'Target video')}
              <select
                value={targetKey}
                onChange={(event) => setTargetKey(event.target.value)}
                disabled={targets.length === 1 || busy}
                className="mt-1.5 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-800 outline-none focus:border-blue-500"
              >
                {targets.map((target) => (
                  <option key={target.key} value={target.key}>
                    {target.filename}
                    {target.directory ? ` — ${target.directory}` : ''}
                  </option>
                ))}
              </select>
            </label>

            <label className="block text-sm font-medium text-gray-700">
              {zh('字幕语言', 'Subtitle language')}
              <select
                value={language}
                onChange={(event) => setLanguage(event.target.value)}
                disabled={busy}
                className="mt-1.5 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-800 outline-none focus:border-blue-500"
              >
                {languageOptions.map((option) => (
                  <option key={option.value || 'unknown'} value={option.value}>
                    {zh(option.zh, option.en)}
                  </option>
                ))}
              </select>
            </label>

            <label className="block text-sm font-medium text-gray-700">
              {zh('字幕文件', 'Subtitle file')}
              <input
                type="file"
                accept=".srt,.ass,.ssa,.vtt"
                onChange={(event) => setFile(event.target.files?.[0] || null)}
                disabled={busy}
                className="mt-1.5 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-700 file:mr-3 file:rounded-md file:border-0 file:bg-blue-50 file:px-3 file:py-1.5 file:text-blue-700"
              />
            </label>

            <p className="rounded-lg bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
              {zh(
                '支持 SRT、ASS、SSA、VTT，最大 10 MB。字幕会复制到视频旁边；再次导入相同语言和格式会覆盖上次手动导入的文件。',
                'Supports SRT, ASS, SSA, and VTT up to 10 MB. The subtitle is copied beside the video; importing the same language and format again replaces the previous manual import.'
              )}
            </p>
            {error ? <p className="text-sm text-red-600">{error}</p> : null}
          </div>
        ) : (
          <p className="mt-5 rounded-lg bg-gray-50 px-3 py-3 text-sm text-gray-600">
            {zh(
              '这个 JAV 没有关联可用的本地视频，无法导入字幕。',
              'This JAV has no available local video for subtitle import.'
            )}
          </p>
        )}

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            {zh('取消', 'Cancel')}
          </button>
          <button
            type="submit"
            disabled={!selectedTarget || !file || busy}
            className="inline-flex items-center gap-1.5 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <UploadFileOutlinedIcon fontSize="small" />
            {busy ? zh('正在导入…', 'Importing…') : zh('导入', 'Import')}
          </button>
        </div>
      </form>
    </AppModal>
  )
}
