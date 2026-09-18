<template>
  <div class="task-page">
    <header class="page-titlebar">
      <div class="page-titlebar-copy">
        <strong>{{ t('workflows.task.common.conversation') }}</strong>
        <p>{{ t('workflows.task.notifications.subtitle') }}</p>
      </div>
      <div class="page-titlebar-actions">
        <a-tooltip
          :title="
            pipelineExpanded
              ? t('workflows.task.notifications.hideOverview')
              : t('workflows.task.notifications.showOverview')
          "
          placement="bottomRight"
        >
          <button
            type="button"
            class="overview-toggle-button"
            :aria-label="
              pipelineExpanded
                ? t('workflows.task.notifications.hideOverview')
                : t('workflows.task.notifications.showOverview')
            "
            :aria-expanded="pipelineExpanded"
            aria-controls="task-overview"
            @click="togglePipelineExpanded"
          >
            <img
              :src="taskOverviewToggleIcon"
              alt=""
              aria-hidden="true"
            />
          </button>
        </a-tooltip>
        <button
          v-if="task"
          type="button"
          class="overview-toggle-button terminal-button"
          :disabled="openingTerminal || !(task.work_dir || task.task_dir)"
          :aria-label="t('workflows.task.progress.openTerminal')"
          :title="t('workflows.task.progress.openTerminal')"
          @click="openSelectedTerminal"
        >
          <CodeOutlined />
        </button>
      </div>
    </header>

    <section
	  v-if="selectedConversation || newConversationPipeline || newConversationExpertGroup || newConversationCodex || newConversationCLI"
      v-show="pipelineExpanded"
      id="task-overview"
      class="task-overview"
      :aria-busy="taskLoading"
    >
      <template v-if="newConversationPipeline">
        <div class="task-overview-head">
          <div class="task-overview-title">
            <h2 :title="t('workflows.task.common.newTask')">
              {{ t('workflows.task.common.newTask') }}
            </h2>
            <span class="task-status">{{ t('workflows.task.common.pendingCreation') }}</span>
          </div>
        </div>
        <div class="task-step-region">
          <AgentStepStrip
            v-if="newConversationSteps.length"
            :steps="newConversationSteps"
            :current-step-index="-1"
            current-step-uuid=""
            selected-step-uuid=""
            read-only
          />
          <div
            v-else
            class="step-region-empty"
          >
            {{ t('workflows.task.notifications.noSteps') }}
          </div>
        </div>
      </template>
	  <template v-else-if="newConversationExpertGroup">
		<div class="task-overview-head"><div class="task-overview-title"><h2>{{ t('workflows.task.common.newTask') }}</h2><span class="task-status">{{ t('workflows.task.common.pendingCreation') }}</span></div></div>
		<div class="task-step-region"><div class="expert-overview"><strong>{{ newConversationExpertGroup.name }}</strong><span>{{ `${t('expertGroups.leader')}：${newConversationExpertGroup.leader?.name || ''}` }}</span><span>{{ t('expertGroups.members', { count: newConversationExpertGroup.members.length }) }}</span></div></div>
	  </template>
	  <template v-else-if="newConversationCodex">
		<div class="task-overview-head">
		  <div class="task-overview-title">
			<h2>{{ t('workflows.task.common.newTask') }}</h2>
			<span class="task-status">{{ t('workflows.task.common.pendingCreation') }}</span>
		  </div>
		</div>
		<div class="task-step-region">
		  <div class="expert-overview">
			<strong>{{ t('workflows.task.assign.vibeCodex') }}</strong>
		  </div>
		</div>
	  </template>
      <template v-else-if="task">
        <div class="task-overview-head">
          <div class="task-overview-title">
            <h2 :title="task.title">{{ task.title }}</h2>
            <span
              class="task-status"
              :class="`status-${taskStatus}`"
              >{{ taskStatusText }}</span
            >
            <span
              v-if="taskPriorityText"
              class="task-priority"
              >{{ taskPriorityText }}</span
            >
          </div>
        </div>
        <div class="task-step-region">
		  <div v-if="isExpertGroup" class="expert-overview"><strong>{{ task.expert_group_name_snapshot }}</strong><span>{{ `${t('expertGroups.leader')}：${sortedSteps.find(step => step.member_role === 'leader')?.name || ''}` }}</span><span>{{ t('expertGroups.members', { count: sortedSteps.filter(step => step.member_role === 'member').length }) }}</span></div>
          <AgentStepStrip
			v-else-if="hasPipelineSnapshot"
            :steps="sortedSteps"
            :current-step-index="currentStepIndex"
            :current-step-uuid="effectiveCurrentStepUuid"
            :selected-step-uuid="selectedStepUuid"
            @select="selectStep"
          />
          <div
            v-else-if="isVibeCoding"
            class="direct-execution-summary"
          >
            <img
              class="direct-execution-summary__logo"
              :src="vibeCodingLogo"
              alt=""
              aria-hidden="true"
            />
            <span class="direct-execution-summary__label">{{
              t('workflows.task.common.directExecution')
            }}</span>
            <strong>{{ `${t('workflows.task.create.vibeCodingMode')} · ${vibeCodingToolName}` }}</strong>
          </div>
          <div
            v-else-if="isCLI"
            class="direct-execution-summary"
          >
            <img
              class="direct-execution-summary__logo"
              :src="cliExecutionLogo"
              alt=""
              aria-hidden="true"
            />
            <span class="direct-execution-summary__label">{{
              t('workflows.task.common.directExecution')
            }}</span>
            <strong>{{ cliRuntimeLabel }}</strong>
          </div>
          <div
            v-else
            class="step-region-empty"
          >
            {{ t('workflows.task.common.unassignedPipeline') }}
          </div>
        </div>
      </template>
      <!-- 新建 CLI 对话此时还没有 task，必须单独成支：否则会落到下面的加载骨架屏，
           页面明明没在加载却一直显示灰块 -->
      <template v-else-if="newConversationCLI">
        <div class="task-overview-head">
          <div class="task-overview-title">
            <h2 :title="t('workflows.task.common.newTask')">
              {{ t('workflows.task.common.newTask') }}
            </h2>
            <span class="task-status">{{ t('workflows.task.common.pendingCreation') }}</span>
          </div>
        </div>
        <div class="task-step-region">
          <div class="direct-execution-summary">
            <img
              class="direct-execution-summary__logo"
              :src="cliExecutionLogo"
              alt=""
              aria-hidden="true"
            />
            <span class="direct-execution-summary__label">{{
              t('workflows.task.common.directExecution')
            }}</span>
            <strong>{{ cliRuntimeSummary || t('workflows.task.assign.cliMode') }}</strong>
          </div>
        </div>
      </template>
      <div
        v-else
        class="task-overview-skeleton"
        aria-hidden="true"
      >
        <div class="overview-skeleton-head">
          <span class="overview-skeleton-title" />
        </div>
        <div class="overview-skeleton-steps">
          <span
            v-for="index in 3"
            :key="index"
          />
        </div>
      </div>
    </section>

    <div
      ref="taskLayoutRef"
      class="task-layout"
      :class="{ 'is-resizing-sidebar': sidebarDragging }"
    >
      <aside
        ref="conversationSidebarRef"
        class="conversation-sidebar"
        :style="
          sidebarResized
            ? { width: `${sidebarWidth}px`, flexBasis: `${sidebarWidth}px` }
            : undefined
        "
      >
        <div class="sidebar-heading">
          <span>{{ t('workflows.task.common.conversation') }}</span>
          <small>{{ displayedConversationCount }}</small>
          <a-dropdown
            v-model:open="pipelineMenuOpen"
            :trigger="['click']"
            placement="bottomRight"
            :get-popup-container="getPipelinePopupContainer"
            @open-change="handlePipelineMenuOpenChange"
          >
            <button
              type="button"
              class="sidebar-add-button"
              :aria-label="t('workflows.task.notifications.viewPipeline')"
              :aria-expanded="pipelineMenuOpen"
            >
              <PlusOutlined />
            </button>
            <template #overlay>
              <div
                class="pipeline-menu"
                role="menu"
                :aria-label="t('workflows.task.common.pipeline')"
              >
                <div class="pipeline-menu-heading">
                  <PipelineFlowIcon />
                  <span>{{ t('workflows.task.common.pipeline') }}</span>
                </div>
                <div
                  v-if="pipelinesLoading"
                  class="pipeline-menu-state"
                >
                  <a-spin size="small" />
                </div>
                <div
                  v-else-if="pipelinesError"
                  class="pipeline-menu-state pipeline-menu-error"
                >
                  <span>{{ pipelinesError }}</span>
                  <button
                    type="button"
                    @click.stop="loadPipelines(true)"
                  >
                    <ReloadOutlined />{{ t('common.actions.retry') }}
                  </button>
                </div>
                <div
                  v-else-if="!pipelines.length"
                  class="pipeline-menu-state"
                >
                  {{ t('workflows.task.common.noPipeline') }}
                </div>
                <div
                  v-else
                  class="pipeline-menu-list"
                >
                  <button
                    v-for="pipeline in pipelines"
                    :key="pipeline.uuid"
                    type="button"
                    role="menuitem"
                    class="pipeline-menu-item"
                    :title="pipeline.name"
                    @click="startNewConversation(pipeline)"
                  >
                    <img
                      v-if="pipeline.avatar"
                      :src="pipeline.avatar"
                      alt=""
                    />
                    <span
                      v-else
                      class="pipeline-avatar-fallback"
                      >{{ pipelineInitials(pipeline.name) }}</span
                    >
                    <span class="pipeline-menu-name">{{ pipeline.name }}</span>
                  </button>
                </div>
				<div class="pipeline-menu-heading">
				  <TeamOutlined />
				  <span>{{ t('agents.expertTeam') }}</span>
				</div>
				<div v-if="expertGroupsLoading" class="pipeline-menu-state"><a-spin size="small" /></div>
				<div v-else-if="expertGroupsError" class="pipeline-menu-state pipeline-menu-error">{{ expertGroupsError }}</div>
				<div v-else class="pipeline-menu-list">
				  <button v-for="group in expertGroups" :key="`expert-${group.uuid}`" type="button" role="menuitem" class="pipeline-menu-item" :disabled="!group.ready" @click="startNewExpertConversation(group)"><img v-if="group.avatar" :src="group.avatar" alt="" /><span v-else class="pipeline-avatar-fallback">{{ pipelineInitials(group.name) }}</span><span class="pipeline-menu-name">{{ group.name }}</span></button>
				</div>
				<div class="pipeline-menu-heading">
				  <img
					class="pipeline-menu-heading-logo"
					:src="vibeCodingLogo"
					alt=""
				  />
				  <span>{{ t('workflows.task.create.vibeCodingMode') }}</span>
				</div>
				<a-tooltip
				  :title="
					codexCapability.available
					  ? ''
					  : codexCapability.message || t('workflows.task.codex.capabilityUnavailable')
				  "
				  placement="right"
				>
				  <span class="pipeline-menu-tooltip">
					<button
					  type="button"
					  role="menuitem"
					  class="pipeline-menu-item"
					  :disabled="!codexCapability.available"
					  @click="startNewCodexConversation"
					>
					  <img :src="codexLogo" alt="" />
					  <span class="pipeline-menu-name">{{ t('workflows.task.notifications.createCodexConversation') }}</span>
					</button>
				  </span>
				</a-tooltip>
				<!-- 直接执行：CLI 与模型下拉内嵌在菜单内，与需求设计图一致 -->
				<div class="pipeline-menu-heading">
				  <img
					class="pipeline-menu-heading-logo"
					:src="cliExecutionLogo"
					alt=""
				  />
				  <span>{{ t('workflows.task.common.directExecution') }}</span>
				</div>
				<div
				  class="direct-cli"
				  @click.stop
				>
				  <a-select
					class="direct-cli__select"
					:value="newConversationCliType || undefined"
					:placeholder="t('workflows.task.assign.chooseCli')"
					:options="cliSelectOptions"
					:loading="cliLoading"
					@change="chooseNewConversationCLIType"
				  />
				  <a-select
					class="direct-cli__select"
					:value="newConversationCliModel || undefined"
					:placeholder="
					  newConversationCliType
						? t('workflows.task.assign.chooseModel')
						: t('workflows.task.assign.chooseCliFirst')
					"
					:options="modelSelectOptions"
					:loading="modelLoading"
					:disabled="!newConversationCliType"
					@change="chooseNewConversationCLIModel"
				  />
				  <a-button
					type="primary"
					block
					class="direct-cli__submit"
					:disabled="!canCreateNewCLIConversation"
					@click="createNewCLIConversation"
				  >
					{{ t('workflows.task.notifications.createConversation') }}
				  </a-button>
				  <p
					v-if="!cliLoading && !cliSelectOptions.some((item) => !item.disabled)"
					class="cli-runtime-hint"
				  >
					{{ t('workflows.task.assign.noCli') }}
				  </p>
				</div>
              </div>
            </template>
          </a-dropdown>
        </div>

        <div class="conversation-list scrollbar--subtle">
          <div
			v-if="newConversationPipeline || newConversationExpertGroup || newConversationCodex || newConversationCLI"
            class="conversation-item active new-conversation-item"
            aria-current="true"
          >
            <span class="conversation-title">
              <strong :title="t('workflows.task.common.newTask')">{{
                t('workflows.task.common.newTask')
              }}</strong>
            </span>
            <span
			  v-if="newConversationSteps[0] || newConversationExpertGroup?.leader || newConversationCodex || newConversationCLI"
              class="conversation-subtitle"
			  :title="conversationStatusLabel(newConversationActorName, t('workflows.task.execution.created'))"
			>
			  {{ conversationStatusLabel(newConversationActorName, t('workflows.task.execution.created')) }}
            </span>
          </div>
          <div
            v-if="notificationsLoading && !conversations.length"
            class="sidebar-loading"
          >
            <a-spin size="small" />
          </div>
          <div
            v-else-if="notificationsError && !conversations.length"
            class="sidebar-error"
          >
            <span>{{ notificationsError }}</span>
            <button
              type="button"
              @click="loadNotifications(false)"
            >
              <ReloadOutlined />{{ t('common.actions.retry') }}
            </button>
          </div>
          <div
            v-for="conversation in conversations"
            :key="conversation.task_uuid"
            class="conversation-item"
            :class="{ active: selectedTaskUuid === conversation.task_uuid }"
            @contextmenu="openContextMenu($event, conversation)"
          >
            <button
              type="button"
              class="conversation-select-button"
              @click="selectConversation(conversation)"
            >
              <span class="conversation-title">
                <strong :title="conversation.title">{{ conversation.title }}</strong>
                <i
                  v-if="conversation.unread"
                  :title="t('workflows.task.common.unread')"
                />
              </span>
              <span class="conversation-subtitle">
				{{ conversationStatusLabel(conversationActorName(conversation.latest), terminalLabel(conversation.latest.status || conversation.latest.terminal_status)) }}
              </span>
            </button>
            <button
              type="button"
              class="conversation-menu-button"
              :aria-label="t('workflows.task.common.moreActions')"
              aria-haspopup="menu"
              aria-controls="conversation-context-menu"
              :aria-expanded="contextTaskUuid === conversation.task_uuid"
              @click.stop="toggleConversationMenu($event, conversation)"
            >
              <img
                :src="conversationMenuIcon"
                alt=""
                aria-hidden="true"
              />
            </button>
          </div>
          <div
            v-if="
			  !newConversationPipeline && !newConversationExpertGroup && !newConversationCodex && !newConversationCLI &&
              !conversations.length &&
              !notificationsLoading &&
              !notificationsError
            "
            class="sidebar-empty"
          >
            <FolderOutlined />
            <span>{{ t('workflows.task.notifications.noConversations') }}</span>
          </div>
        </div>
        <div
          class="sidebar-resize-handle"
          role="separator"
          tabindex="0"
          :aria-label="t('workflows.task.notifications.resize')"
          aria-orientation="vertical"
          :aria-valuemin="MIN_SIDEBAR_WIDTH"
          :aria-valuemax="sidebarMaxWidth"
          :aria-valuenow="Math.round(sidebarWidth)"
          :aria-valuetext="
            t('workflows.task.notifications.pixels', { count: Math.round(sidebarWidth) })
          "
          @pointerdown="startSidebarResize"
          @pointermove="handleSidebarResize"
          @pointerup="finishSidebarResize"
          @pointercancel="finishSidebarResize"
          @lostpointercapture="finishSidebarResize"
          @keydown="handleSidebarResizeKeydown"
        />
      </aside>

      <main
        class="conversation-main"
        :aria-busy="initialTaskLoading || taskRefreshing"
      >
        <template v-if="newConversationPipeline">
          <div class="main-state new-conversation-state">
            <PipelineFlowIcon />
            <h3>
              {{
                t('workflows.task.notifications.startTitle', {
                  pipeline: newConversationPipeline.name,
                })
              }}
            </h3>
            <p>{{ t('workflows.task.notifications.startDescription') }}</p>
          </div>
          <ChatComposer
            ref="newConversationComposerRef"
            v-model="newConversationQuestion"
            :can-ask="true"
            :submitting="submitting"
            :placeholder="t('workflows.task.notifications.inputPlaceholder')"
            :context-text="
              t('workflows.task.notifications.createContext', {
                pipeline: newConversationPipeline.name,
              })
            "
            task-uuid=""
            show-work-directory
            :work-directory="newConversationWorkDir"
            @submit="submitNewConversation"
            @select-work-directory="chooseNewConversationDirectory"
          />
        </template>
		<template v-else-if="newConversationExpertGroup">
		  <div class="main-state new-conversation-state">
			<h3>{{ t('workflows.task.notifications.startTitle', { pipeline: newConversationExpertGroup.name }) }}</h3>
			<p>{{ t('expertGroups.copyIndependent') }}</p>
		  </div>
		  <ChatComposer ref="newConversationComposerRef" v-model="newConversationQuestion" :can-ask="true" :submitting="submitting" :placeholder="t('workflows.task.notifications.inputPlaceholder')" :context-text="newConversationExpertGroup.name" task-uuid="" show-work-directory :work-directory="newConversationWorkDir" @submit="submitNewExpertConversation" @select-work-directory="chooseNewConversationDirectory" />
		</template>
		<template v-else-if="newConversationCodex">
		  <div class="main-state new-conversation-state" aria-hidden="true" />
		  <ChatComposer
			ref="newConversationComposerRef"
			v-model="newConversationQuestion"
			:can-ask="true"
			:submitting="submitting"
			:placeholder="t('workflows.task.notifications.codexInputPlaceholder')"
			:context-text="t('workflows.task.assign.vibeCodex')"
			task-uuid=""
			show-work-directory
			:work-directory="newConversationWorkDir"
			:submit-button-label="t('workflows.task.notifications.openCodex')"
			@submit="submitNewCodexConversation"
			@select-work-directory="chooseNewConversationDirectory"
		  />
		</template>
		<template v-else-if="newConversationCLI">
		  <div class="main-state new-conversation-state">
			<h3>{{ t('workflows.task.notifications.startTitle', { pipeline: t('workflows.task.assign.cliMode') }) }}</h3>
			<p>{{ t('workflows.task.assign.cliCardHint') }}</p>
			<!-- CLI 与模型在「新建对话」菜单内选定，这里只回显已选执行目标 -->
			<p
			  v-if="cliRuntimeSummary"
			  class="cli-runtime-hint cli-runtime-hint--selected"
			>{{ cliRuntimeSummary }}</p>
		  </div>
		  <ChatComposer ref="newConversationComposerRef" v-model="newConversationQuestion" :can-ask="canSubmitNewCLIConversation" :submitting="submitting" :placeholder="t('workflows.task.notifications.inputPlaceholder')" :context-text="t('workflows.task.assign.cliMode')" task-uuid="" :hide-agent-prompt="true" show-work-directory :work-directory="newConversationWorkDir" @submit="submitNewCLIConversation" @select-work-directory="chooseNewConversationDirectory" />
		</template>
        <template v-else-if="selectedConversation || selectedTaskUuid">
          <div
            v-if="taskError && !task"
            class="main-state main-error"
          >
            <span>{{ taskError }}</span>
            <button
              type="button"
              @click="loadSelectedTask()"
            >
              <ReloadOutlined />{{ t('common.actions.retry') }}
            </button>
          </div>
          <div
            v-else-if="initialTaskLoading || !task"
            class="conversation-loading-shell"
            aria-hidden="true"
          >
            <div class="message-scroll message-loading-skeleton scrollbar--subtle">
              <a-skeleton
                active
                :title="false"
                :paragraph="{ rows: 5 }"
              />
            </div>
            <div class="composer-loading-skeleton">
              <div class="composer-loading-body" />
            </div>
          </div>
		  <template v-else-if="task && (hasPipelineSnapshot || isVibeCoding || isExpertGroup || isCLI)">
			<NextStepButton
			  v-if="!isVibeCoding && !isExpertGroup && !isCLI && showNextStepCard"
              :next-step-name="nextStepName"
              :is-last-step="isLastStep"
              :disabled="!canComplete"
              :completing="completing"
              @confirm="completeStep"
            />
            <div
              class="message-scroll scrollbar--subtle"
              :aria-busy="taskRefreshing"
            >
              <div
                v-if="taskRefreshing"
                class="message-refresh-indicator"
                role="status"
                :aria-label="t('workflows.task.notifications.refreshing')"
              >
                <a-spin
                  :spinning="taskRefreshing"
                  :delay="150"
                  size="small"
                />
              </div>
              <StepMessageList
				:items="isVibeCoding ? progress : selectedProgress"
				:steps="isVibeCoding ? [] : sortedSteps"
                :highlight-uuid="highlightUuid"
				:selected-step-name="isVibeCoding ? vibeCodingToolName : selectedStep?.name || ''"
				:fallback-actor-name="isVibeCoding ? vibeCodingToolName : ''"
				:fallback-actor-logo="isVibeCoding ? vibeCodingActorLogo : ''"
                :task-uuid="selectedTaskUuid"
                @copy="copyResult"
              />
            </div>
            <VibeCodingConversationNotice
              v-if="isVibeCoding"
              :tool-name="vibeCodingToolName"
              :show-open-button="task?.execution_tool === 'codex'"
              :opening="codexBusy"
              @open="handleOpenCodex"
            />
            <ChatComposer
			  v-else
              ref="composerRef"
              v-model="question"
              :can-ask="canAsk"
              :submitting="submitting"
              :running="Boolean(activeSelectedProgress)"
              :stopping="stoppingSessionUuid === activeSelectedProgress?.session_uuid"
              :cli-type="composerCliType"
              :model-name="composerModelName"
              :start-mode="selectedStepAwaitingStart"
              :placeholder="composerPlaceholder"
              :context-text="composerContextText"
              :task-uuid="selectedTaskUuid"
			  :current-step="isExpertGroup ? undefined : selectedStep"
			  :document-step="isExpertGroup ? currentStep : selectedStep"
              :steps="sortedSteps"
			  :expert-members="isExpertGroup ? sortedSteps : []"
              :executing-step-uuid="effectiveCurrentStepUuid"
              :hide-agent-prompt="isCLI"
			  @submit="isExpertGroup ? submitExpertMessage($event) : submitQuestion($event)"
              @stop="stopSelectedConversation"
              @prompt-saved="loadSelectedTask"
            />
          </template>
          <div
            v-else-if="task"
            class="main-state"
            :aria-busy="taskRefreshing"
          >
            <div
              v-if="taskRefreshing"
              class="main-refresh-indicator"
              role="status"
              :aria-label="t('workflows.task.notifications.refreshing')"
            >
              <a-spin
                :spinning="taskRefreshing"
                :delay="150"
                size="small"
              />
            </div>
            <FolderOutlined />
            <h3>{{ t('workflows.task.common.unassignedPipeline') }}</h3>
            <p>{{ t('workflows.task.notifications.unassignedDescription') }}</p>
          </div>
        </template>
        <div
          v-else
          class="main-state"
        >
          <CheckCircleOutlined />
          <h3>{{ t('workflows.task.notifications.emptyTitle') }}</h3>
          <p>{{ t('workflows.task.notifications.emptyDescription') }}</p>
        </div>
      </main>
    </div>

    <div
      v-if="contextTaskUuid"
      id="conversation-context-menu"
      class="context-menu"
      role="menu"
      :style="{ left: `${contextX}px`, top: `${contextY}px` }"
      @click.stop
    >
      <button
        type="button"
        role="menuitem"
        @click="openConversationDetail"
      >
        <img
          :src="contextDetailIcon"
          alt=""
          aria-hidden="true"
        />{{ t('workflows.task.common.details') }}
      </button>
      <button
        type="button"
        role="menuitem"
        @click="toggleRead"
      >
        <img
          :src="contextReadIcon"
          alt=""
          aria-hidden="true"
        />{{
          contextConversation?.unread
            ? t('workflows.task.common.markRead')
            : t('workflows.task.common.markUnread')
        }}
      </button>
      <button
        type="button"
        role="menuitem"
        @click="archiveConversation"
      >
        <img
          :src="contextArchiveIcon"
          alt=""
          aria-hidden="true"
        />{{ t('workflows.task.common.archive') }}
      </button>
    </div>

    <TaskDetailInfoModal
      v-model:open="detailModalOpen"
      :task="task"
      :status="taskStatus"
      :rendered-description="renderedDescription"
      @preview-image="showImagePreview"
      @saved="handleTaskSaved"
    />
    <StopExecutionConfirmModal
      :open="stopConfirmOpen"
      :loading="Boolean(stoppingSessionUuid)"
      @close="closeStopConfirm"
      @confirm="confirmStopConversation"
    />
    <TaskImagePreviewModal
      v-model:open="previewImageVisible"
      :image-url="previewImageUrl"
    />
    <AssignPipelineModal
      v-model:open="pipelineConfigModalOpen"
      :preferred-pipeline-uuid="newConversationPipeline?.uuid || ''"
      mode="create"
      @selected="handleCreationPipelineSelected"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRaw, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { message } from 'ant-design-vue'
import {
  CheckCircleOutlined,
  CodeOutlined,
  FolderOutlined,
  PlusOutlined,
  ReloadOutlined,
  TeamOutlined,
} from '@ant-design/icons-vue'
import MarkdownIt from 'markdown-it'
import apiClient, { ApiError } from '@/api/client'
import cliExecutionLogo from '@/assets/icons/task-composer-cli.svg'
import vibeCodingLogo from '@/assets/icons/vibe-coding-logo.svg'
import conversationMenuIcon from '@/assets/icons/common-more-actions.svg'
import contextArchiveIcon from '@/assets/icons/task-context-archive.svg'
import contextDetailIcon from '@/assets/icons/task-context-detail.svg'
import contextReadIcon from '@/assets/icons/task-context-read.svg'
import taskOverviewToggleIcon from '@/assets/icons/task-overview-toggle.svg'
import codexLogo from '@/assets/icons/codex-logo.svg'
import AssignPipelineModal from '@/components/AssignPipelineModal.vue'
import AgentStepStrip from '@/components/task-progress/AgentStepStrip.vue'
import ChatComposer from '@/components/task-progress/ChatComposer.vue'
import NextStepButton from '@/components/task-progress/NextStepButton.vue'
import PipelineFlowIcon from '@/components/task-progress/PipelineFlowIcon.vue'
import StepMessageList from '@/components/task-progress/StepMessageList.vue'
import StopExecutionConfirmModal from '@/components/task-progress/StopExecutionConfirmModal.vue'
import VibeCodingConversationNotice from '@/components/task-progress/VibeCodingConversationNotice.vue'
import { copyText } from '@/utils/clipboard'
import { isStopConfirmSuppressed, suppressStopConfirm } from '@/utils/stopConfirm'
import { selectDirectory, openTerminal } from '@/composables/useDesktop'
import {
  loadCodexCapability,
  openTaskInCodex,
  type CodexCapability,
} from '@/composables/useTaskCodex'
import { useLocalWS, useLocalWSStatus } from '@/composables/useLocalWebSocket'
import { usePipelineStore } from '@/stores/pipeline'
import { useExpertGroupStore } from '@/stores/expert-group'
import { useCliModelOptions } from '@/composables/useCliModelOptions'
import { buildCliKickoffPrompt, pendingCliKickoffUuid } from '@/composables/useCliKickoff'
import { useAppStore } from '@/stores/app'
import type {
  CompleteStepResponse,
	ExpertGroup,
  Pipeline,
  TaskNotification,
  TaskProgress,
} from '@/types/pipeline'
import type { ChatComposerSubmission } from '@/types/task-attachments'
import { isUserMessage, resultText, userMessageText } from '@/components/task-progress/utils'
import type { TaskWithDetails } from '@/types/task-detail'
import {
  deleteTaskView,
  deleteTaskViewsOutside,
  readTaskConversationCache,
  saveTaskConversationViewState,
  saveTaskNotificationSnapshot,
  saveTaskView,
} from '@/utils/taskConversationCache'
import TaskDetailInfoModal from '@/views/workflows/components/TaskDetailInfoModal.vue'
import TaskImagePreviewModal from '@/views/workflows/components/TaskImagePreviewModal.vue'
import { useAppI18n } from '@/i18n'

interface TaskConversation {
  task_uuid: string
  title: string
  items: TaskNotification[]
  latest: TaskNotification
  unread: boolean
}

interface CreateTaskNotification {
  task_uuid: string
  task_title: string
  step_name: string
  session_uuid: string
  status: 'created'
  summary: string
  created_at: number
}

const appStore = useAppStore()
const pipelineStore = usePipelineStore()
const { pipelines, loading: pipelinesLoading, error: pipelinesError } = storeToRefs(pipelineStore)
const expertGroupStore = useExpertGroupStore()
const { items: expertGroups, loading: expertGroupsLoading, error: expertGroupsError } = storeToRefs(expertGroupStore)
const route = useRoute()
const router = useRouter()
const { t } = useAppI18n()

type CreatedTaskResponse = TaskWithDetails & {
  notification?: CreateTaskNotification
}

interface TaskViewCache {
  task: TaskWithDetails
  progress: TaskProgress[]
}

const notificationsLoading = ref(false)
const notificationsError = ref('')
const notifications = ref<TaskNotification[]>([])
const temporaryNotifications = ref<TaskNotification[]>([])
watch(() => [...notifications.value, ...temporaryNotifications.value].filter((item) => !item.is_read).length,
  (count) => appStore.setUnreadTaskNotifications(count))
const pendingReadTasks = new Map<string, { isRead: boolean; notificationUuids: Set<string> }>()
const selectedTaskUuid = ref('')
const selectedStepUuid = ref('')
const selectedProgressUuid = ref('')

const taskLoading = ref(false)
const taskError = ref('')
const task = ref<TaskWithDetails>()
const progress = ref<TaskProgress[]>([])
const taskViewCache = new Map<string, TaskViewCache>()
const submitting = ref(false)
const completing = ref(false)
const stoppingSessionUuid = ref('')
const stopConfirmOpen = ref(false)
const stopConfirmSessionUuid = ref('')
const question = ref('')
const composerRef = ref<InstanceType<typeof ChatComposer>>()
const highlightUuid = ref('')
const pipelineExpanded = ref(true)
const openingTerminal = ref(false)
const codexBusy = ref(false)
const taskLayoutRef = ref<HTMLElement>()
const conversationSidebarRef = ref<HTMLElement>()
const sidebarWidth = ref(300)
const sidebarMaxWidth = ref(300)
const sidebarResized = ref(false)
const sidebarDragging = ref(false)

const pipelineMenuOpen = ref(false)
const newConversationPipeline = ref<Pipeline>()
const newConversationExpertGroup = ref<ExpertGroup>()
const newConversationCodex = ref(false)
const newConversationCLI = ref(false)
const newConversationCliType = ref('')
const newConversationCliModel = ref('')
const newConversationQuestion = ref('')
const newConversationWorkDir = ref('')
const newConversationComposerRef = ref<InstanceType<typeof ChatComposer>>()
const newConversationSubmission = ref<ChatComposerSubmission>()
const codexCapability = ref<CodexCapability>({ available: false })
const codexCapabilityLoaded = ref(false)

// CLI 直接执行：CLI 与模型都由用户当场选定后才能发起对话。
const {
  cliLoading,
  cliOptions,
  loadCliOptions,
  loadModelOptions,
  modelLoading,
  modelOptions,
} = useCliModelOptions()
const cliSelectOptions = computed(() =>
  cliOptions.value.map((cli) => ({ value: cli.type, label: cli.name, disabled: !cli.installed })),
)
const modelSelectOptions = computed(() =>
  modelOptions.value.map((model) => ({ value: model, label: model })),
)
const canSubmitNewCLIConversation = computed(
  () => Boolean(newConversationCliType.value && newConversationCliModel.value),
)
// 菜单内「创建对话」按钮与首条消息提交共用同一份 CLI/模型校验。
const canCreateNewCLIConversation = computed(() => canSubmitNewCLIConversation.value)
const cliRuntimeSummary = computed(() => {
  if (!newConversationCliType.value || !newConversationCliModel.value) return ''
  const cli = cliOptions.value.find((item) => item.type === newConversationCliType.value)
  return `${cli?.name || newConversationCliType.value} · ${newConversationCliModel.value}`
})
const pipelineConfigModalOpen = ref(false)

// 评论 4：对话-新建对话记住上次选择的 CLI 与模型，下次默认填入。
const CLI_PREFERENCE_KEY = 'goteams.conversation.cliPreference'

function readCliPreference(): { cliType: string; model: string } {
  try {
    const raw = localStorage.getItem(CLI_PREFERENCE_KEY)
    if (!raw) return { cliType: '', model: '' }
    const parsed = JSON.parse(raw) as { cliType?: unknown; model?: unknown }
    return {
      cliType: typeof parsed.cliType === 'string' ? parsed.cliType : '',
      model: typeof parsed.model === 'string' ? parsed.model : '',
    }
  } catch {
    return { cliType: '', model: '' }
  }
}

function writeCliPreference(cliType: string, model: string) {
  try {
    localStorage.setItem(CLI_PREFERENCE_KEY, JSON.stringify({ cliType, model }))
  } catch {
    // 存储不可用时静默降级：记忆只是体验增强，不影响功能。
  }
}

const contextTaskUuid = ref('')
const contextX = ref(0)
const contextY = ref(0)
const detailModalOpen = ref(false)
const previewImageUrl = ref('')
const previewImageVisible = ref(false)

let refreshTimer: ReturnType<typeof setTimeout> | undefined
let highlightTimer: ReturnType<typeof setTimeout> | undefined
let taskLoadVersion = 0
let notificationsLoadVersion = 0
let persistentCacheAvailable = true
let notificationsHydrated = false
let lastNotifiedTaskUuids = new Set<string>()
let sidebarResizeState:
  | {
      pointerId: number
      pointerX: number
      width: number
    }
  | undefined

const MIN_SIDEBAR_WIDTH = 140
const MAX_SIDEBAR_WIDTH = 500
const SIDEBAR_KEYBOARD_STEP = 10
const SIDEBAR_WIDTH_STORAGE_KEY = 'goteams.tasks.conversationSidebarWidth'

const wsConnected = useLocalWSStatus()

const taskStatusLabels = computed<Record<string, string>>(() => ({
  pending: t('workflows.task.status.pending'),
  in_progress: t('workflows.task.status.inProgress'),
  blocked: t('workflows.task.status.blocked'),
  done: t('workflows.task.status.done'),
}))
const taskPriorityLabels = computed<Record<string, string>>(() => ({
  urgent: t('workflows.task.priority.urgent'),
  high: t('workflows.task.priority.high'),
  medium: t('workflows.task.priority.medium'),
  low: t('workflows.task.priority.low'),
}))
const terminalLabels = computed<Record<string, string>>(() => ({
  created: t('workflows.task.execution.created'),
  running: t('workflows.task.execution.running'),
  success: t('workflows.task.execution.success'),
  failed: t('workflows.task.execution.failed'),
  stopped: t('workflows.task.execution.stopped'),
  interrupted: t('workflows.task.execution.interrupted'),
}))

const conversations = computed<TaskConversation[]>(() => {
  const grouped = new Map<string, TaskNotification[]>()
  for (const item of [...temporaryNotifications.value, ...notifications.value]) {
    const items = grouped.get(item.task_uuid) || []
    items.push(item)
    grouped.set(item.task_uuid, items)
  }
  return [...grouped.entries()]
    .map(([taskUuid, items]) => {
      items.sort((a, b) => b.created_at - a.created_at)
      const latest = items[0]
      return {
        task_uuid: taskUuid,
        title:
          latest.task_title ||
          latest.title ||
          t('workflows.task.feedback.taskFallback', { id: taskUuid.slice(0, 8) }),
        items,
        latest,
        unread: items.some((item) => !item.is_read),
      }
    })
    .sort((a, b) => b.latest.created_at - a.latest.created_at)
})
const displayedConversationCount = computed(
	() => conversations.value.length + (newConversationPipeline.value || newConversationExpertGroup.value || newConversationCodex.value || newConversationCLI.value ? 1 : 0),
)
const newConversationSteps = computed(() =>
  [...(newConversationPipeline.value?.steps || [])].sort((a, b) => a.sort_order - b.sort_order),
)
const newConversationActorName = computed(() => {
  if (newConversationSteps.value[0]?.name) return newConversationSteps.value[0].name
  if (newConversationExpertGroup.value?.leader?.name) return newConversationExpertGroup.value.leader.name
  if (newConversationCodex.value) return t('workflows.task.assign.vibeCodex')
  return t('workflows.task.assign.cliMode')
})

const selectedConversation = computed(() =>
  conversations.value.find((item) => item.task_uuid === selectedTaskUuid.value),
)
const contextConversation = computed(() =>
  conversations.value.find((item) => item.task_uuid === contextTaskUuid.value),
)
const sortedSteps = computed(() =>
  [...(task.value?.steps || [])].sort((a, b) => a.sort_order - b.sort_order),
)
const hasPipelineSnapshot = computed(() =>
	Boolean(task.value?.execution_mode === 'pipeline' && (task.value?.pipeline_snapshot_uuid || sortedSteps.value.length)),
)
const isVibeCoding = computed(
  () => task.value?.execution_mode === 'vibe_coding',
)
const vibeCodingToolName = computed(() =>
  task.value?.execution_tool === 'codex'
    ? t('workflows.task.assign.codex')
    : task.value?.execution_tool || t('workflows.task.create.vibeCodingMode'),
)
const vibeCodingActorLogo = computed(() =>
  task.value?.execution_tool === 'codex' ? codexLogo : vibeCodingLogo,
)
const isExpertGroup = computed(() => task.value?.execution_mode === 'expert_group')
// 需求 2204：CLI 直接执行没有流水线与多 Agent，对话页也要能展示该任务的动态与输入框。
// 需求 2202 评论 4：直接执行 CLI 时输入框不展示「Agent 提示词」入口。
const isCLI = computed(() => task.value?.execution_mode === 'cli')
// CLI 任务的执行目标固定展示为「CLI · 模型」。
const cliRuntimeLabel = computed(() => {
  const tool = task.value?.execution_tool || ''
  const cli = cliOptions.value.find((item) => item.type === tool)
  const model = task.value?.execution_model || sortedSteps.value[0]?.model_name || ''
  return [cli?.name || tool, model].filter(Boolean).join(' · ')
})
const effectiveCurrentStepUuid = computed(
  () =>
    task.value?.current_step_uuid ||
    sortedSteps.value.find((step) => step.status === 'active')?.uuid ||
    sortedSteps.value[0]?.uuid ||
    '',
)
const currentStep = computed(() =>
  sortedSteps.value.find((step) => step.uuid === effectiveCurrentStepUuid.value),
)
const currentStepIndex = computed(() =>
  sortedSteps.value.findIndex((step) => step.uuid === effectiveCurrentStepUuid.value),
)
const selectedStep = computed(() =>
  sortedSteps.value.find((step) => step.uuid === selectedStepUuid.value),
)
const selectedStepIndex = computed(() =>
  sortedSteps.value.findIndex((step) => step.uuid === selectedStepUuid.value),
)
const selectedProgress = computed(() =>
	isExpertGroup.value ? progress.value : progress.value.filter((item) => item.task_step_uuid === selectedStepUuid.value),
)
const latestSelectedAgentProgress = computed(() =>
  [...selectedProgress.value].reverse().find((item) => !isUserMessage(item)),
)
const activeSelectedProgress = computed(() =>
  [...selectedProgress.value]
    .reverse()
    .find(
      (item) => !isUserMessage(item) && (item.status === 'created' || item.status === 'running'),
    ),
)
const composerCliType = computed(
  () => latestSelectedAgentProgress.value?.cli_type || selectedStep.value?.cli_type || '',
)
const composerModelName = computed(
  () =>
    latestSelectedAgentProgress.value?.model ||
    selectedStep.value?.model_name ||
    selectedStep.value?.model ||
    '',
)
const initialTaskLoading = computed(() => taskLoading.value && !task.value)
const taskRefreshing = computed(() => taskLoading.value && Boolean(task.value))
const selectedIsCurrent = computed(() => selectedStepUuid.value === effectiveCurrentStepUuid.value)
const selectedStepReached = computed(() =>
  Boolean(
    selectedStep.value &&
    (selectedIsCurrent.value ||
      selectedStep.value.status === 'completed' ||
      (currentStepIndex.value >= 0 &&
        selectedStepIndex.value >= 0 &&
        selectedStepIndex.value < currentStepIndex.value) ||
      selectedProgress.value.length > 0),
  ),
)
const hasRunningSelectedConversation = computed(() => Boolean(activeSelectedProgress.value))
const currentStepLocked = computed(
  () =>
    !task.value ||
    task.value.status === 'done' ||
    Boolean(task.value.current_step_completed) ||
    currentStep.value?.status === 'completed',
)
const canAsk = computed(
	() => !taskLoading.value && (isExpertGroup.value || selectedStepReached.value) && !hasRunningSelectedConversation.value,
)
const selectedStepAwaitingStart = computed(
  () =>
    selectedIsCurrent.value &&
    !currentStepLocked.value &&
    selectedStep.value?.status === 'active' &&
    selectedProgress.value.length === 0,
)
const canComplete = computed(
  () =>
    selectedIsCurrent.value &&
    !currentStepLocked.value &&
    canAsk.value &&
    !submitting.value &&
    !completing.value &&
    selectedProgress.value.length > 0,
)
const lastProgressTerminal = computed(() => {
  const last = [...selectedProgress.value].reverse().find((item) => !isUserMessage(item))
  return Boolean(last && !['created', 'running'].includes(last.status))
})
const isLastStep = computed(() =>
  Boolean(currentStep.value && sortedSteps.value.at(-1)?.uuid === currentStep.value.uuid),
)
const nextStepName = computed(() =>
  isLastStep.value ? '' : sortedSteps.value[currentStepIndex.value + 1]?.name || '',
)
const showNextStepCard = computed(
  () => selectedIsCurrent.value && !currentStepLocked.value && lastProgressTerminal.value,
)
const composerPlaceholder = computed(() => {
  if (taskLoading.value) return t('workflows.task.feedback.refreshingWait')
  if (!selectedStepReached.value) return t('workflows.task.detail.stepUnavailable')
  if (hasRunningSelectedConversation.value) return t('workflows.task.feedback.agentRunning')
  // CLI 直接执行没有多 Agent，用指令式占位文案。
  if (isCLI.value) return t('workflows.task.detail.cliMessagePlaceholder')
  return t('workflows.task.feedback.mentionPlaceholder')
})
const composerContextText = computed(() =>
  t('workflows.task.detail.conversationContext', {
    step: selectedStep.value?.name || 'Agent',
    index: Math.max(selectedStepIndex.value + 1, 1),
  }),
)
const taskStatus = computed(() => statusValue(task.value?.status))
const taskStatusText = computed(() => taskStatusLabels.value[taskStatus.value] || taskStatus.value)
const taskPriorityText = computed(
  () => taskPriorityLabels.value[String(task.value?.priority || '').toLowerCase()] || '',
)

const md = new MarkdownIt({ breaks: true, linkify: true })
const renderedDescription = computed(() => {
  const text = task.value?.description
  if (!text) return ''
  // 兼容接口可能返回的历史 HTML 描述；纯文本和 Markdown 仍统一渲染。
  if (/<[a-z][\s\S]*>/i.test(text)) return text
  return md.render(text)
})

function handleTaskSaved() {
  void loadSelectedTask()
}

function statusValue(status?: string) {
  if (['todo', 'pending'].includes(status || '')) return 'pending'
  if (['active', 'running', 'developing', 'developed', 'in_progress'].includes(status || ''))
    return 'in_progress'
  if (['done', 'completed'].includes(status || '')) return 'done'
  return status || 'pending'
}

function terminalLabel(status?: string) {
  return terminalLabels.value[status || ''] || t('workflows.task.execution.waiting')
}

function routeQueryValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}

// 需求 2204（评论 5 调整后口径）：指派给 CLI 后不再预填输入框，而是任务与隐式
// 步骤就绪后自动向 CLI 发送 task.md 引用与固定提示词。两个触发来源：
// - 对话内新建 CLI 对话：创建任务成功后置 autoKickoffTaskUuid；
// - 三条指派链路（新建任务 / 看板弹窗 / 详情指派）：openCliConversation 置
//   pendingCliKickoffUuid（模块级 ref；不能走路由 query，会被 syncConversationRoute 重写掉）。
// 发送复用 submitQuestion 的 runs 链路；意图只消费一次，失败不重试避免重复轰炸。
// 注意：本段引用的 selectedStep / activeSelectedProgress / canAsk 等都在上方定义，
// watch 源数组会立即求值，不能把本段提前到它们之前（会触发 TDZ 错误）。
const autoKickoffTaskUuid = ref('')
const cliKickoffSending = ref(false)

const cliKickoffPending = computed(() => {
  const target = selectedTaskUuid.value
  if (!target) return false
  return autoKickoffTaskUuid.value === target || pendingCliKickoffUuid.value === target
})

// 就绪条件：目标任务是 CLI 直接执行、隐式步骤（cli-direct）已加载为当前步骤，
// 且可以发起提问（任务加载完、无进行中会话、非提交中）。
const cliKickoffTaskReady = computed(() => {
  const loaded = task.value
  if (!loaded || loaded.uuid !== selectedTaskUuid.value) return null
  if (loaded.execution_mode !== 'cli') return null
  const step = selectedStep.value
  if (!step || step.step_key !== 'cli-direct') return null
  return loaded
})

watch(
  [cliKickoffPending, cliKickoffTaskReady, canAsk, submitting, activeSelectedProgress],
  async ([pending, loaded, ask, busy, active]) => {
    if (!pending || cliKickoffSending.value) return
    if (!loaded || !ask || busy || active) return
    cliKickoffSending.value = true
    try {
      // 先消费意图再发送：状态回写触发重渲染也不会重复触发。
      autoKickoffTaskUuid.value = ''
      pendingCliKickoffUuid.value = ''
      const prompt = buildCliKickoffPrompt(
        loaded.task_dir || '',
        t('workflows.task.notifications.cliKickoffPrompt'),
      )
      if (!prompt) return
      await submitQuestion({
        content: prompt.content,
        display_content: prompt.content,
        config: {
          cli_type: loaded.execution_tool || '',
          model_name: loaded.execution_model || '',
        },
      })
    } finally {
      cliKickoffSending.value = false
    }
  },
  { immediate: true },
)

async function syncConversationRoute(taskUuid: string, stepUuid = '') {
  if (
    routeQueryValue(route.query.taskUuid) === taskUuid &&
    routeQueryValue(route.query.stepUuid) === stepUuid
  ) {
    return
  }
  await router.replace({
    name: 'tasks',
    query: {
      taskUuid: taskUuid || undefined,
      stepUuid: taskUuid && stepUuid ? stepUuid : undefined,
    },
  })
}

async function selectConversationFromRoute() {
  const routeTaskUuid = routeQueryValue(route.query.taskUuid)
  if (!routeTaskUuid) return
  let conversation = conversations.value.find((item) => item.task_uuid === routeTaskUuid)
  if (!conversation) {
    await loadNotifications(true)
    conversation = conversations.value.find((item) => item.task_uuid === routeTaskUuid)
  }
  const routeStepUuid = routeQueryValue(route.query.stepUuid)
  // 指派给 CLI 后跳转过来的任务可能还没有通知记录：直接按路由选中，
  // 否则详情区会停在空白态。
  if (!conversation) {
    if (selectedTaskUuid.value === routeTaskUuid) return
    clearNewConversation()
    selectedTaskUuid.value = routeTaskUuid
    selectedStepUuid.value = routeStepUuid
    selectedProgressUuid.value = ''
    closeContextMenu()
    void persistViewState()
    await loadSelectedTask()
    return
  }
  const nextStepUuid = routeStepUuid || conversation.latest.task_step_uuid
  if (
	!newConversationPipeline.value && !newConversationExpertGroup.value && !newConversationCodex.value && !newConversationCLI.value &&
    selectedTaskUuid.value === conversation.task_uuid &&
    selectedStepUuid.value === nextStepUuid
  ) {
    return
  }
  clearNewConversation()
  selectedTaskUuid.value = conversation.task_uuid
  selectedStepUuid.value = nextStepUuid
  selectedProgressUuid.value = routeStepUuid ? '' : conversation.latest.progress_uuid || ''
  closeContextMenu()
  void persistViewState()
  await loadSelectedTask()
}

function showSystemNotification(conversation: TaskConversation) {
  if (typeof window === 'undefined' || typeof Notification === 'undefined') return
  if (Notification.permission !== 'granted') return

  const notification = new Notification(
    t('workflows.task.feedback.readyTitle', {
      step: conversation.latest.step_name || t('workflows.task.common.agentOrchestration'),
    }),
    {
      body: t('workflows.task.feedback.openTask', { task: conversation.title }),
      tag: conversation.task_uuid,
    },
  )
  notification.onclick = () => {
    window.focus()
    void syncConversationRoute(conversation.task_uuid)
  }
}

async function ensureNotificationPermission() {
  if (typeof window === 'undefined' || typeof Notification === 'undefined') return
  if (Notification.permission !== 'default') return
  try {
    await Notification.requestPermission()
  } catch {
    // 通知权限申请失败不影响任务页面主流程
  }
}

function pipelineInitials(name?: string) {
  return (name || t('workflows.task.notifications.pipelineInitial'))
    .trim()
    .slice(0, 1)
    .toUpperCase()
}

function clearRefreshTimer() {
  if (!refreshTimer) return
  clearTimeout(refreshTimer)
  refreshTimer = undefined
}

// WS 连接正常时进度刷新依赖 task.changed 推送，轮询仅在断线期间兜底，避免长任务持续请求 /progress。
function scheduleRefresh() {
  clearRefreshTimer()
  if (
    wsConnected.value ||
    !progress.value.some(
      (item) => !isUserMessage(item) && (item.status === 'created' || item.status === 'running'),
    )
  ) {
    return
  }
  refreshTimer = setTimeout(() => {
    void loadSelectedTask(true)
  }, 2000)
}

function clearSelectedTask() {
  taskLoadVersion += 1
  clearRefreshTimer()
  task.value = undefined
  progress.value = []
  selectedStepUuid.value = ''
  selectedProgressUuid.value = ''
  taskError.value = ''
  taskLoading.value = false
}

function resolveSelectedStepUuid(
  loadedTask: TaskWithDetails,
  loadedProgress: TaskProgress[],
  preferredStepUuid: string,
) {
  if (preferredStepUuid && loadedTask.steps.some((step) => step.uuid === preferredStepUuid)) {
    return preferredStepUuid
  }
  const latestProgressStep = loadedProgress.at(-1)?.task_step_uuid || ''
  const validLatestProgressStep =
    latestProgressStep && loadedTask.steps.some((step) => step.uuid === latestProgressStep)
      ? latestProgressStep
      : ''
  return (
    validLatestProgressStep ||
    loadedTask.current_step_uuid ||
    loadedTask.steps.find((step) => step.status === 'active')?.uuid ||
    loadedTask.steps[0]?.uuid ||
    ''
  )
}

function restoreCachedTaskView(taskUuid: string) {
  const cached = taskViewCache.get(taskUuid)
  if (!cached) {
    task.value = undefined
    progress.value = []
    return false
  }
  task.value = cached.task
  progress.value = cached.progress
  selectedStepUuid.value = resolveSelectedStepUuid(
    cached.task,
    cached.progress,
    selectedStepUuid.value,
  )
  return true
}

function reportCacheFailure(action: string, error: unknown) {
  persistentCacheAvailable = false
  const reason = error instanceof Error ? error.message : String(error)
  console.warn(`[task-conversation-cache] ${action}失败: ${reason}`)
}

function currentNotificationSnapshot() {
  const uniqueItems = new Map<string, TaskNotification>()
  for (const item of [...toRaw(temporaryNotifications.value), ...toRaw(notifications.value)]) {
    uniqueItems.set(item.uuid, item)
  }
  return [...uniqueItems.values()]
}

async function persistNotificationSnapshot() {
  if (!persistentCacheAvailable) return
  try {
    await saveTaskNotificationSnapshot(currentNotificationSnapshot())
  } catch (error) {
    reportCacheFailure('保存通知快照', error)
  }
}

async function persistViewState() {
  if (!persistentCacheAvailable) return
  try {
    await saveTaskConversationViewState(selectedTaskUuid.value, pipelineExpanded.value)
  } catch (error) {
    reportCacheFailure('保存页面状态', error)
  }
}

async function persistTaskView(
  taskUuid: string,
  loadedTask: TaskWithDetails,
  loadedProgress: TaskProgress[],
) {
  if (!persistentCacheAvailable) return
  try {
    await saveTaskView(taskUuid, loadedTask, loadedProgress)
  } catch (error) {
    reportCacheFailure('保存会话详情', error)
  }
}

async function removePersistentTaskView(taskUuid: string) {
  if (!persistentCacheAvailable) return
  try {
    await deleteTaskView(taskUuid)
  } catch (error) {
    reportCacheFailure('删除会话详情', error)
  }
}

async function reconcileTaskViewCache(validTaskUuids: Set<string>) {
  for (const taskUuid of taskViewCache.keys()) {
    if (!validTaskUuids.has(taskUuid)) taskViewCache.delete(taskUuid)
  }
  if (!persistentCacheAvailable) return
  try {
    await deleteTaskViewsOutside(validTaskUuids)
  } catch (error) {
    reportCacheFailure('清理失效会话', error)
  }
}

async function restorePersistentTaskConversationCache() {
  try {
    const cached = await readTaskConversationCache()
    notifications.value = cached.notifications
    temporaryNotifications.value = []
    taskViewCache.clear()
    cached.taskViews.forEach((item) => {
      taskViewCache.set(item.taskUuid, { task: item.task, progress: item.progress })
    })
    pipelineExpanded.value = cached.pipelineExpanded
    if (!conversations.value.length) return false

    const selectedConversationFromCache =
      conversations.value.find((item) => item.task_uuid === cached.lastSelectedTaskUuid) ||
      conversations.value[0]
    selectedTaskUuid.value = selectedConversationFromCache.task_uuid
    selectedStepUuid.value = selectedConversationFromCache.latest.task_step_uuid
    selectedProgressUuid.value = selectedConversationFromCache.latest.progress_uuid || ''
    restoreCachedTaskView(selectedConversationFromCache.task_uuid)
    void persistViewState()
    return true
  } catch (error) {
    reportCacheFailure('读取本地缓存', error)
    return false
  }
}

async function loadNotifications(preserveSelection = true) {
  const requestVersion = ++notificationsLoadVersion
  const hadCachedNotifications = conversations.value.length > 0
  notificationsLoading.value = true
  notificationsError.value = ''
  try {
    const result = await apiClient.get<{ items: TaskNotification[] }>('/notifications')
    if (requestVersion !== notificationsLoadVersion) return
    const loadedNotifications = result.items || []
    // 读取列表可能与点击消红点并发，只覆盖本次操作时已存在的通知，
    // 新到达的通知仍保留服务端的未读状态。
    for (const item of loadedNotifications) {
      const pending = pendingReadTasks.get(item.task_uuid)
      if (pending?.notificationUuids.has(item.uuid)) item.is_read = pending.isRead
    }
    const previousTaskUuids = lastNotifiedTaskUuids
    notifications.value = loadedNotifications
    temporaryNotifications.value = temporaryNotifications.value.filter(
      (temporary) => !loadedNotifications.some((item) => item.task_uuid === temporary.task_uuid),
    )
    const currentTaskUuids = new Set(loadedNotifications.map((item) => item.task_uuid))
    if (notificationsHydrated) {
      for (const conversation of conversations.value) {
        if (
          conversation.unread &&
          !previousTaskUuids.has(conversation.task_uuid) &&
          currentTaskUuids.has(conversation.task_uuid)
        ) {
          showSystemNotification(conversation)
        }
      }
    }
    lastNotifiedTaskUuids = currentTaskUuids
    notificationsHydrated = true
    const previousSelectedTaskUuid = selectedTaskUuid.value
    const previousSelectedStepUuid = selectedStepUuid.value
    const routeTaskUuid = routeQueryValue(route.query.taskUuid)
    const routeStepUuid = routeQueryValue(route.query.stepUuid)
    const routeConversation = conversations.value.find((item) => item.task_uuid === routeTaskUuid)
	if (!newConversationPipeline.value && !newConversationExpertGroup.value && !newConversationCodex.value && !newConversationCLI.value && routeConversation) {
      selectedTaskUuid.value = routeConversation.task_uuid
      selectedStepUuid.value = routeStepUuid || routeConversation.latest.task_step_uuid
      selectedProgressUuid.value = routeStepUuid ? '' : routeConversation.latest.progress_uuid || ''
    } else if (
	  !newConversationPipeline.value && !newConversationExpertGroup.value && !newConversationCodex.value && !newConversationCLI.value &&
      (!preserveSelection ||
        !conversations.value.some((item) => item.task_uuid === selectedTaskUuid.value))
    ) {
      const first = conversations.value[0]
      selectedTaskUuid.value = first?.task_uuid || ''
      selectedStepUuid.value = first?.latest.task_step_uuid || ''
      selectedProgressUuid.value = first?.latest.progress_uuid || ''
    }
    const selectionChanged = previousSelectedTaskUuid !== selectedTaskUuid.value
    const selectedStepChanged = previousSelectedStepUuid !== selectedStepUuid.value
    if (!conversations.value.length) {
      selectedTaskUuid.value = ''
      clearSelectedTask()
    } else if (selectionChanged && selectedTaskUuid.value) {
      const requestedStepUuid = selectedStepUuid.value
      clearSelectedTask()
      selectedStepUuid.value = requestedStepUuid
      restoreCachedTaskView(selectedTaskUuid.value)
    }

    const validTaskUuids = new Set(conversations.value.map((item) => item.task_uuid))
    await Promise.all([
      persistNotificationSnapshot(),
      persistViewState(),
      reconcileTaskViewCache(validTaskUuids),
    ])
    if ((selectionChanged || selectedStepChanged) && selectedTaskUuid.value) {
      void loadSelectedTask()
    }
  } catch (error) {
    if (requestVersion !== notificationsLoadVersion) return
    if (hadCachedNotifications || conversations.value.length) {
      message.warning(t('workflows.task.feedback.cachedConversations'))
    } else {
      notificationsError.value =
        error instanceof Error ? error.message : t('workflows.task.feedback.conversationLoadFailed')
      message.error(notificationsError.value)
    }
  } finally {
    if (requestVersion === notificationsLoadVersion) notificationsLoading.value = false
  }
}

async function loadSelectedTask(silent = false) {
  const taskUuid = selectedTaskUuid.value
  if (!taskUuid) return
  const requestVersion = ++taskLoadVersion
  clearRefreshTimer()
  if (!silent) {
    if (task.value?.uuid !== taskUuid) {
      const restoredFromCache = restoreCachedTaskView(taskUuid)
      if (restoredFromCache) void locateTarget('auto')
    }
    taskLoading.value = true
    taskError.value = ''
  }
  try {
    const [loadedTask, loadedProgress] = await Promise.all([
      apiClient.get<TaskWithDetails>(`/tasks/${taskUuid}`),
      apiClient.get<{ items: TaskProgress[] } | TaskProgress[]>(`/tasks/${taskUuid}/progress`),
    ])
    if (requestVersion !== taskLoadVersion || taskUuid !== selectedTaskUuid.value) return
    const progressItems = Array.isArray(loadedProgress)
      ? loadedProgress
      : loadedProgress.items || []
    const sortedProgress = [...progressItems].sort((a, b) => a.created_at - b.created_at)
    const nextSelectedStepUuid = resolveSelectedStepUuid(
      loadedTask,
      sortedProgress,
      selectedStepUuid.value,
    )
    taskViewCache.set(taskUuid, { task: loadedTask, progress: sortedProgress })
    task.value = loadedTask
    progress.value = sortedProgress
    selectedStepUuid.value = nextSelectedStepUuid
    if (
      routeQueryValue(route.query.taskUuid) === taskUuid &&
      routeQueryValue(route.query.stepUuid) &&
      routeQueryValue(route.query.stepUuid) !== nextSelectedStepUuid
    ) {
      await syncConversationRoute(taskUuid, nextSelectedStepUuid)
    }
    taskError.value = ''
    void persistTaskView(taskUuid, loadedTask, sortedProgress)
    await locateTarget('auto')
  } catch (error) {
    if (requestVersion === taskLoadVersion && !silent) {
      if (task.value?.uuid === taskUuid) {
        taskError.value = ''
        message.warning(t('workflows.task.feedback.cachedProgress'))
      } else {
        taskError.value =
          error instanceof Error ? error.message : t('workflows.task.feedback.progressLoadFailed')
        message.error(taskError.value)
      }
    }
  } finally {
    if (requestVersion === taskLoadVersion) {
      taskLoading.value = false
      scheduleRefresh()
    }
  }
}

async function initialize() {
  notificationsLoading.value = true
  const restoredFromCache = await restorePersistentTaskConversationCache()
  if (selectedTaskUuid.value) void loadSelectedTask()
  await loadNotifications(restoredFromCache)
  // 直接打开带 taskUuid 的链接（指派 CLI 后的跳转就是这种）时路由 watcher 不会触发，
  // 这里补一次路由选中，避免停在空白态。
  await selectConversationFromRoute()
}

async function setTaskRead(taskUuid: string, isRead: boolean) {
  const conversation = conversations.value.find((item) => item.task_uuid === taskUuid)
  if (!conversation || pendingReadTasks.has(taskUuid)) return
  const previous = new Map(conversation.items.map((item) => [item.uuid, item.is_read]))
  pendingReadTasks.set(taskUuid, { isRead, notificationUuids: new Set(previous.keys()) })
  conversation.items.forEach((item) => { item.is_read = isRead })
  try {
    await apiClient.put(`/notifications/tasks/${encodeURIComponent(taskUuid)}/read`, { is_read: isRead })
  } catch (error) {
    for (const item of [...notifications.value, ...temporaryNotifications.value]) {
      if (previous.has(item.uuid)) item.is_read = previous.get(item.uuid)!
    }
    message.error(error instanceof Error ? error.message : t('workflows.task.feedback.readFailed'))
  } finally {
    pendingReadTasks.delete(taskUuid)
    // 完成写入后重新读取，同时使写入前发出的列表请求失效。
    await loadNotifications(true)
  }
}

async function selectConversation(conversation: TaskConversation, markRead = true) {
  if (markRead && conversation.unread) void setTaskRead(conversation.task_uuid, true)
  clearNewConversation()
  selectedTaskUuid.value = conversation.task_uuid
  selectedStepUuid.value = conversation.latest.task_step_uuid
  selectedProgressUuid.value = conversation.latest.progress_uuid || ''
  void syncConversationRoute(conversation.task_uuid, selectedStepUuid.value)
  closeContextMenu()
  void persistViewState()
  await loadSelectedTask()
}

async function selectStep(uuid: string) {
  selectedStepUuid.value = uuid
  selectedProgressUuid.value = ''
  void syncConversationRoute(selectedTaskUuid.value, uuid)
  await locateTarget('smooth')
  const last = progress.value.filter((item) => item.task_step_uuid === uuid).at(-1)
  if (last) flashTarget(last.uuid)
}

async function locateTarget(behavior: 'auto' | 'smooth' = 'auto') {
  await nextTick()
  const requested = selectedProgressUuid.value
    ? progress.value.find((item) => item.uuid === selectedProgressUuid.value)
    : undefined
  const target =
    requested ||
    progress.value.filter((item) => item.task_step_uuid === selectedStepUuid.value).at(-1)
  if (!target) return
  const container = document.querySelector<HTMLElement>('.message-scroll')
  const targetElement = document.getElementById(`progress-${target.uuid}`)
  if (!container || !targetElement || !container.contains(targetElement)) return
  const containerRect = container.getBoundingClientRect()
  const targetRect = targetElement.getBoundingClientRect()
  const centeredOffset = Math.max((container.clientHeight - targetRect.height) / 2, 0)
  const top = container.scrollTop + targetRect.top - containerRect.top - centeredOffset
  container.scrollTo({ top: Math.max(top, 0), behavior })
}

function flashTarget(uuid: string) {
  highlightUuid.value = uuid
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    highlightUuid.value = ''
  }, 1200)
}

function stopSelectedConversation() {
  const sessionUuid = activeSelectedProgress.value?.session_uuid
  if (!sessionUuid || stoppingSessionUuid.value) {
    if (!sessionUuid) message.warning(t('workflows.task.feedback.sessionNotFound'))
    return
  }
  if (isStopConfirmSuppressed()) {
    void stopConversation(sessionUuid)
    return
  }
  stopConfirmSessionUuid.value = sessionUuid
  stopConfirmOpen.value = true
}

function closeStopConfirm() {
  stopConfirmOpen.value = false
  stopConfirmSessionUuid.value = ''
}

function confirmStopConversation(suppressFutureConfirm: boolean) {
  const sessionUuid = stopConfirmSessionUuid.value
  if (suppressFutureConfirm) suppressStopConfirm()
  if (sessionUuid) void stopConversation(sessionUuid)
}

async function stopConversation(sessionUuid: string) {
  stoppingSessionUuid.value = sessionUuid
  try {
    await apiClient.post(`/tasks/sessions/${encodeURIComponent(sessionUuid)}/stop`, {})
    message.success(t('workflows.task.feedback.stopped'))
    await Promise.all([loadNotifications(true), loadSelectedTask()])
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.feedback.stopFailed'))
  } finally {
    stoppingSessionUuid.value = ''
    closeStopConfirm()
  }
}

async function submitQuestion(submission: ChatComposerSubmission) {
  const text = submission.content.trim()
  const step = selectedStep.value
  if (!text || !step || !canAsk.value || submitting.value) return
  const startingStep = selectedStepAwaitingStart.value
  submitting.value = true
  try {
    const path = startingStep
      ? `/tasks/${selectedTaskUuid.value}/steps/${step.uuid}/runs`
      : `/tasks/${selectedTaskUuid.value}/steps/${step.uuid}/questions`
    await apiClient.post(path, {
      question: text,
      display_question: submission.display_content?.trim() || text,
      request_id: crypto.randomUUID(),
      cli_type: submission.config.cli_type,
      model_name: submission.config.model_name,
    })
    question.value = ''
    composerRef.value?.resetAfterSubmit()
    selectedProgressUuid.value = ''
    message.success(
      startingStep
        ? t('workflows.task.feedback.agentStarted')
        : t('workflows.task.feedback.messageSent'),
    )
    await setTaskRead(selectedTaskUuid.value, true)
    await Promise.all([loadNotifications(true), loadSelectedTask()])
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.feedback.messageFailed'),
    )
  } finally {
    submitting.value = false
  }
}

async function completeStep() {
  const step = currentStep.value
  if (!step || !canComplete.value || completing.value) return
  completing.value = true
  try {
    const result = await apiClient.post<CompleteStepResponse>(
      `/tasks/${selectedTaskUuid.value}/steps/${step.uuid}/complete`,
      { auto_start: false },
    )
    const wasLastStep = sortedSteps.value.at(-1)?.uuid === step.uuid
    selectedProgressUuid.value = ''
    if (!wasLastStep && result.next_step_uuid) {
      selectedStepUuid.value = result.next_step_uuid
      await syncConversationRoute(selectedTaskUuid.value, result.next_step_uuid)
    }
    if (wasLastStep) {
      message.success(t('workflows.task.feedback.taskCompleted'))
    } else {
      message.success(t('workflows.task.feedback.nextStepManual'))
    }
    await setTaskRead(selectedTaskUuid.value, true)
    await Promise.all([loadNotifications(true), loadSelectedTask()])
    const nextCurrent = task.value?.current_step_uuid
    if (nextCurrent && nextCurrent !== selectedStepUuid.value) {
      selectedStepUuid.value = nextCurrent
      await syncConversationRoute(selectedTaskUuid.value, nextCurrent)
      await locateTarget()
    }
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.feedback.nextStepFailed'),
    )
  } finally {
    completing.value = false
  }
}

async function copyResult(item: TaskProgress) {
  try {
    await copyText(isUserMessage(item) ? userMessageText(item) : resultText(item))
    message.success(t('components.feedback.copied'))
  } catch {
    message.warning(t('components.feedback.copyFailed'))
  }
}

function openContextMenu(event: MouseEvent, conversation: TaskConversation) {
  event.preventDefault()
  showContextMenu(conversation, event.clientX, event.clientY)
}

function showContextMenu(conversation: TaskConversation, x: number, y: number) {
  contextTaskUuid.value = conversation.task_uuid
  contextX.value = Math.min(x, window.innerWidth - 190)
  contextY.value = Math.min(y, window.innerHeight - 150)
}

function toggleConversationMenu(event: MouseEvent, conversation: TaskConversation) {
  if (contextTaskUuid.value === conversation.task_uuid) {
    closeContextMenu()
    return
  }
  const triggerRect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  showContextMenu(conversation, triggerRect.right - 174, triggerRect.bottom + 4)
}

function closeContextMenu() {
  contextTaskUuid.value = ''
}

function togglePipelineExpanded() {
  pipelineExpanded.value = !pipelineExpanded.value
  void persistViewState()
}

async function openSelectedTerminal() {
  const workDir = task.value?.work_dir || task.value?.task_dir || ''
  if (!workDir || openingTerminal.value) return
  openingTerminal.value = true
  try { await openTerminal(workDir) } catch (error) { message.error(error instanceof Error ? error.message : t('workflows.task.progress.openTerminalFailed')) } finally { openingTerminal.value = false }
}

async function handleOpenCodex() {
  if (
    !selectedTaskUuid.value ||
    task.value?.execution_mode !== 'vibe_coding' ||
    task.value.execution_tool !== 'codex' ||
    codexBusy.value
  ) {
    return
  }
  codexBusy.value = true
  try {
    const openResult = await openTaskInCodex(selectedTaskUuid.value)
    if (openResult.opened) {
      message.success(t('workflows.task.detail.codexOpened'))
    } else if (openResult.copied) {
      message.warning(t('workflows.task.detail.codexCopiedFallback'))
    } else {
      message.warning(t('workflows.task.detail.codexUnavailableAfterAssign'))
    }
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.detail.codexOpenFailed'))
  } finally {
    codexBusy.value = false
  }
}

async function openConversationDetail() {
  const conversation = contextConversation.value
  if (!conversation) return
  if (selectedTaskUuid.value !== conversation.task_uuid) {
    await selectConversation(conversation)
  } else {
    closeContextMenu()
    if (task.value?.uuid !== conversation.task_uuid) await loadSelectedTask()
  }
  if (task.value?.uuid === conversation.task_uuid) detailModalOpen.value = true
}

async function toggleRead() {
  const conversation = contextConversation.value
  if (!conversation) return
  await setTaskRead(conversation.task_uuid, conversation.unread)
  closeContextMenu()
}

async function archiveConversation() {
  const conversation = contextConversation.value
  if (!conversation) return
  const taskUuid = conversation.task_uuid
  try {
    await apiClient.put(`/notifications/tasks/${taskUuid}/archive`, { is_archived: true })
    taskViewCache.delete(taskUuid)
    notifications.value = notifications.value.filter((item) => item.task_uuid !== taskUuid)
    temporaryNotifications.value = temporaryNotifications.value.filter(
      (item) => item.task_uuid !== taskUuid,
    )
    await Promise.all([removePersistentTaskView(taskUuid), persistNotificationSnapshot()])
    if (selectedTaskUuid.value === taskUuid) {
      const next = conversations.value[0]
      if (next) await selectConversation(next, false)
      else {
        selectedTaskUuid.value = ''
        clearSelectedTask()
        await syncConversationRoute('')
        await persistViewState()
      }
    }
    message.success(t('workflows.task.feedback.archived'))
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.feedback.archiveFailed'),
    )
  } finally {
    closeContextMenu()
  }
}

async function loadPipelines(force = false) {
  try {
    await pipelineStore.loadPipelines(force)
  } catch {
    // 请求错误由 Store 统一转换为菜单内的错误状态
  }
}

function conversationStatusLabel(name: string | undefined, status: string) {
  return `${name || t('workflows.task.common.agentOrchestration')} · ${status}`
}

function conversationActorName(notification: TaskNotification) {
  if (notification.execution_mode === 'vibe_coding') {
    return notification.execution_tool === 'codex'
      ? t('workflows.task.assign.codex')
      : notification.execution_tool || t('workflows.task.create.vibeCodingMode')
  }
  return notification.step_name || t('workflows.task.common.agentOrchestration')
}

async function loadExpertGroups(force = false) {
  try { await expertGroupStore.load(force) } catch { /* Store owns the visible error. */ }
}

async function loadConversationCodexCapability(force = false) {
  if (codexCapabilityLoaded.value && !force) return
  try {
    codexCapability.value = await loadCodexCapability()
    codexCapabilityLoaded.value = true
  } catch (error) {
    codexCapability.value = {
      available: false,
      message:
        error instanceof Error
          ? error.message
          : t('workflows.task.codex.capabilityUnavailable'),
    }
  }
}

function handlePipelineMenuOpenChange(open: boolean) {
  pipelineMenuOpen.value = open
	if (open) {
	  void loadPipelines()
	  void loadExpertGroups()
	  void loadCliOptions()
	  void loadConversationCodexCapability()
	}
}

function clearNewConversation() {
  newConversationPipeline.value = undefined
	newConversationExpertGroup.value = undefined
  newConversationCodex.value = false
  newConversationCLI.value = false
  // CLI 与模型的记忆值保留：评论 4 要求下次打开新建对话时默认填入。
  newConversationQuestion.value = ''
  newConversationWorkDir.value = ''
  newConversationSubmission.value = undefined
  newConversationComposerRef.value?.resetAfterSubmit()
  pipelineConfigModalOpen.value = false
}

function startNewConversation(pipeline: Pipeline) {
  closeContextMenu()
  pipelineMenuOpen.value = false
  selectedTaskUuid.value = ''
  clearSelectedTask()
  newConversationSubmission.value = undefined
  newConversationComposerRef.value?.resetAfterSubmit()
  newConversationPipeline.value = pipeline
	newConversationExpertGroup.value = undefined
  newConversationCodex.value = false
  newConversationCLI.value = false
  newConversationQuestion.value = ''
  newConversationWorkDir.value = ''
  void syncConversationRoute('')
}

function startNewExpertConversation(group: ExpertGroup) {
  if (!group.ready) return
  closeContextMenu(); pipelineMenuOpen.value = false; selectedTaskUuid.value = ''; clearSelectedTask()
  newConversationPipeline.value = undefined; newConversationExpertGroup.value = group
  newConversationCodex.value = false
  newConversationCLI.value = false
  newConversationQuestion.value = ''; newConversationWorkDir.value = ''; void syncConversationRoute('')
}

function startNewCLIConversation() {
  closeContextMenu(); pipelineMenuOpen.value = false; selectedTaskUuid.value = ''; clearSelectedTask()
  newConversationPipeline.value = undefined; newConversationExpertGroup.value = undefined
  newConversationCodex.value = false
  newConversationCLI.value = true
  newConversationQuestion.value = ''; newConversationWorkDir.value = ''; void syncConversationRoute('')
  if (!cliOptions.value.length) void loadCliOptions()
  // 评论 4：回填上次成功使用的 CLI 与模型作为默认值；
  // 模型要等该 CLI 的模型列表加载完后校验存在才填，避免残留失效选项。
  const preference = readCliPreference()
  if (preference.cliType) {
    newConversationCliType.value = preference.cliType
    void loadModelOptions(preference.cliType).then(() => {
      if (
        preference.model &&
        newConversationCliType.value === preference.cliType &&
        modelOptions.value.includes(preference.model)
      ) {
        newConversationCliModel.value = preference.model
      }
    })
  }
}

function startNewCodexConversation() {
  if (!codexCapability.value.available) return
  closeContextMenu()
  pipelineMenuOpen.value = false
  selectedTaskUuid.value = ''
  clearSelectedTask()
  newConversationSubmission.value = undefined
  newConversationComposerRef.value?.resetAfterSubmit()
  newConversationPipeline.value = undefined
  newConversationExpertGroup.value = undefined
  newConversationCodex.value = true
  newConversationCLI.value = false
  newConversationQuestion.value = ''
  newConversationWorkDir.value = ''
  void syncConversationRoute('')
}

function chooseNewConversationCLIType(cliType: string) {
  newConversationCliType.value = cliType
  newConversationCliModel.value = ''
  writeCliPreference(cliType, '')
  void loadModelOptions(cliType)
}

// 需求设计图：CLI 与模型在「新建对话」菜单内选完，点「创建对话」进入待创建状态。
function createNewCLIConversation() {
  if (!canCreateNewCLIConversation.value) return
  startNewCLIConversation()
}

function chooseNewConversationCLIModel(model: string) {
  newConversationCliModel.value = model
  if (newConversationCliType.value) {
    writeCliPreference(newConversationCliType.value, model)
  }
}

async function chooseNewConversationDirectory() {
  try {
    const selected = await selectDirectory(newConversationWorkDir.value)
    if (selected) newConversationWorkDir.value = selected
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : t('workflows.task.feedback.chooseDirectoryFailed'),
    )
  }
}

function createTaskTitle(text: string) {
  return text.replace(/\s+/g, ' ').trim().slice(0, 50)
}

function createStepConfigs(pipeline: Pipeline) {
  return (pipeline.steps || []).map((step) => ({
    step_uuid: step.uuid,
    source_step_id: step.source_step_id,
    cloud_step_id: step.cloud_step_id,
    cli_type: step.cli_type,
    model: step.model,
    model_name: step.model_name,
  }))
}

async function submitNewConversation(
  submission?: ChatComposerSubmission,
  pipelineOverride?: Pipeline,
) {
  if (submission) newConversationSubmission.value = submission
  const currentSubmission = submission || newConversationSubmission.value
  const promptText = currentSubmission?.content.trim() || ''
  const displayText = currentSubmission?.display_content?.trim() || promptText
  const pipeline = pipelineOverride || newConversationPipeline.value
  if (!displayText || !pipeline || submitting.value) return
  if (!newConversationWorkDir.value.trim()) {
    message.warning(t('workflows.task.feedback.chooseDirectory'))
    return
  }
  submitting.value = true
  try {
    const createdTask = await apiClient.post<CreatedTaskResponse>('/tasks', {
      title: createTaskTitle(displayText) || t('workflows.task.feedback.imageTask'),
      description: promptText,
      pipeline_uuid: pipeline.uuid,
      step_configs: createStepConfigs(pipeline),
      work_dir: newConversationWorkDir.value.trim(),
      work_dirs: [newConversationWorkDir.value.trim()],
      create_notification: true,
    })
    const notification =
      createdTask.notification ||
      ({
        task_uuid: createdTask.uuid,
        task_title:
          createdTask.title ||
          createTaskTitle(displayText) ||
          t('workflows.task.feedback.imageTask'),
        step_name:
          createdTask.steps?.[0]?.name ||
          pipeline.steps?.[0]?.name ||
          t('workflows.task.common.agentOrchestration'),
        session_uuid: '',
        status: 'created',
        summary: t('workflows.task.feedback.taskCreated'),
        created_at: Date.now(),
      } satisfies CreateTaskNotification)
    const currentStepUuid = createdTask.current_step_uuid || createdTask.steps?.[0]?.uuid || ''
    temporaryNotifications.value = [
      {
        uuid: `created-${notification.task_uuid}-${notification.created_at}`,
        task_uuid: notification.task_uuid,
        task_title: notification.task_title,
        task_step_uuid: currentStepUuid,
        step_name: notification.step_name,
        status: notification.status,
        summary: notification.summary,
        is_read: true,
        created_at: notification.created_at,
      },
      ...temporaryNotifications.value.filter((item) => item.task_uuid !== notification.task_uuid),
    ]
    selectedTaskUuid.value = createdTask.uuid
    selectedStepUuid.value = currentStepUuid
    selectedProgressUuid.value = ''
    task.value = createdTask
    progress.value = []
    taskViewCache.set(createdTask.uuid, { task: createdTask, progress: [] })
    newConversationPipeline.value = undefined
    newConversationCLI.value = false
    newConversationQuestion.value = ''
    newConversationWorkDir.value = ''
    newConversationSubmission.value = undefined
    newConversationComposerRef.value?.resetAfterSubmit()
    await syncConversationRoute(createdTask.uuid, currentStepUuid)
    await Promise.all([
      persistNotificationSnapshot(),
      persistViewState(),
      persistTaskView(createdTask.uuid, createdTask, []),
    ])
    message.success(notification.summary)
    await loadSelectedTask()
  } catch (error) {
    if (error instanceof ApiError && error.code === 'pipeline_incomplete') {
      pipelineConfigModalOpen.value = true
      message.warning(error.message)
    } else {
      message.error(
        error instanceof Error ? error.message : t('workflows.task.feedback.taskCreateFailed'),
      )
    }
  } finally {
    submitting.value = false
  }
}

async function submitNewExpertConversation(submission: ChatComposerSubmission) {
  const group = newConversationExpertGroup.value
  const promptText = submission.content.trim()
  if (!group?.ready || !promptText || !newConversationWorkDir.value.trim() || submitting.value) {
    if (!newConversationWorkDir.value.trim()) message.warning(t('workflows.task.feedback.chooseDirectory'))
    return
  }
  submitting.value = true
  try {
    const createdTask = await apiClient.post<CreatedTaskResponse>('/tasks', {
      title: createTaskTitle(submission.display_content || promptText),
      description: promptText,
      execution_mode: 'expert_group',
      expert_group_uuid: group.uuid,
      work_dir: newConversationWorkDir.value.trim(),
      work_dirs: [newConversationWorkDir.value.trim()],
      status: 'active',
      create_notification: true,
    })
    const leaderStep = createdTask.steps?.find((step) => step.member_role === 'leader')
    const notification = createdTask.notification
    temporaryNotifications.value = [{
      uuid: `created-${createdTask.uuid}-${Date.now()}`,
      task_uuid: createdTask.uuid,
      task_title: createdTask.title,
      task_step_uuid: leaderStep?.uuid || '',
      step_name: leaderStep?.name || group.leader?.name || t('expertGroups.leader'),
      status: notification?.status || 'created',
      summary: notification?.summary || t('workflows.task.feedback.taskCreated'),
      is_read: true,
      created_at: notification?.created_at || Date.now(),
    }, ...temporaryNotifications.value.filter((item) => item.task_uuid !== createdTask.uuid)]
    selectedTaskUuid.value = createdTask.uuid; selectedStepUuid.value = leaderStep?.uuid || ''
    newConversationExpertGroup.value = undefined; newConversationQuestion.value = ''; newConversationWorkDir.value = ''
    message.success(t('workflows.task.feedback.taskCreated')); await syncConversationRoute(createdTask.uuid, selectedStepUuid.value)
    await loadSelectedTask()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.feedback.taskCreateFailed'))
  } finally { submitting.value = false }
}

async function submitNewCodexConversation(submission: ChatComposerSubmission) {
  const promptText = submission.content.trim()
  const displayText = submission.display_content?.trim() || promptText
  if (!promptText || !newConversationCodex.value || submitting.value) return
  if (!newConversationWorkDir.value.trim()) {
    message.warning(t('workflows.task.feedback.chooseDirectory'))
    return
  }
  submitting.value = true
  let taskCreated = false
  try {
    const workDir = newConversationWorkDir.value.trim()
    const createdTask = await apiClient.post<CreatedTaskResponse>('/tasks', {
      title: createTaskTitle(displayText) || t('workflows.task.feedback.imageTask'),
      description: promptText,
      execution_mode: 'vibe_coding',
      execution_tool: 'codex',
      work_dir: workDir,
      work_dirs: [workDir],
      status: 'active',
      create_notification: true,
    })
    taskCreated = true
    const notification = createdTask.notification
    temporaryNotifications.value = [
      {
        uuid: `created-${createdTask.uuid}-${notification?.created_at || Date.now()}`,
        task_uuid: createdTask.uuid,
        task_title:
          createdTask.title ||
          createTaskTitle(displayText) ||
          t('workflows.task.feedback.imageTask'),
        task_step_uuid: '',
        step_name: t('workflows.task.assign.codex'),
        status: notification?.status || 'created',
        execution_mode: 'vibe_coding',
        execution_tool: 'codex',
        summary: notification?.summary || t('workflows.task.feedback.taskCreated'),
        is_read: true,
        created_at: notification?.created_at || Date.now(),
      },
      ...temporaryNotifications.value.filter((item) => item.task_uuid !== createdTask.uuid),
    ]
    selectedTaskUuid.value = createdTask.uuid
    selectedStepUuid.value = ''
    selectedProgressUuid.value = ''
    task.value = createdTask
    progress.value = []
    taskViewCache.set(createdTask.uuid, { task: createdTask, progress: [] })
    newConversationCodex.value = false
    newConversationQuestion.value = ''
    newConversationWorkDir.value = ''
    newConversationSubmission.value = undefined
    newConversationComposerRef.value?.resetAfterSubmit()
    await syncConversationRoute(createdTask.uuid)
    await Promise.all([
      persistNotificationSnapshot(),
      persistViewState(),
      persistTaskView(createdTask.uuid, createdTask, []),
    ])

    const openResult = await openTaskInCodex(createdTask.uuid)
    if (openResult.opened) {
      message.success(t('workflows.task.detail.codexOpened'))
    } else if (openResult.copied) {
      message.warning(t('workflows.task.detail.codexCopiedFallback'))
    } else {
      message.warning(t('workflows.task.detail.codexUnavailableAfterAssign'))
    }
    await loadSelectedTask()
  } catch (error) {
    message.error(
      error instanceof Error
        ? error.message
        : taskCreated
          ? t('workflows.task.detail.codexOpenFailed')
          : t('workflows.task.feedback.taskCreateFailed'),
    )
  } finally {
    submitting.value = false
  }
}

// CLI 直接执行对话：任务创建后即进入进行中，等用户第一条消息触发 RunStep。
async function submitNewCLIConversation(submission: ChatComposerSubmission) {
  const promptText = submission.content.trim()
  if (!promptText || !newConversationCliType.value || !newConversationCliModel.value || submitting.value) return
  if (!newConversationWorkDir.value.trim()) {
    message.warning(t('workflows.task.feedback.chooseDirectory'))
    return
  }
  submitting.value = true
  try {
    const createdTask = await apiClient.post<CreatedTaskResponse>('/tasks', {
      title: createTaskTitle(submission.display_content || promptText),
      description: promptText,
      execution_mode: 'cli',
      execution_tool: newConversationCliType.value,
      model_name: newConversationCliModel.value,
      work_dir: newConversationWorkDir.value.trim(),
      work_dirs: [newConversationWorkDir.value.trim()],
      status: 'active',
      create_notification: true,
    })
    const cliStep = createdTask.steps?.[0]
    temporaryNotifications.value = [{
      uuid: `created-${createdTask.uuid}-${Date.now()}`,
      task_uuid: createdTask.uuid,
      task_title: createdTask.title,
      task_step_uuid: cliStep?.uuid || '',
      step_name: cliStep?.name || t('workflows.task.assign.cliMode'),
      status: createdTask.notification?.status || 'created',
      summary: createdTask.notification?.summary || t('workflows.task.feedback.taskCreated'),
      is_read: true,
      created_at: createdTask.notification?.created_at || Date.now(),
    }, ...temporaryNotifications.value.filter((item) => item.task_uuid !== createdTask.uuid)]
    selectedTaskUuid.value = createdTask.uuid; selectedStepUuid.value = cliStep?.uuid || ''
    // 评论 4：CLI 与模型记忆值保留，作为下次新建对话的默认值。
    // 评论 1：创建任务成功后自动向 CLI 发送起始指令，无需用户再发一条；
    // 等 loadSelectedTask 把隐式步骤加载完，由 cliKickoff watcher 触发发送。
    autoKickoffTaskUuid.value = createdTask.uuid
    newConversationCLI.value = false
    newConversationQuestion.value = ''; newConversationWorkDir.value = ''
    message.success(t('workflows.task.feedback.taskCreated')); await syncConversationRoute(createdTask.uuid, selectedStepUuid.value)
    await loadSelectedTask()
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('workflows.task.feedback.taskCreateFailed'))
  } finally { submitting.value = false }
}

async function submitExpertMessage(submission: ChatComposerSubmission) {
  const text = submission.content.trim()
  if (!text || submitting.value || activeSelectedProgress.value) return
  submitting.value = true
  try {
    await apiClient.post(`/tasks/${selectedTaskUuid.value}/expert-messages`, {
      content: text,
      display_content: submission.display_content?.trim() || text,
      member_uuid: submission.member_uuid,
      request_id: crypto.randomUUID(),
    })
    question.value = ''; composerRef.value?.resetAfterSubmit(); message.success(t('workflows.task.feedback.messageSent'))
    await Promise.all([loadNotifications(true), loadSelectedTask()])
  } catch (error) { message.error(error instanceof Error ? error.message : t('workflows.task.feedback.messageFailed')) }
  finally { submitting.value = false }
}

function handleCreationPipelineSelected(pipeline: Pipeline) {
  newConversationPipeline.value = pipeline
  pipelineConfigModalOpen.value = false
  void submitNewConversation(undefined, pipeline)
}

function getPipelinePopupContainer(trigger: HTMLElement) {
  return trigger.parentElement || document.body
}

function showImagePreview(url: string) {
  previewImageUrl.value = url
  previewImageVisible.value = true
}

function syncSidebarMetrics() {
  const layoutWidth = taskLayoutRef.value?.getBoundingClientRect().width || MIN_SIDEBAR_WIDTH
  sidebarMaxWidth.value = Math.max(
    MIN_SIDEBAR_WIDTH,
    Math.min(MAX_SIDEBAR_WIDTH, Math.floor(layoutWidth)),
  )
  if (sidebarResized.value) {
    sidebarWidth.value = Math.min(sidebarMaxWidth.value, sidebarWidth.value)
    return
  }
  sidebarWidth.value = conversationSidebarRef.value?.getBoundingClientRect().width || 300
}

function restoreSidebarWidth() {
  syncSidebarMetrics()
  try {
    const cachedWidth = window.localStorage.getItem(SIDEBAR_WIDTH_STORAGE_KEY)
    if (!cachedWidth) return
    const width = Number(cachedWidth)
    if (Number.isFinite(width)) setSidebarWidth(width)
  } catch {
    // 本地存储不可用时保留页面默认宽度
  }
}

function persistSidebarWidth() {
  try {
    window.localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(Math.round(sidebarWidth.value)))
  } catch {
    // 本地存储不可用不影响侧栏宽度调整
  }
}

function setSidebarWidth(width: number) {
  sidebarResized.value = true
  sidebarWidth.value = Math.min(sidebarMaxWidth.value, Math.max(MIN_SIDEBAR_WIDTH, width))
}

function startSidebarResize(event: PointerEvent) {
  if (event.button !== 0) return
  syncSidebarMetrics()
  sidebarResizeState = {
    pointerId: event.pointerId,
    pointerX: event.clientX,
    width: sidebarWidth.value,
  }
  sidebarDragging.value = true
  const target = event.currentTarget as HTMLElement
  target.setPointerCapture(event.pointerId)
  event.preventDefault()
}

function handleSidebarResize(event: PointerEvent) {
  if (!sidebarResizeState || sidebarResizeState.pointerId !== event.pointerId) return
  setSidebarWidth(sidebarResizeState.width + event.clientX - sidebarResizeState.pointerX)
  event.preventDefault()
}

function finishSidebarResize(event: PointerEvent) {
  if (!sidebarResizeState || sidebarResizeState.pointerId !== event.pointerId) return
  const target = event.currentTarget as HTMLElement
  sidebarResizeState = undefined
  sidebarDragging.value = false
  persistSidebarWidth()
  if (target.hasPointerCapture(event.pointerId)) target.releasePointerCapture(event.pointerId)
}

function handleSidebarResizeKeydown(event: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  syncSidebarMetrics()
  if (event.key === 'Home') setSidebarWidth(MIN_SIDEBAR_WIDTH)
  else if (event.key === 'End') setSidebarWidth(sidebarMaxWidth.value)
  else {
    setSidebarWidth(
      sidebarWidth.value +
        (event.key === 'ArrowLeft' ? -SIDEBAR_KEYBOARD_STEP : SIDEBAR_KEYBOARD_STEP),
    )
  }
  persistSidebarWidth()
  event.preventDefault()
}

function handleDocumentKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  closeContextMenu()
  pipelineMenuOpen.value = false
}

useLocalWS('task.changed', (data: { task_uuid?: string }) => {
  void loadNotifications(true)
  if (data.task_uuid === selectedTaskUuid.value) void loadSelectedTask(true)
})

watch(wsConnected, (connected) => {
  if (connected) {
    // 重连成功后补偿拉取一次，随后继续依赖推送刷新
    if (selectedTaskUuid.value) void loadSelectedTask(true)
  } else {
    scheduleRefresh()
  }
})

watch(
  () => [routeQueryValue(route.query.taskUuid), routeQueryValue(route.query.stepUuid)] as const,
  () => {
    void selectConversationFromRoute()
  },
)

watch(pipelines, (items) => {
  if (
    newConversationPipeline.value &&
    !items.some((item) => item.uuid === newConversationPipeline.value?.uuid)
  ) {
    newConversationPipeline.value = undefined
  }
})

onMounted(() => {
  document.addEventListener('click', closeContextMenu)
  document.addEventListener('keydown', handleDocumentKeydown)
  window.addEventListener('resize', syncSidebarMetrics)
  restoreSidebarWidth()
  void ensureNotificationPermission()
  void initialize()
})

onBeforeUnmount(() => {
  taskLoadVersion += 1
  notificationsLoadVersion += 1
  clearRefreshTimer()
  if (highlightTimer) clearTimeout(highlightTimer)
  document.removeEventListener('click', closeContextMenu)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', syncSidebarMetrics)
})
</script>

<style scoped>
.task-page {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: #fff;
}

.page-titlebar {
  display: flex;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  justify-content: flex-start;
  gap: 12px;
  padding: 0 24px;
  border-bottom: 1px solid #f0f0f0;
  background: #fff;
}

.page-titlebar-copy {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 12px;
}

.page-titlebar-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  margin-left: auto;
  gap: 8px;
}

.page-titlebar strong {
  color: #262626;
  font-size: 20px;
  font-weight: 600;
  line-height: 28px;
}

.page-titlebar p {
  overflow: hidden;
  margin: 0;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.overview-toggle-button {
  display: inline-flex;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 6px;
  background: transparent;
  cursor: pointer;
  transition: background-color 180ms ease;
}

.overview-toggle-button:hover {
  background: #e4e6eb;
}

.overview-toggle-button:active {
  background: #d9dce2;
}

.overview-toggle-button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.overview-toggle-button img {
  display: block;
  width: 16px;
  height: 16px;
}

.task-overview {
  position: relative;
  flex: 0 0 auto;
  height: 140px;
  padding: 24px 24px 0 24px;
  overflow: hidden;
  box-sizing: border-box;
  border-bottom: 1px solid #d9d9d9;
  background: #fff;
}

.task-overview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 22px;
}

.task-overview-title {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.task-overview-title h2 {
  overflow: hidden;
  margin: 0;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-status,
.task-priority {
  flex: 0 0 auto;
  border: 1px solid currentColor;
  border-radius: 6px;
  padding: 0px 6px;
  font-size: 12px;
  line-height: 16px;
}

.task-status {
  color: #d97706;
}

.task-status.status-done {
  color: #16a34a;
}

.task-status.status-blocked {
  color: #ef4444;
}

.task-priority {
  color: #ef4444;
}

.sidebar-add-button:focus-visible,
.conversation-select-button:focus-visible,
.conversation-menu-button:focus-visible,
.context-menu button:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: 2px;
}

.task-step-region {
  overflow: hidden;
  height: auto;
  box-sizing: border-box;
}

.task-step-region :deep(.step-strip) {
  padding-bottom: 5px;
}

.step-region-empty {
  padding: 16px 0 20px;
  color: #8c8c8c;
  font-size: 14px;
}

/* 需求设计图：CLI 任务的执行目标是固定文案「直接执行：CLI · 模型」 */
.direct-execution-summary {
  display: flex;
  padding: 8px 0 12px;
  align-items: center;
  gap: 8px;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
}

.direct-execution-summary__logo {
  width: 14px;
  height: 14px;
  flex: 0 0 14px;
  object-fit: contain;
}

.direct-execution-summary__label::after {
  content: ':';
}

.direct-execution-summary strong {
  color: #262626;
  font-weight: 500;
}
.expert-overview { display:flex; align-items:center; gap:16px; min-height:44px; padding:0 24px; color:#595959; }
.expert-overview strong { color:#262626; }
.expert-overview span { font-size:12px; }

.task-overview-skeleton {
  min-height: 60px;
}

.overview-skeleton-head {
  display: flex;
  min-height: 60px;
  box-sizing: border-box;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 8px 24px 0;
}

.overview-skeleton-title,
.overview-skeleton-steps span,
.composer-loading-body {
  border-radius: 6px;
  background: #f0f1f3;
}

.overview-skeleton-title {
  width: min(360px, 42%);
  height: 28px;
}

.overview-skeleton-steps {
  display: flex;
  min-height: 72px;
  box-sizing: border-box;
  align-items: center;
  gap: 36px;
  padding: 0 24px 8px;
}

.overview-skeleton-steps span {
  width: 155px;
  height: 40px;
}

.task-layout {
  display: flex;
  min-height: 0;
  flex: 1;
}

.task-layout.is-resizing-sidebar {
  cursor: col-resize;
  user-select: none;
}

.conversation-sidebar {
  position: relative;
  display: flex;
  width: 300px;
  min-width: 140px;
  max-width: 500px;
  min-height: 0;
  flex: 0 0 300px;
  flex-direction: column;
  border-right: 1px solid #f0f0f0;
  background: #fff;
}

.sidebar-resize-handle {
  position: absolute;
  z-index: 1;
  top: 0;
  right: -6px;
  bottom: 0;
  width: 12px;
  cursor: col-resize;
  touch-action: none;
}

.sidebar-resize-handle::after {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 50%;
  width: 1px;
  background: transparent;
  content: '';
  transform: translateX(-50%);
}

.sidebar-resize-handle:hover::after,
.sidebar-resize-handle:focus-visible::after,
.is-resizing-sidebar .sidebar-resize-handle::after {
  width: 2px;
  background: #3157e2;
}

.sidebar-resize-handle:focus-visible {
  outline: none;
}

.sidebar-heading {
  position: relative;
  display: flex;
  min-height: 56px;
  flex: 0 0 56px;
  align-items: center;
  gap: 8px;
  padding: 16px 24px;
  color: #262626;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.sidebar-heading small {
  display: inline-flex;
  width: 22px;
  height: 22px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  padding: 0;
  color: #8c8c8c;
  background: #edeff2;
  font-size: 12px;
  font-weight: 400;
  line-height: 14px;
}

.sidebar-add-button {
  appearance: none;
  position: absolute;
  top: 16px;
  right: 24px;
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 0;
  color: #2475fc;
  background: transparent;
  cursor: pointer;
}

.sidebar-add-button:hover,
.sidebar-add-button[aria-expanded='true'] {
  background: rgba(49, 87, 226, 0.08);
}

.pipeline-menu {
  width: 208px;
  overflow: hidden;
  border-radius: 6px;
  padding: 2px;
  background: #fff;
  box-shadow:
    0 8px 10px -5px rgba(0, 0, 0, 0.08),
    0 16px 24px 2px rgba(0, 0, 0, 0.04),
    0 6px 30px 5px rgba(0, 0, 0, 0.05);
}

.pipeline-menu-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px 4px;
  color: #8c8c8c;
  font-size: 14px;
  line-height: 22px;
}

.pipeline-menu-list {
  max-height: 280px;
  overflow-y: auto;
  padding-bottom: 2px;
}

.pipeline-menu-item {
  display: flex;
  width: 100%;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  border: 0;
  border-radius: 6px;
  padding: 5px 16px;
  color: #262626;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  line-height: 22px;
  text-align: left;
}

.pipeline-menu-item:hover:not(:disabled),
.pipeline-menu-item:focus-visible {
  background: #f5f5f5;
}

.pipeline-menu-item:focus-visible {
  outline: 2px solid #3157e2;
  outline-offset: -2px;
}

.pipeline-menu-item:disabled {
  color: #bfbfbf;
  cursor: not-allowed;
}

.pipeline-menu-tooltip {
  display: block;
}

.pipeline-menu-item img,
.pipeline-avatar-fallback {
  display: inline-flex;
  width: 20px;
  height: 20px;
  flex: 0 0 20px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  object-fit: cover;
}

.pipeline-avatar-fallback {
  color: #3157e2;
  background: #eef2ff;
  font-size: 10px;
  font-weight: 600;
}

.pipeline-avatar-fallback--cli img {
  width: 14px;
  height: 14px;
}

/* CLI 直接执行的新建对话状态：CLI 与模型两个下拉并排。 */
/* 需求设计图：直接执行分组的 CLI/模型下拉与「创建对话」按钮直接内嵌在菜单里 */
.direct-cli {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 2px 12px 6px;
}

.direct-cli__select {
  width: 100%;
}

.direct-cli__submit {
  margin-top: 2px;
}

.pipeline-menu-heading-logo {
  width: 14px;
  height: 14px;
  flex: 0 0 14px;
  object-fit: contain;
}

.cli-runtime-hint--selected {
  margin-top: 4px;
  color: #262626;
}

.cli-runtime-hint {
  margin: 0;
  color: #8c8c8c;
  font-size: 12px;
}

.pipeline-menu-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pipeline-menu-state {
  display: flex;
  min-height: 72px;
  align-items: center;
  justify-content: center;
  padding: 12px;
  color: #8c8c8c;
  font-size: 12px;
  text-align: center;
}

.pipeline-menu-error {
  flex-direction: column;
  gap: 8px;
}

.pipeline-menu-error button,
.sidebar-error button,
.main-error button {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 0;
  border-radius: 6px;
  padding: 4px 8px;
  color: #3157e2;
  background: #eef2ff;
  cursor: pointer;
}

.conversation-list {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 4px;
  overflow-y: scroll;
  padding: 8px 24px;
  scrollbar-color: rgba(140, 149, 168, 0.42) transparent;
  scrollbar-gutter: stable;
}

.conversation-list::-webkit-scrollbar-thumb {
  background: rgba(140, 149, 168, 0.35);
}

.conversation-item {
  position: relative;
  display: flex;
  width: 100%;
  min-height: 70px;
  align-items: center;
  gap: 10px;
  margin: 0;
  border: 0;
  border-radius: 6px;
  padding: 12px;
  color: inherit;
  background: #fff;
  text-align: left;
}

.conversation-item:hover {
  background: #f2f4f7;
}

.conversation-item.active {
  background: #e5efff;
}

.new-conversation-item {
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  cursor: default;
}

.conversation-select-button {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  border: 0;
  padding: 0;
  color: inherit;
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.conversation-menu-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  padding: 4px;
  color: #000;
  background: transparent;
  cursor: pointer;
  opacity: 0;
  pointer-events: none;
  transition:
    opacity 180ms ease,
    background-color 180ms ease;
}

.conversation-item:hover .conversation-menu-button,
.conversation-item:focus-within .conversation-menu-button,
.conversation-menu-button[aria-expanded='true'] {
  opacity: 1;
  pointer-events: auto;
}

.conversation-menu-button:hover {
  background: #e4e6eb;
}

.conversation-menu-button:active {
  background: #d9dce2;
}

.conversation-menu-button img {
  display: block;
  width: 16px;
  height: 16px;
}

@media (prefers-reduced-motion: reduce) {
  .conversation-menu-button {
    transition: none;
  }
}

.conversation-title {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.conversation-title strong {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  color: #262626;
  font-size: 14px;
  font-weight: 500;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conversation-title i {
  width: 8px;
  height: 8px;
  flex: 0 0 8px;
  border-radius: 50%;
  background: #fb363f;
}

.conversation-subtitle {
  width: 100%;
  overflow: hidden;
  color: #8c8c8c;
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-loading,
.sidebar-error,
.sidebar-empty {
  display: flex;
  min-height: 220px;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  color: #8c8c8c;
  font-size: 12px;
  text-align: center;
}

.sidebar-empty :deep(svg) {
  color: #cbd5e1;
  font-size: 24px;
}

.conversation-main {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  background: #fff;
}

.conversation-loading-shell {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
}

.message-scroll {
  position: relative;
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 16px 32px 24px;
}

.message-loading-skeleton {
  padding-top: 32px;
}

.message-loading-skeleton :deep(.ant-skeleton-paragraph) {
  margin: 0;
}

.message-loading-skeleton :deep(.ant-skeleton-paragraph > li) {
  height: 18px;
  margin-top: 22px;
}

.composer-loading-skeleton {
  min-height: 136px;
  flex: 0 0 136px;
  box-sizing: border-box;
  padding: 8px 22px 24px;
  background: #fff;
}

.composer-loading-body {
  height: 94px;
  border: 1px solid #e5e7eb;
  background: #f7f8fa;
}

.message-refresh-indicator {
  position: sticky;
  z-index: 2;
  top: 0;
  display: flex;
  height: 0;
  justify-content: center;
  pointer-events: none;
}

.main-refresh-indicator {
  position: absolute;
  z-index: 2;
  top: 16px;
  left: 50%;
  pointer-events: none;
  transform: translateX(-50%);
}

.message-refresh-indicator :deep(.ant-spin) {
  margin-top: 4px;
  border-radius: 999px;
  padding: 5px 9px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.08);
}

.message-scroll :deep(.message-list-card) {
  overflow: visible;
  border: 0;
  border-radius: 0;
  box-shadow: none;
}

.message-scroll :deep(.message-list) {
  padding: 0;
}

.main-state {
  position: relative;
  display: flex;
  min-height: 0;
  flex: 1;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 8px;
  color: #8c8c8c;
  text-align: center;
}

.main-state > :deep(svg) {
  color: #cbd5e1;
  font-size: 34px;
}

.main-state h3 {
  margin: 4px 0 0;
  color: #595959;
  font-size: 14px;
}

.main-state p {
  margin: 0;
  font-size: 12px;
}

.new-conversation-state :deep(.pipeline-flow-icon) {
  width: 34px;
  height: 34px;
  color: #3157e2;
}

.context-menu {
  position: fixed;
  z-index: 1000;
  width: 174px;
  padding: 5px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.14);
}

.context-menu button {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 8px 9px;
  border: 0;
  border-radius: 5px;
  color: #4b5563;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  text-align: left;
}

.context-menu button:hover {
  background: #f5f6f8;
}

.context-menu button > img {
  display: block;
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
}

@media (max-width: 900px) {
  .conversation-sidebar {
    width: 240px;
    flex-basis: 240px;
  }

  .conversation-list,
  .sidebar-heading {
    padding-right: 12px;
    padding-left: 12px;
  }

  .message-scroll {
    padding-right: 20px;
    padding-left: 20px;
  }
}

@media (max-width: 700px) {
  .page-titlebar p {
    display: none;
  }

  .task-overview-head {
    align-items: flex-start;
    flex-direction: column;
    padding-bottom: 8px;
  }

  .task-overview-skeleton,
  .overview-skeleton-head {
    min-height: 88px;
  }

  .task-overview {
    min-height: 160px;
    max-height: 160px;
  }

  .task-overview-enter-from,
  .task-overview-leave-to {
    min-height: 0;
    max-height: 0;
  }

  .overview-skeleton-head {
    align-items: flex-start;
    justify-content: center;
    flex-direction: column;
    gap: 12px;
    padding-bottom: 8px;
  }

  .overview-skeleton-title {
    width: min(320px, 72%);
    height: 24px;
  }

  .task-overview-title h2 {
    font-size: 16px;
    line-height: 24px;
  }

  .task-step-region {
    padding-right: 16px;
    padding-left: 16px;
  }

  .composer-loading-skeleton {
    min-height: 176px;
    flex-basis: 176px;
    padding-right: 16px;
    padding-left: 16px;
  }

  .composer-loading-body {
    height: 134px;
  }

  .conversation-sidebar {
    width: 210px;
    flex-basis: 210px;
  }

  .conversation-item time {
    display: none;
  }

  .conversation-title,
  .conversation-subtitle {
    padding-right: 0;
  }
}
</style>
