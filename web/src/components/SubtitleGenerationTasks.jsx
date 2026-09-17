import { useEffect, useMemo, useState } from 'react'
import PauseCircleOutlineIcon from '@mui/icons-material/PauseCircleOutline'
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline'
import SubtitlesOutlinedIcon from '@mui/icons-material/SubtitlesOutlined'

import AppModal from '@/components/AppModal'
import { useSubtitleGenerations } from '@/subtitleGeneration'
import { getErrorMessage } from '@/utils/errors'
import { zh } from '@/utils/i18n'

const statusLabels = {
  queued: ['排队中', 'Queued'],
  running: ['生成中', 'Generating'],
  paused: ['已暂停', 'Paused'],
  completed: ['已完成', 'Completed'],
  failed: ['失败', 'Failed'],
}

const statusClasses = {
  queued: 'bg-amber-100 text-amber-800',
  running: 'bg-blue-100 text-blue-800',
  paused: 'bg-violet-100 text-violet-800',
  completed: 'bg-emerald-100 text-emerald-800',
  failed: 'bg-red-100 text-red-700',
}

const phaseLabels = {
  transcribing: ['识别语音', 'Transcribing'],
  translating: ['翻译简中', 'Translating to Chinese'],
  installing: ['导入简中', 'Installing Chinese'],
}

function formatJobTime(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString()
}

export function subtitleGenerationLabel(job) {
  if (!job) return ''
  const progress = Math.max(0, Math.min(100, Number(job.progress) || 0))
  switch (job.status) {
    case 'queued':
      return zh('字幕排队中', 'Subtitle queued')
    case 'running':
      return phaseLabels[job.phase]
        ? zh(
            `${phaseLabels[job.phase][0]} ${progress.toFixed(0)}%`,
            `${phaseLabels[job.phase][1]} ${progress.toFixed(0)}%`
          )
        : zh(`字幕 ${progress.toFixed(0)}%`, `Subtitle ${progress.toFixed(0)}%`)
    case 'paused':
      return zh(`字幕已暂停 ${progress.toFixed(0)}%`, `Subtitle paused ${progress.toFixed(0)}%`)
    case 'completed':
      return zh('字幕已生成', 'Subtitle generated')
    case 'failed':
      return zh('字幕生成失败', 'Subtitle failed')
    default:
      return String(job.status || '')
  }
}

export function subtitleGenerationClass(job) {
  return statusClasses[job?.status] || 'bg-gray-100 text-gray-700'
}

export default function SubtitleGenerationTasks() {
  const { jobs, error: loadError, loading, refresh, pause, resume } = useSubtitleGenerations()
  const [open, setOpen] = useState(false)
  const [actionKey, setActionKey] = useState('')
  const [actionError, setActionError] = useState('')
  const activeCount = useMemo(
    () => jobs.filter((job) => ['queued', 'running', 'paused'].includes(job.status)).length,
    [jobs]
  )

  useEffect(() => {
    if (open) void refresh({ silent: true })
  }, [open, refresh])

  const handleAction = async (job, action) => {
    const key = `${job.video_id}:${job.location_id}`
    setActionKey(key)
    setActionError('')
    try {
      if (action === 'pause') await pause(job)
      else await resume(job)
    } catch (err) {
      setActionError(getErrorMessage(err))
    } finally {
      setActionKey('')
    }
  }

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="fixed bottom-5 right-5 z-40 inline-flex items-center gap-2 rounded-full bg-violet-600 px-4 py-2.5 text-sm font-medium text-white shadow-lg hover:bg-violet-700"
      >
        <SubtitlesOutlinedIcon fontSize="small" />
        <span>{zh('字幕任务', 'Subtitle jobs')}</span>
        {activeCount > 0 ? (
          <span className="inline-flex min-w-5 items-center justify-center rounded-full bg-white px-1.5 text-xs font-semibold text-violet-700">
            {activeCount}
          </span>
        ) : null}
      </button>

      {open ? (
        <AppModal
          open
          onClose={() => setOpen(false)}
          ariaLabel={zh('字幕生成任务', 'Subtitle generation jobs')}
          contentClassName="mx-4 flex max-h-[88vh] w-full max-w-3xl flex-col rounded-2xl bg-white p-5 shadow-2xl"
        >
          <div className="flex items-center justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold text-gray-900">
                {zh('字幕生成任务', 'Subtitle generation jobs')}
              </h2>
              <p className="mt-1 text-xs text-gray-500">
                {zh(
                  '任务只会在这里点击暂停后停止运行；关闭窗口或切换页面不会停止。',
                  'Jobs pause only when Pause is clicked here. Closing this window or changing pages does not stop them.'
                )}
              </p>
            </div>
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
            >
              {zh('关闭', 'Close')}
            </button>
          </div>

          {actionError || loadError ? (
            <p className="mt-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">
              {actionError || loadError}
            </p>
          ) : null}

          <div className="mt-4 min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
            {loading && jobs.length === 0 ? (
              <div className="py-12 text-center text-sm text-gray-500">
                {zh('正在加载…', 'Loading…')}
              </div>
            ) : jobs.length === 0 ? (
              <div className="rounded-xl border border-dashed border-gray-300 py-12 text-center text-sm text-gray-500">
                {zh('暂无字幕生成任务', 'No subtitle generation jobs')}
              </div>
            ) : (
              jobs.map((job) => {
                const key = `${job.video_id}:${job.location_id}`
                const busy = actionKey === key
                const progress = Math.max(0, Math.min(100, Number(job.progress) || 0))
                const labels = statusLabels[job.status] || [job.status, job.status]
                return (
                  <div key={key} className="rounded-xl border border-gray-200 p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 flex-1">
                        <div
                          className="truncate text-sm font-medium text-gray-900"
                          title={job.filename}
                        >
                          {job.filename}
                        </div>
                        <div className="mt-1 text-xs text-gray-500">
                          {job.model} · {String(job.device || '').toUpperCase()} ·{' '}
                          {job.language || zh('自动检测', 'Auto detect')} →{' '}
                          {zh('简体中文', 'Simplified Chinese')}
                        </div>
                        <div className="mt-1 text-xs text-gray-400">
                          {formatJobTime(job.created_at)}
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <span
                          className={`rounded-full px-2.5 py-1 text-xs font-medium ${statusClasses[job.status] || 'bg-gray-100 text-gray-700'}`}
                        >
                          {job.status === 'running' && phaseLabels[job.phase]
                            ? zh(phaseLabels[job.phase][0], phaseLabels[job.phase][1])
                            : zh(labels[0], labels[1])}
                        </span>
                        {job.status === 'queued' || job.status === 'running' ? (
                          <button
                            type="button"
                            onClick={() => handleAction(job, 'pause')}
                            disabled={busy}
                            className="inline-flex items-center gap-1 rounded-lg border border-violet-300 px-2.5 py-1 text-xs font-medium text-violet-700 hover:bg-violet-50 disabled:opacity-50"
                          >
                            <PauseCircleOutlineIcon fontSize="small" />
                            {zh('暂停', 'Pause')}
                          </button>
                        ) : null}
                        {job.status === 'paused' ? (
                          <button
                            type="button"
                            onClick={() => handleAction(job, 'resume')}
                            disabled={busy}
                            className="inline-flex items-center gap-1 rounded-lg bg-violet-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-violet-700 disabled:opacity-50"
                          >
                            <PlayCircleOutlineIcon fontSize="small" />
                            {zh('继续', 'Resume')}
                          </button>
                        ) : null}
                      </div>
                    </div>

                    <div className="mt-3 flex items-center gap-3">
                      <div className="h-2 flex-1 overflow-hidden rounded-full bg-gray-200">
                        <div
                          className={`h-full transition-all ${job.status === 'failed' ? 'bg-red-500' : job.status === 'completed' ? 'bg-emerald-500' : 'bg-violet-600'}`}
                          style={{ width: `${progress}%` }}
                        />
                      </div>
                      <span className="w-10 text-right text-xs tabular-nums text-gray-600">
                        {progress.toFixed(0)}%
                      </span>
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
                      <p className="mt-2 break-all rounded bg-red-50 px-2 py-1.5 text-xs text-red-700">
                        {job.error}
                      </p>
                    ) : null}
                  </div>
                )
              })
            )}
          </div>
        </AppModal>
      ) : null}
    </>
  )
}
