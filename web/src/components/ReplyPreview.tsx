import { useTranslation } from '../i18n'
import type { ReplyInfo } from '../types'

const PREVIEW_MAX_LENGTH = 80

interface ReplyPreviewProps {
  replyTo: ReplyInfo
  onClick: () => void
}

export default function ReplyPreview({ replyTo, onClick }: ReplyPreviewProps) {
  const { t } = useTranslation()
  const preview = replyTo.content.length > PREVIEW_MAX_LENGTH
    ? replyTo.content.slice(0, PREVIEW_MAX_LENGTH) + '...'
    : replyTo.content
  const icon = replyTo.type === 'user' ? '👤' : '🤖'

  return (
    <button
      className="reply-preview"
      onClick={onClick}
      data-testid="reply-preview"
      title={t('replyingTo')}
    >
      <span className="reply-preview-bar" />
      <div className="reply-preview-content">
        <span className="reply-preview-icon">{icon}</span>
        <span className="reply-preview-text">{preview}</span>
      </div>
    </button>
  )
}
