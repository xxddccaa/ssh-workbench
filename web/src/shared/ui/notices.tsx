import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { Info, X } from 'lucide-react'

const NoticeContext = createContext<(message: string) => void>(() => undefined)

/**
 * Toasts are shown as a manual popover so they enter the browser's top layer
 * above any open modal <dialog>; re-showing moves them to the top again.
 */
export function Notices({ children }: { children: ReactNode }) {
  const [message, setMessage] = useState<{ id: number; text: string } | null>(null)
  const toast = useRef<HTMLDivElement>(null)
  const notify = useCallback((text: string) => setMessage({ id: Date.now(), text }), [])
  useEffect(() => {
    if (!message) return
    const element = toast.current
    if (element && typeof element.showPopover === 'function') {
      if (element.matches(':popover-open')) element.hidePopover()
      element.showPopover()
    }
    const timer = window.setTimeout(() => setMessage(null), 7000)
    return () => window.clearTimeout(timer)
  }, [message])
  return (
    <NoticeContext.Provider value={notify}>
      {children}
      {message && (
        <div ref={toast} popover="manual" className="toast" role="status">
          <Info size={18} />
          <span>{message.text}</span>
          <button className="icon-button" aria-label="关闭提示" onClick={() => setMessage(null)}>
            <X size={15} />
          </button>
        </div>
      )}
    </NoticeContext.Provider>
  )
}

export function useNotice() {
  return useContext(NoticeContext)
}
