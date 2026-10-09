import apiClient from './client'
import type { paths, components } from '@/types/knowledge-api'
import type { ReferenceSpillResult } from '@/types/knowledge-reference'

// S-IN-01: 知识库路由封装，路径与字段均来自 output_openapi.yaml

type RootConfig = paths['/knowledge/root']['get']['responses'][200]['content']['application/json']
type PutRootResult = paths['/knowledge/root']['put']['responses'][200]['content']['application/json']
type PutRootBody = paths['/knowledge/root']['put']['requestBody']['content']['application/json']
type ScanResult = paths['/knowledge/scan']['post']['responses'][200]['content']['application/json']
type SpillBody =
  paths['/tasks/{uuid}/files/reference']['post']['requestBody']['content']['application/json']
type SpillResponse =
  paths['/tasks/{uuid}/files/reference']['post']['responses'][200]['content']['application/json']

export type KnowledgeDocument = components['schemas']['Document']
export type KnowledgeFolderNode = components['schemas']['FolderNode']
export type KnowledgeDocumentPath = components['schemas']['DocumentPath']
export type KnowledgeSearchResult = components['schemas']['SearchResult']
export type KnowledgeMigrateConflictItem = components['schemas']['MigrateConflictItem']
export type KnowledgeMigrateRollback = components['schemas']['MigrateRollback']

type CreateDocumentBody = paths['/knowledge/documents']['post']['requestBody']['content']['application/json']
type UpdateDocumentBody = paths['/knowledge/documents/{uuid}']['put']['requestBody']['content']['application/json']
type CreateFolderBody = paths['/knowledge/folders']['post']['requestBody']['content']['application/json']
type UpdateFolderBody = paths['/knowledge/folders/{id}']['put']['requestBody']['content']['application/json']
type DeleteFolderBody = paths['/knowledge/folders/{id}']['delete']['requestBody']['content']['application/json']
type ConfigValueBody = paths['/config/key/{key}']['put']['requestBody']['content']['application/json']

export function getKnowledgeRoot() {
  return apiClient.get<RootConfig>('/knowledge/root')
}

// S-IN-06: 更换 KR 必须显式声明 mode=migrate（缺省/非法后端一律 400，禁止静默沿用旧的"仅切指向"语义）
export function migrateKnowledgeRoot(dir: string) {
  return apiClient.put<PutRootResult>('/knowledge/root', {
    dir,
    mode: 'migrate',
  } satisfies PutRootBody)
}

export function scanKnowledge() {
  return apiClient.post<ScanResult>('/knowledge/scan')
}

export function listKnowledgeFolders() {
  return apiClient.get<{ data: KnowledgeFolderNode[] }>('/knowledge/folders')
}

export function createKnowledgeFolder(body: CreateFolderBody) {
  return apiClient.post<KnowledgeFolderNode>('/knowledge/folders', body)
}

export function updateKnowledgeFolder(id: number, body: UpdateFolderBody) {
  return apiClient.put<{ id: number; updated_at: number }>(`/knowledge/folders/${id}`, body)
}

export function deleteKnowledgeFolder(id: number, mode: 'keep' | 'purge') {
  return apiClient.delete<{ id: number; mode: 'keep' | 'purge'; deleted: boolean }>(
    `/knowledge/folders/${id}`,
    { mode } satisfies DeleteFolderBody,
  )
}

export function listKnowledgeDocuments(folderId?: number, keyword?: string) {
  return apiClient.get<{ data: KnowledgeDocument[] }>('/knowledge/documents', {
    folder_id: folderId,
    keyword,
  })
}

export function createKnowledgeDocument(body: CreateDocumentBody) {
  return apiClient.post<KnowledgeDocument>('/knowledge/documents', body)
}

export function getKnowledgeDocument(uuid: string) {
  return apiClient.get<KnowledgeDocument>(`/knowledge/documents/${uuid}`)
}

export function updateKnowledgeDocument(uuid: string, body: UpdateDocumentBody) {
  return apiClient.put<KnowledgeDocument>(`/knowledge/documents/${uuid}`, body)
}

export function deleteKnowledgeDocument(uuid: string) {
  return apiClient.delete<{ uuid: string; deleted: boolean }>(`/knowledge/documents/${uuid}`)
}

export function restoreKnowledgeDocument(uuid: string) {
  return apiClient.post<{ uuid: string; restored: boolean; updated_at: number }>(
    `/knowledge/documents/${uuid}/restore`,
  )
}

export function hardDeleteKnowledgeDocument(uuid: string) {
  return apiClient.delete<{ uuid: string; deleted: boolean; permanent: boolean }>(
    `/knowledge/documents/${uuid}/permanent`,
  )
}

export function getKnowledgeDocumentPath(uuid: string) {
  return apiClient.get<KnowledgeDocumentPath>(`/knowledge/documents/${uuid}/path`)
}

export function searchKnowledge(q: string) {
  return apiClient.get<{ items: KnowledgeSearchResult[]; total: number }>('/knowledge/search', { q })
}

export function listKnowledgeTrash() {
  return apiClient.get<{ data: KnowledgeDocument[] }>('/knowledge/trash')
}

// S-IN-08 / S-DA-12: 超长引用内容唯一命名落盘到任务产出目录（契约见 03/output_openapi.yaml）
export function spillTaskReferenceFile(
  taskUuid: string,
  body: SpillBody,
): Promise<ReferenceSpillResult> {
  return apiClient.post<SpillResponse>(`/tasks/${encodeURIComponent(taskUuid)}/files/reference`, body)
}

export function getConfigKey(key: string) {
  return apiClient.get<{ value: string }>(`/config/key/${encodeURIComponent(key)}`)
}

export function putConfigKey(key: string, value: string) {
  return apiClient.put<{ key: string; value: string }>(`/config/key/${encodeURIComponent(key)}`, {
    value,
  } satisfies ConfigValueBody)
}
