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

## D7 — LLM çıktısı: zorunlu tool-use + katı JSON Schema · `REVİZE EDİLDİ → bkz. D33 (structured outputs)`
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

## D25 — Migration aracı: goose (gömülü SQL, `provider` API'si, Postgres session lock) · `Kesin (kullanıcı kararı)`
- Alternatifler: golang-migrate, elle yazılmış `schema.sql`, ORM auto-migrate.
- Gerekçe: düz SQL dosyaları (`00001_*.sql`, `-- +goose Up/Down`) `go:embed` ile binary'ye gömülür → ayrı dosya kopyalamak yok, `migrate` servisi tek binary. `goose.NewProvider` + `WithSessionLocker` ile migration Postgres advisory lock altında çalışır: iki süreç aynı anda başlasa da biri bekler, ikincisi yapacak iş bulamaz. ORM auto-migrate şemayı kodda gizler ve review'da savunması zor.
- Bedeli: goose ek bağımlılık getirir (go.mod'a sqlite sürücüleri dolaylı girer, derlenmez). Kabul edildi.

## D26 — `users` üzerinde türev kolonlar (`last_cart_add_at`, `last_order_at`, `order_count`, `total_spent`) · `Kesin`
- Alternatif: segment sorgusunda her seferinde `cart_events`/`orders` üzerinde `EXISTS`/agregasyon.
- Gerekçe: segment kuralı compiler'ı tek tablo, join'siz, indeksli `WHERE` üretir; hem güvenlik (whitelist edilmiş sabit SQL ifadeleri, D8) hem hız için basit. "Alışveriş yapmayan" `last_order_at < last_cart_add_at` olarak ifade edilir; ayrı bir `purchased_since_last_cart_add` kolonu yok (iki kolondan türer, tutarsızlık riski yok).
- Tutarlılık: ham tablolar bu uygulamada yalnız seed ile yazılır (insert-only); seed türev değerleri üretim sırasında hesaplar. İki test bunu kilitler: birim testi (Go'da ham olaylardan yeniden hesap) ve entegrasyon testi (SQL agregasyonu ile kolonlar aynı mı). Gerçek bir sistemde bu kolonlar trigger/olay ile güncellenirdi; kapsam dışı.
- `id`'ler `GENERATED BY DEFAULT` (ALWAYS değil): seed deterministik id'leri `COPY` ile yazar, sonra sequence'ler ilerletilir (test: sonraki id 5001).

## D27 — Mock provider'da "kabul-sonrası-hata" modu (`PROVIDER_FAIL_AFTER_ACCEPT_RATE`) · `Kesin`
- Amaç: idempotency'nin var olma sebebini **gerçekten** sınamak. Provider mesajı kaydeder ama istemciye 5xx döner. Worker bunu başarısızlık sanıp aynı `Idempotency-Key` ile tekrar dener; provider replay (`Idempotent-Replay: true`) döner ve **ikinci teslimat oluşmaz**.
- Bu mod olmadan "5xx = hiç gitmedi" varsayımı test edilir, ama gerçek dünyanın zor durumu (zaman aşımı/5xx ama mesaj gitti) test edilmemiş olur. Test: her ilk deneme kabul-sonrası-hata, her anahtar 3 kez denenir → tam olarak 1 tekil teslimat.
- Replay, rate limit'ten **sonra** ama kaos'tan **önce** değerlendirilir: replay de limite sayılır (gerçek sağlayıcılar böyle davranır), ama enjekte hatalara tabi değildir.

## D28 — Mock provider'da `/admin/*` yüzeyi YOK · `Kesin (kullanıcı kararı)`
- Provider yalnız `POST /v1/sms/send`, `POST /v1/email/send`, `GET /healthz` sunar (testle kilitli: `/admin/*`, `/metrics` 404).
- Gerekçe: gerçek bir provider'a müşteri tarafından test amaçlı istatistik ucu eklenmez; ayrıca kapsam dışı yüzey = savunulacak ek yüzey. "100/sn aşılmadı" ve "tekil teslim sayısı = tekil key sayısı" iddiaları testlerde provider'ı **kod içinde** kurup sayaçlarına (`Server.Stats()`) doğrudan bakarak doğrulanır; ayrıca sayaca güvenmemek için pencere kontrolü testte bağımsız olarak yeniden hesaplanır.
- Elle denemek için her 429/5xx/replay **sebebiyle** loga yazılır: `reason` ∈ {`injected`, `rate_limit`, `fail_after_accept`, `timeout`, `replay`, doğrulama kodları}.

## D29 — `language`, `gender`, `birth_year` kolonları YOK · `Kesin (kullanıcı kararı)`
- `language` yalnız "dile göre içerik" bonusu için gerekirdi; bonus kapsam dışı. `gender`/`birth_year` hiçbir gereksinim tarafından kullanılmaz (segment örnekleri şehir/sepet/sipariş/izin).
- Gerekçe: kullanılmayan kolon = test edilmeyen, savunulması gereken yüzey; LLM'e whitelist'te sunulacak alan sayısı küçük kalınca doğrulama da basitleşir. Gerektiğinde migration ile eklenir.
- Bilinçli eklenenler: izinli ama telefonu/emaili boş kullanıcılar ve bozuk telefon/email (%0,5). Bozuk kayıtlar provider'dan kalıcı 400 alır → retry edilmeden `failed` yolunu uçtan uca gösterir (D12).

## D30 — `Clock` arayüzü (`Now`, `Sleep(ctx, d)`) yalnız zamana bağlı mantıkta · `Kesin (kullanıcı kararı)`
- Gerçek (`Real`) ve sahte (`Fake`) uygulama. `Fake.Sleep` bloklamaz, sahte zamanı ilerletir → testlerde **gerçek sleep yok**, `make test` hızlı kalır (provider paketi ~25 ms, 10 sn'lik timeout senaryosu dahil).
- Kullanım yeri: provider'ın kayan pencere limiter'ı ve gecikme/timeout simülasyonu. İleride yalnız worker'ın limiter'ı ve retry/backoff'u. Başka koda **yayılmaz** (seed `now`'ı parametre olarak alır; saf fonksiyon, Clock gerekmez).
- Gerekçe: limiter ve backoff zamanı ölçen mantıktır; gerçek saatle test etmek yavaş ve kırılgan (flaky) olurdu. Arayüz minimum tutuldu (iki metot), `time.Timer`/`Ticker` soyutlamaları bilerek yok.

## D31 — Kaos oranları testlerde yalnız 0 veya 1 · `Kesin (kullanıcı kararı)`
- Rastgelelik yüzünden bazen geçen bazen kalan test olmaz. `rate=0` hiç, `rate=1` her zaman tetikler (`Float64() < rate`). Tek istisna: aynı seed'in aynı hata dizisini ürettiğini kanıtlayan test (karşılaştırma iki koşu arasında, sabit sonuca bağlı değil).
- Varsayılan (demo) oranlar `.env.example`'da açıklanır ve nasıl düşürüleceği yazılır.

## D32 — Postgres healthcheck TCP üzerinden (`pg_isready -h 127.0.0.1`) · `Önerildi`
- Bulgu (Aşama 1'de `make clean` sonrası taze volume'da yaşandı): imaj ilk açılışta yalnız unix socket dinleyen **geçici** sunucu başlatır. Socket tabanlı `pg_isready` bu aşamada "healthy" der, `migrate` bağlanmaya çalışınca `connection refused` alır. TCP kontrolü yalnız gerçek sunucu kalkınca geçer.
- Neden önemli: hata yalnız **ilk kurulumda** çıkar (mevcut volume'da görünmez), yani değerlendiricinin `docker compose up`'ında çıkacak tam hata budur.

## D33 — JSON için structured outputs; zorunlu `tool_choice` yok · `Kesin` (D7'nin revizyonu)
- Alternatif (D7'deki plan): `tool_choice` ile belirli bir tool'u çağırmaya zorlamak.
- Gerekçe: `claude-sonnet-5-5` (varsayılan model) `tool_choice: any|tool` isteğini **HTTP 400** ile reddediyor. Aynı garantiyi `output_config.format` (JSON Schema) ile alıyoruz: model şemaya uyan JSON üretir, biz yine de katı biçimde ayrıştırıp doğrularız (savunma derinliği, D8).
- Aynı model ailesinin diğer kısıtlarına uyuyoruz ve testle kilitledik (adapter sahte sunucuya karşı test eder): `temperature/top_p/top_k` gönderilmez, `thinking` ayarı gönderilmez (varsayılan adaptive; derinlik `output_config.effort` ile: kitle çıkarma `low`, metin yazma `medium`), prefill yok (retry = yeni kullanıcı turu), `max_tokens` 8000 (thinking payı dahil).
- Şema dili aralık/uzunluk/özyineleme ifade edemez → 160/80 sınırları, sayı aralıkları ve alan değerleri **yalnız backend'de** doğrulanır. Bu zaten tasarımın ilkesi: şemaya güvenmeyiz.
- Bedeli: Go SDK'nın yeni major'ı (`anthropic-sdk-go/v2`) Go 1.25 ister → toolchain 1.25'e çıktı (D42).

## D34 — Segment DSL düz (iç içe grup yok) · `Kesin (kullanıcı kararı)`
- Alternatif: iç içe `all/any/not` ağacı.
- Gerekçe: structured outputs özyinelemeli şemayı desteklemez; düz liste + tek `match` (`all|any`) şemaya sığar, doğrulaması ve derlemesi basit, saldırı yüzeyi küçük. `not` ihtiyacı `neq`/`not_in` ile karşılanır. Brief'ler pratikte "ve" ile bağlanan koşullardır.
- **Güncelleme (D50):** "Son N günde sipariş vermemiş" için DSL'e bir OR/iç içe grup eklemedik; düz kalıyor. Bunun yerine whitelist'e NULL'ı da kapsayan tek bir alan eklendi (`days_since_last_order_or_never`). Düzlük kararı geçerli; değişen, düz kuralın bu en yaygın kampanyayı ifade edebilmesi.
- Bedeli: "(A veya B) ve C" gibi karma ifadeler yok; LLM bunu ifade edemezse `unsupported`/`needs_clarification` döner. Kabul edildi.

## D35 — Değişkenler `first_name` ve `city`; tek yerde tanımlı (`content.Variables`) · `Kesin (kullanıcı kararı)`
- `last_name` çıkarıldı (kullanılmıyor; uzunluk bütçesini ve doğrulama yüzeyini gereksiz büyütüyordu). Kolon `users` tablosunda duruyor ama şablonda kullanılamaz (test: `{{last_name}}` → `unknown_variable`).
- Tek kayıt: yeni değişken eklemek = `content.Variables`'a bir satır (+ `Recipient` alanı). Doğrulayıcı, render, LLM prompt'u, UI limit/ipucu listesi ve `VARIABLE_BUDGET_<AD>` env'i bu listeden türer; aynı bilgiyi birden çok yere yazma riski yok.

## D36 — Değişkenler sabit karakter payıyla sayılır (first_name 20, city 15) · `Kesin (kullanıcı kararı)`
- Alternatif (ilk plan): kitledeki **en uzun** değerle en kötü durum hesabı.
- Gerekçe: tek bir çok uzun isimli kullanıcı (seed'de 30 kişi, 30–45 karakter) yüzünden herkesin mesajı kısalırdı. Sabit pay hem öngörülebilir hem de modele söylenebilir (prompt'ta "`{{first_name}}` 20 karakter sayılır").
- **Garanti nasıl korunuyor:** gönderimde (`content.Render`) değer boşsa veya payı aşıyorsa kısa bir yedek sözcük yazılır ("değerli müşterimiz" 18, "bölgeniz" 8 karakter; payı aşmaması başlangıçta doğrulanır). Böylece doğrulanan uzunluk **her alıcı için gerçek üst sınırdır**. Test: kenar şablonlar, seed'in tüm 5.000 kullanıcısı için render edilir; hiçbir SMS 160'ı, hiçbir konu 80'i aşmaz ve konuda satır sonu çıkmaz.
- Config'de duruyor: `VARIABLE_BUDGET_FIRST_NAME`, `VARIABLE_BUDGET_CITY` (varsayılanlar `content.Variables`'ta); payın yedek sözcükten küçük olması başlangıçta hata verir.
- Bedeli: payı aşan isimli ~30 kişi "Merhaba değerli müşterimiz" alır (isim yerine). İsmin kesilmesi (yarım isim) daha kötü olurdu.

## D37 — SMS 160 ve konu 80 asla aşılmaz; konu tek satır; aynı kural her yerde · `Kesin (kullanıcı kararı)`
- Üç giriş noktası, tek doğrulayıcı (`content.Policy.Validate`): (1) LLM çıktısı → aşarsa hata metni modele geri verilir (repair loop, 3 deneme); (2) 3 denemede olmazsa **limite uyan** hazır şablon (ve şablon başlangıçta ayrıca doğrulanır; pay ayarları şablonu bozacak kadar büyükse değişkensiz şablon döner); (3) elle düzenleme (UI'da canlı + `PUT` sunucuda): geçersizse **reddedilir, asla sessizce kesilmez**. Aşama 4'te onayda tekrar.
- E-posta konusunda satır sonu yasak (`\n`, `\r`, NEL, U+2028/2029); sonda kalan boşluk/satır sonu kırpılır, ortadakiler hata. SMS ve gövdede satır sonu serbest. Kontrol karakterleri her yerde yasak.
- Karakter = Unicode karakter (rune), bayt değil (`ş` 1 sayılır); UI aynı kuralı code point ile uygular.
- Hata yolları ayrımı: model reddederse (`refusal`) veya servise ulaşılamazsa → fallback şablon + sebebi UI'a söylenir; **API anahtarı hatalı** ise fallback'e saklanmaz, açık hata verilir (yanlış yapılandırmayı gizlemek tehlikeli).

## D38 — SSE istemcisi elle yazıldı (kütüphane yok) · `Kesin (kullanıcı kararı)`
- `EventSource` yalnız GET'tir; mesaj üretimi POST. `@microsoft/fetch-event-source` yerine ~40 satırlık `SSEParser` + `fetch` okuyucu: yeni bağımlılık yok, her chunk sınırında doğru çalıştığı testle kanıtlı (tüm bölme noktaları, bayt bayt, CRLF), UTF-8 çok baytlı karakterler `TextDecoder({stream:true})` ile korunur.
- Sunucu tarafında başlıklar ilk olaydan önce **tembel** yazılır: akış başlamadan oluşan hata (ör. kampanya yok, durum uygun değil) normal JSON + doğru HTTP kodu olur; akış başladıktan sonraki hata `error` olayıdır.

## D39 — Refusal için sunucu tarafı fallback beta'sı kullanılmıyor · `Kesin (kullanıcı kararı)`
- `stop_reason: "refusal"` açıkça ele alınır (`RefusalError`): segmentte `llm_refused` (422), mesajda fallback şablon. Model değiştirerek yeniden deneyen `server-side-fallback` beta'sı eklenmedi (ek beta bağımlılığı, kapsam dışı). İstenirse ileride eklenir.

## D40 — `deliveries` tablosu ve `UNIQUE (campaign_id, user_id, channel)` · `Kesin (kullanıcı kararı)`
- Kuyruk tablosu (önceki planlardaki `messages`) migration `00002`'de, sınırlamasıyla birlikte yaratıldı: aynı kampanyada aynı kişiye aynı kanaldan **ikinci satır veritabanı seviyesinde imkânsız**. Kuyruğa alma `INSERT … ON CONFLICT DO NOTHING`; onay iki kez (hatta eşzamanlı) çalışsa da çift gönderim oluşmaz (D11'in 1. katmanı). Testle kanıtlandığı yer Aşama 4'tür (tablo bu aşamada henüz kullanılmıyor, şema gözden geçirilsin diye öne alındı).
- `deliveries.id` aynı zamanda provider'a giden `Idempotency-Key`'dir: tek satır = tek anahtar.
- İsim: `messages` içerik (mesaj metni) ile karışıyordu; `deliveries` "bir kişiye bir kanaldan gönderim" demek.

## D41 — Gerçek SMS'te Türkçe karakter limiti 70'tir; biz CASE'e uyup 160 karakter sayıyoruz · `Not`
- Türkçe karakterli (ş, ğ, ı, İ …) bir SMS gerçek ağlarda UCS-2 kodlanır ve tek mesaj 70 karakterle sınırlıdır; CASE.md 160 dediği için karakter sayıyoruz (bayt/segment hesabı yok). README'de de not edilecek.

## D42 — Go 1.25 · `Önerildi`
- Anthropic Go SDK v2 `go >= 1.25` ister. `golang:1.25-alpine` ile derlenir; compose/Dockerfile güncellendi, host'ta Go gerekmez (Docker).

## D43 — Mock LLM: anahtar kelimeyle çalışan "akıllı" mod + senaryo modu; LLM'e kullanıcı verisi gitmez · `Önerildi`
- Varsayılan mock, brief'i Türkçe anahtar kelimelerle (şehir, "son N gün", sepet, "satın almayan", "%30") okuyup gerçek modelle aynı JSON'u üretir ve metni parça parça akıtır (`MOCK_LLM_DELAY_MS`); belirsiz brief'e `needs_clarification`, yaş/cinsiyet gibi desteklenmeyene `unsupported` döner. Çıktıları gerçek doğrulayıcılardan geçmek zorundadır (testle kanıtlı).
- Test için senaryo modu: bozuk JSON, kısaltılmış (`max_tokens`), reddetme, API hatası, akış kopması. Mock çağrıları kaydeder; testler "modele hangi geri bildirim gitti" sorusunu doğrular.
- Gizlilik: LLM'e yalnız brief, şema, şehir listesi ve değişken payları gider; **hiçbir kullanıcı kaydı gitmez**. Brief `<brief>` etiketleri arasında "güvenilmeyen veri" olarak sunulur (`</brief>` kaçışı temizlenir); çıktısı yine doğrulanır.
- Her deneme (geçerli/geçersiz, gerçek/mock) `llm_generations` tablosuna yazılır: ham çıktı (≤8 KB), neden reddedildi, token, süre.
- **Güncelleme (D44–D49):** mock artık kendi regex'lerini taşımıyor; segment tarafında `brief.Analyze` ile "kusursuz model" gibi davranıyor, mesaj tarafında teklifi yazıyor ve kelime tekrarı üretmiyor.

## D44 — Brief'teki hiçbir koşul sessizce atlanmaz · `Kesin (kullanıcı kararı)`
- **Kural:** Brief'te geçen bir kitle koşulu ya kurala girer, ya da cevap `needs_clarification` / `unsupported` olur ve ekranda *hangi ifadenin* neden uygulanamadığı yazar. "Anlamadım ama devam ettim" yoktur.
- **Olay:** "…sepete ekleme yapmış ancak sipariş oluşturmamış…" brief'inde "sipariş oluşturmamış" kuralda yoktu; kitle 54 çıktı, doğrusu 50'ydi (sepete ekleyip sonra alışveriş yapmış 4 kişi de gidecekti). Kök neden: mock'un "almamış" sözlüğünde bu ifade yoktu ve eşleşmeyen ifade hiçbir hata üretmeden düşüyordu. Gerçek Claude'da da aynı açık vardı: `Parse` yalnızca kuralın *kendi içinde* geçerli olup olmadığına bakar, kuralın brief'i *karşılayıp karşılamadığına* bakmaz; prompt'ta "kısmen ifade edilebilir" durumu tanımsızdı.
- **Üç katman (`internal/brief`):**
  1. `Analyze`: brief'i kodla okur (şehir/hariç, zaman penceresi, aldı/almadı, teklif). Veritabanında olmayan (yaş, cinsiyet, ürün kategorisi/marka, doğum günü, üyelik/kayıt tarihi, e-posta etkileşimi) ya da belirsiz (takvime bağlı "bu ay/geçen hafta", saat/dakika, birden fazla süre, tutar eşiği, çelişen alma/almama, kodu verilmemiş kupon, tanımadığımız "…mamış/…meyen" ifadesi) bir şey görürse cevabı **kod verir ve modele sorulmaz**. Sebep: model "yardımcı olmak için" zor kısmı atlayan taraftır; atlayacağı şeyi ona hiç göstermiyoruz.
  2. `Verify`: model cevap verirse kural brief'teki her parçayı içermeli (şehirler, pencere, satın almama, teklif); eksik, yanlış değerli ya da brief'te olmayan (uydurma) parça için hata mesajı modele geri verilir (repair, 3 deneme).
  3. Model düzeltemezse sonuç sessiz bir kitle değil, neyin uygulanamadığını yazan `needs_clarification` sorusudur.
- **Alternatifler:** (a) yalnız prompt'a "atlama" yazmak: model uyarsa bile kanıt yok, test edilemez. (b) Modele ayrıca "brief'in neresini kullanmadın?" diye sormak: ikinci bir güvenilmez çıktı. (c) Her şeyi sözlüğe bırakmak: sözlük sonlu; bu yüzden (2) ve prompt kuralı birlikte çalışır.
- **Bedeli / sınırı (dürüst):** Sözlük yalnız bildiği ifadeleri yakalar. Sözlükte olmayan bir kısıt (ör. "VIP müşteriler") mock'ta sessizce görmezden gelinebilir; gerçek Claude'da prompt kuralı ve `unsupported` yargısı devreye girer ama kod garantisi yoktur. Bilinmeyen "…mamış/…meyen" ifadeleri sorulur, bu da nadiren yanlış alarm (ör. "kaçırmamış") üretebilir; kabul edildi: yanlış alarm bir soru, sessiz atlama yanlış kampanyadır.
- **Denetim:** Kodla karara bağlanan brief'lerde model çağrılmadığı için `llm_generations` satırı oluşmaz; sonuç `campaigns.segment_status/explanation/question` alanlarında durur.

## D45 — Teklif (`offer`) segment adımında çıkarılır ve kuralla birlikte saklanır · `Kesin (kullanıcı kararı)`
- Şema: `offer {kind: none|percent|amount|free_shipping|coupon_code, value, code}`. `segment_rule` jsonb içinde durur (migration yok; eski kampanyalarda alan yoksa `none`).
- Brief'te teklif birden fazlaysa (`%15` ve ücretsiz kargo, ya da iki farklı yüzde) veya geçersizse (`%150`) tahmin edilmez: `needs_clarification`. `%` ve `yüzde` yazımları, "yüzde on beş", "100 TL", "1.000 TL", `₺`, "ücretsiz kargo", kupon kodu desteklenir. Kod yalnız "kod/kupon" kelimesine bitişik BÜYÜK HARF ya da rakamlı bir sözcükse alınır; "kod" geçip kod yoksa sorulur. Teklif `Verify` ile brief'ten bağımsız doğrulanır (model "unuttu" veya "uydurdu" ise düzeltmeye girer).
- Takvime bağlı süreler ("bu ay", "geçen hafta") **desteklenmiyor**: kuralda yalnız kayan pencere var; sorulur. "bu ay sonuna kadar geçerli" gibi bir son-kullanma ifadesi segment koşulu sayılmaz.
- "Ödemeyi tamamlamamış": ödeme adımı verimizde ayrı olay değil; "sepete ekleyip satın almamış" olarak yorumlanır ve açıklamada yazılır. "Son N günde sipariş vermemiş" (sepetsiz) önce **unsupported** dönüyordu; artık destekleniyor (D50).

## D46 — Mesaj doğrulaması: teklif ve kelime tekrarı · `Kesin (kullanıcı kararı)`
- `content.Policy.Quality(content, offer)`: (1) SMS teklifi aynen yazmalı; e-posta konusu veya metni yazmalı; (2) brief'teki dışında bir yüzde yazılamaz, teklif yoksa hiç yüzde yazılamaz (uydurma indirim); (3) aynı cümlede birbirine ≤2 kelime mesafedeki iki sözcük aynı kelime ya da aynı kök olamaz ("özel fırsat fırsatından").
- **Model çıktısı için sert:** sorunlar repair geri bildirimi olur (`validateMessages`), 3 denemede düzelmezse şablon. **Elle düzenleme için uyarı:** kişi metni değiştirebilir (son karar insanda), engellenmez; `report.warnings` ile ekranda sarı uyarı çıkar. `Report.OK` yalnız sert kurallara bağlı kalır (160/80 vb.).
- Şablon fallback'i de teklifi yazar (`Fallback(offer)`); yoksa model başarısız olduğunda indirim sessizce kaybolurdu. Test: her teklif türü için şablon hem `Validate` hem `Quality` geçer.
- Kelime tekrarı sezgiseldir: aynı cümlede, ≤2 kelime mesafede, ≥5 harflik ortak kök (kısa kelimeler karşılaştırılmaz). Bilinen sınır: sıradan bir tekrar da yakalanabilir; sonuç yalnız bir yeniden yazım turudur.

## D47 — Test seti tablo tabanlı, önce kırmızı · `Kesin (kullanıcı kararı)`
- `internal/brief/brief_test.go`: her satır = brief, beklenen koşullar, beklenen teklif, beklenen durum (ok / needs_clarification / unsupported); ok olmayan satırlarda ekranda görünen metin ilgili ifadeyi anmalı. Ayrıca modelin bir koşulu/teklifi düşürdüğü senaryolar (scripted mock) ve "model hiç düzeltemezse soru" testi.
- `content/quality_test.go`, `offer/offer_test.go`, `campaign/offer_test.go` (gerçek DB: raporlanan brief'in kitlesi doğrudan SQL ile eşleşir), `llm/offer_messages_test.go`.
- Düzeltmeden önce testler gerçekten kırmızıydı (raporlanan brief: kitle 54, beklenen 50).

## D48 — `brief` paketinde sözlük tek yerde; mock ve doğrulayıcı aynı okuyucuyu kullanır · `Kesin`
- Mock'un kendi regex'leri kaldırıldı. Bu bilinçli bir takas: mock artık "kusursuz model"dir, bu yüzden tablo testi mock'u değil sözlüğü ve doğrulayıcıyı sınar; modelin hata yolları scripted `Raw` adımlarıyla ayrıca denenir.
- Ortak yardımcılar: `trtext.Fold` (Türkçe katlama, tek yerde), `offer` paketi (tür, doğrulama, metinde arama).

## D49 — Yeni şablon/prompt kuralları gerçek Claude'a da gider · `Önerildi`
- Segment prompt'u: `offer` alanının nasıl doldurulacağı ve "brief'in hiçbir kısmını sessizce atlama; ifade edemiyorsan unsupported/needs_clarification" kuralı. Mesaj prompt'u: teklifin SMS ve e-postada aynen yazılması, başka indirim sayısı yazılmaması, kelime tekrarı yasağı; kullanıcı mesajında teklif açıkça verilir. Bu iki prompt gerçek modelde `make test-live` ile denenmedi (key yok); mock ve doğrulayıcı katmanı test edildi.

## D50 — "Son N günde sipariş vermemiş": hiç siparişi olmayan da koşulu sağlar · `Kesin (kullanıcı kararı)`
- **Sorun:** "Son 30 günde sipariş vermemiş" = hiç sipariş vermemişler ∪ son siparişi 30 günden eski olanlar. `days_since_last_order gt 30` NULL zaman damgasında hiç eşleşmediği için hiç sipariş vermemişleri **sessizce düşürürdü** (D44'ün yasakladığı şey).
- **Çözüm:** yeni alan `days_since_last_order_or_never` (yalnız `gt`, `gte`). SQL: `(last_order_at IS NULL OR last_order_at < $1)`. `lte/lt/eq` bu alanda anlamsız (NULL asla "N gün içinde" olamaz), bu yüzden reddedilir. "Son 30 günde sipariş vermemiş" = `gt 30`; "son 30 günde sipariş vermiş" (`days_since_last_order lte 30`) ile tüm kullanıcıları ayrık biçimde böler (testte 5000 = ikisinin toplamı).
- **Alternatifler:** (a) `match: any` + `order_count eq 0` ∪ `days_since_last_order gt N`: `any` diğer koşullarla (şehir) birleşemez, çünkü DSL düz ve tek `match` var (şehir de OR'a girerdi); yanlış kitle. (b) İç içe grup: D34'ü bozar, structured outputs şeması özyinelemeyi desteklemez. (c) Tek alan: şema, derleyici ve doğrulayıcıya bir satır; seçildi.
- **Koruma:** model `days_since_last_order gt 30` ya da `order_count eq 0` yazarsa `Verify` hatayı verip düzeltmeye gönderir (testli: bu "benzer görünen" yanlış kural kabul edilmez). Sepet anılan brief'ler (`sepete ekleyip sipariş vermemiş`) eskisi gibi `purchased_since_last_cart_add eq false` kullanır.
- Sözlük genişledi: "satın alma yapmamış / alım yapmamış" ve "satın alma yapan" kalıpları da tanınır.
- Test: `brief_test` tablosu (7 yeni satır), `TestNoOrderInWindowRejectsTheLookalikes`, `segment` derleme/ayrıştırma satırları, gerçek DB'de `TestNoOrderInWindowIncludesPeopleWhoNeverOrdered` (doğrudan SQL ile eşleşir; önce kırmızıydı).

## D51 — Bilinen sınır: yazım hatası "satın almama" koşulunu düşürebilir · `Açık iş (kullanıcı kararı: şimdi çözülmüyor)`
- **Gözlem (gerçek Claude ile):** brief'te yazım hatası olduğunda (ör. "oluşturmmaış") `purchased_since_last_cart_add eq false` koşulu yine sessizce düştü.
- **Neden korumadı:** `Analyze` yazım hatalı sözcüğü ne "bilinen negasyon" ne de `…mamış/…meyen` kalıbı olarak yakalar ("oluşturmmaış" kalıba uymaz), dolayısıyla brief'te satın almama ifadesi hiç görülmez. `Verify` yalnız `Analyze`'ın gördüğü ifadeleri zorunlu kılar, o yüzden model koşulu düşürse de itiraz edilmez. Bu, D44'ün "sözlük sonlu" sınırının bir örneği.
- **Karar:** şimdi çözülmüyor; PLAN.md "Açık işler"e ve README "Bilinen sınırlamalar"a yazıldı. Olası çözümler (yapılmadı): yazım toleranslı eşleme (bulanık/Levenshtein), ya da modelin kendi çıkardığı "brief'te gördüğüm koşullar" listesiyle kuralı çapraz doğrulama.

---

## Açık / onay bekleyenler
- Yazım hatalı "satın almama" ifadesi (D51): bilinen sınır, çözülecek.
- Başka yok. D3, D19, D20, D21, D24 ve PLAN §9 varsayımları kullanıcı tarafından onaylandı.
