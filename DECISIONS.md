# DECISIONS.md

Format: **Karar → Alternatifler → Gerekçe → Bedeli / nasıl savunulur.**
Durum: `Önerildi` (onay bekliyor) · `Kesin` (CASE.md'nin dayattığı) · her şey kullanıcı onayına tabidir.

---

## D1 — Backend dili: Go · `Kesin`
- Alternatif: PHP.
- Gerekçe: eşzamanlılık (worker havuzu, limiter, SSE) için goroutine'ler doğal; tek statik binary; küçük imajlar.
- Bedeli: ORM ekosistemi küçük → pgx + elle SQL (zaten segment compiler için istiyoruz).

## D2 — Veritabanı: PostgreSQL 16 · `Önerildi`
- Alternatifler: SQLite, MySQL.
- Gerekçe: `FOR UPDATE SKIP LOCKED` (kuyruk), `jsonb` (segment kuralı, LLM audit), `ON CONFLICT` (idempotency), advisory lock (seed), güçlü transaction garantisi.
- Bedeli: ek container; kabul edilebilir (zaten compose var).

## D3 — Kuyruk: Postgres tabanlı (`messages` tablosu) · `Önerildi — son karar kullanıcıda`
- Alternatifler: Redis (asynq/streams), RabbitMQ. Karşılaştırma PLAN.md §4'te.
- Gerekçe:
  1. Onay + kuyruğa yazma **tek transaction** → "kampanya onaylandı ama mesajlar kuyruğa girmedi" durumu yok (broker ile DB arasında dual-write/outbox derdi yok).
  2. Idempotency `UNIQUE(campaign_id,user_id,channel)` ile aynı yerde.
  3. Dashboard zaten bu tabloyu okur; ayrı durum deposu gerekmez.
  4. Ek altyapı yok → `docker compose up` basit, test gerçek DB ile.
  5. İhtiyaç ~200 msg/sn (2 kanal × 100); PG rahat taşır.
- Bedeli: polling gecikmesi (~100–500 ms), ölçek tavanı, dağıtık rate limit yok, tablo şişmesi (vacuum).
- Savunma: gereksinim "asenkron kuyruk" der, teknoloji serbest. Kuyruk `queue.Queue` arayüzünün arkasında tutulur; iş mantığı (sender, campaign) yalnız bu arayüzü bilir. Geçiş yolu açık.
- **Neden Redis değil:** (a) Redis ayrı bir depo → "onay DB'de, mesajlar Redis'te" dual-write sorunu; biri yazılıp diğeri yazılamazsa kampanya onaylı ama mesajsız kalır (outbox/reconciliation gerekir). (b) Idempotency kısıtını ve dashboard durumunu yine DB'de tutacağız, yani Redis ikinci bir doğruluk kaynağı olurdu. (c) Kalıcılık AOF/RDB ayarına bağlı; mesaj kaybı riski DB'den yüksek. Redis'in asıl artısı (dağıtık rate limit, yüksek throughput) bu kapsamda ihtiyaç değil.
- **Neden RabbitMQ değil:** Ayrı bir servis, yönetim ve konfigürasyon yükü (exchange/queue/DLX/TTL); retry gecikmesi için DLX+TTL zinciri `next_attempt_at` kolonundan daha hantal; yine de durum ve idempotency için DB gerekir. Case ölçeğinde (kanal başına 100 msg/sn) maliyeti faydasını aşar.
- **Ölçek sınırı (dürüst):** Tasarım kanal başına birkaç yüz–binlerce msg/sn ve birkaç yüz bin–milyon satıra kadar rahat; `SKIP LOCKED` birden çok worker'ı destekler. Ancak (1) kanal başına 100/sn limiti süreç içi olduğu için worker **tek replica** olmalı; çoklu replika için dağıtık limiter gerekir. (2) `messages` tablosu büyüdükçe tamamlanmış satırlar arşivlenmeli/partition edilmeli, vacuum izlenmeli. (3) Polling aralığı gecikme/DB yükü takasıdır. Bunların ötesinde (çok yüksek throughput, çok bölgeli dağıtım) Redis Streams/RabbitMQ'ya geçilir; arayüz sayesinde değişiklik tek pakette kalır.

## D4 — Servis ayrımı: api / worker / mock-provider / web / postgres / migrate · `Önerildi`
- Alternatif: tek monolit süreç (worker goroutine olarak API içinde).
- Gerekçe: worker'ı bağımsız kapatıp "çökme → yeniden alma" senaryosunu gösterebilmek; API'nin yavaş provider'dan etkilenmemesi; mock-provider gerçek bir dış servis gibi ayrı (ağ üzerinden hata/gecikme gerçek).
- Bedeli: daha fazla container; tek modül + tek Dockerfile ile hafifletildi.

## D5 — Tek Go modülü, çok binary, tek Dockerfile (multi-stage hedefler) · `Önerildi`
- Gerekçe: ortak paketler (config, store, llm, segment) tekrar yok; bir kez derleme cache'i; servis başına sapma yok.

## D6 — İstemci iletişimi: SSE (WebSocket değil) · `Önerildi`
- Alternatifler: WebSocket, polling.
- Gerekçe: iki kullanım da tek yönlü (sunucu→istemci); SSE düz HTTP üstünde, otomatik yeniden bağlanma (EventSource), proxy dostu, WebSocket'e göre çok daha az kod.
- Mesaj üretimi POST gerektirir → `fetch` + stream okuyucu (EventSource yalnız GET).
- Dashboard: olay yerine **snapshot** (her ~1 sn DB agregasyonu). Neden: worker ayrı süreç → API'ye olay taşımak (LISTEN/NOTIFY veya pub/sub) fazladan hareketli parça; snapshot kopma/yeniden bağlanmada kendiliğinden doğru, `Last-Event-ID` gerekmez. Bedeli: ≤1 sn gecikme (canlı izleme için yeterli).

## D7 — LLM çıktısı: zorunlu tool-use + katı JSON Schema · `Önerildi`
- Alternatifler: "JSON ver" diye prompt'lamak + regex/parse; markdown code-fence ayıklamak.
- Gerekçe: `tool_choice` ile modeli şemalı çağrıya zorlamak parse hatalarını azaltır; yine de **şemaya güvenmeyiz**, backend ayrıca doğrular (savunma derinliği). LLM'in cevabı *veri*, kod değil.

## D8 — Segment: SQL yerine kısıtlı DSL (AST) + whitelist + parametreli derleyici · `Önerildi` (F1 gereği)
- Alternatifler: LLM'den SQL `WHERE` isteyip regex/SQL-parser ile temizlemek; ORM filtre string'i.
- Gerekçe: SQL'i "temizlemek" kara liste mantığıdır, kırılgandır. DSL'de ifade gücü baştan sınırlı: alan/operatör whitelist, tipli değerler, derinlik/boyut limiti. SQL **yalnız bizim kodumuzda** üretilir; değerler bind parametresi. Güvenlik özelliği tasarımla gelir, filtreyle değil.
- Bedeli: ifade gücü sınırlı (yeni alan = kod değişikliği). Bu kasıtlı; fazlasını istemek `unsupported` döner.
- İzin (consent) filtresi LLM kuralının parçası değil, sistem tarafından eklenir → LLM izni "unutsa/atlatsa" bile etkisizdir.

## D9 — Geçersiz LLM çıktısı: hata geri beslemeli repair loop (max 3 deneme) + güvenli fallback · `Önerildi`
- Gerekçe: doğrulama hatası çoğu zaman düzeltilebilir ("SMS 183 karakter") → hatayı modele geri vermek ucuz ve etkili. Sonsuz döngü yok (3 deneme + toplam süre bütçesi).
- Fallback politikası **bilinçli olarak asimetrik**:
  - Segment: fallback yok (yanlış kitleye gitmektense dur) → 422 + kullanıcıdan brief netleştirme.
  - İçerik: sessiz kesme yok; deterministik şablon taslağı + "AI üretemedi" uyarısı; insan zaten onaylıyor.
- Belirsizlik için `needs_clarification` / `unsupported` durumları şemada birinci sınıf: model tahmin yürütmek yerine soru sorabilir.
- Her deneme `llm_generations`'a yazılır → denetlenebilirlik + review'da gösterilebilir.

## D10 — LLM soyutlaması + senaryo tabanlı mock · `Önerildi` (A2 gereği)
- `llm.Client` arayüzü; `anthropic` ve `mock` uygulamaları. `LLM_MODE=auto` → key yoksa mock.
- Mock senaryo tabanlı (bozuk JSON, limit aşan metin, 429, kopan stream…) → hata yolları deterministik test edilir. "Mutlu yol" mock'u tek başına yetersiz; asıl değer hata yollarını test edebilmekte.
- Anthropic adapter'ı `httptest` sahte sunucuya karşı ayrıca test edilir; gerçek API testi opt-in (`make test-live`).
- `docker compose up` key'siz çalışır → değerlendirici hemen deneyebilir.

## D11 — Idempotency: üç katman · `Önerildi`
1. DB benzersizlik kısıtı + `ON CONFLICT DO NOTHING`; onay geçişi CAS.
2. Provider'a `Idempotency-Key` (= message id); mock provider dedupe eder.
3. `sent` satırlar asla yeniden claim edilmez.
- Gerekçe: dağıtık sistemde "tam bir kez" yoktur; **at-least-once + idempotent alıcı** etkin olarak aynı sonucu verir. Timeout durumunda mesaj gitmiş olabilir; retry'ın güvenli olması anahtara bağlıdır.
- Kapsam: aynı kampanya içinde. Kampanyalar arası dedupe yapılmaz (belirsizlik #1).

## D12 — Retry: backoff + jitter, 429'u ayrı ele alma, kalıcı hatayı retry etme · `Önerildi`
- 5xx/timeout: 2s·2ⁿ full jitter, tavan 60s, max 5. 4xx: kalıcı → retry yok (boşa yük). 429: `Retry-After`'a uy, kanalı duraklat, deneme bütçesini tüketme (kapasite sorunu mesajın hatası değil), ama toplam yaş sınırı (10 dk) ile sonsuz bekleme engellenir.
- `next_attempt_at` kolonu ile uygulanır (ayrı gecikme kuyruğu yok). Çöken worker için `locked_until` lease.
- Jitter nedeni: aynı anda hata alan mesajların aynı anda geri gelip "thundering herd" yapmaması.

## D13 — Rate limit: kanal başına süreç içi token bucket (`rate=100`, `burst` çok küçük) · `Önerildi`
- Alternatifler: Redis tabanlı dağıtık limiter, sabit pencere sayacı.
- Gerekçe: tek worker için en basit ve doğru; `burst=1` eşit aralıkla (10 ms) gönderir, böylece herhangi bir 1 sn penceresinde 100'ü aşma riski yok (büyük burst pencere kenarında ~200'e çıkabilir).
- Provider tarafı da bağımsız sayar → limiter hatası 429 olarak yakalanır (iki yönlü doğrulama).
- Bedeli: worker tek replica olmalı. README'de açık yazılır; ölçekte dağıtık limiter gerekir.

## D14 — Karakter limiti: rune sayısı, render sonrası en kötü durum, 3 noktada doğrulama · `Önerildi`
- `len()` (byte) Türkçe karakterde yanlış sayar → rune. Değişkenli şablonda en uzun `first_name` ile hesap.
- Doğrulama: LLM çıktısı, kullanıcı düzenlemesi (canlı + sunucu), onay. Prompt'a "160 karakter" yazmak yeterli sayılmaz.
- Not: gerçek SMS'te GSM-7/UCS-2 segment mantığı farklıdır; case 160 karakter dediği için basit tuttuk, README'de belirtilir.

## D15 — Email gövdesi düz metin, HTML'i biz üretir · `Önerildi`
- LLM'den HTML almak XSS/bozuk markup riski. Şablon + escape bizde; UI'da `v-html` yok.

## D16 — Seed: Go ile deterministik, idempotent, `migrate` one-shot servisi · `Önerildi`
- Alternatif: `docker-entrypoint-initdb.d` SQL dosyaları.
- Gerekçe: initdb yalnız ilk volume'da çalışır; SQL ile gerçekçi/deterministik veri zor; Go seed testlerde de kullanılabilir. `SEED=42` → her çalıştırmada aynı sayılar (test beklentileri sabit). Advisory lock + "users boş mu" kontrolü → tekrar `up` güvenli.
- Edge-case verisi bilerek eklenir (izinsiz, telefonsuz, çok uzun isim).

## D17 — Compose bağımlılık sırası: healthcheck + `service_completed_successfully` · `Önerildi`
- `postgres(healthy) → migrate(completed) → api/worker`. `sleep`/retry-loop hack'i yok. API ayağa kalktığında şema+seed hazır olur.
- Runtime imajı alpine (healthcheck için `wget`); distroless'ın küçük boyut avantajından vazgeçildi.
- Yalnız `web` (8080) asıl giriş; DB host'a açılmaz (güvenlik + port çakışması yok).

## D18 — Frontend → API: nginx reverse proxy (CORS yok) · `Önerildi`
- Aynı origin → CORS ve credential sorunu yok; SSE için `proxy_buffering off`. Bedeli: nginx config bakımı (küçük).

## D19 — Geliştirme akışı: `docker compose watch` + `air` (docker-compose.dev.yml) · `Kesin (kullanıcı kararı)`
- Alternatifler: bind mount + air (Windows'ta dosya olayları güvenilmez), host'ta Go/Vite (hızlı ama toolchain gerekir).
- Gerekçe: `watch` sync host'tan olay okur, container içine yazar; `air` container içinde inotify ile derler → Windows sorunu yok, rebuild yok. Aşama 0'da doğrulandı: Go değişikliği air'i tetikledi, yeni Vite dosyası sync edildi.
- Vite dev server'da `/api` proxy'si `vite.config.ts`'te (`API_PROXY_TARGET`); nginx'te `proxy_http_version 1.1`, `Connection ""`, `proxy_buffering off`, `proxy_read_timeout 3600s`.

## D20 — Test: Docker içinde `make test`, host'ta Go/Node gerekmez · `Kesin (kullanıcı kararı)`
- Tüm komutlar Docker içinde (`backend-test`, `frontend-test` servisleri, `test` profili).
- Entegrasyon testleri gerçek PG ile (mock'lanmış DB kuyruk davranışını kanıtlamaz).
- `make test` her zaman mock LLM: test servisleri `ANTHROPIC_API_KEY`'i `.env`'den **almaz** (`ANTHROPIC_API_KEY: ""`, `LLM_MODE: mock` sabit). `.env`'de key olsa bile testler mock'a bağlı kalır, internet gerekmez.
- Gerçek Claude duman testi `make test-live` (`live` profili, `-tags=live`); `make test`'e dahil değil.

## D24 — LLM modu: varsayılan mock, key varsa otomatik gerçek Claude · `Kesin (kullanıcı kararı)`
- `.env.example`: `ANTHROPIC_API_KEY=` (boş), `LLM_MODE=auto`, `CLAUDE_MODEL=claude-sonnet-5-5`.
- `auto`: key doluysa `anthropic` adapter, boşsa mock. `mock`: key olsa bile mock. `claude`: key zorunlu, yoksa açılışta hata. Key'i sonradan `.env`'ye yazıp restart → kod değişikliği olmadan gerçek modele geçiş.
- Aktif mod açılışta loga (`LLM mode active`, key asla loglanmaz) ve UI'a (üst çubuktaki rozet: "Mock LLM" / "Claude (model)") yansır; `GET /api/info` ile taşınır.
- Çözümleme mantığı `config.ResolveLLM` içinde, tablo tabanlı testli.

## D21 — Kapsam: yalnızca CASE.md'de istenenler; bonuslar yapılmayacak · `Kesin (kullanıcı kararı)`
- Case "çekirdek akışı sağlam yap" diyor. Bonuslar (zamanlanmış gönderim, A/B, dile göre içerik, CI) **kapsam dışı**; README'de "yapılmadı" olarak listelenir.
- Kural: CASE.md'de olmayan bir özellik gerekirse önce kullanıcıya sorulur.
- Kanal seçimi yok: SMS ve email **her zaman birlikte** üretilir ve gönderilir (kullanıcı kararı; PLAN §9 #7).
- Değişkenler: `first_name`, `last_name`, `city`.

## D22 — Auth yok, çok kiracılık yok · `Önerildi`
- Case kapsamı dışında; README'de belirtilir.

## D23 — Gözlemlenebilirlik: `/healthz` (liveness), `/readyz` (DB), `/metrics` (Prometheus) · `Önerildi`
- Liveness/readiness ayrımı (Docker healthcheck readiness'i kullanır). Metrikler: kuyruk derinliği (status×channel), gönderim sonuç sayaçları, retry/429 sayısı, LLM çağrı süresi/hata/retry sayısı.

---

## Açık / onay bekleyenler
- Yok. D3, D19, D20, D21, D24 ve PLAN §9 varsayımları kullanıcı tarafından onaylandı.
