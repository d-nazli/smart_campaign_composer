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
  margin: 0;
  width: 100%;
  max-width: 270px;
  padding: 9px;
  border-radius: 34px;
  background: #15181e;
  box-shadow: var(--shadow-device);
}
.screen {
  min-height: 300px;
  border-radius: 26px;
  background: #f1f2f4;
  color: #15181e;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.status {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 9px 18px 4px;
  font-size: 0.7rem;
  font-weight: 650;
}
.notch {
  width: 64px;
  height: 14px;
  border-radius: 14px;
  background: #15181e;
}
.thread {
  padding: 18px 12px;
}
.who {
  text-align: center;
  font-size: 0.7rem;
  color: #5e6573;
  margin-bottom: 12px;
  font-weight: 600;
}
.bubble {
  display: inline-block;
  max-width: 92%;
  padding: 9px 13px;
  border-radius: 18px 18px 18px 5px;
  background: #dcdfe4;
  font-size: 0.88rem;
  line-height: 1.4;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.bubble.empty {
  background: transparent;
  border: 1px dashed #aeb4be;
  color: #5e6573;
}
.time {
  margin: 4px 0 0 6px;
  font-size: 0.68rem;
  color: #5e6573;
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
