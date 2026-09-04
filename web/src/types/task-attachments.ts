import type { ConversationRuntimeConfig } from './pipeline'

export interface ChatComposerSubmission {
  content: string
  display_content?: string
  config: ConversationRuntimeConfig
}
