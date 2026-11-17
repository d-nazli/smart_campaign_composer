<script setup lang="ts">
import { computed } from 'vue'
import { fillSample } from '../utils/preview'

// An SMS inside a phone screen. {{first_name}} is replaced with a sample name.
const props = defineProps<{ text: string; typing?: boolean }>()
const shown = computed(() => fillSample(props.text))
</script>

<template>
  <figure class="phone" aria-label="SMS önizlemesi">
    <div class="screen">
      <div class="status" aria-hidden="true"><span>09:41</span><span class="notch" /><span>5G</span></div>
      <div class="thread">
        <div class="who">MARKA</div>
        <div class="bubble" :class="{ empty: !shown && !typing }">
          <template v-if="shown || typing">
            {{ shown }}<span v-if="typing" class="cursor" aria-hidden="true" />
          </template>
          <template v-else>Mesaj üretilince burada görünür.</template>
        </div>
        <div class="time" aria-hidden="true">şimdi</div>
      </div>
    </div>
  </figure>
</template>

<style scoped>
.phone {
  margin: 0 auto;
  width: 100%;
  max-width: 300px;
  padding: 10px;
  border-radius: 36px;
  background: var(--device-frame);
  box-shadow: var(--shadow-device);
}
.screen {
  min-height: 320px;
  border-radius: 28px;
  background: var(--device-screen);
  color: var(--device-fg);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.status {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 20px 4px;
  font-size: 0.72rem;
  font-weight: 650;
}
.notch {
  width: 68px;
  height: 15px;
  border-radius: 15px;
  background: var(--device-frame);
}
.thread {
  padding: 20px 14px;
}
.who {
  text-align: center;
  font-size: 0.72rem;
  color: var(--device-muted);
  margin-bottom: 12px;
  font-weight: 600;
}
.bubble {
  display: inline-block;
  max-width: 92%;
  padding: 10px 14px;
  border-radius: 18px 18px 18px 5px;
  background: var(--device-bubble);
  font-size: var(--text-sm);
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.bubble.empty {
  background: transparent;
  border: 1px dashed var(--device-dot);
  color: var(--device-muted);
}
.time {
  margin: 4px 0 0 6px;
  font-size: 0.7rem;
  color: var(--device-muted);
}
.cursor {
  display: inline-block;
  width: 2px;
  height: 1em;
  margin-left: 1px;
  vertical-align: text-bottom;
  background: currentColor;
  animation: blink 1s steps(1) infinite;
}
@keyframes blink {
  50% {
    opacity: 0;
  }
}
</style>
