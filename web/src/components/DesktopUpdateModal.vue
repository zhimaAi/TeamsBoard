<template>
  <a-modal
    :open="store.modalOpen"
    :title="t('settings.updateFound')"
    :width="584"
    :footer="null"
    :closable="status !== 'installing'"
    :keyboard="status !== 'installing'"
    :mask-closable="false"
    centered
    class="desktop-update-modal"
    @cancel="handleClose"
  >
    <template v-if="update">
      <div v-if="status !== 'installing'" class="update-content">
        <p>{{ t('settings.updateReady') }}</p>
        <div class="version-panel">
          <div><span>{{ t('settings.currentVersion') }}</span><strong>{{ displayVersion(update.currentVersion) }}</strong></div>
          <span class="version-arrow" aria-hidden="true"></span>
          <div><span>{{ t('settings.latestVersion') }}</span><strong class="latest">{{ displayVersion(update.release?.version) }}</strong></div>
        </div>
        <p v-if="update.release?.asset" class="update-meta">
          {{ t('settings.updatePackage') }} {{ formatSize(update.release.asset.size) }}
          <span v-if="publishedDate">{{ publishedDate }} {{ t('settings.published') }}</span>
        </p>

        <template v-if="status === 'downloading'">
          <h3>{{ t('settings.downloadingUpdate') }}</h3>
          <div class="progress-row">
            <span>{{ progressSummary }}</span>
            <strong>{{ percentLabel }}</strong>
          </div>
          <a-progress :percent="percent" :show-info="false" />
        </template>
        <template v-else-if="status === 'downloaded'">
          <div class="completed"><span aria-hidden="true"></span><div>
            <h3>{{ t('settings.downloadCompleted') }}</h3>
            <p>{{ update.platform === 'darwin' ? t('settings.macInstallHint') : t('settings.restartHint') }}</p>
          </div></div>
        </template>
        <template v-else-if="status === 'error'">
          <a-alert type="error" show-icon :message="update.error || t('settings.updateFailed')" />
        </template>
        <template v-else>
          <h3>{{ t('settings.releaseNotes') }}</h3>
          <div v-if="update.release?.notes" class="release-notes" v-html="renderedNotes"></div>
          <p v-else class="update-meta">{{ t('settings.noReleaseNotes') }}</p>
        </template>
      </div>
      <div v-else class="installing">
        <div class="installing-versions">{{ displayVersion(update.currentVersion) }} <span class="version-arrow" aria-hidden="true"></span> {{ displayVersion(update.release?.version) }}</div>
        <h2>{{ t('settings.installingUpdate') }}</h2>
        <p>{{ t('settings.installingHint') }}</p>
      </div>

      <div v-if="status !== 'installing'" class="update-footer">
        <template v-if="status === 'downloading'">
          <a-button @click="handleCancelDownload">{{ t('settings.cancelDownload') }}</a-button>
        </template>
        <template v-else-if="status === 'downloaded'">
          <a-button @click="handleLater">{{ t('settings.installLater') }}</a-button>
          <a-button type="primary" @click="handleInstall">
            {{ update.platform === 'darwin' ? t('settings.openInstaller') : t('settings.restartAndInstall') }}
          </a-button>
        </template>
        <template v-else>
          <a-button @click="handleLater">{{ t('settings.remindLater') }}</a-button>
          <a-button v-if="update.release?.asset" type="primary" :loading="startingDownload" :disabled="!update.installSupported" @click="handleDownload">
            {{ status === 'error' ? t('settings.retryDownload') : t('settings.downloadUpdate') }}
          </a-button>
        </template>
      </div>
    </template>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import MarkdownIt from 'markdown-it'
import { useUpdateStore } from '@/stores/update'
import { useAppI18n } from '@/i18n'

const { t } = useAppI18n()
const store = useUpdateStore()
const update = computed(() => store.state)
const status = computed(() => update.value?.status || 'idle')
const startingDownload = ref(false)
const markdown = new MarkdownIt({ html: false, linkify: false })
markdown.renderer.rules.image = (tokens, index) => markdown.utils.escapeHtml(tokens[index].content)
const renderedNotes = computed(() => markdown.render(update.value?.release?.notes || ''))
const publishedDate = computed(() => {
  const date = update.value?.release?.publishedAt
  if (!date || Number.isNaN(Date.parse(date))) return ''
  return new Date(date).toLocaleDateString(undefined, { year: 'numeric', month: '2-digit', day: '2-digit' })
})
const percent = computed(() => {
  const progress = update.value?.progress
  return progress?.total ? Math.min(100, Math.floor(progress.received * 100 / progress.total)) : 0
})
const percentLabel = computed(() => `${percent.value}%`)
const progressSummary = computed(() => {
  const progress = update.value?.progress
  return `${formatSpeed(progress?.bytesPerSecond || 0)}  ${formatSize(progress?.received || 0)} / ${formatSize(progress?.total || 0)}`
})

function displayVersion(version?: string) { return `v${version || '—'}` }
function formatSize(bytes: number) { return `${(bytes / 1024 / 1024).toFixed(1)} MB` }
function formatSpeed(bytes: number) { return `${formatSize(bytes)}/s` }
async function handleLater() {
  try { await store.later() } catch (error) { message.error(String(error)) }
}
async function handleClose() {
  try { await store.close() } catch (error) { message.error(String(error)) }
}
async function handleDownload() {
  startingDownload.value = true
  try { await store.download() } catch (error) { message.error(String(error)) }
  finally { startingDownload.value = false }
}
async function handleCancelDownload() {
  try { await store.cancelDownload() } catch (error) { message.error(String(error)) }
}
async function handleInstall() {
  try {
    const result = await store.install()
    if (result.manual) message.info(t('settings.macInstallHint'))
  } catch (error) { message.error(String(error)) }
}
</script>

<style scoped>
.update-content { padding: 12px 8px 20px; color: #595959; }
.update-content > p:first-child { margin: 0 0 18px; }
.version-panel { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); align-items: center; padding: 24px; border-radius: 16px; background: #f5f5f7; }
.version-panel > div { display: flex; flex-direction: column; gap: 1px; min-height: 53px; }
.version-panel span { color: #8c8c8c; font-size: 12px; line-height: 20px; }
.version-panel strong { color: #262626; font-size: 24px; line-height: 32px; }
.version-panel strong.latest { color: #3157e2; }
.version-panel > span.version-arrow {
  display: grid;
  place-items: center;
  justify-self: center;
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 2px 8px rgba(15, 23, 42, .07);
  color: #3157e2;
  font-size: 16px;
  line-height: 1;
}
.version-arrow::before { content: '→'; }
.update-meta { display: flex; gap: 16px; margin: 10px 0 26px; color: #8c8c8c; font-size: 12px; }
h3 { color: #262626; font-size: 16px; font-weight: 600; margin: 0 0 14px; }
.release-notes { max-height: 220px; overflow-y: auto; line-height: 1.6; overflow-wrap: anywhere; }
.release-notes :deep(ul) { margin: 0; padding-inline-start: 24px; list-style-type: disc; }
.release-notes :deep(ol) { margin: 0; padding-inline-start: 24px; list-style-type: decimal; }
.release-notes :deep(li) { display: list-item; margin-bottom: 6px; }
.release-notes :deep(p) { margin: 0 0 8px; }
.progress-row { display: flex; justify-content: space-between; align-items: end; color: #8c8c8c; font-size: 12px; }
.progress-row strong { color: #262626; font-size: 24px; }
.completed { display: flex; gap: 16px; align-items: start; }
.completed > span { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 50%; background: #16a34a; color: white; }
.completed > span::before { content: '✓'; }
.completed p { margin: 0; }
.update-footer { display: flex; justify-content: end; gap: 8px; padding: 16px 8px 0; border-top: 1px solid #f0f0f0; }
.installing { text-align: center; padding: 64px 16px 72px; }
.installing-versions { color: #3157e2; font-size: 18px; }
.installing-versions span { padding: 0 24px; }
.installing h2 { margin: 18px 0 8px; }
.installing p { color: #8c8c8c; }
@media (max-width: 600px) { .version-panel { padding: 16px; } }
</style>
