<template>
  <section
    class="onboarding-guide"
    aria-labelledby="onboarding-guide-label"
  >
    <div class="guide-header">
      <div
        id="onboarding-guide-label"
        class="guide-eyebrow"
        aria-live="polite"
      >
        <span>{{ t('workflows.task.newConversation.onboarding.eyebrow') }}</span>
        <template v-if="phase === 'intro'">
          <span
            class="eyebrow-divider"
            aria-hidden="true"
          />
          <span class="revisit-hint">
            {{ t('workflows.task.newConversation.onboarding.revisitHint') }}
          </span>
        </template>
        <span
          v-else
          class="guide-progress"
        >
          {{
            t('workflows.task.newConversation.onboarding.progress', {
              current: currentStep,
              total: stepCount,
            })
          }}
        </span>
      </div>
      <button
        type="button"
        class="guide-skip"
        @click="emit('skip')"
      >
        <span>{{ t('workflows.task.newConversation.onboarding.skip') }}</span>
        <CloseOutlined aria-hidden="true" />
      </button>
    </div>

    <div
      v-if="phase === 'intro'"
      class="guide-card is-intro"
    >
      <div class="card-title-row">
        <img
          class="spark"
          :src="sparkIcon"
          alt=""
        />
        <h2 class="card-title">{{ t('workflows.task.newConversation.onboarding.introTitle') }}</h2>
      </div>
      <p class="card-body">{{ t('workflows.task.newConversation.onboarding.introBody') }}</p>
      <div class="intro-actions">
        <a-button
          type="primary"
          @click="emit('start')"
        >
          {{ t('workflows.task.newConversation.onboarding.start') }}
        </a-button>
      </div>
      <img
        class="robot robot-intro"
        :src="introRobot"
        alt=""
      />
    </div>

    <div
      v-else
      class="guide-track"
    >
      <template
        v-for="step in steps"
        :key="step.id"
      >
        <div
          v-if="step.id === phase"
          class="guide-card"
        >
          <div class="card-title-row">
            <span class="step-index">{{ step.index }}</span>
            <h2 class="card-title">{{ step.activeTitle }}</h2>
          </div>
          <p
            v-if="phase === 'describe'"
            class="card-body"
          >
            <template v-if="hasPrompt">
              {{ t('workflows.task.newConversation.onboarding.describeBody') }}
            </template>
            <template v-else>
              {{ t('workflows.task.newConversation.onboarding.describeLead') }}
              <button
                type="button"
                class="example-link"
                @click="emit('fillExample')"
              >
                {{ t('workflows.task.newConversation.onboarding.exampleAction') }}
              </button>
              {{ t('workflows.task.newConversation.onboarding.describeTail') }}
            </template>
          </p>
          <p
            v-else
            class="card-body"
          >
            {{ step.body }}
          </p>
          <div
            class="progress"
            aria-hidden="true"
          >
            <span
              v-for="segment in stepCount"
              :key="segment"
              class="progress-segment"
              :class="{ 'is-current': segment === currentStep }"
            />
          </div>
          <img
            class="robot robot-step"
            :src="stepRobot"
            alt=""
          />
        </div>
        <div
          v-else
          class="guide-side"
        >
          <div class="side-main">
            <span class="side-badge">
              <CheckOutlined
                v-if="step.index < currentStep"
                aria-hidden="true"
              />
              <template v-else>{{ step.index }}</template>
            </span>
            <div class="side-copy">
              <p class="side-caption">
                {{
                  step.index < currentStep
                    ? t('workflows.task.newConversation.onboarding.completed')
                    : t('workflows.task.newConversation.onboarding.stepLabel', { step: step.index })
                }}
              </p>
              <p class="side-title">{{ step.collapsedTitle }}</p>
            </div>
          </div>
          <RightOutlined
            class="side-chevron"
            aria-hidden="true"
          />
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CheckOutlined, CloseOutlined, RightOutlined } from '@ant-design/icons-vue'
import { useAppI18n } from '@/i18n'
import type { OnboardingPhase } from '@/composables/useOnboardingGuide'
import introRobot from '@/assets/onboarding-robot-intro.png'
import stepRobot from '@/assets/onboarding-robot-step.png'
import sparkIcon from '@/assets/icons/onboarding-spark.svg'

const props = defineProps<{
  phase: OnboardingPhase
  hasPrompt: boolean
}>()

const emit = defineEmits<{
  skip: []
  start: []
  fillExample: []
}>()

const { t } = useAppI18n()
const stepCount = 3

const currentStep = computed(() => {
  if (props.phase === 'directory') return 2
  if (props.phase === 'describe') return 3
  return 1
})

const steps = computed(() => [
  {
    id: 'execution' as const,
    index: 1,
    activeTitle: t('workflows.task.newConversation.onboarding.executionTitle'),
    collapsedTitle: t('workflows.task.newConversation.onboarding.executionTitle'),
    body: t('workflows.task.newConversation.onboarding.executionBody'),
  },
  {
    id: 'directory' as const,
    index: 2,
    activeTitle: t('workflows.task.newConversation.onboarding.directoryTitle'),
    collapsedTitle: t('workflows.task.newConversation.onboarding.directoryTitle'),
    body: t('workflows.task.newConversation.onboarding.directoryBody'),
  },
  {
    id: 'describe' as const,
    index: 3,
    activeTitle: t('workflows.task.newConversation.onboarding.describeTitle'),
    collapsedTitle: t('workflows.task.newConversation.onboarding.stepDescribe'),
    body: '',
  },
])
</script>

<style scoped>
.onboarding-guide {
  width: 100%;
}

.guide-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
  min-height: 20px;
}

.guide-eyebrow {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
}

.eyebrow-divider {
  width: 1px;
  height: 12px;
  border-radius: 1px;
  background: #d9d9d9;
  flex: none;
}

.revisit-hint {
  min-width: 0;
}

.guide-progress {
  color: #3157e2;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.guide-skip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: none;
  border: none;
  background: transparent;
  padding: 0;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
  cursor: pointer;
}

.guide-skip:hover {
  color: #595959;
}

.guide-skip:focus-visible,
.example-link:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
  border-radius: 4px;
}

.guide-track {
  display: flex;
  align-items: stretch;
  gap: 8px;
  width: 100%;
}

.guide-card {
  position: relative;
  display: flex;
  flex: 1 1 280px;
  flex-direction: column;
  min-width: 0;
  min-height: 138px;
  padding: 16px;
  overflow: hidden;
  background: #ffffff;
  border: 1px solid #c0d6fc;
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.08);
}

.guide-card.is-intro {
  flex: none;
  width: 100%;
  padding-right: 128px;
}

.guide-side {
  position: relative;
  display: flex;
  flex: 0 1 168px;
  flex-direction: column;
  width: 168px;
  min-width: 120px;
  min-height: 138px;
  padding: 12px 14px;
  overflow: hidden;
  background: #f7f8fa;
  border-radius: 14px;
}

.card-title-row,
.side-main {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.side-main {
  gap: 10px;
}

.spark {
  width: 16px;
  height: 16px;
  margin-top: 3px;
  flex: none;
}

.step-index,
.side-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  margin-top: 1px;
  flex: none;
  border-radius: 999px;
  font-size: 12px;
  line-height: 20px;
}

.step-index {
  background: #3157e2;
  color: #ffffff;
}

.side-badge {
  background: #ffffff;
  border: 1px solid #d8dbe1;
  color: #8c8c8c;
  font-size: 12px;
}

.card-title,
.side-title,
.side-caption,
.card-body {
  margin: 0;
}

.card-title {
  color: #23262a;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.card-body {
  margin-top: 4px;
  max-width: calc(100% - 72px);
  color: #595959;
  font-size: 12px;
  line-height: 20px;
}

.guide-card.is-intro .card-body {
  max-width: none;
}

.intro-actions {
  position: relative;
  z-index: 1;
  margin-top: 16px;
}

.example-link {
  border: none;
  background: transparent;
  padding: 0;
  color: #2475fc;
  font: inherit;
  font-size: 12px;
  line-height: 20px;
  cursor: pointer;
}

.example-link:hover {
  text-decoration: underline;
}

.progress {
  display: flex;
  gap: 5px;
  width: min(100%, 326px);
  margin-top: auto;
  padding-top: 12px;
}

.progress-segment {
  flex: 1;
  height: 4px;
  border-radius: 999px;
  background: #e7e9ed;
}

.progress-segment.is-current {
  background: #3157e2;
}

.side-copy {
  min-width: 0;
}

.side-caption {
  color: #8c8c8c;
  font-size: 12px;
  line-height: 16px;
}

.side-title {
  margin-top: 1px;
  color: #23262a;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.side-chevron {
  position: absolute;
  right: 12px;
  bottom: 12px;
  color: #e6e6e6;
  font-size: 22px;
  pointer-events: none;
}

.robot {
  position: absolute;
  object-fit: contain;
  pointer-events: none;
  user-select: none;
}

.robot-intro {
  right: 8px;
  bottom: 0;
  width: 115px;
  height: 80px;
}

.robot-step {
  right: 0;
  bottom: 0;
  width: 80px;
  height: 57px;
}

@media (max-width: 720px) {
  .guide-side {
    display: none;
  }

  .guide-card.is-intro {
    padding-right: 16px;
  }

  .robot {
    display: none;
  }

  .card-body {
    max-width: none;
  }
}
</style>
