import { useEffect, useMemo, useRef, useState } from 'react'
import AutoAwesomeOutlinedIcon from '@mui/icons-material/AutoAwesomeOutlined'
import CloseIcon from '@mui/icons-material/Close'

import { fetchVideoSubtitleGeneration, generateVideoSubtitle } from '@/api'
import AppModal from '@/components/AppModal'
import { getErrorMessage } from '@/utils/errors'
import { zh } from '@/utils/i18n'

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

const statusLabels = {
  queued: ['等待执行', 'Queued'],
  running: ['正在生成', 'Generating'],
  completed: ['生成完成', 'Completed'],
  failed: ['生成失败', 'Failed'],
}

const phaseLabels = {
  transcribing: ['正在识别原始语音', 'Transcribing source audio'],
  translating: ['正在翻译为简体中文', 'Translating to Simplified Chinese'],
  installing: ['正在写入中文字幕', 'Installing Chinese subtitles'],
}

export default function SubtitleGenerateModal({ item, onClose, onGenerated }) {
  const targets = useMemo(() => subtitleTargets(item), [item])
  const [targetKey, setTargetKey] = useState('')
  const [model, setModel] = useState('medium')
  const [device, setDevice] = useState('cuda')
  const [language, setLanguage] = useState('ja')
  const [job, setJob] = useState(null)
  const [error, setError] = useState('')
  const reportedRef = useRef(false)

  useEffect(() => {
    setTargetKey(targets[0]?.key || '')
    setModel('medium')
    setDevice('cuda')
    setLanguage('ja')
    setJob(null)
    setError('')
    reportedRef.current = false
  }, [item, targets])

  const selectedTarget = targets.find((target) => target.key === targetKey) || targets[0]
  const active = job?.status === 'queued' || job?.status === 'running'

  useEffect(() => {
    if (!active || !job?.video_id || !job?.location_id) return undefined
    let cancelled = false
    const timer = window.setInterval(async () => {
      try {
        const next = await fetchVideoSubtitleGeneration(job.video_id, job.location_id)
        if (!cancelled) setJob(next)
      } catch (pollError) {
        if (!cancelled) setError(getErrorMessage(pollError))
      }
    }, 1000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [active, job?.location_id, job?.video_id])

  useEffect(() => {
    if (job?.status !== 'completed' || reportedRef.current) return
    reportedRef.current = true
    void onGenerated?.(job, item)
  }, [item, job, onGenerated])

  const handleSubmit = async (event) => {
    event.preventDefault()
    if (!selectedTarget || active) return
    setError('')
    reportedRef.current = false
    try {
      const next = await generateVideoSubtitle(selectedTarget.videoID, {
        locationId: selectedTarget.locationID,
        model,
        device,
        language,
      })
      setJob(next)
    } catch (submitError) {
      setError(getErrorMessage(submitError))
    }
  }

  const progress = Math.max(0, Math.min(100, Number(job?.progress) || 0))
  const statusLabel =
    job?.status === 'running' && phaseLabels[job?.phase]
      ? phaseLabels[job.phase]
      : statusLabels[job?.status]

  return (
    <AppModal
      open={Boolean(item)}
      onClose={onClose}
      ariaLabel={zh('生成字幕', 'Generate subtitles')}
      contentClassName="mx-4 w-full max-w-xl rounded-2xl bg-white p-5 shadow-2xl"
    >
      <form onSubmit={handleSubmit}>
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold text-gray-900">
              {zh('生成字幕', 'Generate subtitles')}
            </h2>
            <p className="mt-1 text-sm text-gray-500">{String(item?.code || '').trim()}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-full p-1.5 text-gray-500 hover:bg-gray-100"
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
                disabled={targets.length === 1 || active}
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

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <label className="block text-sm font-medium text-gray-700">
                {zh('模型', 'Model')}
                <select
                  value={model}
                  onChange={(event) => setModel(event.target.value)}
                  disabled={active}
                  className="mt-1.5 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm"
                >
                  <option value="medium">medium</option>
                  <option value="large-v3">large-v3</option>
                </select>
              </label>
              <label className="block text-sm font-medium text-gray-700">
                {zh('设备', 'Device')}
                <select
                  value={device}
                  onChange={(event) => setDevice(event.target.value)}
                  disabled={active}
                  className="mt-1.5 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm"
                >
                  <option value="cuda">CUDA GPU</option>
                  <option value="cpu">CPU</option>
                </select>
              </label>
              <label className="block text-sm font-medium text-gray-700">
                {zh('源语音语言', 'Source speech language')}
                <select
                  value={language}
                  onChange={(event) => setLanguage(event.target.value)}
                  disabled={active}
                  className="mt-1.5 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm"
                >
                  <option value="ja">{zh('日语', 'Japanese')}</option>
                  <option value="zh">{zh('中文', 'Chinese')}</option>
                  <option value="en">{zh('英语', 'English')}</option>
                  <option value="">{zh('自动检测', 'Auto detect')}</option>
                </select>
              </label>
            </div>

            <p className="rounded-lg bg-blue-50 px-3 py-2 text-xs leading-5 text-blue-800">
              {zh(
                '先识别原始语音，再在线翻译并校验为简体中文。只有 .zh-cn.generated.srt 成功写入并自动导入后任务才会完成。关闭弹窗不会停止生成。',
                'The source audio is transcribed first, then translated and validated as Simplified Chinese. The job completes only after a .zh-cn.generated.srt file is written and imported. Closing this dialog does not stop it.'
              )}
            </p>

            {job ? (
              <div className="rounded-lg border border-gray-200 p-3">
                <div className="flex items-center justify-between text-sm">
                  <span className="font-medium">
                    {statusLabel ? zh(statusLabel[0], statusLabel[1]) : job.status}
                  </span>
                  <span>{progress.toFixed(0)}%</span>
                </div>
                <div className="mt-2 h-2 overflow-hidden rounded-full bg-gray-200">
                  <div
                    className={`h-full transition-all ${job.status === 'failed' ? 'bg-red-500' : 'bg-blue-600'}`}
                    style={{ width: `${progress}%` }}
                  />
                </div>
                {job.subtitle_path || job.subtitle ? (
                  <p
                    className="mt-2 break-all text-xs text-emerald-700"
                    title={job.subtitle_path || job.subtitle}
                  >
                    {job.subtitle_path || job.subtitle}
                  </p>
                ) : null}
                {job.error ? (
                  <p className="mt-2 break-all text-xs text-red-600">{job.error}</p>
                ) : null}
              </div>
            ) : null}
            {error ? <p className="text-sm text-red-600">{error}</p> : null}
          </div>
        ) : (
          <p className="mt-5 rounded-lg bg-gray-50 px-3 py-3 text-sm text-gray-600">
            {zh('这个 JAV 没有关联可用的本地视频。', 'This JAV has no available local video.')}
          </p>
        )}

        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
          >
            {zh('关闭', 'Close')}
          </button>
          <button
            type="submit"
            disabled={!selectedTarget || active}
            className="inline-flex items-center gap-1.5 rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white hover:bg-violet-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <AutoAwesomeOutlinedIcon fontSize="small" />
            {active ? zh('生成中…', 'Generating…') : zh('开始生成', 'Generate')}
          </button>
        </div>
      </form>
    </AppModal>
  )
}
