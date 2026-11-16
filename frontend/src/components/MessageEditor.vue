<script setup lang="ts">
import { computed } from 'vue'
import type { ContentField, Issue, Limits, MessageContent, VariableInfo } from '../types'
import { checkContent } from '../utils/limits'
import { activeField } from '../utils/preview'
import CharCounter from './CharCounter.vue'
import EmailPreview from './EmailPreview.vue'
import PhonePreview from './PhonePreview.vue'

const props = defineProps<{
  modelValue: MessageContent
  readonly: boolean
  limits: Limits
  variables: VariableInfo[]
  /** The text is being written by the model right now (shows the blinking cursor). */
  typing?: boolean
  /** Problems reported by the server (e.g. after a rejected save). */
  serverIssues?: Issue[]
  /** Quality warnings for the saved text (offer missing, repeated word); they do not block saving. */
  warnings?: Issue[]
}>()
const emit = defineEmits<{ 'update:modelValue': [MessageContent] }>()

const check = computed(() => checkContent(props.modelValue, props.limits, props.variables))
const writing = computed(() => (props.typing ? activeField(props.modelValue) : null))

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

function warningsFor(field: ContentField): Issue[] {
  return (props.warnings ?? []).filter((w) => w.field === field)
}

function update(field: ContentField, value: string) {
  emit('update:modelValue', { ...props.modelValue, [field]: value })
}
</script>

<template>
  <div class="editor">
    <div class="fields">
      <p class="muted small vars">
        Kişiselleştirme (karakter sınırlarına sabit pay olarak sayılır):
        <code v-for="t in tokens" :key="t.name" :title="t.hint">{{ t.token }} = {{ t.budget }}</code>
      </p>

      <div class="field">
        <div class="row">
          <label for="sms">SMS</label>
          <CharCounter :used="check.smsLength" :max="limits.sms_max" variant="ring" />
        </div>
        <textarea
          id="sms"
          rows="4"
          :value="modelValue.sms_body"
          :readonly="readonly"
          :aria-invalid="issuesFor('sms_body').length > 0"
          @input="update('sms_body', ($event.target as HTMLTextAreaElement).value)"
        />
        <p v-for="i in issuesFor('sms_body')" :key="i.message" class="issue">{{ i.message }}</p>
        <p v-for="w in warningsFor('sms_body')" :key="w.message" class="warning" role="status">⚠ {{ w.message }}</p>
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
        <p v-for="w in warningsFor('email_subject')" :key="w.message" class="warning" role="status">⚠ {{ w.message }}</p>
      </div>

      <div class="field">
        <div class="row"><label for="body">E-posta metni</label></div>
        <textarea
          id="body"
          rows="9"
          :value="modelValue.email_body"
          :readonly="readonly"
          :aria-invalid="issuesFor('email_body').length > 0"
          @input="update('email_body', ($event.target as HTMLTextAreaElement).value)"
        />
        <p v-for="i in issuesFor('email_body')" :key="i.message" class="issue">{{ i.message }}</p>
        <p v-for="w in warningsFor('email_body')" :key="w.message" class="warning" role="status">⚠ {{ w.message }}</p>
      </div>
    </div>

    <aside class="previews" aria-label="Önizleme">
      <p class="muted small cap">Önizleme: kişi adı yerine örnek olarak “Elif” kullanılır.</p>
      <PhonePreview :text="modelValue.sms_body" :typing="writing === 'sms_body'" />
      <EmailPreview
        :subject="modelValue.email_subject"
        :body="modelValue.email_body"
        :typing-in="writing === 'email_subject' || writing === 'email_body' ? writing : null"
      />
    </aside>
  </div>
</template>

<style scoped>
.editor {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 340px);
  gap: 28px;
  align-items: start;
}
.field {
  margin-bottom: 18px;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
  min-height: 24px;
}
label {
  font-weight: 600;
}
textarea[readonly],
input[readonly] {
  background: var(--surface-2);
}
[aria-invalid='true'] {
  border-color: var(--error) !important;
}
.issue {
  color: var(--error);
  font-size: 0.85rem;
  margin: 4px 0 0;
}
.warning {
  color: var(--warn);
  font-size: 0.85rem;
  margin: 4px 0 0;
}
.vars code {
  margin: 0 4px;
}
.previews {
  display: grid;
  gap: 18px;
  justify-items: center;
  position: sticky;
  top: 76px;
}
.cap {
  margin: 0;
  justify-self: start;
}

@media (max-width: 860px) {
  .editor {
    grid-template-columns: minmax(0, 1fr);
  }
  .previews {
    position: static;
  }
}
</style>
