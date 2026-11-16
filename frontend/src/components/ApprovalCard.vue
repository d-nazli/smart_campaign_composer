<script setup lang="ts">
import { computed } from 'vue'
import type { Issue, LanguageInfo, Segment } from '../types'
import { describeOffer } from '../utils/formatOffer'
import { FALLBACK_LANGUAGES, languageSummary } from '../utils/languages'
import AnimatedNumber from './AnimatedNumber.vue'

// The summary of what will be sent. The buttons live in the wizard's bottom bar;
// `confirming` is the second step of the approval: the card asks "are you sure?".
const props = defineProps<{
  segment: Segment
  warnings: Issue[]
  confirming: boolean
  /** Already approved: the card is read-only. */
  frozen: boolean
  error: string | null
  languages?: LanguageInfo[]
}>()

const sms = computed(() => props.segment.audience?.sms_reachable ?? 0)
const email = computed(() => props.segment.audience?.email_reachable ?? 0)
const people = computed(() => {
  const a = props.segment.audience
  return a ? a.sms_reachable + a.email_reachable - a.both_reachable : 0
})
const offer = computed(() => describeOffer(props.segment.offer))
const spoken = computed(() => languageSummary(props.segment.languages, props.languages ?? FALLBACK_LANGUAGES))
</script>

<template>
  <section class="card approval">
    <h2>Onay</h2>
    <p v-if="frozen" class="notice info" role="status">Kampanya onaylandı; bu özet artık değiştirilemez.</p>

    <div class="summary">
      <div class="people">
        <strong class="num big"><AnimatedNumber :value="people" from-zero /></strong>
        <span class="muted">kişiye gönderilecek</span>
      </div>
      <div class="channels">
        <div class="tile sms">
          <strong class="num"><AnimatedNumber :value="sms" from-zero /></strong>
          <span class="cap">SMS</span>
        </div>
        <div class="tile email">
          <strong class="num"><AnimatedNumber :value="email" from-zero /></strong>
          <span class="cap">e-posta</span>
        </div>
      </div>
    </div>

    <dl class="facts">
      <div>
        <dt>Teklif</dt>
        <dd>{{ offer }}</dd>
      </div>
      <div>
        <dt>Kitle</dt>
        <dd>{{ segment.explanation }}</dd>
      </div>
      <div v-if="spoken.length">
        <dt>Diller</dt>
        <dd data-testid="approval-languages">
          <span v-for="l in spoken" :key="l.code" class="lang">{{ l.display }} <strong class="num">{{ l.count }}</strong></span>
        </dd>
      </div>
    </dl>

    <ul v-if="warnings.length" class="warnings" aria-label="Uyarılar">
      <li v-for="w in warnings" :key="(w.language ?? '') + w.field + w.code + w.message" class="notice warn">
        ⚠ <strong v-if="w.language">{{ w.language.toUpperCase() }}:</strong> {{ w.message }}
      </li>
    </ul>

    <Transition name="fade">
      <div v-if="confirming" class="confirm" role="alertdialog" aria-labelledby="confirm-title">
        <p id="confirm-title">
          <strong>Emin misiniz?</strong>
          Mesajlar <strong>{{ sms }} SMS</strong> ve <strong>{{ email }} e-posta</strong> olarak gönderilecek. Onaydan sonra metin ve kitle
          değiştirilemez.
        </p>
      </div>
    </Transition>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.summary {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px 32px;
  margin-bottom: 18px;
}
.people {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.big {
  font-size: 3rem;
  line-height: 1;
  font-weight: 650;
  min-width: 3ch;
}
.channels {
  display: flex;
  gap: 10px;
}
.tile {
  min-width: 112px;
  padding: 10px 16px;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
}
.tile strong {
  font-size: 1.6rem;
  line-height: 1.15;
}
.tile .cap {
  font-size: 0.85rem;
}
.tile.sms {
  background: var(--sms-soft);
  color: var(--sms);
}
.tile.email {
  background: var(--email-soft);
  color: var(--email);
}
.facts {
  margin: 0 0 12px;
  display: grid;
  gap: 8px;
}
.facts div {
  display: grid;
  grid-template-columns: 5.5rem 1fr;
  gap: 12px;
}
dt {
  color: var(--muted);
}
dd {
  margin: 0;
}
.lang {
  display: inline-block;
  margin-right: 14px;
}
.warnings {
  list-style: none;
  margin: 0;
  padding: 0;
}
.warnings .notice {
  margin: 8px 0;
}
.confirm {
  margin-top: 14px;
  padding: 14px 16px;
  border-radius: 12px;
  background: var(--accent-soft);
  border: 1px solid var(--accent);
}
.confirm p {
  margin: 0;
}
@media (max-width: 560px) {
  .big {
    font-size: 2.3rem;
  }
  .facts div {
    grid-template-columns: 1fr;
    gap: 0;
  }
}
</style>
