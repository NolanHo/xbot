import { useTranslation } from '../i18n'
import type { Tab } from '../hooks/useTabManager'

interface TabBarProps {
  tabs: Tab[]
  activeTabId: string
  onTabClick: (chatId: string) => void
  onTabClose: (chatId: string) => void
}

export default function TabBar({ tabs, activeTabId, onTabClick, onTabClose }: TabBarProps) {
  const { t } = useTranslation()

  if (tabs.length === 0) return null

  return (
    <div className="tab-bar" role="tablist" data-testid="tab-bar">
      <div className="tab-bar-scroll">
        {tabs.map(tab => {
          const isActive = tab.chatId === activeTabId
          return (
            <div
              key={tab.chatId}
              className={`tab-item ${isActive ? 'tab-item-active' : ''}`}
              role="tab"
              aria-selected={isActive}
              data-testid={`tab-item-${tab.chatId}`}
            >
              <button
                className="tab-item-label"
                onClick={() => onTabClick(tab.chatId)}
                title={tab.label}
              >
                <span className="tab-item-text">{tab.label || t('unnamedSession')}</span>
              </button>
              <button
                className="tab-item-close"
                onClick={(e) => { e.stopPropagation(); onTabClose(tab.chatId) }}
                title={t('closeTab')}
                aria-label={`${t('closeTab')} ${tab.label}`}
              >
                ✕
              </button>
            </div>
          )
        })}
      </div>
    </div>
  )
}
