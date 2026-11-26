import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, messageOf, uploadFile } from '../../shared/api/client'
import { checkItems, type UploadItem } from '../../shared/lib/upload-items'

export type TransferStatus = 'queued' | 'uploading' | 'done' | 'error' | 'cancelled'
export interface Transfer {
  id: string
  terminalID: string
  host: string
  directory: string
  name: string
  size: number
  percent: number
  status: TransferStatus
  error?: string
}
type Job = { transfer: Transfer; item: UploadItem; abort?: AbortController }

export function useTransfers() {
  const [transfers, setTransfers] = useState<Transfer[]>([])
  const jobs = useRef(new Map<string, Job>())
  const waiting = useRef<string[]>([])
  const active = useRef(new Set<string>())
  const disposed = useRef(false)
  const queries = useQueryClient()
  const publish = useCallback(() => {
    if (!disposed.current)
      setTransfers(Array.from(jobs.current.values(), (job) => ({ ...job.transfer })))
  }, [])

  const drain = useCallback(
    function startWaiting() {
      while (!disposed.current && active.current.size < 2 && waiting.current.length > 0) {
        const identifier = waiting.current.shift()!
        const job = jobs.current.get(identifier)
        if (!job || job.transfer.status !== 'queued') continue
        const { transfer, item } = job
        job.abort = new AbortController()
        transfer.status = 'uploading'
        transfer.error = undefined
        transfer.percent = 0
        active.current.add(identifier)
        publish()
        void uploadFile(
          transfer.terminalID,
          transfer.directory,
          item.file,
          job.abort.signal,
          (percent) => {
            transfer.percent = percent
            publish()
          },
          item.relativePath,
        )
          .then(() => {
            transfer.status = 'done'
            transfer.percent = 100
            if (!disposed.current)
              void queries.invalidateQueries({ queryKey: ['files', transfer.terminalID] })
          })
          .catch((problem: unknown) => {
            transfer.status =
              problem instanceof ApiError && problem.code === 'upload_cancelled'
                ? 'cancelled'
                : 'error'
            transfer.error = messageOf(problem)
          })
          .finally(() => {
            active.current.delete(identifier)
            publish()
            startWaiting()
          })
      }
    },
    [publish, queries],
  )

  useEffect(() => {
    disposed.current = false
    const currentJobs = jobs.current
    // Browser uploads read from this page; closing it interrupts them, so
    // ask before leaving while any upload is queued or running.
    const preventLoss = (event: BeforeUnloadEvent) => {
      if (
        Array.from(currentJobs.values()).some((job) =>
          ['queued', 'uploading'].includes(job.transfer.status),
        )
      ) {
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', preventLoss)
    return () => {
      disposed.current = true
      window.removeEventListener('beforeunload', preventLoss)
      for (const job of currentJobs.values()) job.abort?.abort()
    }
  }, [])

  function enqueue(terminalID: string, host: string, directory: string, items: UploadItem[]) {
    const problem = checkItems(items)
    if (problem) throw new Error(problem)
    if (items.length + jobs.current.size > 10000)
      throw new Error('任务列表最多保留 10,000 项，请先清理已完成任务。')
    for (const item of items) {
      const identifier = crypto.randomUUID()
      const transfer: Transfer = {
        id: identifier,
        terminalID,
        host,
        directory,
        name: item.relativePath,
        size: item.file.size,
        percent: 0,
        status: 'queued',
      }
      jobs.current.set(identifier, { transfer, item })
      waiting.current.push(identifier)
    }
    publish()
    drain()
  }

  function cancel(identifier: string) {
    const job = jobs.current.get(identifier)
    if (!job) return
    if (job.transfer.status === 'queued') {
      job.transfer.status = 'cancelled'
      publish()
    } else job.abort?.abort()
  }

  function retry(identifier: string) {
    const job = jobs.current.get(identifier)
    if (!job || !['error', 'cancelled'].includes(job.transfer.status)) return
    job.transfer.status = 'queued'
    waiting.current.push(identifier)
    publish()
    drain()
  }

  function clearFinished() {
    for (const [identifier, job] of jobs.current) {
      if (['done', 'error', 'cancelled'].includes(job.transfer.status))
        jobs.current.delete(identifier)
    }
    publish()
  }

  return {
    transfers,
    enqueue,
    cancel,
    retry,
    clearFinished,
    pending: (terminalID?: string) =>
      transfers.some(
        (transfer) =>
          (!terminalID || transfer.terminalID === terminalID) &&
          ['queued', 'uploading'].includes(transfer.status),
      ),
  }
}
