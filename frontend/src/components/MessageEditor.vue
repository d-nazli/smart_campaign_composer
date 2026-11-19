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
  /** The locale being edited: decides which fallback word the hint names; shown over the previews. */
  locale?: string
  /** Its writing direction: the fields and the previews follow it. */
  dir?: 'ltr' | 'rtl'
}>()

// Layout: the "Mesajlar" panel (the wizard fills its header, the locale column on the left and
// the area above the fields through slots) and, on the right, the previews of the locale on
// screen. 65 / 35, the previews stay in view; one column below 1280px, where the locale
// column becomes a picker above the fields.
const emit = defineEmits<{ 'update:modelValue': [MessageContent] }>()

const check = computed(() => checkContent(props.modelValue, props.limits, props.variables))
const writing = computed(() => (props.typing ? activeField(props.modelValue) : null))

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
    <Card title="Mesajlar" :flush="!!$slots.locales">
      <template v-if="$slots.actions" #actions><slot name="actions" /></template>
      <slot name="tabs" />
      <div :class="{ msg: !!$slots.locales }">
      <div v-if="$slots.locales" class="msg__locales"><slot name="locales" /></div>
      <div class="editor">
        <slot name="top" />

        <div class="field">
          <div class="row">
            <label for="sms" class="label">SMS</label>
            <CharCounter :used="check.smsLength" :max="limits.sms_max" variant="ring" />
          </div>
          <textarea
            id="sms"
            rows="3"
            :dir="dir"
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
            :dir="dir"
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
            :dir="dir"
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
      </div>
    </Card>

    <aside aria-label="Önizleme">
      <Card title="Önizleme">
        <template v-if="locale" #actions><Badge data-testid="preview-locale">{{ locale }}</Badge></template>
        <div class="previews">
          <PhonePreview :text="modelValue.sms_body" :typing="writing === 'sms_body'" :dir="dir" :lang="locale" />
          <EmailPreview
            :dir="dir"
            :lang="locale"
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
.msg {
  display: grid;
  grid-template-columns: 208px minmax(0, 1fr);
}
.msg__locales {
  border-right: 1px solid var(--border);
  min-width: 0;
}
.msg .editor {
  padding: var(--s-6);
  min-width: 0;
}
@media (max-width: 1279px) {
  .msg {
    grid-template-columns: minmax(0, 1fr);
  }
  .msg__locales {
    border-right: 0;
    padding: var(--s-4) var(--s-6) 0;
  }
}
@media (max-width: 767px) {
  .msg__locales,
  .msg .editor {
    padding-inline: var(--s-4);
  }
}
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
.previews {
  display: grid;
  gap: var(--s-5);
  justify-items: center;
}
</style>
