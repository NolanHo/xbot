import { useRef, useCallback, useEffect, useState } from 'react'
import { IconThinking, IconX } from './Icons'
import { useTranslation } from '../i18n'

interface AskUserQuestion {
  question: string
  options?: string[]
}

interface AskUserData {
  questions: AskUserQuestion[]
  answers: Record<string, string>
  currentQ: number
}

interface AskUserPanelProps {
  askUser: AskUserData
  onSubmit: (answers: Record<string, string>) => void
  onCancel: (answers: Record<string, string>) => void
}

export default function AskUserPanel({ askUser, onSubmit, onCancel }: AskUserPanelProps) {
  const [currentQ, setCurrentQ] = useState(askUser.currentQ)
  const [answers, setAnswers] = useState<Record<string, string>>(askUser.answers)
  const inputRef = useRef<HTMLInputElement>(null)
  const { t } = useTranslation()

  // Auto-focus the input when question changes
  useEffect(() => {
    const q = askUser.questions[currentQ]
    if (q && !q.options) {
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [currentQ, askUser.questions])

  const submitAnswer = useCallback((value: string) => {
    if (!value.trim()) return
    const newAnswers = { ...answers, [currentQ]: value.trim() }
    if (currentQ < askUser.questions.length - 1) {
      setAnswers(newAnswers)
      setCurrentQ(prev => prev + 1)
      setTimeout(() => inputRef.current?.focus(), 0)
    } else {
      onSubmit(newAnswers)
    }
  }, [answers, currentQ, askUser.questions.length, onSubmit])

  // Escape key to cancel
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onCancel(answers)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [answers, onCancel])

  const currentQuestion = askUser.questions[currentQ]
  if (!currentQuestion) return null

  return (
    <div className="border border-slate-600/50 rounded-xl bg-slate-800/95 backdrop-blur-sm shadow-lg max-w-4xl mx-auto mb-1 askuser-inline">
      {/* Header */}
      <div className="flex items-center justify-between px-4 pt-3 pb-2">
        <span className="text-xs font-medium text-slate-300 flex items-center gap-1.5">
          <IconThinking className="inline" style={{width:14,height:14}} />
          {t('agentNeedsInput')}
        </span>
        <div className="flex items-center gap-2">
          <span className="text-[11px] text-slate-500">
            {currentQ + 1} / {askUser.questions.length}
          </span>
          <button
            onClick={() => onCancel(answers)}
            className="text-slate-500 hover:text-slate-300 transition-colors"
            aria-label={t('cancel')}
          >
            <IconX style={{width:14,height:14}} />
          </button>
        </div>
      </div>

      {/* Question */}
      <div className="px-4 pb-3">
        <p className="text-sm text-slate-200 mb-2.5">{currentQuestion.question}</p>
        {currentQuestion.options && currentQuestion.options.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {currentQuestion.options.map((opt, i) => (
              <button
                key={i}
                onClick={() => submitAnswer(opt)}
                className="px-3 py-1.5 rounded-lg border border-slate-600 text-xs text-slate-200 hover:bg-blue-500/15 hover:border-blue-500/50 transition-colors"
                aria-label={opt}
              >
                {opt}
              </button>
            ))}
          </div>
        ) : (
          <div className="flex gap-2">
            <input
              type="text"
              ref={inputRef}
              autoFocus
              placeholder={t('inputAnswer')}
              className="flex-1 px-3 py-1.5 bg-slate-700/80 border border-slate-600 rounded-lg text-sm text-white placeholder-slate-400 focus:outline-none focus:border-blue-500 transition-colors"
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  submitAnswer((e.target as HTMLInputElement).value)
                }
              }}
            />
            <button
              onClick={() => submitAnswer(inputRef.current?.value || '')}
              className="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-xs rounded-lg transition-colors"
              aria-label={t('submit')}
            >
              {t('submit')}
            </button>
          </div>
        )}
      </div>

      {/* Footer: prev / cancel */}
      {askUser.questions.length > 1 && (
        <div className="px-4 pb-2.5 flex justify-between items-center">
          {currentQ > 0 ? (
            <button
              onClick={() => setCurrentQ(prev => prev - 1)}
              className="text-[11px] text-slate-500 hover:text-slate-300 transition-colors"
              aria-label={t('previousQuestion')}
            >
              ← {t('previousQuestion')}
            </button>
          ) : (
            <div />
          )}
        </div>
      )}
    </div>
  )
}
