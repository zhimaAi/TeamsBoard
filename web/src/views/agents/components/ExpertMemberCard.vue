<template>
  <article class="member-card">
    <img :src="member.avatar || AGENT_AVATARS[0]" alt="" />
	<span><strong>{{ member.name }}</strong><small>{{ member.description || t('agents.noDescription') }}</small><em>{{ `${member.cli_type} / ${member.model_name}` }}</em></span>
    <div><button type="button" :aria-label="t('expertGroups.editAgent')" @click="emit('edit')"><EditOutlined /></button><button type="button" :aria-label="t('expertGroups.deleteAgent')" @click="emit('remove')"><DeleteOutlined /></button></div>
  </article>
</template>
<script setup lang="ts">
import { DeleteOutlined, EditOutlined } from '@ant-design/icons-vue'
import { useAppI18n } from '@/i18n'
import type { ExpertMember } from '@/types/pipeline'
import { AGENT_AVATARS } from './agentPipeline'
defineProps<{ member: ExpertMember }>()
const emit = defineEmits<{ edit: []; remove: [] }>()
const { t } = useAppI18n()
</script>
<style scoped>
.member-card{display:flex;min-width:0;align-items:center;gap:12px;padding:16px;border:1px solid #d9d9d9;border-radius:12px;background:#fff}.member-card>img{width:44px;height:44px;flex:0 0 44px;border-radius:50%;object-fit:cover}.member-card>span{display:flex;min-width:0;flex:1;flex-direction:column}.member-card strong,.member-card small,.member-card em{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.member-card small,.member-card em{color:#8c8c8c;font-size:12px;font-style:normal}.member-card>div{display:flex}.member-card button{width:30px;height:30px;border:0;border-radius:6px;color:#595959;background:transparent;cursor:pointer}.member-card button:hover{background:#f0f0f0}
</style>
