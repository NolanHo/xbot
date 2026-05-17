import { useState, useCallback, useEffect } from 'react'

export interface Tab {
  chatId: string
  label: string
}

const STORAGE_KEY = 'xbot-open-tabs'
const ACTIVE_KEY = 'xbot-active-tab'

function loadTabs(): Tab[] {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

function saveTabs(tabs: Tab[]) {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(tabs))
  } catch {
    // sessionStorage may be unavailable
  }
}

function loadActiveTab(): string {
  try {
    return sessionStorage.getItem(ACTIVE_KEY) || ''
  } catch {
    return ''
  }
}

function saveActiveTab(id: string) {
  try {
    sessionStorage.setItem(ACTIVE_KEY, id)
  } catch { /* ignore */ }
}

export interface UseTabManagerReturn {
  tabs: Tab[]
  activeTabId: string
  openTab: (chatId: string, label: string) => void
  closeTab: (chatId: string) => void
  switchTab: (chatId: string) => void
  renameTab: (chatId: string, label: string) => void
}

/**
 * Manages open browser-style tabs for multi-session support.
 * Persists to sessionStorage for page refresh recovery.
 */
export function useTabManager(
  onSwitchChat: (chatId: string) => void,
  onNewChat: () => void,
): UseTabManagerReturn {
  const [tabs, setTabs] = useState<Tab[]>(loadTabs)
  const [activeTabId, setActiveTabId] = useState<string>(loadActiveTab)

  // Persist on change
  useEffect(() => { saveTabs(tabs) }, [tabs])
  useEffect(() => { saveActiveTab(activeTabId) }, [activeTabId])

  const openTab = useCallback((chatId: string, label: string) => {
    setTabs(prev => {
      if (prev.some(t => t.chatId === chatId)) return prev
      return [...prev, { chatId, label }]
    })
    setActiveTabId(chatId)
  }, [])

  const switchTab = useCallback((chatId: string) => {
    setActiveTabId(chatId)
    onSwitchChat(chatId)
  }, [onSwitchChat])

  const closeTab = useCallback((chatId: string) => {
    setTabs(prev => {
      const idx = prev.findIndex(t => t.chatId === chatId)
      if (idx === -1) return prev
      const next = prev.filter(t => t.chatId !== chatId)

      // If closing the active tab, switch to adjacent
      if (chatId === activeTabId) {
        if (next.length === 0) {
          // No tabs left — create new session
          onNewChat()
          setActiveTabId('')
        } else {
          const newActive = next[Math.min(idx, next.length - 1)]
          setActiveTabId(newActive.chatId)
          onSwitchChat(newActive.chatId)
        }
      }
      return next
    })
  }, [activeTabId, onSwitchChat, onNewChat])

  const renameTab = useCallback((chatId: string, label: string) => {
    setTabs(prev => prev.map(t => t.chatId === chatId ? { ...t, label } : t))
  }, [])

  return { tabs, activeTabId, openTab, closeTab, switchTab, renameTab }
}
