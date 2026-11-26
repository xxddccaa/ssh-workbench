import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../../shared/api/client'
import type { Attempt, AttemptRequest } from '../../shared/api/types'

const finished = new Set(['connected', 'failed', 'cancelled'])

/**
 * Drives one asynchronous connection attempt: start, long-poll for state
 * changes, answer prompts and cancel. The hook owns the polling loop so a
 * closed dialog never leaves an attempt waiting for an answer.
 */
export function useAttempt() {
  const [attempt, setAttempt] = useState<Attempt | null>(null)
  const active = useRef<{ id: string; abort: AbortController } | null>(null)

  const cancel = useCallback(() => {
    const current = active.current
    active.current = null
    if (!current) return
    current.abort.abort()
    void api.cancelAttempt(current.id).catch(() => undefined)
  }, [])

  useEffect(() => cancel, [cancel])

  const start = useCallback(
    async (input: AttemptRequest): Promise<Attempt> => {
      cancel()
      const abort = new AbortController()
      let view = await api.startAttempt(input)
      active.current = { id: view.id, abort }
      setAttempt(view)
      while (!finished.has(view.state)) {
        view = await api.waitAttempt(view.id, view.version, abort.signal)
        if (abort.signal.aborted) break
        setAttempt(view)
      }
      if (active.current?.id === view.id) active.current = null
      return view
    },
    [cancel],
  )

  const resume = useCallback(
    async (initial: Attempt): Promise<Attempt> => {
      cancel()
      const abort = new AbortController()
      let view = initial
      active.current = { id: view.id, abort }
      setAttempt(view)
      while (!finished.has(view.state)) {
        view = await api.waitAttempt(view.id, view.version, abort.signal)
        if (abort.signal.aborted) break
        setAttempt(view)
      }
      if (active.current?.id === view.id) active.current = null
      return view
    },
    [cancel],
  )

  const answer = useCallback(
    async (accept: boolean, responses?: string[]) => {
      if (!attempt?.prompt) return
      await api.answerAttempt(attempt.id, attempt.prompt.id, accept, responses)
    },
    [attempt],
  )

  const reset = useCallback(() => {
    cancel()
    setAttempt(null)
  }, [cancel])

  return { attempt, start, resume, answer, cancel, reset }
}
