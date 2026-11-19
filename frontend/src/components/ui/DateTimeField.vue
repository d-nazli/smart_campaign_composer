<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { CalendarClock, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import {
  HOURS,
  MINUTES,
  WEEKDAYS,
  dayDisabled,
  displayValue,
  endOfDay,
  longValue,
  momentDisabled,
  monthGrid,
  monthLabel,
  sameDay,
  shiftMonth,
  startOfDay,
  withDay,
  withTime,
} from '../../utils/calendar'
import { fromLocalInput, toLocalInput } from '../../utils/schedule'
import Button from './Button.vue'

export interface Shortcut {
  label: string
  /** A "YYYY-MM-DDTHH:mm" value. */
  value: string
}

// Picks a date and a time without the browser's own widget. The value is the same
// "YYYY-MM-DDTHH:mm" string a datetime-local input uses, in the viewer's time zone.
// Days outside min/max are shown but cannot be chosen.
const props = withDefaults(
  defineProps<{
    modelValue: string
    label: string
    min?: string
    max?: string
    invalid?: boolean
    placeholder?: string
    shortcuts?: Shortcut[]
    testid?: string
  }>(),
  { placeholder: 'Tarih ve saat seçin' },
)
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const value = computed(() => fromLocalInput(props.modelValue))
const min = computed(() => (props.min ? fromLocalInput(props.min) : null))
const max = computed(() => (props.max ? fromLocalInput(props.max) : null))

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const popup = ref<HTMLElement | null>(null)

const anchor = () => value.value ?? min.value ?? new Date()
const view = ref({ year: anchor().getFullYear(), month: anchor().getMonth() })
const today = new Date()

const grid = computed(() => monthGrid(view.value.year, view.value.month))
const title = computed(() => monthLabel(view.value.year, view.value.month))
const canPrev = computed(() => {
  const before = shiftMonth(view.value.year, view.value.month, -1)
  return !min.value || endOfDay(new Date(before.year, before.month + 1, 0)).getTime() >= min.value.getTime()
})
const canNext = computed(() => {
  const after = shiftMonth(view.value.year, view.value.month, 1)
  return !max.value || startOfDay(new Date(after.year, after.month, 1)).getTime() <= max.value.getTime()
})

const two = (n: number) => String(n).padStart(2, '0')
// Ids that tie each time column to its heading.
const uid = Math.random().toString(36).slice(2, 8)
const hourHead = `ui-dt-h-${uid}`
const minuteHead = `ui-dt-m-${uid}`

function pickDay(day: Date) {
  if (dayDisabled(day, min.value, max.value)) return
  emit('update:modelValue', toLocalInput(withDay(value.value, day, min.value, max.value)))
}
function pickHour(h: number) {
  const m = value.value ? value.value.getMinutes() : 0
  emit('update:modelValue', toLocalInput(withTime(value.value, anchor(), h, m, min.value, max.value)))
}
function pickMinute(m: number) {
  const h = value.value ? value.value.getHours() : 9
  emit('update:modelValue', toLocalInput(withTime(value.value, anchor(), h, m, min.value, max.value)))
}
const hourOff = (h: number) => {
  const day = value.value ?? anchor()
  const start = new Date(day.getFullYear(), day.getMonth(), day.getDate(), h, 0)
  const end = new Date(day.getFullYear(), day.getMonth(), day.getDate(), h, 59)
  return (min.value !== null && end.getTime() < min.value.getTime()) || (max.value !== null && start.getTime() > max.value.getTime())
}
const minuteOff = (m: number) => {
  const day = value.value ?? anchor()
  const h = value.value ? value.value.getHours() : 9
  return momentDisabled(new Date(day.getFullYear(), day.getMonth(), day.getDate(), h, m), min.value, max.value)
}

// Each time column is one Tab stop (the chosen option, or the first one that can be
// chosen); the arrow keys, Home and End move inside it, Enter or Space chooses.
function tabStop(list: readonly number[], chosen: number | null, off: (n: number) => boolean): number {
  if (chosen !== null && !off(chosen)) return chosen
  return list.find((n) => !off(n)) ?? list[0]
}
const hourStop = computed(() => tabStop(HOURS, value.value ? value.value.getHours() : null, hourOff))
const minuteStop = computed(() => tabStop(MINUTES, value.value ? value.value.getMinutes() : null, minuteOff))

function moveInColumn(e: KeyboardEvent) {
  const col = e.currentTarget as HTMLElement
  const options = Array.from(col.querySelectorAll<HTMLButtonElement>('[role="option"]:not(:disabled)'))
  if (options.length === 0) return
  const at = options.indexOf(document.activeElement as HTMLButtonElement)
  let next: number
  switch (e.key) {
    case 'ArrowDown':
    case 'ArrowRight':
      next = Math.min(at + 1, options.length - 1)
      break
    case 'ArrowUp':
    case 'ArrowLeft':
      next = Math.max(at - 1, 0)
      break
    case 'Home':
      next = 0
      break
    case 'End':
      next = options.length - 1
      break
    default:
      return
  }
  e.preventDefault()
  options[next].focus()
}

function shift(by: number) {
  view.value = shiftMonth(view.value.year, view.value.month, by)
}

function usable(s: Shortcut): boolean {
  const d = fromLocalInput(s.value)
  return d !== null && !momentDisabled(d, min.value, max.value)
}
function useShortcut(s: Shortcut) {
  emit('update:modelValue', s.value)
  const d = fromLocalInput(s.value)
  if (d) view.value = { year: d.getFullYear(), month: d.getMonth() }
  open.value = false
  trigger.value?.focus()
}

async function toggle() {
  open.value = !open.value
  if (open.value) {
    view.value = { year: anchor().getFullYear(), month: anchor().getMonth() }
    await nextTick()
    // The time columns open on the chosen hour and minute, not at 00:00.
    popup.value?.querySelectorAll<HTMLElement>('.ui-dt__col').forEach((col) => {
      const on = col.querySelector<HTMLElement>('.is-on')
      if (on) col.scrollTop += on.getBoundingClientRect().top - col.getBoundingClientRect().top - col.clientHeight / 2 + on.clientHeight / 2
    })
    popup.value?.querySelector<HTMLElement>('[aria-pressed="true"], .ui-dt__day:not(:disabled)')?.focus({ preventScroll: true })
  }
}
function close(returnFocus = true) {
  open.value = false
  if (returnFocus) trigger.value?.focus()
}

function onOutside(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) close(false)
}
watch(open, (o) => {
  if (o) document.addEventListener('mousedown', onOutside)
  else document.removeEventListener('mousedown', onOutside)
})
onBeforeUnmount(() => document.removeEventListener('mousedown', onOutside))

const shown = computed(() => (value.value ? displayValue(value.value) : ''))
const written = computed(() => (value.value ? longValue(value.value) : ''))
</script>

<template>
  <div ref="root" class="ui-dt" @keydown.esc.stop="open && close()">
    <button
      ref="trigger"
      type="button"
      class="ui-dt__trigger"
      :class="{ 'is-invalid': invalid, 'is-empty': !shown }"
      aria-haspopup="dialog"
      :aria-expanded="open"
      :aria-label="shown ? `${label}: ${shown}` : label"
      :aria-invalid="invalid || undefined"
      :data-testid="testid"
      @click="toggle"
    >
      <CalendarClock :size="16" aria-hidden="true" />
      <span class="ui-dt__text num">{{ shown || placeholder }}</span>
    </button>

    <div v-if="open" ref="popup" class="ui-dt__pop" role="dialog" :aria-label="label">
      <div class="ui-dt__cal">
        <div class="ui-dt__nav">
          <button type="button" class="ui-dt__arrow" :disabled="!canPrev" aria-label="Önceki ay" @click="shift(-1)"><ChevronLeft :size="16" aria-hidden="true" /></button>
          <strong class="ui-dt__month" aria-live="polite">{{ title }}</strong>
          <button type="button" class="ui-dt__arrow" :disabled="!canNext" aria-label="Sonraki ay" @click="shift(1)"><ChevronRight :size="16" aria-hidden="true" /></button>
        </div>
        <div class="ui-dt__week" aria-hidden="true"><span v-for="w in WEEKDAYS" :key="w">{{ w }}</span></div>
        <div class="ui-dt__days">
          <button
            v-for="c in grid"
            :key="c.date.getTime()"
            type="button"
            class="ui-dt__day num"
            :class="{ 'is-out': !c.inMonth, 'is-today': sameDay(c.date, today), 'is-on': value !== null && sameDay(c.date, value) }"
            :disabled="dayDisabled(c.date, min, max)"
            :aria-pressed="value !== null && sameDay(c.date, value)"
            :aria-label="c.date.toLocaleDateString('tr-TR', { day: 'numeric', month: 'long', year: 'numeric', weekday: 'long' })"
            @click="pickDay(c.date)"
          >
            {{ c.date.getDate() }}
          </button>
        </div>
      </div>

      <div class="ui-dt__time" role="group" aria-label="Saat ve dakika">
        <div class="ui-dt__colwrap">
          <span :id="hourHead" class="ui-dt__head">Saat</span>
          <div class="ui-dt__col" role="listbox" aria-label="Saat" :aria-labelledby="hourHead" @keydown="moveInColumn">
            <button
              v-for="h in HOURS"
              :key="h"
              type="button"
              role="option"
              class="ui-dt__cell num"
              :class="{ 'is-on': value !== null && value.getHours() === h }"
              :aria-selected="value !== null && value.getHours() === h"
              :tabindex="h === hourStop ? 0 : -1"
              :disabled="hourOff(h)"
              @click="pickHour(h)"
            >
              {{ two(h) }}
            </button>
          </div>
        </div>
        <div class="ui-dt__colwrap">
          <span :id="minuteHead" class="ui-dt__head">Dakika</span>
          <div class="ui-dt__col" role="listbox" aria-label="Dakika" :aria-labelledby="minuteHead" @keydown="moveInColumn">
            <button
              v-for="m in MINUTES"
              :key="m"
              type="button"
              role="option"
              class="ui-dt__cell num"
              :class="{ 'is-on': value !== null && value.getMinutes() === m }"
              :aria-selected="value !== null && value.getMinutes() === m"
              :tabindex="m === minuteStop ? 0 : -1"
              :disabled="minuteOff(m)"
              @click="pickMinute(m)"
            >
              {{ two(m) }}
            </button>
          </div>
        </div>
      </div>

      <p class="ui-dt__chosen" aria-live="polite" data-testid="dt-chosen">
        <template v-if="written">Seçilen: <strong class="num">{{ written }}</strong></template>
        <template v-else>Henüz zaman seçilmedi</template>
      </p>

      <div class="ui-dt__foot">
        <div class="ui-dt__shortcuts">
          <button v-for="s in shortcuts ?? []" :key="s.label" type="button" class="ui-dt__chip" :disabled="!usable(s)" @click="useShortcut(s)">{{ s.label }}</button>
        </div>
        <Button variant="primary" size="sm" :disabled="!value" @click="close()">Tamam</Button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ui-dt {
  position: relative;
  display: inline-block;
}
.ui-dt__trigger {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  min-height: var(--control-h);
  min-width: 240px;
  padding: 0 var(--s-3);
  border: 1px solid var(--border-strong);
  border-radius: var(--r-sm);
  background: var(--surface);
  color: var(--fg);
  font-weight: 400;
  text-align: left;
}
.ui-dt__trigger:hover:not(:disabled) {
  background: var(--surface);
  border-color: var(--muted);
}
.ui-dt__trigger.is-empty .ui-dt__text {
  color: var(--muted);
}
.ui-dt__trigger.is-invalid {
  border-color: var(--error);
}
.ui-dt__trigger[aria-expanded='true'] {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}
.ui-dt__pop {
  position: absolute;
  z-index: 40;
  top: calc(100% + 6px);
  left: 0;
  display: grid;
  grid-template-columns: auto auto;
  gap: var(--s-4);
  padding: var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-pop);
}
.ui-dt__cal {
  width: 252px;
}
.ui-dt__nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--s-2);
}
.ui-dt__month {
  font-weight: 600;
}
.ui-dt__arrow {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--muted);
}
.ui-dt__week,
.ui-dt__days {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
}
.ui-dt__week {
  margin-bottom: 2px;
  color: var(--muted);
  font-size: var(--text-xs);
  text-align: center;
}
.ui-dt__day {
  height: 34px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--r-sm);
  background: transparent;
  font-weight: 400;
}
.ui-dt__day.is-out {
  color: var(--muted);
  opacity: 0.55;
}
.ui-dt__day.is-today {
  border-color: var(--border-strong);
}
.ui-dt__day.is-on,
.ui-dt__day.is-on:hover:not(:disabled) {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-fg);
  opacity: 1;
}
.ui-dt__day:disabled {
  opacity: 0.28;
  text-decoration: line-through;
}
.ui-dt__time {
  display: flex;
  gap: var(--s-2);
}
.ui-dt__colwrap {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  min-height: 0;
  min-width: 0;
}
.ui-dt__head {
  color: var(--muted);
  font-size: var(--text-xs);
  font-weight: 600;
  text-align: center;
}
.ui-dt__chosen {
  grid-column: 1 / -1;
  margin: 0;
  color: var(--muted);
  font-size: var(--text-sm);
}
.ui-dt__chosen strong {
  color: var(--fg);
}
.ui-dt__col {
  max-height: 258px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 52px;
  overflow-y: auto;
  padding-right: 2px;
}
.ui-dt__cell {
  flex: none;
  height: 30px;
  padding: 0;
  border: 1px solid transparent;
  border-radius: var(--r-sm);
  background: transparent;
  font-weight: 400;
}
.ui-dt__cell.is-on,
.ui-dt__cell.is-on:hover:not(:disabled) {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--accent);
  font-weight: 600;
}
.ui-dt__cell:disabled {
  opacity: 0.28;
}
.ui-dt__foot {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
  padding-top: var(--s-3);
  border-top: 1px solid var(--border);
}
.ui-dt__shortcuts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-2);
}
.ui-dt__chip {
  height: 26px;
  padding: 0 var(--s-3);
  border-radius: 999px;
  font-size: var(--text-xs);
  border: 1px solid var(--border-strong);
}
@media (max-width: 560px) {
  .ui-dt__pop {
    grid-template-columns: 1fr;
    left: 0;
    right: 0;
  }
  .ui-dt__cal {
    width: 100%;
  }
  .ui-dt__head {
    text-align: left;
  }
  .ui-dt__time .ui-dt__col {
    flex-direction: row;
    width: auto;
    overflow-x: auto;
    overflow-y: hidden;
  }
  .ui-dt__time {
    flex-direction: column;
  }
  .ui-dt__cell {
    width: 40px;
  }
}
</style>
