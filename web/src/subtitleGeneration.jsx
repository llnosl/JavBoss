import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'

import { fetchSubtitleGenerations, pauseSubtitleGeneration, resumeSubtitleGeneration } from '@/api'
import { getErrorMessage } from '@/utils/errors'

const SubtitleGenerationContext = createContext(null)

export function SubtitleGenerationProvider({ children }) {
  const [jobs, setJobs] = useState([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async ({ silent = false } = {}) => {
    if (!silent) setLoading(true)
    try {
      const items = await fetchSubtitleGenerations()
      setJobs(items)
      setError('')
      return items
    } catch (refreshError) {
      setError(getErrorMessage(refreshError))
      return []
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      if (cancelled) return
      await refresh({ silent: true })
    }
    void refresh()
    const timer = window.setInterval(load, 1000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [refresh])

  const pause = useCallback(async (job) => {
    const updated = await pauseSubtitleGeneration(job.video_id, job.location_id)
    setJobs((current) =>
      current.map((item) =>
        item.video_id === updated.video_id && item.location_id === updated.location_id
          ? updated
          : item
      )
    )
    return updated
  }, [])

  const resume = useCallback(async (job) => {
    const updated = await resumeSubtitleGeneration(job.video_id, job.location_id)
    setJobs((current) =>
      current.map((item) =>
        item.video_id === updated.video_id && item.location_id === updated.location_id
          ? updated
          : item
      )
    )
    return updated
  }, [])

  const value = useMemo(
    () => ({ jobs, error, loading, refresh, pause, resume }),
    [error, jobs, loading, pause, refresh, resume]
  )

  return (
    <SubtitleGenerationContext.Provider value={value}>
      {children}
    </SubtitleGenerationContext.Provider>
  )
}

export function useSubtitleGenerations() {
  const context = useContext(SubtitleGenerationContext)
  if (!context) {
    throw new Error('useSubtitleGenerations must be used within SubtitleGenerationProvider')
  }
  return context
}
