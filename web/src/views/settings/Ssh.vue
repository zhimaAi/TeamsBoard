<script setup lang="ts">
import ConfigResourceTable, { type ConfigField } from '@/components/ConfigResourceTable.vue'

const fields: ConfigField[] = [
  { key: 'name', label: '名称', required: true, placeholder: '例如：测试服务器' },
  { key: 'host', label: '主机', required: true, placeholder: '192.168.1.10' },
  { key: 'port', label: '端口', type: 'number', defaultValue: 22, width: 90 },
  { key: 'username', label: '用户名', placeholder: 'root' },
  { key: 'password', label: '密码', type: 'password', table: false },
  // { key: 'key_path', label: 'Private key path', placeholder: 'C:\Users\me\.ssh\id_ed25519' },
]
</script>

<template>
  <ConfigResourceTable
    title="SSH 配置"
    endpoint="/config/ssh-profiles"
    description="供数据库隧道和远程命令使用。密码写入系统安全存储，不会在列表中返回。"
    :fields="fields"
    :testable="true"
  />
</template>
