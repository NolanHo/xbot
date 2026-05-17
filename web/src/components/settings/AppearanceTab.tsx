import { useEffect, useState, useCallback } from 'react'

import type { ShowToastFn, Theme, FontSize, Language, UserSettings } from './shared'
import { lsGet, lsSet, fetchSettings, saveSettings, FONT_SIZE_MAP, DEFAULT_SETTINGS } from './shared'

interface AppearanceTabProps {
  showToast: ShowToastFn
  onNicknameChange?: (nickname: string) => void
  onSavingChange?: (saving: boolean) => void
}

export default function AppearanceTab({ showToast, onNicknameChange, onSavingChange }: AppearanceTabProps) {
  const [theme, setTheme] = useState<Theme>(() => lsGet('theme', DEFAULT_SETTINGS.theme))
  const [fontSize, setFontSize] = useState<FontSize>(() => lsGet('font_size', DEFAULT_SETTINGS.font_size))
  const [nickname, setNickname] = useState<string>(() => lsGet('nickname', DEFAULT_SETTINGS.nickname))
  const [language, setLanguage] = useState<Language>(() => lsGet('language', DEFAULT_SETTINGS.language))

  // Load settings from server on mount
  useEffect(() => {
    fetchSettings().then((s) => {
      setTheme(s.theme as Theme)
      setFontSize(s.font_size as FontSize)
      setNickname(s.nickname)
      setLanguage(s.language as Language)
    })
  }, [])

  // Apply theme
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    lsSet('theme', theme)
  }, [theme])

  // Apply font size
  useEffect(() => {
    document.documentElement.style.setProperty('--xbot-font-size', FONT_SIZE_MAP[fontSize])
    lsSet('font_size', fontSize)
  }, [fontSize])

  // Persist nickname locally
  useEffect(() => {
    lsSet('nickname', nickname)
  }, [nickname])

  // Persist language locally
  useEffect(() => {
    lsSet('language', language)
  }, [language])

  const handleSave = useCallback(async (updates: Partial<UserSettings>) => {
    onSavingChange?.(true)
    const ok = await saveSettings(updates)
    onSavingChange?.(false)
    if (ok) {
      showToast('设置已保存', 'success')
    } else {
      showToast('保存失败，请重试', 'error')
    }
  }, [showToast, onSavingChange])

  const sectionClass = 'settings-section'
  const sectionTitleClass = 'settings-section-title'

  return (
    <div className={sectionClass}>
      <div className={sectionTitleClass}>🎨 外观 Appearance</div>

      <div className="settings-item">
        <label className="settings-label">主题 Theme</label>
        <select
          className="settings-select"
          value={theme}
          onChange={(e) => {
            const v = e.target.value as Theme
            setTheme(v)
            handleSave({ theme: v, font_size: fontSize, nickname, language })
          }}
        >
          <option value="dark">深色 Dark</option>
          <option value="light">浅色 Light</option>
        </select>
      </div>

      <div className="settings-item">
        <label className="settings-label">字体大小 Font Size</label>
        <select
          className="settings-select"
          value={fontSize}
          onChange={(e) => {
            const v = e.target.value as FontSize
            setFontSize(v)
            handleSave({ theme, font_size: v, nickname, language })
          }}
        >
          <option value="small">小 Small</option>
          <option value="medium">中 Medium</option>
          <option value="large">大 Large</option>
        </select>
      </div>

      <div className="settings-item">
        <label className="settings-label">昵称 Nickname</label>
        <input
          type="text"
          className="settings-input"
          placeholder="输入昵称..."
          maxLength={32}
          value={nickname}
          onChange={(e) => setNickname(e.target.value)}
          onBlur={() => {
            onNicknameChange?.(nickname)
            handleSave({ theme, font_size: fontSize, nickname, language })
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              ;(e.target as HTMLInputElement).blur()
            }
          }}
        />
      </div>

      <div className="settings-item">
        <label className="settings-label">语言 Language</label>
        <select
          className="settings-select"
          value={language}
          onChange={(e) => {
            const v = e.target.value as Language
            setLanguage(v)
            handleSave({ theme, font_size: fontSize, nickname, language: v })
          }}
        >
          <option value="zh-CN">简体中文</option>
          <option value="en">English</option>
        </select>
      </div>
    </div>
  )
}
