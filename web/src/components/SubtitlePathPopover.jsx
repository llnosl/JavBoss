import { ClickAwayListener, Popper } from '@mui/material'
import { useEffect, useMemo, useRef, useState } from 'react'

import { updateVideoSubtitlePath } from '@/api'
import { getErrorMessage } from '@/utils/errors'
import { zh } from '@/utils/i18n'
import { collectExternalSubtitlePaths } from '@/utils/subtitlePaths'
import { subtitleLanguageLabels } from '@/utils/subtitles'

export default function SubtitlePathPopover({ videos, detailText, onUpdated, children }) {
  const entries = useMemo(() => collectExternalSubtitlePaths(videos), [videos])
  const closeTimerRef = useRef(null)
  const [anchorEl, setAnchorEl] = useState(null)
  const [editingKey, setEditingKey] = useState('')
  const [path, setPath] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(
    () => () => {
      if (closeTimerRef.current) window.clearTimeout(closeTimerRef.current)
    },
    []
  )

  const cancelClose = () => {
    if (!closeTimerRef.current) return
    window.clearTimeout(closeTimerRef.current)
    closeTimerRef.current = null
  }

  const openPanel = (event) => {
    cancelClose()
    setAnchorEl(event.currentTarget)
  }

  const closePanel = () => {
    if (busy) return
    setAnchorEl(null)
    setEditingKey('')
    setPath('')
    setError('')
  }

  const scheduleClose = () => {
    cancelClose()
    closeTimerRef.current = window.setTimeout(closePanel, 180)
  }

  const beginEdit = (entry) => {
    setEditingKey(entry.key)
    setPath(entry.absolutePath)
    setError('')
  }

  const savePath = async (entry) => {
    const nextPath = path.trim()
    if (!nextPath || busy) return
    setBusy(true)
    setError('')
    try {
      const result = await updateVideoSubtitlePath(entry.videoID, entry.subtitleID, {
        locationId: entry.locationID,
        path: nextPath,
      })
      await onUpdated?.(result)
      setEditingKey('')
      setPath('')
    } catch (saveError) {
      setError(getErrorMessage(saveError))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <span
        className="inline-flex"
        onMouseEnter={openPanel}
        onMouseLeave={scheduleClose}
        onFocus={openPanel}
        onBlur={scheduleClose}
      >
        {children}
      </span>
      <Popper
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        placement="bottom-start"
        className="z-[1600]"
        modifiers={[{ name: 'offset', options: { offset: [0, 6] } }]}
        onClick={(event) => event.stopPropagation()}
      >
        <ClickAwayListener onClickAway={closePanel}>
          <div
            role="dialog"
            aria-label={zh('字幕文件路径', 'Subtitle file paths')}
            className="w-[min(34rem,calc(100vw-2rem))] rounded-xl border border-gray-200 bg-white p-3 shadow-xl"
            onMouseEnter={cancelClose}
            onMouseLeave={scheduleClose}
          >
            <div className="text-sm font-semibold text-gray-900">
              {zh('字幕文件路径', 'Subtitle file paths')}
            </div>
            {detailText ? <div className="mt-0.5 text-xs text-gray-500">{detailText}</div> : null}

            {entries.length ? (
              <div className="mt-2 max-h-72 space-y-2 overflow-y-auto pr-1">
                {entries.map((entry) => {
                  const labels = subtitleLanguageLabels(entry.language)
                  const language = labels ? zh(labels[0], labels[1]) : zh('未知语言', 'Unknown')
                  const editing = editingKey === entry.key
                  return (
                    <div key={entry.key} className="rounded-lg bg-gray-50 p-2">
                      <div className="flex items-center justify-between gap-2 text-xs">
                        <span
                          className="min-w-0 truncate font-medium text-gray-700"
                          title={entry.videoLabel}
                        >
                          {entry.videoLabel} · {language}
                        </span>
                        {!editing ? (
                          <button
                            type="button"
                            onClick={() => beginEdit(entry)}
                            className="shrink-0 rounded px-2 py-1 font-medium text-blue-600 hover:bg-blue-50"
                          >
                            {zh('编辑', 'Edit')}
                          </button>
                        ) : null}
                      </div>
                      {editing ? (
                        <div className="mt-1.5">
                          <input
                            value={path}
                            onChange={(event) => setPath(event.target.value)}
                            onKeyDown={(event) => {
                              if (event.key === 'Enter') void savePath(entry)
                              if (event.key === 'Escape') setEditingKey('')
                            }}
                            disabled={busy}
                            className="w-full rounded-md border border-gray-300 bg-white px-2 py-1.5 text-xs text-gray-800 outline-none focus:border-blue-500"
                          />
                          <div className="mt-1.5 flex justify-end gap-1.5">
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() => setEditingKey('')}
                              className="rounded border border-gray-300 px-2 py-1 text-xs text-gray-600 hover:bg-white disabled:opacity-50"
                            >
                              {zh('取消', 'Cancel')}
                            </button>
                            <button
                              type="button"
                              disabled={busy || !path.trim()}
                              onClick={() => void savePath(entry)}
                              className="rounded bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50"
                            >
                              {busy ? zh('保存中…', 'Saving…') : zh('保存', 'Save')}
                            </button>
                          </div>
                        </div>
                      ) : (
                        <div
                          className="mt-1 break-all text-xs leading-5 text-gray-600"
                          title={entry.absolutePath}
                        >
                          {entry.absolutePath}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            ) : (
              <div className="mt-2 rounded-lg bg-gray-50 px-2.5 py-2 text-xs text-gray-600">
                {zh(
                  '当前没有可编辑的外挂字幕路径。',
                  'There are no editable external subtitle paths.'
                )}
              </div>
            )}
            <div className="mt-2 text-[11px] leading-4 text-amber-700">
              {zh(
                '新路径必须位于当前媒体目录内，并指向已存在的字幕文件；保存不会移动或重命名文件。',
                'The new path must point to an existing subtitle inside the current media directory. Saving does not move or rename files.'
              )}
            </div>
            {error ? <div className="mt-2 text-xs text-red-600">{error}</div> : null}
          </div>
        </ClickAwayListener>
      </Popper>
    </>
  )
}
