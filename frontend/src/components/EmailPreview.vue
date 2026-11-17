<script setup lang="ts">
import { computed } from 'vue'
import { fillSample } from '../utils/preview'

// An e-mail inside a mail client window. {{first_name}} is replaced with a sample name.
const props = defineProps<{ subject: string; body: string; typingIn?: 'email_subject' | 'email_body' | null }>()
const subject = computed(() => fillSample(props.subject))
const body = computed(() => fillSample(props.body))
</script>

<template>
  <figure class="mail" aria-label="E-posta önizlemesi">
    <div class="chrome" aria-hidden="true">
      <span class="d" /><span class="d" /><span class="d" />
      <span class="title">Gelen kutusu</span>
    </div>
    <div class="msg">
      <h3 class="subject" :class="{ empty: !subject && typingIn !== 'email_subject' }">
        <template v-if="subject || typingIn === 'email_subject'">
          {{ subject }}<span v-if="typingIn === 'email_subject'" class="cursor" aria-hidden="true" />
        </template>
        <template v-else>Konu burada görünür</template>
      </h3>
      <div class="from">
        <span class="avatar" aria-hidden="true">M</span>
        <div>
          <strong>Marka</strong> <span class="addr">&lt;merhaba@marka.example&gt;</span>
          <div class="to">Alıcı: Elif</div>
        </div>
      </div>
      <div class="body" :class="{ empty: !body && typingIn !== 'email_body' }">
        <template v-if="body || typingIn === 'email_body'">
          {{ body }}<span v-if="typingIn === 'email_body'" class="cursor" aria-hidden="true" />
        </template>
        <template v-else>E-posta metni üretilince burada görünür.</template>
      </div>
    </div>
  </figure>
</template>

<style scoped>
.mail {
  margin: 0 auto;
  width: 100%;
  max-width: 420px;
  border-radius: 12px;
  background: var(--device-surface);
  color: var(--device-fg);
  box-shadow: var(--shadow-device);
  overflow: hidden;
  border: 1px solid var(--device-line);
}
.chrome {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 14px;
  background: var(--device-chrome);
  border-bottom: 1px solid var(--device-line);
}
.d {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--device-dot);
}
.title {
  margin-left: 8px;
  font-size: 0.78rem;
  color: var(--device-muted);
  font-weight: 600;
}
.msg {
  padding: 18px 20px 20px;
}
.subject {
  margin: 0 0 12px;
  font-family: var(--font-num);
  font-size: 1.125rem;
  font-weight: 650;
  line-height: 1.3;
  overflow-wrap: anywhere;
}
.from {
  display: flex;
  gap: 10px;
  align-items: center;
  padding-bottom: 12px;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--device-line);
  font-size: var(--text-sm);
}
.avatar {
  display: grid;
  place-items: center;
  flex: none;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--device-avatar);
  color: var(--device-surface);
  font-weight: 650;
}
.addr,
.to {
  color: var(--device-muted);
}
.to {
  font-size: 0.8rem;
}
.body {
  font-size: var(--text-sm);
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 300px;
  overflow: auto;
}
.empty {
  color: var(--device-dot);
  font-weight: 400;
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
