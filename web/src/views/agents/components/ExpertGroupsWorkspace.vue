<template>
  <div class="expert-workspace">
    <section v-if="selectedGroup" class="expert-detail-pane">
      <header class="detail-heading">
		<div>
		  <h2 :title="selectedGroup.name">{{ selectedGroup.name }}</h2>
		  <p :title="selectedGroup.description || t('agents.noDescription')">{{ selectedGroup.description || t('agents.noDescription') }}</p>
		</div>
        <span :class="selectedGroup.ready ? 'status-ready' : 'status-draft'">{{ selectedGroup.ready ? t('expertGroups.ready') : t('expertGroups.notReady') }}</span>
      </header>
      <div class="detail-scroll">
        <section class="member-section">
		  <div class="member-section-title"><strong>{{ t('expertGroups.leader') }}</strong><AgentAddDropdown :label="t('expertGroups.chooseLeader')" @create="openMemberEditor('leader')" @copy="openReusablePicker('leader')" /></div>
		  <MemberCard v-if="selectedGroup.leader" :member="selectedGroup.leader" @edit="openMemberEditor('leader', selectedGroup.leader)" @remove="removeMember(selectedGroup.leader)" />
          <a-empty v-else :description="t('expertGroups.chooseLeader')" />
        </section>
        <section class="member-section">
		  <div class="member-section-title"><strong>{{ t('expertGroups.members', { count: selectedGroup.members.length }) }}</strong><AgentAddDropdown :label="t('expertGroups.addMember')" @create="openMemberEditor('member')" @copy="openReusablePicker('member')" /></div>
          <div class="member-grid">
			<MemberCard v-for="member in selectedGroup.members" :key="member.uuid" :member="member" @edit="openMemberEditor('member', member)" @remove="removeMember(member)" />
          </div>
          <a-empty v-if="!selectedGroup.members.length" :description="t('expertGroups.addMember')" />
        </section>
      </div>
    </section>
    <section v-else class="expert-detail-empty"><a-empty :description="t('expertGroups.selectHint')" /></section>

	<PipelineEditorModal v-model:open="groupEditorOpen" kind="expert_group" :expert-group="editingGroup" @expert-created="handleGroupSaved" @expert-updated="handleGroupSaved" />
	<AgentEditorModal v-model:open="memberEditorOpen" :expert-group-uuid="selectedUuid" :expert-member="editingMember" :expert-role="memberRole" :current-avatar="editingMember?.avatar" @saved="reload(selectedUuid)" />
	<CopyAgentModal v-model:open="copyModalOpen" :target-expert-group-uuid="selectedUuid" :expert-role="memberRole" :source-steps="reusableSteps" :loading="reusableLoading" @saved="reload(selectedUuid)" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { message, Modal } from 'ant-design-vue'
import apiClient from '@/api/client'
import { useExpertGroupStore } from '@/stores/expert-group'
import type { ExpertGroup, ExpertMember, ReusableAgent } from '@/types/pipeline'
import { useAppI18n } from '@/i18n'
import { type ReusablePipelineStep } from './agentPipeline'
import MemberCard from './ExpertMemberCard.vue'
import AgentAddDropdown from './AgentAddDropdown.vue'
import AgentEditorModal from './AgentEditorModal.vue'
import CopyAgentModal from './CopyAgentModal.vue'
import PipelineEditorModal from './PipelineEditorModal.vue'

const { t } = useAppI18n()
const props = defineProps<{ selectedUuid: string }>()
const emit = defineEmits<{ 'update:selectedUuid': [uuid: string] }>()
const store = useExpertGroupStore()
const { items: groups } = storeToRefs(store)
const selectedUuid = computed({
	get: () => props.selectedUuid,
	set: (uuid: string) => emit('update:selectedUuid', uuid),
})
const selectedGroup = computed(() => groups.value.find((item) => item.uuid === selectedUuid.value))
const groupEditorOpen = ref(false)
const memberEditorOpen = ref(false)
const copyModalOpen = ref(false)
const editingGroup = ref<ExpertGroup>()
const editingMember = ref<ExpertMember>()
const memberRole = ref<'leader' | 'member'>('member')
const reusableAgents = ref<ReusableAgent[]>([])
const reusableLoading = ref(false)
const reusableSteps = computed<ReusablePipelineStep[]>(() => reusableAgents.value.map((agent, index) => ({
	...agent,
	pipelineName: agent.source_name,
	sort_order: index,
})))

async function reload(preferred = selectedUuid.value) {
  try {
    await store.load(true)
    selectedUuid.value = groups.value.some((item) => item.uuid === preferred) ? preferred : groups.value[0]?.uuid || ''
  } catch (error) {
    message.error(error instanceof Error ? error.message : t('expertGroups.loadFailed'))
  }
}
function openGroupEditor(group?: ExpertGroup) {
  editingGroup.value = group
  groupEditorOpen.value = true
}
function handleGroupSaved(group: ExpertGroup) {
	store.upsert(group)
	selectedUuid.value = group.uuid
}
async function handleGroupAction(key: string | number, group: ExpertGroup) {
  if (key === 'edit') return openGroupEditor(group)
  if (key === 'copy') {
    try { const item = await apiClient.post<ExpertGroup>(`/expert-groups/${group.uuid}/copy`, {}); await reload(item.uuid); message.success(t('expertGroups.copied', { name: item.name })) } catch (error) { message.error(error instanceof Error ? error.message : t('expertGroups.copyFailed')) }
    return
  }
  Modal.confirm({ title: t('expertGroups.deleteConfirm', { name: group.name }), okType: 'danger', onOk: async () => { try { await apiClient.delete(`/expert-groups/${group.uuid}`); store.remove(group.uuid); selectedUuid.value = groups.value[0]?.uuid || '' } catch (error) { message.error(error instanceof Error ? error.message : t('expertGroups.deleteFailed')); throw error } } })
}
function openMemberEditor(role: 'leader' | 'member', member?: ExpertMember) {
	memberRole.value = role
	editingMember.value = member
	memberEditorOpen.value = true
}
function removeMember(member: ExpertMember) { if (!selectedGroup.value) return; Modal.confirm({ title: t('expertGroups.deleteAgent'), okType: 'danger', onOk: async () => { try { await apiClient.delete(`/expert-groups/${selectedGroup.value?.uuid}/members/${member.uuid}`); await reload(selectedUuid.value); message.success(t('expertGroups.memberRemoved')) } catch (error) { message.error(error instanceof Error ? error.message : t('expertGroups.memberDeleteFailed')); throw error } } }) }
async function openReusablePicker(role: 'leader' | 'member') {
	memberRole.value = role
	copyModalOpen.value = true
	reusableLoading.value = true
	try {
		const result = await apiClient.get<{ items: ReusableAgent[] }>('/expert-groups/reusable-agents')
		reusableAgents.value = result.items || []
	} catch (error) {
		message.error(error instanceof Error ? error.message : t('expertGroups.loadFailed'))
	} finally {
		reusableLoading.value = false
	}
}

watch(() => groups.value.length, () => { if (!selectedUuid.value) selectedUuid.value = groups.value[0]?.uuid || '' }, { immediate: true })
defineExpose({ openGroupEditor, handleGroupAction })
</script>

<style scoped>
.expert-workspace{display:flex;min-height:0;flex:1;background:#fff}.expert-list-pane{display:flex;width:348px;min-height:0;flex:0 0 348px;flex-direction:column;border-right:1px solid #f0f0f0;overflow:hidden}.pane-header,.member-section-title,.detail-heading{display:flex;align-items:center;justify-content:space-between;gap:12px}.pane-header{height:64px;flex:0 0 64px;padding:0 20px;border-bottom:1px solid #f0f0f0}.group-list,.detail-scroll{overflow-y:auto}.group-list{min-height:0;flex:1;padding:16px}.group-card{display:flex;position:relative;margin-bottom:12px;border:1px solid #d9d9d9;border-radius:12px;background:#fff}.group-card.active{border-color:#3157e2;background:#e5efff}.group-main{display:flex;min-width:0;flex:1;gap:12px;padding:14px;border:0;background:transparent;text-align:left;cursor:pointer}.group-main>img{width:42px;height:42px;border-radius:50%;object-fit:cover}.group-copy{display:flex;min-width:0;flex:1;flex-direction:column}.group-copy strong,.group-copy small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.group-copy small{color:#8c8c8c}.avatar-row{display:flex;align-items:center;margin-top:10px}.avatar-row img{width:24px;height:24px;margin-right:-5px;border:2px solid #fff;border-radius:50%;object-fit:cover}.avatar-row em{margin-left:12px;font-size:11px;font-style:normal}.ready,.status-ready{color:#16a34a}.draft,.status-draft{color:#ed744a}.more-button{width:32px;height:32px;margin:8px;border:0;border-radius:6px;background:transparent;cursor:pointer}.expert-detail-pane{display:flex;min-width:0;min-height:0;flex:1;flex-direction:column}.detail-heading{min-height:76px;flex:0 0 76px;padding:0 24px;border-bottom:1px solid #f0f0f0}.detail-heading>div{min-width:0;flex:1}.detail-heading h2,.detail-heading p{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.detail-heading h2{margin:0;font-size:20px}.detail-heading p{margin:4px 0 0;color:#8c8c8c}.detail-heading>span{max-width:380px;flex:0 0 auto;font-size:12px}.detail-scroll{min-height:0;flex:1;padding:24px}.member-section{margin-bottom:28px}.member-section-title{margin-bottom:12px}.member-section-title :deep(.ant-btn){height:32px;padding-inline:16px;border-radius:6px;box-shadow:none}.member-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.expert-detail-empty{display:flex;flex:1;align-items:center;justify-content:center}@media(max-width:900px){.expert-workspace{flex-direction:column;overflow-y:auto}.expert-list-pane{width:100%;flex-basis:auto;border-right:0}.group-list{height:auto;max-height:360px}.member-grid{grid-template-columns:1fr}}
</style>
