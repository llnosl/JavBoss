import AutoAwesomeOutlinedIcon from '@mui/icons-material/AutoAwesomeOutlined'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import AppModal from '@/components/AppModal'
import { fetchJavTitleTranslation, startJavTitleTranslation } from '@/api'
import { getErrorMessage } from '@/utils/errors'
import { zh } from '@/utils/i18n'

const activeStatuses = new Set(['queued', 'running'])

const statusLabels = {
  queued: ['排队中', 'Queued'],
  running: ['翻译中', 'Translating'],
  completed: ['已完成', 'Completed'],
  completed_with_errors: ['部分完成', 'Completed with errors'],
}

const statusClasses = {
  queued: 'bg-amber-100 text-amber-800',
  running: 'bg-blue-100 text-blue-800',
  completed: 'bg-emerald-100 text-emerald-800',
  completed_with_errors: 'bg-red-100 text-red-700',
}

export default function JavTitleTranslationModal({ open, onClose, onUpdated }) {
  const [job, setJob] = useState(null)
  const [configured, setConfigured] = useState(true)
  const [loading, setLoading] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')
  const notifiedCompletion = useRef('')

  const refresh = useCallback(async ({ silent = false } = {}) => {
    if (!silent) setLoading(true)
    try {
      const payload = await fetchJavTitleTranslation()
      setConfigured(Boolean(payload?.configured))
      setJob(payload?.job || null)
      setError('')
      return payload?.job || null
    } catch (err) {
      setError(getErrorMessage(err))
      return null
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => {
    const shouldPoll = activeStatuses.has(job?.status)
    if (!open && !shouldPoll) return undefined
    if (open) void refresh()
    const timer = window.setInterval(() => {
      void refresh({ silent: true })
    }, 2000)
    return () => window.clearInterval(timer)
  }, [job?.status, open, refresh])

  useEffect(() => {
    const completionKey = String(job?.completed_at || '')
    if (!completionKey || notifiedCompletion.current === completionKey) return
    notifiedCompletion.current = completionKey
    onUpdated?.()
  }, [job?.completed_at, onUpdated])

  const progress = useMemo(() => {
    const total = Math.max(0, Number(job?.total) || 0)
    if (total === 0) return job?.status === 'completed' ? 100 : 0
    return Math.max(
      0,
      Math.min(100, (((Number(job?.completed) || 0) + (Number(job?.failed) || 0)) / total) * 100)
    )
  }, [job])
  const active = activeStatuses.has(job?.status)

  const handleStart = async (force) => {
    setStarting(true)
    setError('')
    try {
      const nextJob = await startJavTitleTranslation({ force })
      setConfigured(true)
      setJob(nextJob)
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setStarting(false)
    }
  }

  if (!open) return null
  const labels = statusLabels[job?.status] || [job?.status || '', job?.status || '']

  return (
    <AppModal
      open
      onClose={onClose}
      ariaLabel={zh('中文标题批量翻译', 'Batch Chinese title translation')}
      contentClassName="mx-4 flex max-h-[88vh] w-full max-w-2xl flex-col rounded-2xl bg-white p-5 shadow-2xl"
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-lg font-semibold text-gray-900">
            <AutoAwesomeOutlinedIcon className="text-violet-600" fontSize="small" />
            {zh('中文标题批量翻译', 'Batch Chinese title translation')}
          </h2>
          <p className="mt-1 text-xs leading-5 text-gray-500">
            {zh(
              '使用 DeepSeek 将全部本地 JAV 的原始标题翻译为自然、通顺、偏口语化的简体中文。原始标题会保留，完成后页面优先显示中文标题。关闭窗口不会停止任务。',
              'Use DeepSeek to translate all local JAV titles into natural Simplified Chinese. Original titles remain stored and Chinese titles are shown first. Closing this window does not stop the job.'
            )}
          </p>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
        >
          {zh('关闭', 'Close')}
        </button>
      </div>

      {!configured ? (
        <p className="mt-4 rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800">
          {zh(
            '未配置 DeepSeek API Key，请先设置 JAVBOSS_DEEPSEEK_API_KEY 并重新启动后端。',
            'JAVBOSS_DEEPSEEK_API_KEY is not configured. Set it and restart the backend.'
          )}
        </p>
      ) : null}
      {error ? (
        <p className="mt-4 break-all rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">
          {error}
        </p>
      ) : null}

      <div className="mt-4 min-h-0 flex-1 overflow-y-auto">
        {loading && !job ? (
          <div className="py-12 text-center text-sm text-gray-500">
            {zh('正在加载…', 'Loading…')}
          </div>
        ) : job ? (
          <div className="rounded-xl border border-gray-200 p-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div className="text-sm font-medium text-gray-900">
                  {job.force
                    ? zh('重新翻译全部标题', 'Retranslate all titles')
                    : zh('翻译缺失或已变化的标题', 'Translate missing or changed titles')}
                </div>
                <div className="mt-1 text-xs text-gray-500">
                  {zh(
                    `共 ${job.total || 0} 个 · 成功 ${job.completed || 0} 个 · 失败 ${job.failed || 0} 个`,
                    `${job.total || 0} total · ${job.completed || 0} completed · ${job.failed || 0} failed`
                  )}
                </div>
              </div>
              <span
                className={`rounded-full px-2.5 py-1 text-xs font-medium ${statusClasses[job.status] || 'bg-gray-100 text-gray-700'}`}
              >
                {zh(labels[0], labels[1])}
              </span>
            </div>
            <div className="mt-4 flex items-center gap-3">
              <div className="h-2 flex-1 overflow-hidden rounded-full bg-gray-200">
                <div
                  className={`h-full transition-all ${job.status === 'completed_with_errors' ? 'bg-amber-500' : 'bg-violet-600'}`}
                  style={{ width: `${progress}%` }}
                />
              </div>
              <span className="w-11 text-right text-xs tabular-nums text-gray-600">
                {progress.toFixed(0)}%
              </span>
            </div>
            {job.current_code ? (
              <p className="mt-2 text-xs text-gray-500">
                {zh(`正在处理：${job.current_code}`, `Processing: ${job.current_code}`)}
              </p>
            ) : null}
            {job.error ? (
              <p className="mt-3 break-all rounded bg-red-50 px-2 py-1.5 text-xs text-red-700">
                {job.error}
              </p>
            ) : null}
            {Array.isArray(job.failures) && job.failures.length > 0 ? (
              <div className="mt-3 space-y-1 text-xs text-red-700">
                {job.failures.slice(0, 20).map((failure) => (
                  <div key={`${failure.id}:${failure.code}`} className="break-all">
                    {failure.code || `#${failure.id}`}：{failure.error}
                  </div>
                ))}
                {job.failures.length > 20 ? (
                  <div>
                    {zh(
                      `另有 ${job.failures.length - 20} 个失败项`,
                      `${job.failures.length - 20} more failures`
                    )}
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        ) : (
          <div className="rounded-xl border border-dashed border-gray-300 py-10 text-center text-sm text-gray-500">
            {zh('尚未执行中文标题翻译', 'No Chinese title translation job yet')}
          </div>
        )}
      </div>

      <div className="mt-4 flex flex-wrap justify-end gap-2 border-t border-gray-100 pt-4">
        <button
          type="button"
          onClick={() => handleStart(false)}
          disabled={!configured || active || starting}
          className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white hover:bg-violet-700 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {starting ? zh('正在启动…', 'Starting…') : zh('翻译缺失标题', 'Translate missing titles')}
        </button>
        <button
          type="button"
          onClick={() => handleStart(true)}
          disabled={!configured || active || starting}
          className="rounded-lg border border-violet-300 px-4 py-2 text-sm font-medium text-violet-700 hover:bg-violet-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {zh('重新翻译全部', 'Retranslate all')}
        </button>
      </div>
    </AppModal>
  )
}
