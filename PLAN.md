# PLAN.md — Smart Campaign Composer

> Durum: **ONAYLANDI.** Kullanıcı kararları: Postgres kuyruğu (`queue.Queue` arayüzü arkasında), `docker compose watch`, varsayılan mock LLM (`LLM_MODE=auto`), **kanal seçimi yok (SMS+email her zaman birlikte)**, **Aşama 7 (bonus) yapılmayacak**, kapsam dışı özellik gerekirse önce kullanıcıya sorulacak.
> İlerleme: **Aşama 0, 1, birleşik Aşama 2+3 (segment + mesaj üretimi) Aşama 4 (onay + kuyruk + gönderim) ve Aşama 5 (dashboard, `/metrics`, `/healthz`, `-race`) tamamlandı.** Aşama 6 (cilalama; arayüzün baştan tasarımı dahil) bekliyor.
> Aşama 1 sapmaları: `language` kolonu yok (D29), mock provider'da `/admin/*` yok (D28), türev kolonlar seed'de Go'da hesaplanıp SQL ile doğrulanıyor (D26), Postgres healthcheck TCP (D32).
> Aşama 2+3 sapmaları (nedenleri DECISIONS.md D33–D43): Sonnet 5.5 zorunlu `tool_choice`'u reddettiği için **structured outputs**; DSL **düz** (iç içe `all/any/not` yok); değişkenler yalnız `first_name` ve `city` ve **sabit karakter payıyla** sayılır; kuyruk tablosunun adı `deliveries`; Go 1.25.
> Aşama 2+3 düzeltmesi (D44–D49): brief'teki hiçbir koşul sessizce atlanmaz (`internal/brief`: unsupported/needs_clarification kodla belirlenir, model kuralı brief'e karşı doğrulanır); segment adımı **teklifi** (`offer`) çıkarır, mesaj doğrulaması teklifi ve kelime tekrarını denetler.
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
| api ⇒ worker | **Postgres kuyruğu** (`deliveries` tablosu) | Doğrudan çağrı yok; onayda gönderimler tek transaction'da yazılır |
| worker → mock-provider | HTTP/JSON + `Idempotency-Key` | Gerçek provider'ı taklit eder |
| worker → api (durum) | Yok; durum DB'de | API, DB'den agregasyon okuyup SSE'ye basar |

### 2.3 Uçtan uca akış

```
 Pazarlamacı (Vue SPA)
      │ 1. brief (POST /api/campaigns)
      ▼
 ┌──────────────────────────── api (Go) ────────────────────────────┐
 │ 2. LLM.Segment(brief)  ──►  Claude (structured outputs, JSON)     │
 │       ◄── JSON kural                                              │
 │ 3. segment.Parse()  (katı decode + alan/operatör whitelist + tip) │
 │      ├─ geçersiz → hata geri bildirimiyle retry (max 3) → 422     │
 │      └─ geçerli → segment.Compile() → PARAMETRELİ SQL            │
 │ 4. COUNT: toplam / sms-izinli / email-izinli  ─────► Postgres     │
 │      ▼ UI: "1.240 kişi (SMS 910, Email 1.100)"                    │
 │ 5. POST .../messages/stream  ──► Claude (stream) ──► SSE ──► UI   │
 │      stream sonu: content.Validate() (160/80, değişkenler)        │
 │      ├─ geçersiz → retry + hata geri bildirimi → fallback         │
 │ 6. UI'da önizleme + düzenleme (sunucu yine doğrular)              │
 │ 7. POST .../approve  (idempotent: draft→approved CAS)             │
 │      TEK TX: segment'i snapshot'la → izinli alıcılar için         │
 │      deliveries INSERT … ON CONFLICT DO NOTHING (campaign,user,ch)│
 └───────────────────────────────┬───────────────────────────────────┘
                                 ▼
                      Postgres  deliveries (= kuyruk)
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

Gerçek şema `backend/internal/store/migrations/` altındadır.

- `users(id, first_name, last_name, email, phone, city, sms_consent, email_consent, last_cart_add_at, last_order_at, order_count, total_spent, created_at)` — `language` yok (D29). `last_name` kolonu duruyor ama **değişken değil** (D35).
- `cart_events(id, user_id, product_name, added_at)`, `orders(id, user_id, total, created_at)` — "alışveriş geçmişi"; `users` üzerindeki türev kolonlar bunlardan seed'de hesaplanır (segment sorgusu basit ve hızlı kalır)
- `campaigns(id, brief, status, segment_status, segment_rule jsonb, explanation, question, matched, sms_reachable, email_reachable, both_reachable, created_at, updated_at)` — kanal seçimi yok, SMS ve email her zaman birlikte
- `campaign_contents(campaign_id PK, sms_body, email_subject, email_body, source llm|fallback|edited, updated_at)` — pazarlamacının onayladığı şablon
- `deliveries(id, campaign_id, user_id, channel, status, attempts, next_attempt_at, locked_until, provider_message_id, last_error, sent_at, …, CONSTRAINT UNIQUE(campaign_id, user_id, channel))` — **kuyruğun kendisi**; tablo Aşama 2+3 migration'ında yaratıldı (şema birlikte gözden geçirilsin diye), kullanımı Aşama 4'te. Önceki planlardaki `messages` adı bu tabloyla değişti.
- `llm_generations(id, campaign_id, kind, attempt, mode, model, valid, problems jsonb, raw_output, input_tokens, output_tokens, latency_ms, error)` — denetim izi (review'da "LLM ne dedi, biz ne yaptık" gösterir)

### 2.6 Segment DSL (LLM'in üreteceği şey, SQL değil)

`status` alanı `ok`, `needs_clarification` veya `unsupported` değerini alır. **DSL düzdür** (iç içe grup yok): Claude'un structured outputs şeması özyinelemeli yapıyı desteklemez; `not` ise `neq`/`not_in` operatörleriyle karşılanır (D34).

```json
{
  "status": "ok",
  "match": "all",
  "conditions": [
    {"field": "city", "op": "eq", "value": "İstanbul"},
    {"field": "days_since_last_cart_add", "op": "lte", "value": 30},
    {"field": "purchased_since_last_cart_add", "op": "eq", "value": false}
  ],
  "explanation": "…",
  "question": ""
}
```
- Alan **whitelist**'i tek yerde: `backend/internal/segment/fields.go` (`city`, `days_since_last_cart_add`, `days_since_last_order`, `days_since_last_order_or_never` (D50: "son N günde sipariş vermemiş", hiç siparişi olmayanlar dahil), `order_count`, `total_spent`, `purchased_since_last_cart_add`). Her alanın tipi, aralığı ve izinli operatörleri kodda tanımlı; JSON şeması ve LLM prompt'u bu listeden türetilir.
- `match`: `all` / `any`. 1–8 koşul; boş kural reddedilir (herkesi seçerdi). Şehirler bilinen listeye kanonikleştirilir (İstanbul/Istanbul/istanbul).
- Compiler: alan → **sabit SQL ifadesi** (kodda), değer → **bind parametresi** (`$1…`). Hiçbir LLM string'i SQL metnine girmez.
- İzin filtresi (`sms_consent`, telefon var mı…) **LLM kuralının parçası değil**; sistem tarafından her zaman eklenir (`segment.SMSReachableSQL` / `EmailReachableSQL`; Aşama 4'te enqueue aynı sabitleri kullanır).

---

## 3. Kritik konular — nasıl çözülecek

### 3.1 Claude çıktısının doğrulanması, SQL'e gitmemesi
1. **Structured outputs:** `output_config.format` ile JSON Schema verilir; serbest metin/markdown parse etmeyiz. (İlk planda "zorunlu tool-use" yazıyordu; `claude-sonnet-5-5` `tool_choice: any/tool` isteğini 400 ile reddettiği için değişti, D33.) Şema aralık/uzunluk ifade edemez; bunlar yalnız backend'de doğrulanır.
2. **Katmanlı doğrulama (backend):** JSON parse → strict struct decode (`DisallowUnknownFields`) → alan/op whitelist → tip/aralık kontrolü → koşul sayısı/boyut limiti → şehir kanonikleştirme (İstanbul/Istanbul/istanbul → bilinen listeden).
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
- `llm.Client` arayüzü **tek bir denemeyi** yapar ve ham metin döner: `Segment(ctx, req) (Output, error)` ve `Messages(ctx, req, onDelta) (Output, error)`. Doğrulama ve retry arayüzün dışında (`llm.Repair`), böylece gerçek ve mock istemci için aynıdır.
- Uygulamalar: `anthropic` (gerçek) ve `mock`.
- Mock **senaryo tabanlı**: "1. çağrıda bozuk JSON, 2.'de geçerli", "160 aşan SMS", "bilinmeyen alan", "429", "stream ortasında kopma" → retry/fallback yolları deterministik test edilir.
- `LLM_MODE=auto`: `ANTHROPIC_API_KEY` boşsa `mock`. Böylece `docker compose up` key'siz de demo çalışır (mock, örnek brief'e makul cevap verir).
- `anthropic` adapter'ı ayrıca `httptest` sahte Anthropic sunucusuna karşı test edilir (SSE parse). Gerçek API testi yalnız `ANTHROPIC_API_KEY` varsa çalışır (`make test`'e dahil değil).

### 3.4 Idempotency (aynı kişiye aynı mesaj iki kez gitmesin)
Üç katman:
1. **DB:** `UNIQUE(campaign_id, user_id, channel)` + `INSERT … ON CONFLICT DO NOTHING` → onay endpoint'i iki kez çağrılsa/çökse bile çift satır oluşmaz. Onay CAS ile tek kez geçer.
2. **Gönderim:** worker provider'a `Idempotency-Key: <delivery_id>` yollar; mock provider aynı anahtarı görürse yeniden göndermez, önceki sonucu döner. Timeout/çökme sonrası retry bu yüzden güvenlidir.
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
- **Limit:** yalnız prompt'a güvenmeyiz. `content.Validate`: karakter = **rune** sayısı (Türkçe `ş,ğ,İ` doğru sayılsın); SMS ≤160, email konusu ≤80 **ve tek satır**; değişkenler yalnız `content.Variables` kaydından (`first_name`, `city`).
- **Sabit pay (D36):** her değişken, gerçek değeri ne olursa olsun belirli bir karakter payı olarak sayılır: `first_name` 20, `city` 15 (env: `VARIABLE_BUDGET_*`). Gönderimde değer boşsa veya payı aşıyorsa kısa bir yedek sözcük yazılır ("değerli müşterimiz", "bölgeniz"), yani doğrulanan uzunluk **her alıcı için gerçek üst sınırdır** (testle kanıtlı: seed'deki 30 uzun isimli kullanıcı dahil). Pay prompt'a yazılır, model ne kadar yeri olduğunu bilir.
- Doğrulama 3 yerde: LLM çıktısı (aşarsa repair loop → 3 denemede olmazsa limite uyan fallback), kullanıcı düzenlemesi (canlı UI + sunucu `PUT`), onay (Aşama 4'te yeniden).
- Email gövdesi düz metin; HTML'i biz şablonla üretir ve escape ederiz (LLM'den HTML almayız → XSS yok). UI'da `v-html` kullanılmaz.

### 3.8 SSE
- **Mesaj üretimi:** `POST /api/campaigns/:id/messages/stream` — `text/event-stream`; event'ler: `attempt`, `delta` (alan+parça), `retry` (neden reddedildi), `validated` / `fallback` (final içerik + rapor), `done`, `error`. POST gerektiği için tarayıcıda `fetch` + `ReadableStream` ve elle yazılmış küçük bir SSE ayrıştırıcı (`frontend/src/api/sse.ts`, yeni bağımlılık yok; D38), `EventSource` değil.
- Claude'un structured-output JSON'u metin delta'ları olarak gelir; küçük bir artımlı JSON string tarayıcısı (`llm.FieldStream`) büyüyen alan değerlerini çıkarıp UI'a *taslak* olarak akıtır; **geçerli sayılmaz** — `validated` event'i gelmeden onay mümkün değil. Nihai metin her zaman tam çıktıdan ayrıştırılıp doğrulanır.
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

### Aşama 2+3 (birleşik) — Segment + mesaj üretimi + stream ✅ tamamlandı
- Segment: düz DSL, whitelist, katı ayrıştırıcı, parametreli derleyici, kitle sayımı (izin + iletişim bilgisi sistem tarafından uygulanır). Mesaj: değişken kaydı (`first_name` 20, `city` 15 sabit pay), doğrulayıcı (SMS 160, konu 80 + tek satır), render (pay aşılırsa yedek sözcük), limite uyan fallback. LLM: `llm.Client`, `Repair` döngüsü (3 deneme, geri bildirimli), senaryo tabanlı mock, Claude adapter'ı (structured outputs), artımlı JSON alan akıtıcı. API: `POST /api/campaigns`, `GET /api/campaigns/{id}`, `POST …/messages/stream` (SSE), `PUT …/content`. UI: brief → kitle kartı → akan mesajlar → karakter sayaçları → düzenle/kaydet. Migration `00002`: `campaigns`, `campaign_contents`, `llm_generations`, `deliveries` (+ `UNIQUE(campaign_id, user_id, channel)`).
- **Çalışması gereken:** Örnek brief → İstanbul / son 30 gün / satın almamış → 300 kişi (SMS 203, e-posta 227); mesajlar akar; limit aşan metin otomatik düzeltilir veya fallback gelir; elle düzenleme aynı kuralla doğrulanır.
- **Test:** `make test` (key'siz, mock LLM); `make test-live` yalnız key ile.

### Aşama 4 — Onay, kuyruk, worker, gönderim ⭐ ✅ tamamlandı (kararlar: DECISIONS D52–D56)
**Uygulandı:** 1 onay · 2 `internal/queue` (+ migration 00003/00004) · 3 retry politikası · 4 pacer · 5 sender + worker (+ `WORKER_*` ayarları, `cmd/worker`) · 6 `GET` ilerleme sayaçları + onay kutusu + 1 sn polling · 7 belgeler. Canlı denemede (docker compose, mock LLM): 801 SMS + 896 e-posta kuyruğa alındı → 797 + 892 gönderildi, 4 + 4 `invalid_recipient` (retry yok), kampanya `completed`.

**Onaylanan uygulama sırası (her adım: önce test, kırmızıyı gör, sonra kod; adım sonunda testler çalıştırılıp gösterilir):**
1. Onay: `POST /api/campaigns/{id}/approve` (CAS + tek TX enqueue; onaydan sonra `PUT content` 409).
2. `queue` paketi: Claim (`SKIP LOCKED`, lease), MarkSent, Retry, Fail, Skip; zaman parametre.
3. Retry politikası (saf fonksiyon): 429 → `Retry-After`, deneme harcanmaz, yaş sınırı 10 dk; 5xx/timeout → ≤5 deneme, full jitter; diğer 4xx → kalıcı `failed`.
4. Pacer: kanal başına strict, `Clock`'lu, **varsayılan 90/s** (ayarlanabilir; provider tam 100/s kayan pencere uyguluyor, kenarda gereksiz 429 yememek için), 429'da kanal duraklar.
5. Worker: kanal başına claimer + N gönderici; izin yeniden kontrol (`skipped`), `Render`, provider çağrısı (`Idempotency-Key = delivery.id`, 5 sn istemci zaman aşımı), tamamlanma tespiti.
6. API/UI minimum: `GET /api/campaigns/{id}` kanal bazlı sayaçlar; onay butonu; polling ile yenileme.
7. Belgeler (DECISIONS D52+, README kapsam dışı notları).
- **Onaylanan kararlar:** (a) pacer 90/s; (b) elle düzenlemeden gelen *uyarılar* onayı engellemez (D46), yalnız sert limitler engeller, onay ekranı uyarıları tekrar gösterir; (c) kitle onay anında yeniden hesaplanır, gerçek kuyruğa alınan sayı yanıtta döner; (d) worker tek replica, e-posta düz metin, "delivered" bildirimi yok.
- Approve (CAS + tek TX enqueue), `queue.Queue` arayüzü + Postgres uygulaması, worker, kanal başına strict pacer (`Clock`'lu), retry politikası, idempotency, izin yeniden kontrolü, tamamlanma tespiti.
- **Çalışması gereken:** Onaylanan kampanya gönderilir; provider 429/5xx verse de gönderimler sonunda `sent`/`failed`; çift onay çift gönderim üretmez; 100/s aşılmaz; kalıcı 4xx (bozuk telefon/email) retry edilmeden `failed`.
- **Test:** entegrasyon testleri (gerçek PG + kod içinde kurulu provider, sahte saat): aynı approve 2× ve eşzamanlı → satır sayısı değişmez; worker çöküşü (MarkSent atlanır) → yeniden alınır, provider'da tek kayıt; 429 → `Retry-After`'a uyulur; hiçbir 1 sn penceresinde >100 yok; seed'deki 30 uzun isimli kullanıcıya giden SMS ≤160; `make test`.

#### Aşama 4 ayrıntılı tasarım: gönderim tarafı

**Onay** — `POST /api/campaigns/{id}/approve`, tek transaction:
1. CAS: `UPDATE campaigns SET status='approved', approved_at=now() WHERE id=$1 AND status='content_ready'`. 0 satır etkilenirse ve kampanya zaten `approved|sending|completed` ise **aynı sonuç** döner (idempotent tekrar), başka durumda 409.
2. Saklı içerik `content.Validate` ile yeniden doğrulanır (geçersizse onay reddedilir) ve saklı kural `segment`'e yeniden verilip derlenir (o anki `now` ile; onay anındaki gerçek kitle).
3. Alıcılar `deliveries`'e yazılır, kanal başına bir `INSERT … SELECT`: `INSERT INTO deliveries (campaign_id, user_id, channel) SELECT $1, id, 'sms' FROM users WHERE <kural> AND (sms_consent AND phone IS NOT NULL) ON CONFLICT (campaign_id, user_id, channel) DO NOTHING` (e-posta için aynısı). İzin koşulu Aşama 2'deki `segment.SMSReachableSQL` / `EmailReachableSQL` sabitleridir: sayımda gösterilenle gönderilen aynı kümedir.
4. Yanıt: kanal başına kuyruğa alınan sayı. `UNIQUE(campaign_id, user_id, channel)` + `ON CONFLICT DO NOTHING` sayesinde onay iki kez (hatta eşzamanlı) çalışsa da çift satır oluşmaz.

**Kuyruk** — `internal/queue.Queue` arayüzü: `Claim(ctx, channel, limit, lease, now)`, `MarkSent`, `Retry(at, err, countAttempt)`, `Fail`, `Skip`. Postgres uygulaması: `UPDATE deliveries SET status='sending', locked_until=$now+lease WHERE id IN (SELECT id FROM deliveries WHERE channel=$1 AND next_attempt_at<=$now AND (status='pending' OR (status='sending' AND locked_until<$now)) ORDER BY next_attempt_at LIMIT $n FOR UPDATE SKIP LOCKED) RETURNING …` (kullanıcı verisiyle birlikte). Zaman SQL'e **parametre** verilir (`now()` değil), bu yüzden testler sahte saatle lease/backoff'u gerçek bekleme olmadan sınar. Çöken worker'ın `sending` satırı lease dolunca yeniden alınır.

**Worker** — kanal başına bir "claimer" küçük partiler (≤20, lease 30 sn) çeker ve N (varsayılan 20) gönderici goroutine'e dağıtır; hepsi kanalın **tek pacer'ını** paylaşır. Pacer, `Clock` ile eşit aralıklı (10 ms) slot rezerve eder → herhangi bir 1 sn penceresinde ≤100 (burst yok); 429 alınca `PauseUntil(Retry-After)` ile kanal duraklatılır. Her iş: (1) pacer.Wait, (2) izin/iletişim bilgisini **yeniden kontrol et** (artık yoksa `skipped`), (3) `content.Render` ile alıcıya göre metni üret (pay aşılırsa yedek sözcük → SMS ≤160 garantisi), (4) provider'a `POST` (`Idempotency-Key = delivery.id`, istemci zaman aşımı 5 sn < provider'ın 10 sn "timeout" kaosu), (5) sonucu işle.

**Sonuç işleme** (§3.5 tablosu): 2xx → `sent` (+ `provider_message_id`); 429 → `Retry-After` kadar sonra, **deneme hakkı harcanmaz** ama teslimat yaşı üst sınırı (10 dk) var; 5xx/zaman aşımı/bağlantı hatası → `attempts+1`, backoff `min(60 sn, 2 sn·2ⁿ)` **full jitter**, en fazla 5 deneme; diğer 4xx (ör. `invalid_recipient`) → kalıcı `failed`, retry yok; deneme/yaş aşımı → `failed` + `last_error`. Retry politikası saf bir fonksiyon (rastgelelik enjekte edilir), zaman `Clock`'tan.

**Tamamlanma** — worker, bekleyen/gönderilen satırı kalmayan kampanyaları `completed` yapar (`approved→sending` ilk claim'de). Kampanya sonucu: sent / failed / skipped sayıları `deliveries`'ten okunur (Aşama 5 dashboard'u).

**Kaçınılan tuzaklar:** "gönderdim ama `sent` yazamadım" çökmesi → aynı `Idempotency-Key` ile yeniden gönderim → provider replay döner, ikinci teslimat yok (D27 modu bunu sürekli sınar); provider'ın 5xx döndüğü ama mesajı aldığı durum → aynı çözüm; iki worker → `SKIP LOCKED` ile aynı satırı iki kişi alamaz; `now()` yerine parametre → test edilebilirlik.

**Kapsam dışı (README'de not):** worker tek replica (limiter süreç içi, D13); e-posta gövdesi düz metin olarak gönderilir (HTML şablonu yok); teslim bildirimi ("delivered") yok, `sent` = provider kabul etti.

### Aşama 5 — Dashboard, metrics, health ⭐ ✅ tamamlandı (kararlar: DECISIONS D57–D61)
**Uygulandı:** migration `00005` · `queue.Observe` · `internal/metrics` (+ api `GET /metrics`) · api `/healthz` DB kontrolü (+ compose healthcheck) · `GET /api/campaigns` + Dashboard sekmesi · `make test-race` (temiz) · README. (Aşağıdaki liste onaylanan plandır.)
### (Aşama 5 planı) KÜÇÜK TUTULACAK: yalnız CASE'in zorunlu 3 maddesi
- **Zorunlu (CASE):** (1) Dashboard'da gönderim durumu canlı akar (SSE ya da polling; Aşama 4'teki durum sayaçları üzerine kurulur). (2) `/metrics`: kanal bazlı **gönderilen, başarısız, tekrar denenen, 429 sayısı, kuyruk uzunluğu**. (3) API'de de `/healthz` olsun ve **veritabanı bağlantısını kontrol etsin**.
- **Yapılmayacak (kullanıcı kararı):** throughput grafiği, "son hatalar" listesi, süs. Arayüz sonraki bir aşamada baştan tasarlanacak; dashboard sade ve işlevsel kalır.
- **Onaylanan uygulama sırası (her adım: önce test, kırmızıyı gör, sonra kod):**
  1. Migration `00005`: `deliveries.provider_calls/provider_micros` (provider yanıt süresi) ve `llm_generations.refused/fell_back` (refusal ve fallback'i sezgiyle değil açıkça kaydet). `queue.Observe`; servis `refused` ve `fell_back` yazar.
  2. `internal/metrics`: DB'den (deliveries, llm_generations, campaigns) Prometheus metin formatı üretir; api `GET /metrics`. **Gönderim:** kanal bazlı sent/failed/skipped, retry, 429, kuyruk uzunluğu, saniyedeki gönderim hızı (son 10 sn), provider yanıt süresi (toplam+adet). **LLM:** segment/mesaj üretim sayısı, ilk denemede geçerli, repair denemesi, fallback, refusal, needs_clarification/unsupported, süre (toplam+adet), token.
  3. api `/healthz` veritabanını kontrol eder (503 aksi halde); compose healthcheck bunu kullanır.
  4. `GET /api/campaigns` (liste, kanal bazlı sent/failed/retry/429) + Dashboard sayfası (ilerleme çubuklu, 2 sn polling).
  5. `-race` (CGO_ENABLED=1, Docker) ile kuyruk, worker, eşzamanlı onay testleri; hata çıkarsa systematic-debugging.
  6. README: hızlı demo notu (`PROVIDER_429_RATE=0`) belirgin, `/metrics` ve `/healthz` adresleri; DECISIONS.
- **Test:** sayaçlar gönderim sırasında canlı değişir; `curl .../metrics`; DB'yi durdur → api `/healthz` hata döner.

### Aşama 6 — Cilalama ve teslim
- **Arayüzün baştan tasarımı (kullanıcı kararı; Aşama 5 dashboard'u bilerek sade bırakıldı).** Frontend testleri (vitest: composable + kritik bileşenler), hata/boş durum UI'ları, README (kurulum, mimari, yapılmayanlar), DECISIONS son hali, `make test` Windows notları, ai-sessions export + temizlik, repo'yu GitHub'a push.
- **Test:** temiz klonla `cp .env.example .env && docker compose up` ve `make test` sıfırdan geçer.

### Açık işler (bilinen eksikler, henüz çözülmedi)
- **Yazım hatası satın-almama koşulunu düşürebilir (D51).** Gerçek Claude ile görüldü: brief'te "oluşturmmaış" gibi bir yazım hatası olunca `purchased_since_last_cart_add eq false` sessizce düştü. `brief.Analyze` sözlüğü yazım hatasını tanımadığı için `Verify` koşulu zorunlu kılmıyor (D44'ün "sözlük sonlu" sınırı). Aday çözümler: yazım toleranslı eşleme veya modelin çıkardığı koşul listesiyle çapraz doğrulama. Kullanıcı kararı: şimdi çözülmüyor; Aşama 6'da vakit varsa ele alınır, README'de bilinen sınırlama olarak duruyor.

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
| 6 | Hangi değişkenler | **Güncel:** yalnız `first_name` ve `city` (`last_name` çıkarıldı, D35); tek bir kayıtta (`content.Variables`), eklemek kolay. Sabit karakter payı: 20 ve 15 (D36); boş/uzun değerde yedek sözcük | Kullanıcı kararı |
| 7 | Her iki kanal zorunlu mu | **Kullanıcı kararı:** kanal seçimi yok, SMS ve email her zaman birlikte üretilir | Onaylandı |
| 8 | 100/s kapsamı | Kanal başına, tüm kampanyalar toplamı, worker tek replica | Dağıtık limiter bilinçli kapsam dışı |
| 9 | Kullanıcı segmenti düzenleyebilir mi | Hayır; kural salt-okunur gösterilir, brief yeniden yazılır | Kesilebilecek özellik zaten çıkarıldı |
| 10 | Teslim durumu | `sent` (provider kabul etti) yeterli; "delivered" webhook'u yok | Bonus/kapsam dışı |
| 11 | Gönderimden sonra snapshot | Onayda alıcılar `deliveries`'e yazılır (snapshot); sonradan değişen veri sayıyı etkilemez, yalnız izin gönderim anında yeniden kontrol edilir | |
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
3. PLAN §9 varsayımları geçerli; #7 değişti (kanal seçimi yok). Değişkenler: yalnız `first_name` ve `city`, sabit paylarla (20 / 15); SMS 160 ve konu 80 asla aşılmaz, konu tek satır. Düz DSL, structured outputs, refusal fallback beta'sı yok, SSE elle yazılmış ayrıştırıcıyla.
4. LLM: varsayılan mock, `LLM_MODE=auto`; key doluysa Claude; `CLAUDE_MODEL` varsayılan `claude-sonnet-5-5`; `make test` daima mock; `make test-live` ayrı; mod log ve UI'da görünür.
5. Kapsam: bonuslar yapılmayacak; kapsam dışı özellik için önce sorulacak.

### Aşama 5.1 ✅ tamamlandı (DECISIONS D62–D66) — Teklif şartları ve "herkese" kitlesi (kullanıcı geri bildirimi: "anneler gününe özel 1000 tl üzeri alışverişe 100tl indirim… son kullanım 31.12.2026")
Sorun: "1000 TL üzeri" kitle koşulu sanıldı; oysa teklifin şartıydı.
1. `offer.Offer`: `min_spend` (alt limit, TL) ve `expires_at` (YYYY-MM-DD). Brief'ten çıkarılır (`1000 TL üzeri alışverişe`, `son kullanım 31.12.2026`, `31 Aralık 2026'ya kadar`…); mesaj doğrulaması SMS'te ve e-postada ikisinin de geçtiğini ve başka bir tarih yazılmadığını denetler; prompt'lar "alışveriş tutarı şartları teklife aittir, kitleye değil" der.
2. Kitle koşulu: brief açıkça "herkese / tüm müşterilere" diyorsa açık işaretli koşul `all_customers eq true` (boş kural değil; izin filtresi yine uygulanır). Hiçbir kitle belirtilmemişse `needs_clarification`: "Bu kampanya kime gönderilsin? Tüm müşterilere mi?" + ekranda "Tüm müşterilere gönder" düğmesi.
3. `needs_clarification` mesajlarındaki örnekler brief'teki sayıyı kullanır.
4. Test seti (tablo) + `make test-live`'a bu brief ve "herkese %10 indirim" örnekleri.
