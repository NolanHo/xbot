import { useEffect, useState } from 'react'
import ChatPage from './ChatPage'
import { ToastProvider } from './contexts/ToastContext'
import { NotificationProvider } from './contexts/NotificationContext'
import { MediaPlayerProvider } from './contexts/MediaPlayerContext'
import ErrorBoundary from './components/ErrorBoundary'
import { initWebVitals } from './webVitals'
import { useTranslation } from './i18n'

// Apply saved theme immediately before React renders (prevents flash)
const savedTheme = localStorage.getItem('xbot-theme') || 'dark'
document.documentElement.setAttribute('data-theme', savedTheme)

// Initialize Web Vitals collection (dev-only logging)
initWebVitals()

// Desktop GUI: session token injected by native shell via URL hash or cookie.
function getSessionToken(): string | null {
  // 1. URL hash: #token=xxx
  const hash = window.location.hash
  if (hash.startsWith('#token=')) {
    return hash.slice(7)
  }
  // 2. Cookie: xbot_session=xxx
  const match = document.cookie.match(/(?:^|;\s*)xbot_session=([^;]+)/)
  if (match) return match[1]
  return null
}

function App() {
  const [authed, setAuthed] = useState(false)
  const [loading, setLoading] = useState(true)
  const { setLocale } = useTranslation()

  useEffect(() => {
    const token = getSessionToken()
    if (token) {
      // Set cookie for subsequent API calls
      document.cookie = `xbot_session=${token}; path=/`
      // Clear hash
      if (window.location.hash.startsWith('#token=')) {
        history.replaceState(null, '', window.location.pathname)
      }
    }

    // Verify auth by fetching history
    fetch('/api/history')
      .then((r) => {
        setAuthed(r.ok)
        if (r.ok) {
          fetch('/api/settings')
            .then((sr) => sr.json())
            .then((data) => {
              if (data.ok && data.settings) {
                const s = data.settings
                if (s.theme && s.theme !== savedTheme) {
                  localStorage.setItem('xbot-theme', s.theme)
                  document.documentElement.setAttribute('data-theme', s.theme)
                }
                if (s.language) {
                  localStorage.setItem('xbot-language', s.language)
                  setLocale(s.language)
                }
                if (s.font_size) localStorage.setItem('xbot-font-size', s.font_size)
                if (s.image_brightness) localStorage.setItem('xbot-image-brightness', String(s.image_brightness))
              }
            })
            .catch(() => { /* ignore */ })
        }
      })
      .catch(() => setAuthed(false))
      .finally(() => setLoading(false))
  }, [setLocale])

  if (loading) {
    const isDark = savedTheme !== 'light'
    return (
      <ErrorBoundary>
        <MediaPlayerProvider>
          <NotificationProvider>
            <ToastProvider>
              <div className={`flex flex-col items-center justify-center min-h-screen gap-3 ${isDark ? 'bg-slate-900 text-slate-400' : 'bg-stone-100 text-stone-400'}`}>
                <div className="w-6 h-6 border-2 border-current border-t-transparent rounded-full animate-spin" />
                <span className="text-sm">Loading...</span>
              </div>
            </ToastProvider>
          </NotificationProvider>
        </MediaPlayerProvider>
      </ErrorBoundary>
    )
  }

  if (!authed) {
    // Desktop fallback: shouldn't normally happen since auto-login handles this.
    // Show minimal error state.
    return (
      <ErrorBoundary>
        <div className="flex flex-col items-center justify-center min-h-screen gap-3 bg-slate-900 text-slate-400">
          <span className="text-lg">⚠️ 无法连接到 xbot 服务</span>
          <span className="text-sm">请确保 xbot serve 正在运行</span>
        </div>
      </ErrorBoundary>
    )
  }

  return (
    <ErrorBoundary>
      <MediaPlayerProvider>
        <NotificationProvider>
          <ToastProvider>
            <ChatPage onLogout={() => setAuthed(false)} />
          </ToastProvider>
        </NotificationProvider>
      </MediaPlayerProvider>
    </ErrorBoundary>
  )
}

export default App
