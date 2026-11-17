<script setup lang="ts">
import { computed } from 'vue'
import type { ContentField, Issue, Limits, MessageContent, VariableInfo } from '../types'
import { checkContent } from '../utils/limits'
import { activeField } from '../utils/preview'
import Badge from './ui/Badge.vue'
import Card from './ui/Card.vue'
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
  /** The language being edited: decides which fallback word the hint names. */
  language?: string
  /** That language's own name, shown over the previews. */
  languageName?: string
}>()

// Layout: the "Mesajlar" panel (the wizard fills its header and the area above the fields
// through slots: model status, notices, language tabs) and, on the right, the previews
// of the language on screen. 65 / 35, the previews stay in view; one column below 1280px.
const emit = defineEmits<{ 'update:modelValue': [MessageContent] }>()

const check = computed(() => checkContent(props.modelValue, props.limits, props.variables))
const writing = computed(() => (props.typing ? activeField(props.modelValue) : null))

// Built in script: a literal "{{" in the template would be read as interpolation.
const tokens = computed(() =>
  props.variables.map((v) => ({
    name: v.name,
    token: '{{' + v.name + '}}',
    budget: v.budget,
    hint: `${v.budget} karakter pay ayrılır; kişinin değeri bundan uzunsa veya boşsa “${v.fallbacks?.[props.language ?? ''] ?? v.fallback}” yazılır`,
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
  <div class="split split--wide">
    <Card title="Mesajlar">
      <template v-if="$slots.actions" #actions><slot name="actions" /></template>
      <div class="editor">
        <slot name="top" />

        <p v-if="tokens.length" class="vars">
          <span class="label">Kişiselleştirme</span>
          <code v-for="t in tokens" :key="t.name" :title="t.hint">{{ t.token }} = {{ t.budget }}</code>
        </p>

        <div class="field">
          <div class="row">
            <label for="sms" class="label">SMS</label>
            <CharCounter :used="check.smsLength" :max="limits.sms_max" variant="ring" />
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
          <p v-for="w in warningsFor('sms_body')" :key="w.message" class="warning" role="status">⚠ {{ w.message }}</p>
        </div>

        <div class="field">
          <div class="row">
            <label for="subject" class="label">E-posta konusu</label>
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
          <div class="row"><label for="body" class="label">E-posta metni</label></div>
          <textarea
            id="body"
            rows="8"
            :value="modelValue.email_body"
            :readonly="readonly"
            :aria-invalid="issuesFor('email_body').length > 0"
            @input="update('email_body', ($event.target as HTMLTextAreaElement).value)"
          />
          <p v-for="i in issuesFor('email_body')" :key="i.message" class="issue">{{ i.message }}</p>
          <p v-for="w in warningsFor('email_body')" :key="w.message" class="warning" role="status">⚠ {{ w.message }}</p>
        </div>

        <slot name="bottom" />
      </div>
    </Card>

    <aside aria-label="Önizleme">
      <Card title="Önizleme">
        <template v-if="languageName" #actions><Badge>{{ languageName }}</Badge></template>
        <div class="previews">
          <PhonePreview :text="modelValue.sms_body" :typing="writing === 'sms_body'" />
          <EmailPreview
            :subject="modelValue.email_subject"
            :body="modelValue.email_body"
            :typing-in="writing === 'email_subject' || writing === 'email_body' ? writing : null"
          />
        </div>
      </Card>
    </aside>
  </div>
</template>

<style scoped>
.editor {
  display: flex;
  flex-direction: column;
  gap: var(--s-4);
}
.field {
  display: grid;
  gap: 6px;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  min-height: 40px;
}
[aria-invalid='true'] {
  border-color: var(--error) !important;
}
.issue {
  color: var(--error);
  font-size: var(--text-sm);
  margin: 0;
}
.warning {
  color: var(--warn);
  font-size: var(--text-sm);
  margin: 0;
}
.vars {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin: 0;
}
.vars .label {
  margin-right: 4px;
}
.vars code {
  cursor: help;
}
.previews {
  display: grid;
  gap: var(--s-5);
  justify-items: center;
}
</style>
