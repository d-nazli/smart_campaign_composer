<script setup lang="ts">
import { computed } from 'vue'
import type { ContentField, Issue, Limits, MessageContent, VariableInfo } from '../types'
import { checkContent } from '../utils/limits'
import CharCounter from './CharCounter.vue'

const props = defineProps<{
  modelValue: MessageContent
  readonly: boolean
  limits: Limits
  variables: VariableInfo[]
  /** Problems reported by the server (e.g. after a rejected save). */
  serverIssues?: Issue[]
}>()
const emit = defineEmits<{ 'update:modelValue': [MessageContent] }>()

const check = computed(() => checkContent(props.modelValue, props.limits, props.variables))

// Built in script: a literal "{{" in the template would be read as interpolation.
const tokens = computed(() =>
  props.variables.map((v) => ({
    name: v.name,
    token: '{{' + v.name + '}}',
    budget: v.budget,
    hint: `${v.budget} karakter pay ayrılır; kişinin değeri bundan uzunsa veya boşsa “${v.fallback}” yazılır`,
  })),
)

function issuesFor(field: ContentField): Issue[] {
  const seen = new Set<string>()
  return [...check.value.issues, ...(props.serverIssues ?? [])]
    .filter((i) => i.field === field)
    .filter((i) => (seen.has(i.message) ? false : (seen.add(i.message), true)))
}

function update(field: ContentField, value: string) {
  emit('update:modelValue', { ...props.modelValue, [field]: value })
}
</script>

<template>
  <div class="editor">
    <p class="muted small vars">
      Kişiselleştirme (karakter sınırlarına sabit pay olarak sayılır):
      <code v-for="t in tokens" :key="t.name" :title="t.hint">{{ t.token }} = {{ t.budget }}</code>
    </p>

    <div class="field">
      <div class="row">
        <label for="sms">SMS</label>
        <CharCounter :used="check.smsLength" :max="limits.sms_max" />
      </div>
      <textarea
        id="sms"
        rows="3"
        :value="modelValue.sms_body"
        :readonly="readonly"
        :aria-invalid="issuesFor('sms_body').length > 0"
        @input="update('sms_body', ($event.target as HTMLTextAreaElement).value)"
      />
      <p v-for="i in issuesFor('sms_body')" :key="i.message" class="issue">{{ i.message }}</p>
    </div>

    <div class="field">
      <div class="row">
        <label for="subject">E-posta konusu</label>
        <CharCounter :used="check.subjectLength" :max="limits.subject_max" />
      </div>
      <input
        id="subject"
        type="text"
        :value="modelValue.email_subject"
        :readonly="readonly"
        :aria-invalid="issuesFor('email_subject').length > 0"
        @input="update('email_subject', ($event.target as HTMLInputElement).value)"
      />
      <p v-for="i in issuesFor('email_subject')" :key="i.message" class="issue">{{ i.message }}</p>
    </div>

    <div class="field">
      <div class="row"><label for="body">E-posta metni</label></div>
      <textarea
        id="body"
        rows="8"
        :value="modelValue.email_body"
        :readonly="readonly"
        :aria-invalid="issuesFor('email_body').length > 0"
        @input="update('email_body', ($event.target as HTMLTextAreaElement).value)"
      />
      <p v-for="i in issuesFor('email_body')" :key="i.message" class="issue">{{ i.message }}</p>
    </div>
  </div>
</template>

<style scoped>
.field {
  margin-bottom: 16px;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 4px;
}
label {
  font-weight: 600;
}
textarea,
input {
  width: 100%;
  font: inherit;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg);
  color: var(--fg);
  resize: vertical;
}
textarea[readonly],
input[readonly] {
  background: var(--surface);
}
[aria-invalid='true'] {
  border-color: var(--error);
}
.issue {
  color: var(--error);
  font-size: 0.85rem;
  margin: 4px 0 0;
}
.vars code {
  margin: 0 4px;
}
</style>
