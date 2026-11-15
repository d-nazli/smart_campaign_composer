# PLAN.md — Smart Campaign Composer

> Durum: **ONAYLANDI.** Kullanıcı kararları: Postgres kuyruğu (`queue.Queue` arayüzü arkasında), `docker compose watch`, varsayılan mock LLM (`LLM_MODE=auto`), **kanal seçimi yok (SMS+email her zaman birlikte)**, **Aşama 7 (bonus) yapılmayacak**, kapsam dışı özellik gerekirse önce kullanıcıya sorulacak.
> İlerleme: **Aşama 0 ve 1 tamamlandı** (Aşama 1 kullanıcı onayı bekleniyor), Aşama 2–6 bekliyor.
> Aşama 1 sapmaları: `language` kolonu yok (D29), mock provider'da `/admin/*` yok (D28), türev kolonlar seed'de Go'da hesaplanıp SQL ile doğrulanıyor (D26), Postgres healthcheck TCP (D32).
> Alınan kararların gerekçeleri için bkz. [DECISIONS.md](DECISIONS.md).

---

## 1. Gereksinimler (CASE.md'den, eksiksiz)

### 1.1 Ürün / akış
1. Pazarlamacı doğal dille brief yazar (örn. "Son 30 günde sepete ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.").
2. **Kitle:** Claude brief'ten bir *segment kuralı* üretir; arayüz kaç kişiye gideceğini gösterir.
3. **Mesajlar:** SMS ve email içeriği Claude tarafından üretilir ve ekrana **stream edilerek** akar.
4. **Onay:** Pazarlamacı önizler, gerekirse düzenler, onaylar (son karar insanda).
5. **Gönderim:** Mesajlar **kuyruk** üzerinden **mock provider**'a gönderilir.
6. **Canlı takip:** Dashboard gönderim durumunu canlı gösterir.
7. Gerçek kimseye mesaj gitmez; alıcılar sahte kullanıcılar, provider mock.

### 1.2 Fonksiyonel
| # | Alan | Gereksinim |
|---|---|---|
| F1 | Kitle | Segment kuralı backend'de **doğrulanır**; LLM çıktısı **asla doğrudan SQL olarak çalıştırılmaz** |
| F2 | Kitle | İzni olmayan kullanıcıya o kanaldan mesaj gitmez |
| F3 | İçerik | SMS ≤ 160 karakter |
| F4 | İçerik | Email konusu ≤ 80 karakter |
| F5 | İçerik | `{{first_name}}` gibi kişiselleştirme değişkenleri desteklenir |
| F6 | Gönderim | Asenkron kuyruk |
| F7 | Gönderim | Kanal başına saniyede **en fazla 100 mesaj** |
| F8 | Gönderim | Aynı kişiye aynı mesaj **iki kez gitmez** |
| F9 | Gönderim | Retry stratejisi benim tasarımım |
| F10 | Takip | Canlı gönderim durumu |
| F11 | Takip | `metrics` endpoint'i |
| F12 | Takip | `health check` endpoint'i |

### 1.3 AI
| # | Gereksinim |
|---|---|
| A1 | Claude çıktısı yapılandırılmış (JSON) olmalı ve backend'de kontrol edilmeli |
| A2 | Testler gerçek API key olmadan, **mock LLM** ile çalışmalı |
| A3 | Key `.env` içindeki `ANTHROPIC_API_KEY`'den okunur; `.env.example` repoda olur |
| A4 | Çıktıyı *güvenilir* kılmak değerlendirmenin en önemli kısmı (LLM entegrasyonu %25) |

### 1.4 Hazırlanacak kaynaklar
- **Seed:** birkaç bin sahte kullanıcı (ad, email, telefon, şehir, alışveriş geçmişi, kanal izinleri… alanlara ben karar veririm).
- **Mock SMS/email provider:** zaman zaman hata, gecikme ve **429** döndürmeli (retry ve rate limit gerçekten test edilebilsin).

### 1.5 Teknoloji
Frontend **Vue.js**, backend **Go** (seçildi), LLM **Claude API**. DB/kuyruk/kütüphane serbest.

### 1.6 Teslim
- [ ] GitHub/GitLab repo
- [ ] `docker compose up` ile tek komut kurulum (seed + mock provider dahil)
- [ ] `make test`
- [ ] `README.md` (kurulum + kısa mimari)
- [ ] `DECISIONS.md`
- [ ] `ai-sessions/` (Claude session export'ları; key/kişisel bilgi temizlenmiş)
- Yetiştirilemeyenler README'de belirtilecek.

### 1.7 Bonus (opsiyonel)
Zamanlanmış gönderim · A/B varyantları · kullanıcının diline göre içerik · CI pipeline

### 1.8 Değerlendirme ağırlıkları
LLM entegrasyonu %25 · Mimari/kararlar %20 · Backend güvenilirliği %20 · Frontend %20 · Test/kod kalitesi %15
→ Kapsam notu: *"Her şeyi yapmak yerine çekirdek akışı sağlam yap."* 30 dk review görüşmesi olacak.

---

## 2. Mimari

### 2.1 Servisler

| Servis | Teknoloji | Rol |
|---|---|---|
| `web` | Vue 3 + Vite + TS, nginx | SPA. `/api/*`'yi `api`'ye proxy'ler (CORS yok, SSE için buffering kapalı) |
| `api` | Go | Brief → segment → mesaj üretimi (Claude), doğrulama, onay, SSE, health/metrics |
| `worker` | Go (aynı modül, ayrı binary) | Kuyruktan mesaj çeker, rate limit + retry + idempotency ile provider'a gönderir |
| `mock-provider` | Go (aynı modül, ayrı binary) | SMS/email provider taklidi: gecikme, rastgele 5xx, 429 (`Retry-After`), kendi 100/s limiti, `Idempotency-Key` dedupe |
| `postgres` | PostgreSQL 16 | Kullanıcılar, kampanyalar, mesajlar (= kuyruk), LLM audit kaydı |
| `migrate` (tek seferlik) | Go `tool` binary | Migration + seed; bitince çıkar |

Tek Go modülü, tek Dockerfile; `api`, `worker`, `mock-provider`, `tool` ayrı `cmd/` binary'leri. Claude'a yalnız `api` konuşur.

### 2.2 İletişim

| Kimden → Kime | Protokol | Neden |
|---|---|---|
| Tarayıcı → web/api | HTTP/JSON | CRUD |
| api → Tarayıcı (mesaj üretimi) | **SSE** (POST + fetch stream) | Claude token'ları akar; tek yönlü |
| api → Tarayıcı (dashboard) | **SSE** (snapshot'lar) | Tek yönlü canlı durum; WebSocket gereksiz |
| api → Claude | HTTPS (Anthropic SDK, streaming) | Tool-use ile yapılandırılmış çıktı |
| api → Postgres | pgx | |
| api ⇒ worker | **Postgres kuyruğu** (`messages` tablosu) | Doğrudan çağrı yok; onayda mesajlar tek transaction'da yazılır |
| worker → mock-provider | HTTP/JSON + `Idempotency-Key` | Gerçek provider'ı taklit eder |
| worker → api (durum) | Yok; durum DB'de | API, DB'den agregasyon okuyup SSE'ye basar |

### 2.3 Uçtan uca akış

```
 Pazarlamacı (Vue SPA)
      │ 1. brief (POST /api/campaigns)
      ▼
 ┌──────────────────────────── api (Go) ────────────────────────────┐
 │ 2. LLM.GenerateSegment(brief)  ──►  Claude (tool_use, zorunlu)    │
 │       ◄── JSON kural                                              │
 │ 3. segment.Validate()  (şema + alan/operatör whitelist + tipler)  │
 │      ├─ geçersiz → hata geri bildirimiyle retry (max 2) → 422     │
 │      └─ geçerli → segment.Compile() → PARAMETRELİ SQL            │
 │ 4. COUNT: toplam / sms-izinli / email-izinli  ─────► Postgres     │
 │      ▼ UI: "1.240 kişi (SMS 910, Email 1.100)"                    │
 │ 5. POST .../messages/stream  ──► Claude (stream) ──► SSE ──► UI   │
 │      stream sonu: content.Validate() (160/80, değişkenler)        │
 │      ├─ geçersiz → retry + hata geri bildirimi → fallback         │
 │ 6. UI'da önizleme + düzenleme (sunucu yine doğrular)              │
 │ 7. POST .../approve  (idempotent: draft→approved CAS)             │
 │      TEK TX: segment'i snapshot'la → izinli alıcılar için         │
 │      messages INSERT … ON CONFLICT DO NOTHING (campaign,user,ch)  │
 └───────────────────────────────┬───────────────────────────────────┘
                                 ▼
                      Postgres  messages (= kuyruk)
                      status: pending→sending→sent|failed|skipped
                                 │  FOR UPDATE SKIP LOCKED
                                 ▼
 ┌──────────────────────── worker (Go) ──────────────────────────────┐
 │ kanal başına: limiter(100/s) ► claim(batch, lease) ► render(var)  │
 │   ► izin yeniden kontrol ► POST mock-provider (Idempotency-Key)   │
 │ 200 → sent │ 429 → Retry-After + kanal duraklat │ 5xx/timeout →   │
 │ backoff+jitter │ 4xx → failed │ max deneme → failed               │
 └───────────────────────────────┬───────────────────────────────────┘
                                 ▼
                         mock-provider (Go)
                  gecikme · %x 5xx · 429 · 100/s limit · dedupe
                                 
 api ── SSE /campaigns/:id/events (her ~1s DB agregasyon snapshot'ı) ──► Dashboard
 api ── /healthz /readyz /metrics        worker ── /healthz /metrics
```

### 2.4 Kampanya durum makinesi
`draft` (brief) → `segment_ready` → `content_ready` → `approved` → `sending` → `completed`
Her geçiş `UPDATE … WHERE status = <beklenen>` ile (CAS) yapılır; çift tıklama / yeniden deneme güvenlidir.

### 2.5 Veri modeli (özet)

- `users(id, first_name, last_name, email, phone, city, language, sms_consent, email_consent, last_cart_add_at, last_order_at, order_count, total_spent, created_at)`
- `cart_events(id, user_id, product, added_at)`, `orders(id, user_id, total, created_at)` — "alışveriş geçmişi"; `users` üzerindeki türev kolonlar bunlardan seed'de hesaplanır (segment sorgusu basit ve hızlı kalır)
- `campaigns(id, brief, segment_rule jsonb, status, channels[], created_at, approved_at, …)`
- `campaign_contents(campaign_id, channel, subject, body, version)` — pazarlamacının onayladığı şablon
- `messages(id, campaign_id, user_id, channel, status, attempts, next_attempt_at, locked_until, provider_msg_id, last_error, rendered_hash, sent_at, UNIQUE(campaign_id, user_id, channel))`
- `llm_generations(id, campaign_id, kind, model, attempt, raw_output, valid, validation_error, tokens_in, tokens_out, latency_ms)` — denetim izi (review'da "LLM ne dedi, biz ne yaptık" gösterir)

### 2.6 Segment DSL (LLM'in üreteceği şey, SQL değil)

`status` alanı `ok`, `needs_clarification` veya `unsupported` değerini alır.

```json
{
  "status": "ok",
  "rule": {
    "all": [
      {"field": "city", "op": "eq", "value": "İstanbul"},
      {"field": "days_since_last_cart_add", "op": "lte", "value": 30},
      {"field": "purchased_since_last_cart_add", "op": "eq", "value": false}
    ]
  },
  "explanation": "…"
}
```
- Alan **whitelist**'i (`city`, `language`, `order_count`, `total_spent`, `days_since_last_order`, `days_since_last_cart_add`, `purchased_since_last_cart_add`…). Her alanın tipi ve izinli operatörleri kodda tanımlı.
- Gruplar: `all`/`any`/`not`; derinlik ≤ 3, toplam koşul ≤ 10.
- Compiler: alan → **sabit SQL ifadesi** (kodda), değer → **bind parametresi** (`$1…`). Hiçbir LLM string'i SQL metnine girmez.
- İzin filtresi (`sms_consent`, telefon var mı…) **LLM kuralının parçası değil**; sistem tarafından her zaman eklenir.

---

## 3. Kritik konular — nasıl çözülecek

### 3.1 Claude çıktısının doğrulanması, SQL'e gitmemesi
1. **Zorunlu tool-use:** `tool_choice` ile belirli bir tool'u çağırmaya zorlanır; `input_schema` = JSON Schema. Serbest metin/markdown parse etmeyiz.
2. **Katmanlı doğrulama (backend):** JSON parse → strict struct decode (`DisallowUnknownFields`) → alan/op whitelist → tip/aralık kontrolü → derinlik/boyut limiti → şehir kanonikleştirme (İstanbul/Istanbul/istanbul → bilinen listeden).
3. **Compile edilmiş parametreli SQL:** LLM → veri (AST) → kodumuzdaki derleyici → SQL. Arada "string birleştirme" yok.
4. **Prompt injection:** brief kullanıcı girdisidir; LLM'e ne derse desin çıktısı yine whitelist'ten geçer. Sistem prompt'u + brief ayrı roller.
5. Testler: bilinmeyen alan, `1=1; DROP TABLE`, aşırı derinlik, yanlış tip, SQL içeren string değerler → hepsi reddedilir (table-driven).

### 3.2 Claude geçersiz cevap verirse
Segment ve içerik için aynı "repair loop":
```
attempt 1 → doğrula → ok ✓
          → hata → hata mesajını Claude'a geri ver ("sms 183 karakter, ≤160 olmalı") → attempt 2
          → hata → attempt 3 (son)
          → hata → fallback
```
- **API hataları** (429/5xx/overloaded/timeout): SDK seviyesinde backoff'lu retry (max 3), toplam süre bütçesi var.
- **Fallback — segment:** güvenli fallback yoktur (yanlış kitleye gönderim tehlikeli). `422 segment_generation_failed` + UI'da "brief'i netleştir" ve ham hata. Belirsiz brief için Claude `needs_clarification` + soru döner; UI gösterir.
- **Fallback — içerik:** (a) aşırı uzun metni **sessizce kesmeyiz**; (b) 3 denemeden sonra deterministik şablon ("{{first_name}}, …") ile *taslak* doldurulur ve UI'da "AI üretemedi, düzenleyin" uyarısı çıkar. Onayda sunucu yine doğrular.
- Her deneme `llm_generations` tablosuna yazılır (ham çıktı, hata, token).

### 3.3 Mock LLM ile key'siz test
- `llm.Client` arayüzü: `GenerateSegment(ctx, brief) (SegmentResponse, error)` ve `StreamMessages(ctx, req) (<-chan Chunk, error)`.
- Uygulamalar: `anthropic` (gerçek) ve `mock`.
- Mock **senaryo tabanlı**: "1. çağrıda bozuk JSON, 2.'de geçerli", "160 aşan SMS", "bilinmeyen alan", "429", "stream ortasında kopma" → retry/fallback yolları deterministik test edilir.
- `LLM_MODE=auto`: `ANTHROPIC_API_KEY` boşsa `mock`. Böylece `docker compose up` key'siz de demo çalışır (mock, örnek brief'e makul cevap verir).
- `anthropic` adapter'ı ayrıca `httptest` sahte Anthropic sunucusuna karşı test edilir (SSE parse). Gerçek API testi yalnız `ANTHROPIC_API_KEY` varsa çalışır (`make test`'e dahil değil).

### 3.4 Idempotency (aynı kişiye aynı mesaj iki kez gitmesin)
Üç katman:
1. **DB:** `UNIQUE(campaign_id, user_id, channel)` + `INSERT … ON CONFLICT DO NOTHING` → onay endpoint'i iki kez çağrılsa/çökse bile çift satır oluşmaz. Onay CAS ile tek kez geçer.
2. **Gönderim:** worker provider'a `Idempotency-Key: <message_id>` yollar; mock provider aynı anahtarı görürse yeniden göndermez, önceki sonucu döner. Timeout/çökme sonrası retry bu yüzden güvenlidir.
3. **Durum geçişi:** `sent` olan satır asla yeniden claim edilmez; lease süresi dolan `sending` satırı yeniden alınır (at-least-once + idempotent provider = etkin olarak tek sefer).

### 3.5 Retry stratejisi, 429 ve hata yönetimi
| Sonuç | Davranış |
|---|---|
| 2xx | `sent` |
| 429 | `Retry-After` (yoksa backoff) kadar bekle; **kanal limiter'ı o süre duraklatılır**; deneme bütçesini *tüketmez* (ama toplam yaş sınırı var: örn. 10 dk) |
| 5xx / timeout / bağlantı hatası | exponential backoff + full jitter (2s·2ⁿ, tavan 60s), max 5 deneme |
| 4xx (400/422: geçersiz numara vb.) | kalıcı hata → `failed`, retry yok |
| Max deneme / yaş aşıldı | `failed` (+ `last_error`), dashboard'da görünür |
| Worker çöker | `locked_until` lease dolunca satır tekrar claim edilir |
| Gönderim anında izin kalktı / alan boş | `skipped` (neden kaydedilir) |

Retry, `next_attempt_at` kolonuyla yapılır (ayrı delay kuyruğu gerekmez).

### 3.6 Kanal başına 100 mesaj/sn
- Worker'da **kanal başına tek limiter** (`golang.org/x/time/rate`, `rate=100`, `burst=1`→ eşit aralıklı 10 ms; burst>1 pencere kenarında 100'ü aşabilir, o yüzden 1–2).
- Limiter *provider çağrısından önce* beklenir; her kanalın N goroutine'i limiter'ı paylaşır (provider gecikmesi throughput'u düşürmez).
- Claim batch'i küçük (≤20) ve lease (30 sn) bekleme süresini kapsar.
- Mock provider kendi tarafında da 100/s sayar → aşan 429 döner; bu hem limiter'ın doğruluk testi hem de 429 kaynağıdır.
- Limiter birim testi sahte saatle; entegrasyonda "5.000 mesaj ≥ 50 sn sürer, hiçbir 1 sn penceresinde >100 yok".
- Sınır: limiter süreç içi → worker tek replica. Ölçekleme için Redis/PG tabanlı dağıtık limiter gerekir (DECISIONS'ta belirtilir).

### 3.7 Kanal izinleri ve karakter limitleri
- **İzin:** alıcı seti = `segment AND sms_consent AND phone IS NOT NULL` (SMS), `… email_consent AND email IS NOT NULL` (email). Sayım, onay ve worker'da (gönderim anında yeniden) aynı fonksiyon/koşul kullanılır.
- **Limit:** yalnız prompt'a güvenmeyiz. `content.Validate`: karakter = **rune** sayısı (Türkçe `ş,ğ,İ` doğru sayılsın); SMS ≤160, email konusu ≤80; değişkenler yalnız whitelist'ten (`first_name`, `last_name`, `city`); render sonrası uzunluk **en kötü durum** için hesaplanır (segmentteki en uzun `first_name`).
- Doğrulama 3 yerde: LLM çıktısı, kullanıcı düzenlemesi (canlı + sunucu), onay.
- Email gövdesi düz metin; HTML'i biz şablonla üretir ve escape ederiz (LLM'den HTML almayız → XSS yok). UI'da `v-html` kullanılmaz.

### 3.8 SSE
- **Mesaj üretimi:** `POST /api/campaigns/:id/messages/stream` — `text/event-stream`; event'ler: `delta` (kanal+parça), `validated` (final JSON + hata listesi), `retry`, `done`, `error`. POST gerektiği için tarayıcıda `fetch` + `ReadableStream` (`@microsoft/fetch-event-source`), `EventSource` değil.
- Claude'un `input_json_delta` akışı UI'a *taslak* olarak akar; **geçerli sayılmaz** — `validated` event'i gelmeden "Onayla" kapalı.
- **Dashboard:** `GET /api/campaigns/:id/events` (EventSource). Sunucu her ~1 sn DB'den agregasyon (`status`×`channel` sayıları + son hatalar) okuyup snapshot yollar; kampanya bitince `done`. Snapshot modeli: bağlantı kopsa da tek mesajla durum senkron olur, `Last-Event-ID` yönetimi gerekmez.
- nginx: `proxy_buffering off`, `proxy_read_timeout` uzun; sunucu 15 sn'de bir `:keepalive` yorumu yollar.

---

## 4. Veritabanı ve kuyruk seçenekleri

| Kriter | **Postgres kuyruğu** (`SKIP LOCKED`) | **Redis** (list/stream, asynq vb.) | **RabbitMQ** |
|---|---|---|---|
| Ek altyapı | Yok (zaten DB var) | +1 servis | +1 servis (ağır) |
| Onay + enqueue atomikliği | **Tek transaction** | İki sistem → dual-write riski (outbox gerekir) | Aynı (outbox gerekir) |
| Idempotency | `UNIQUE` kısıtı aynı yerde | Ayrı mekanizma | Ayrı mekanizma |
| Dashboard durumu | Tabloyu sorgula | Durum için yine DB lazım | Durum için yine DB lazım |
| Retry/delay | `next_attempt_at` | Sorted set / asynq | DLX + TTL (hantal) |
| Rate limit | Süreç içi limiter | Redis ile dağıtık limiter kolay | Yok (uygulama yapar) |
| Throughput | ~yüzlerce–binlerce/sn (bizim ihtiyaç 200/sn) | Çok yüksek | Yüksek |
| Dayanıklılık | DB garantileri | AOF/RDB ayarına bağlı | Güçlü (durable queue) |
| Test kolaylığı | Gerçek PG ile kolay | Ek container | Ek container |
| Dezavantaj | Polling gecikmesi, tablo şişmesi, ölçek tavanı | Kalıcılık/tutarlılık dikkat ister | Bu kapsam için aşırı |

**Öneri:** Postgres veritabanı + Postgres tabanlı kuyruk. Gerekçe: çekirdek gereksinimler (atomik onay, idempotency, canlı durum) zaten ilişkisel veri; ek broker bu garantileri *zayıflatır* ve karmaşıklık ekler. Kuyruk, `queue.Queue` arayüzünün arkasında; ölçek gerekirse RabbitMQ/Redis'e geçiş yolu açık. **Risk:** değerlendirici "gerçek broker" bekleyebilir → savunma: gereksinim "asenkron kuyruk" diyor, teknoloji serbest; DB-kuyruğu burada doğru mühendislik tercihi. **Nihai karar sende.**

Alternatif DB'ler: SQLite (eşzamanlı yazma/SKIP LOCKED yok), MySQL (olur ama JSONB/SKIP LOCKED desteği PG kadar rahat değil) → Postgres.

---

## 5. Klasör yapısı

```
insider_one_case/
├─ CASE.md  PLAN.md  DECISIONS.md  README.md
├─ Makefile                    # up, down, dev, test, test-backend, test-frontend, lint, seed
├─ docker-compose.yml          # üretim benzeri tam yığın
├─ docker-compose.dev.yml      # dev override (air, vite, compose watch)
├─ .env.example  .gitignore  .gitattributes   # *.sh eol=lf
├─ ai-sessions/                # /export çıktıları (temizlenmiş)
├─ backend/
│  ├─ go.mod
│  ├─ Dockerfile               # hedefler: dev | build | api | worker | mockprovider | tool
│  ├─ .air.toml
│  ├─ cmd/
│  │  ├─ api/main.go
│  │  ├─ worker/main.go
│  │  ├─ mockprovider/main.go
│  │  └─ tool/main.go          # migrate | seed
│  ├─ internal/
│  │  ├─ config/               # env okuma
│  │  ├─ httpapi/              # router, handler'lar, SSE yardımcıları, middleware
│  │  ├─ campaign/             # servis, durum makinesi, repo
│  │  ├─ segment/              # dsl.go, fields.go (whitelist), validate.go, compile.go
│  │  ├─ content/              # schema, validate (limit/değişken), render
│  │  ├─ llm/                  # client.go (arayüz), repair.go (retry döngüsü), prompts/
│  │  │  ├─ anthropic/         # gerçek adapter
│  │  │  └─ mock/              # senaryo tabanlı mock
│  │  ├─ queue/                # postgres kuyruğu: enqueue, claim, ack, nack
│  │  ├─ sender/               # worker döngüsü, limiter, retry politikası, provider client
│  │  ├─ provider/             # mock provider mantığı (chaos, dedupe, limit)
│  │  ├─ events/               # dashboard snapshot üretici
│  │  ├─ store/                # pgx pool, migrations/*.sql (embed)
│  │  ├─ seed/                 # deterministik sahte veri üretici
│  │  └─ metrics/
│  └─ testdata/                # LLM senaryoları, golden dosyalar
└─ frontend/
   ├─ Dockerfile  nginx.conf  package.json  vite.config.ts  tsconfig.json
   └─ src/
      ├─ api/                  # fetch istemcisi, SSE yardımcıları
      ├─ composables/          # useGenerationStream, useCampaignEvents
      ├─ views/                # ComposeView (adım adım), CampaignListView, CampaignDashboardView
      ├─ components/           # SegmentCard, ChannelEditor, CharCounter, StatusBars…
      └─ types/
```

---

## 6. Aşamalar (çekirdek önce, bonus sona)

> Her aşama dikey dilimdir: backend + ilgili UI birlikte biter. Her aşama sonunda çalışır durumda olmalı.

### Aşama 0 — İskelet ve Docker
- Repo, `.env.example`, Makefile, Dockerfile'lar, `docker-compose.yml` (postgres, api, worker, mock-provider, web, migrate), `/healthz` uçları, boş Vue sayfası.
- **Çalışması gereken:** `docker compose up --build` hatasız kalkar; tüm servisler `healthy`.
- **Test:** `curl localhost:8081/healthz` → 200; `localhost:8080` açılır; `docker compose ps` hepsi healthy.

### Aşama 1 — Veri, seed, mock provider
- Migration'lar, deterministik seed (~5.000 kullanıcı; edge-case'ler dahil), mock provider (gecikme, %5xx, 429, 100/s, dedupe, ayarlanabilir env).
- **Çalışması gereken:** `up` sonrası DB dolu, tekrar `up` veriyi çoğaltmaz; provider chaos davranışı gözlenir.
- **Test:** `SELECT count(*) FROM users` ≈ 5000; `up`'ı tekrarla → aynı sayı; provider'a 200 istek döngüsüyle 429/5xx/dedupe görülür; provider unit testleri.

### Aşama 2 — Segment (LLM + doğrulama + sayım) ⭐
- DSL, whitelist, validator, compiler, `llm.Client` + mock, repair loop, `POST /campaigns` + count. UI: brief kutusu + segment kartı (kural özeti, toplam/SMS/email sayıları, needs_clarification).
- **Çalışması gereken:** Örnek brief → İstanbul/30 gün/satın almamış segmenti ve sayısı; kötü LLM cevapları reddedilir.
- **Test:** `make test-backend` (table-driven: injection, bilinmeyen alan, derinlik, tip; mock LLM "bozuk→geçerli" retry; "hep bozuk" → 422); UI'dan brief gir → sayı gelir; izinsiz kullanıcılar sayıda yok (seed'deki bilinen sayıyla karşılaştır).

### Aşama 3 — Mesaj üretimi + stream ⭐
- Content şeması/validator (160/80, değişken), Claude stream adapter, mock stream, SSE endpoint, repair loop + fallback şablon. UI: iki kanal editörü, canlı akış, karakter sayacı, düzenleme.
- **Çalışması gereken:** Mesajlar ekrana akar; limit aşımı UI'da ve sunucuda yakalanır; onay butonu doğrulama geçmeden kapalı.
- **Test:** content unit testleri (Türkçe karakter, çok uzun isim, bilinmeyen `{{x}}`); mock senaryoları (uzun SMS → retry → düzelir; hep uzun → fallback taslak); tarayıcıda akışı izle; SSE'yi `curl -N` ile doğrula.

### Aşama 4 — Onay, kuyruk, worker, gönderim ⭐
- Approve (CAS + tek TX enqueue), Postgres kuyruğu, worker, limiter, retry politikası, idempotency, izin yeniden kontrolü.
- **Çalışması gereken:** Onaylanan kampanya gönderilir; provider 429/5xx verse de mesajlar sonunda `sent`/`failed`; çift onay çift mesaj üretmez; 100/s aşılmaz.
- **Test:** entegrasyon testleri (gerçek PG + httptest provider): aynı approve 2× → satır sayısı değişmez; worker kill → mesaj yeniden alınır, provider'da tek kayıt; 429 → Retry-After'a uyulur; limiter sahte saatle; `make test`.

### Aşama 5 — Dashboard, metrics, health ⭐
- Snapshot SSE, dashboard UI (kanal bazlı progress, sent/failed/retrying, son hatalar, throughput), `/metrics` (Prometheus), `/readyz` (DB), worker metrics.
- **Çalışması gereken:** Gönderim sırasında sayaçlar canlı akar; tarayıcı yenilenince devam eder.
- **Test:** `curl -N .../events`; `curl localhost:8081/metrics` (kuyruk derinliği, sent/failed sayaçları); DB'yi durdur → `/readyz` 503.

### Aşama 6 — Cilalama ve teslim
- Frontend testleri (vitest: composable + kritik bileşenler), hata/boş durum UI'ları, README (kurulum, mimari, yapılmayanlar), DECISIONS son hali, `make test` Windows notları, ai-sessions export + temizlik, repo'yu GitHub'a push.
- **Test:** temiz klonla `cp .env.example .env && docker compose up` ve `make test` sıfırdan geçer.

### Aşama 7 — Bonus: **YAPILMAYACAK** (kullanıcı kararı)
Zamanlanmış gönderim, A/B, dile göre içerik ve CI kapsam dışı; README'de "yapılmadı" olarak belirtilecek.

### Zaman daralırsa kesilecekler (ilk kesilen en üstte)
1. (Bonuslar zaten kapsam dışı)
2. Frontend testlerinin çoğu (yalnız `useGenerationStream` ve `CharCounter` kalsın)
3. Throughput grafiği / "son hatalar" listesi (sadece sayaçlar kalsın)
4. `needs_clarification` UI akışı (backend'de kalsın, UI genel hata göstersin)
5. Fallback şablonu (hata mesajı + manuel yazım)
6. Worker `/metrics` (yalnız api `/metrics`)

**Kesilemez:** segment doğrulama + parametreli SQL, mock LLM + repair loop, idempotency, rate limit, retry/429, izin kontrolü, `docker compose up`, `make test`, DECISIONS.md.

---

## 7. Docker

### 7.1 `docker-compose.yml` — servis rolleri ve bağlantılar

```yaml
# kavramsal taslak (kod aşamasında yazılacak)
name: smart-campaign
networks: { app: {} }          # tek bridge ağı; servisler DNS adıyla erişir

services:
  postgres:                    # veri + kuyruk
    image: postgres:16-alpine
    networks: [app]            # host'a PORT YAYINLANMAZ (dev override'da 15432)
    volumes: [pgdata:/var/lib/postgresql/data]
    healthcheck: { test: ["CMD-SHELL","pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}"], interval: 3s, retries: 20 }

  migrate:                     # tek seferlik: migration + seed
    build: { context: ./backend, target: tool }
    command: ["migrate", "seed"]
    depends_on: { postgres: { condition: service_healthy } }
    restart: "no"

  mock-provider:               # bağımsız, DB'ye ihtiyacı yok
    build: { context: ./backend, target: mockprovider }
    ports: ["8090:8090"]       # elle denemek için
    healthcheck: { test: ["CMD","wget","-qO-","http://localhost:8090/healthz"] }

  api:
    build: { context: ./backend, target: api }
    env_file: .env
    environment: { DATABASE_URL: postgres://…@postgres:5432/…, LLM_MODE: auto }
    ports: ["8081:8080"]
    depends_on:
      migrate: { condition: service_completed_successfully }
    healthcheck: { test: ["CMD","wget","-qO-","http://localhost:8080/readyz"] }

  worker:
    build: { context: ./backend, target: worker }
    environment: { DATABASE_URL: …, PROVIDER_URL: http://mock-provider:8090 }
    depends_on:
      migrate:       { condition: service_completed_successfully }
      mock-provider: { condition: service_healthy }
    healthcheck: { test: ["CMD","wget","-qO-","http://localhost:9091/healthz"] }

  web:                         # nginx: statik SPA + /api proxy → api:8080
    build: { context: ./frontend }
    ports: ["8080:80"]
    depends_on: { api: { condition: service_healthy } }

volumes: { pgdata: {} }
```

- **Port özeti:** `8080` web (asıl giriş) · `8081` api (debug) · `8090` mock-provider (debug) · postgres yayınlanmaz.
- **Bağlantılar:** tarayıcı → `web:80` → (`/api/*`) → `api:8080` → `postgres:5432`; `worker` → `postgres`, `mock-provider:8090`. Her şey tek `app` ağı; servisler DNS adıyla (`postgres`, `mock-provider`).
- **depends_on + healthcheck:** `postgres` healthy → `migrate` çalışır → `completed_successfully` → `api`/`worker` başlar. Böylece API ayağa kalktığında şema + seed hazırdır; yarış yok. Runtime imajı `alpine` (wget için); bu yüzden distroless değil.
- **Secret:** `ANTHROPIC_API_KEY` yalnız `api`'ye `.env` ile verilir; boşsa mock LLM.

### 7.2 Seed otomatik yüklenmesi
- `migrate` one-shot servisi: önce migration (goose, embed edilmiş SQL), sonra seed.
- Seed **idempotent**: `pg_advisory_lock` altında `users` boşsa yükler; doluysa atlar → `docker compose up` tekrar tekrar güvenli.
- **Deterministik** (`SEED=42` ile sabit RNG): test ve demo her seferinde aynı veri/aynı sayılar.
- Veri: ~5.000 kullanıcı, İstanbul ≈ %35, sepet/sipariş geçmişi, izin kombinasyonları (hiç izni olmayan ~%8, yalnız SMS/yalnız email/ikisi), telefonu/emaili boş birkaç kayıt, çok uzun isimli kullanıcılar (uzunluk testi), bilinen sayıda "örnek brief'e uyan" kullanıcı (test beklentisi için).
- Neden `docker-entrypoint-initdb.d` değil: yalnız ilk volume'da çalışır, SQL ile veri üretmek zor, testte yeniden kullanılamaz, deterministik/idempotent kontrol yok.

### 7.3 Geliştirirken her seferinde rebuild etmemek
1. **Önerilen — `docker-compose.dev.yml` + `docker compose watch`:** `api`/`worker`/`mock-provider` `dev` hedefinde `air` ile çalışır; `develop.watch` host'taki değişiklikleri container'a **sync** eder, `air` container içinde yeniden derler (Windows bind-mount inotify sorunu yok). Web için Vite dev server (HMR) + sync. Yalnız `go.mod`/`package.json` değişince `rebuild`. Go modül/build cache'i named volume'da.
2. **Alternatif (en hızlı):** yalnız `postgres` + `mock-provider` Docker'da (`make dev-infra`), Go ve Vite host'ta (`go run` / `npm run dev`). Host'ta Go ve Node kurulu olmalı.
3. Tam yığın (`docker compose up --build`) yalnız aşama sonlarında ve teslimden önce doğrulama için.
4. `.dockerignore`, katmanlı Dockerfile (önce `go.mod`/`go.sum` + `go mod download`, sonra kaynak) ile prod build'ler de cache'ten hızlı.

### 7.4 Windows notları
- `make` Windows'ta varsayılan yok: `choco install make` veya Git Bash/WSL. **`make test` Docker içinde çalışır** (`docker compose run --rm …`), host'ta Go/Node gerekmez.
- Repo'yu WSL2 dosya sisteminde tutmak bind-mount performansı için daha iyi; değilse `compose watch` (sync) zaten bunu aşar.
- `.gitattributes` ile `*.sh`, `Dockerfile` `eol=lf` (CRLF yüzünden `exec format error`/`bad interpreter` yaşanmasın).

---

## 8. Test stratejisi (`make test`)

| Katman | İçerik | Gereken |
|---|---|---|
| Unit (Go) | segment validator/compiler (injection dahil), content validator, retry politikası, limiter (sahte saat), repair loop (mock LLM) | Hiçbir şey |
| Entegrasyon (Go) | Gerçek PG (compose test servisi): kuyruk claim eşzamanlılığı, idempotent enqueue, uçtan uca approve→worker→`httptest` provider (chaos açık) | `postgres` container |
| Adapter | Anthropic adapter ↔ sahte SSE sunucusu | Hiçbir şey |
| Frontend | vitest: stream composable, CharCounter, segment kartı | Node (container içinde) |
| (Opsiyonel) | Gerçek Claude duman testi | `ANTHROPIC_API_KEY`; `make test-live`, `make test`'e dahil değil |

`make test` = backend unit + entegrasyon + frontend; hepsi Docker içinde, API key gerektirmez.

---

## 9. Belirsizlikler ve önerilen kararlar

| # | Belirsizlik | Önerim | Not |
|---|---|---|---|
| 1 | "Aynı mesaj iki kez gitmez" kapsamı | `(kampanya, kullanıcı, kanal)` benzersiz + provider idempotency key. Kampanyalar arası dedupe **yok** | İstenirse `(user, channel, content_hash)` eklenir |
| 2 | "Alışveriş yapmayan" tanımı | Son sepete eklemeden *sonra* sipariş yok (`purchased_since_last_cart_add = false`) | Alternatif: son 30 günde hiç sipariş yok |
| 3 | Şehir yazımı (İstanbul/Istanbul) | Bilinen şehir listesine kanonikleştir; bilinmiyorsa `needs_clarification` | |
| 4 | SMS 160 = ? | 160 **rune**, render sonrası en kötü durumda. Gerçekte GSM-7/UCS-2 segmentleri farklı; README'de not | Türkçe karakterli gerçek SMS 70 karakterdir |
| 5 | Limit değişkenli metne nasıl uygulanır | Şablonu ve render edilmiş en kötü durumu doğrula | |
| 6 | Hangi değişkenler | `first_name`, `last_name`, `city`; boşsa varsayılan ("değerli müşterimiz") | |
| 7 | Her iki kanal zorunlu mu | **Kullanıcı kararı:** kanal seçimi yok, SMS ve email her zaman birlikte üretilir | Onaylandı |
| 8 | 100/s kapsamı | Kanal başına, tüm kampanyalar toplamı, worker tek replica | Dağıtık limiter bilinçli kapsam dışı |
| 9 | Kullanıcı segmenti düzenleyebilir mi | Hayır; kural salt-okunur gösterilir, brief yeniden yazılır | Kesilebilecek özellik zaten çıkarıldı |
| 10 | Teslim durumu | `sent` (provider kabul etti) yeterli; "delivered" webhook'u yok | Bonus/kapsam dışı |
| 11 | Gönderimden sonra snapshot | Onayda alıcılar `messages`'a yazılır (snapshot); sonradan değişen veri sayıyı etkilemez, yalnız izin gönderim anında yeniden kontrol edilir | |
| 12 | İçerik dili | Brief'in dili (Türkçe); dil-bazlı içerik bonus | |
| 13 | Model | `CLAUDE_MODEL` env, varsayılan `claude-sonnet-5-5` | Maliyet için segment çağrısı daha küçük modele alınabilir |
| 14 | Auth / çok kiracı | Yok; README'de belirt | |
| 15 | Ölçek | Case "milyonlar" der; biz binlerle çalışırız, tasarım ölçeklenebilir ama kanıtlanmış değil | README'de dürüst not |
| 16 | `make` Windows'ta | Makefile + aynı komutların PowerShell karşılığı README'de | |
| 17 | Repo | Şu an git repo değil: `git init` + GitHub repo adı sana ait | Aşama 0'da |
| 18 | ai-sessions | Her aşama sonunda `/export`; commit öncesi key/kişisel veri taraması | |

---

## 10. Onaylanan kararlar
1. Kuyruk: Postgres (`SKIP LOCKED`), `queue.Queue` arayüzü arkasında.
2. Dev: `docker compose watch` (`docker-compose.dev.yml`); tüm komutlar Docker içinde, host'ta Go/Node gerekmez.
3. PLAN §9 varsayımları geçerli; #7 değişti (kanal seçimi yok). Değişkenler: `first_name`, `last_name`, `city`.
4. LLM: varsayılan mock, `LLM_MODE=auto`; key doluysa Claude; `CLAUDE_MODEL` varsayılan `claude-sonnet-5-5`; `make test` daima mock; `make test-live` ayrı; mod log ve UI'da görünür.
5. Kapsam: bonuslar yapılmayacak; kapsam dışı özellik için önce sorulacak.
