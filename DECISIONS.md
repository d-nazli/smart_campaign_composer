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
  1. `Analyze`: brief'i kodla okur (şehir/hariç, zaman penceresi, aldı/almadı, teklif). Veritabanında olmayan (yaş, cinsiyet [D83 ile destekleniyor], ürün kategorisi/marka, doğum günü, üyelik/kayıt tarihi, e-posta etkileşimi) ya da belirsiz (takvime bağlı "bu ay/geçen hafta", saat/dakika, birden fazla süre, tutar eşiği, çelişen alma/almama, kodu verilmemiş kupon, tanımadığımız "…mamış/…meyen" ifadesi) bir şey görürse cevabı **kod verir ve modele sorulmaz**. Sebep: model "yardımcı olmak için" zor kısmı atlayan taraftır; atlayacağı şeyi ona hiç göstermiyoruz.
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

## D52 — Onay: tek transaction, CAS, saklı kural yeniden ayrıştırılır · `Kesin (kullanıcı kararı + uygulama)`
- `POST /api/campaigns/{id}/approve`: `UPDATE … SET status='approved' WHERE status='content_ready'` (CAS) + kayıtlı metnin yeniden doğrulanması + kitlenin o anki zamanla yeniden sayılması + kanal başına `INSERT…SELECT … ON CONFLICT DO NOTHING`, hepsi tek TX. Eşzamanlı ikinci çağrı satır kilidinde bekler, kaybeder, aynı kuyruk sayılarını `already_approved: true` ile döner. Test: 8 eşzamanlı onay → tek kazanan, satırlar bir kez.
- **Test sırasında bulunan hata:** veritabanından okunan kural JSON'dan geri gelince sayılar `float64` oluyordu, derleyici `int64` bekliyor. Çözüm: saklı kural `segment.Parse` ile yeniden doğrulanıp kanonikleştirilir. Bu aynı zamanda DB'den gelen verinin doğrulamasız SQL'e gitmemesi demektir.
- Onay anındaki kitle ekrandakinden farklı olabilir ("son 30 gün" kayar, veri değişir). Yanıtta gerçek kuyruk sayısı döner; arayüz fark varsa söyler (`approvalDifference`). Eski sayı sessizce gösterilmez.
- Elle düzenlemeden gelen *uyarılar* (D46) onayı engellemez; onay yanıtında ve onay kutusunda tekrar gösterilir. Yalnız sert limit ihlali (160/80…) engeller ve hiçbir şeyi değiştirmeden 422 döner.
- Onaydan sonra metin dondurulur: `PUT content` ve yeniden üretim 409. Sebep: aynı `Idempotency-Key` ile giden yeniden denemenin aynı metni taşıması gerekir (provider parmak izi tutar).
- Migration `00003`: `campaigns.approved_at`.

## D53 — Kuyruk (`internal/queue`) · `Önerildi → uygulandı`
- Claim: `FOR UPDATE SKIP LOCKED` + kilit süresi (lease); süresi dolmuş `sending` satırı yeniden alınır. Zaman SQL'e parametredir (testler sahte saatle). İlk claim kampanyayı `approved→sending` yapar.
- Sonuç yazan metotlar (`MarkSent/Retry/Fail/Skip`) yalnız `status='sending'` satırı değiştirir: lease'i kaybetmiş bir worker'ın geç cevabı final durumu ezemez.
- Çöken worker'ın satırı yeniden alınırken **deneme hakkı harcanmaz** (suçlu provider değil); aynı `Idempotency-Key` ile yeniden gidince provider kayıttan cevap verir (testte: tek mesaj, 1 replay).
- `Finish`: bekleyen/uçan satırı kalmayan kampanya `completed` olur; alıcısı hiç olmayan onay da (yoksa sonsuza dek `approved` kalırdı).
- **Migration `00004`: `deliveries.retries`, `deliveries.rate_limited`.** Sayaçlar süreç belleğinde değil satırda tutulur; Aşama 5'te api `/metrics` için worker'a bağlanmadan "tekrar denenen" ve "429 sayısı"nı DB'den okur, yeniden başlatmada kaybolmaz.

## D54 — Retry politikası ve pacer · `Kesin (kullanıcı kararı + uygulama)`
- Saf fonksiyon `RetryPolicy.Decide`: 2xx→sent; **429→`Retry-After` kadar bekle, deneme harcanmaz**; 5xx/408/zaman aşımı/bağlantı hatası→en fazla 5 başarısız deneme, `min(60 sn, 2 sn·2ⁿ)` **full jitter**; diğer 4xx (ör. `invalid_recipient`)→kalıcı `failed`, retry yok. Teslimat 10 dk'dan eskiyse (veya beklemek sınırı aşacaksa) retry edilmez, `failed` + "too old".
- Pacer: kanal başına tek, **varsayılan 90/sn** (`WORKER_RATE_PER_SEC`). Aralık yukarı yuvarlanır, bu yüzden gerçek hız hiçbir zaman ayarı geçmez; boşta kalan zaman "patlama kredisi" biriktirmez. Test: hiçbir 1 sn penceresi hızı aşmaz (1, 7, 90, 100, 333/sn); gerçek saatle provider 100/sn limitini uygularken 300 mesaj hiç 429 görmeden gider.
- **Bedeli (dürüst):** 429 alınca *tüm kanal* `Retry-After` kadar durur. Provider'ın varsayılan kaosu %3 rastgele 429 enjekte ettiği için bu, kanal hızını düşürür: canlı denemede kanal başına yaklaşık 10–20 mesaj/sn, 1.700 mesaj ≈ 80 sn. Yalnız o mesajı bekletmek daha hızlı olurdu ama gerçek bir hız sınırında diğer isteklerin de 429 yemesine yol açar. Hızlı demo için `.env`'de `PROVIDER_429_RATE=0` yeterli.

## D55 — Worker davranışı · `Önerildi → uygulandı`
- İzin ve iletişim bilgisi gönderimden hemen önce yeniden kontrol edilir (onaydan sonra izni çeken `skipped`, sebebi `last_error`'da). Test: izni çekilen 10 kişiye hiç istek gitmez.
- Metin gönderim anında alıcıya göre render edilir (uzun isimler yedek sözcüğe düşer, SMS ≤160 garantisi). Test: tüm istekler için SMS ≤160, konu ≤80 tek satır, kalmış `{{…}}` yok, teklif mesajda var.
- `Idempotency-Key` = delivery id. Kapanışta uçuştaki gönderimlerin sonucu kendi süresiyle (5 sn) yazılır; kapanış yüzünden yarıda kesilen çağrı "başarısız deneme" sayılmaz, lease dolunca yeniden alınır.
- Bir `RunOnce` (test/tek seferlik) ve sürekli `Run` (kanal başına claimer + `WORKER_CONCURRENCY` gönderici) aynı `process` kodunu kullanır.
- Ayarlar: `WORKER_RATE_PER_SEC` (90), `WORKER_CONCURRENCY` (20), `WORKER_MAX_ATTEMPTS` (5), `WORKER_REQUEST_TIMEOUT_MS` (5000; provider'ın 10 sn "asılma" kaosundan kısa).

## D56 — Arayüz: onay kutusu ve 1 sn polling · `Önerildi → uygulandı`
- "Onayla ve gönder" iki adımlı (sayılar + uyarılar gösterilir, "Evet, gönder"). Kaydedilmemiş düzenleme varken onay kapalı. Onaydan sonra editör salt okunur, kitle/metin kilitli.
- Gönderim ilerlemesi (kanal başına gönderildi / başarısız / atlandı / bekliyor) her saniye `GET /api/campaigns/{id}` ile güncellenir; `completed` olunca durur. CASE'in Aşama 5 maddesi "canlı akış (SSE ya da polling)" bu polling ile zaten karşılanıyor; Aşama 5'te bu ekran yeniden kullanılır.

## D57 — `/metrics`: veritabanından okunur, el yazımı Prometheus formatı · `Kesin (kullanıcı kararı + uygulama)`
- Api ve worker ayrı süreçler. Sayaçları worker belleğinde tutup ayrıca bir uç açmak yerine her scrape'te mevcut tablolardan (`deliveries`, `llm_generations`, `campaigns`) hesaplanır: tek doğruluk kaynağı, yeniden başlatmada kaybolmaz, api worker'a bağlanmaz. Bedeli: her scrape 3 toplulaştırma sorgusu (bu ölçekte ms; çok büyürse `deliveries` için özet tablo gerekir).
- Biçim el yazımı (bağımlılık yok); testte ayrı bir doğrulayıcı HELP/TYPE sırasını, tekrarlı seriyi ve satır sözdizimini denetler. Etiketler: `channel` (sms/email), `kind` (segment/messages), `status`.
- **Tüm seriler sıfırdan başlar** (hiç olay yokken bile görünür): `rate()` ve panolar ilk scrape'ten çalışsın diye.
- Veritabanı okunamazsa scrape **503** verir; sıfırlarla dolu bir sayfa "her şey sakin" gibi görünürdü.
- "Saniyedeki gönderim hızı" = son 10 sn'de `sent_at`'i olan teslimat / 10 (gauge). Sayaçlardan `rate()` ile de hesaplanabilir. Süreler **sum/count** (summary) olarak verilir; histogram/yüzdelik yok (kabul edilen sınır).
- LLM: "üretim" = bir ilk deneme (`attempt=1`) ve onun düzeltmeleri; "repair denemesi" = `attempt>1` satırları. `needs_clarification`/`unsupported` sayıları `campaigns.segment_status`'tan gelir, çünkü bunların çoğu kodla karara bağlanır ve hiç LLM satırı üretmez (D44).
- **Fallback ve refusal artık açıkça kaydedilir** (migration `00005`: `llm_generations.refused`, `fell_back`). Sezgiyle ("son deneme geçersizse fallback") saymak zaman aşımı gibi durumlarda yanlış sayardı; test: fallback yalnızca ilgili üretimin son denemesinde işaretlenir, sonraki başarılı üretim işareti taşımaz. Bilinen sınır: eski satırlarda (migration öncesi) bu bayraklar yoktur.
- Mock LLM token harcamaz; token sayaçları yalnız gerçek Claude ile artar.

## D58 — Provider yanıt süresi satıra yazılır · `Kesin (uygulama)`
- `deliveries.provider_calls/provider_micros` (migration `00005`); worker her provider çağrısından sonra `queue.Observe` ile ekler. Sonuç yazımından ayrı tutuldu: sonuç kaybolsa (çökme) ölçüm kaybolmaz. Retry'lar dahil her çağrı sayılır (bu yüzden `_count` gönderilen sayısından büyük olabilir). Süre worker'ın `Clock`'uyla ölçülür (testte sahte saatle kesin).
- Bedeli: gönderim başına bir ek `UPDATE` (180/sn civarı; bu ölçekte sorun değil).

## D59 — api `/healthz` veritabanını kontrol eder · `Kesin (kullanıcı kararı)`
- Api'de `/healthz` artık DB ping'i yapar (2 sn zaman aşımı; 200 `ok` / 503 `unavailable`); `/readyz` aynı kontrolle duruyor. docker-compose'daki api healthcheck'i `/healthz`'a geçti. Worker'ın `:9091/healthz`'ı da aynı `httpx.HealthzWith(pool.Ping)` ile DB'yi kontrol eder (kullanıcı kararı); canlı denemede DB açıkken 200, DB durdurulunca 503 döndü.

## D60 — Dashboard: sade, polling, hash ile sekme · `Kesin (kullanıcı kararı)`
- `GET /api/campaigns?limit=` (varsayılan 50, en çok 200; sınır dışı değer kıstırılır, sayı olmayan 400). Her satır: durum, kitle, onaylıysa kanal bazlı ilerleme + `retries`/`rate_limited`. Tek sorguyla tüm onaylı kampanyaların sayımı gelir (kampanya başına sorgu yok).
- Arayüz: router bağımlılığı yok, `#/dashboard` hash'i ile iki sekme; 2 sn polling; hata olursa son liste "eski olabilir" uyarısıyla kalır. Bilinçli olarak sade: arayüz sonraki bir aşamada baştan tasarlanacak.
- Taslaklar nedenini söyler ("Desteklenmiyor", "Soru bekliyor"), "Taslak" demekle yetinilmez (D44 ile uyumlu).

## D61 — `-race` ayrı hedef · `Kesin (kullanıcı kararı)`
- `make test-race` / `backend-test-race`: Debian tabanlı `golang:1.25` aşaması (`CGO_ENABLED=1`); mevcut Alpine imajında C derleyicisi yok. `make test`'e dahil değil (≈2 dk, instrumentation yüzünden yavaş).
- Sonuç: tüm paketler (kuyruk, worker, eşzamanlı onay, pacer dahil) temiz. Dedektörün bu kurulumda gerçekten çalıştığı, kasıtlı yarışlı bir programın yakalanmasıyla doğrulandı. Kuyruk/worker testleri çoğunlukla `Concurrency=1` ile deterministik; gerçek eşzamanlılığı gerçek saatle çalışan 20 göndericili test ve 8 eşzamanlı onay testi sınar.

## D62 — Teklifin şartları (`min_spend`, `expires_at`) teklife aittir, kitleye değil · `Kesin (kullanıcı kararı)`
- **Olay:** "anneler gününe özel 1000 tl üzeri alışverişe 100tl indirim kullandır son kullanım 31.12.2026" brief'inde "1000 TL üzeri" kitle koşulu sanılıp soru soruldu (mock'ta bir tutar eşiği engeli, gerçek Claude'da `total_spent` riski: eski doğrulayıcı brief'te " tl" geçmesini `total_spent` için yeterli sayıyordu). Oysa bu indirimin *şartı*: kimin mesaj alacağını değil, indirimin hangi alışverişte geçerli olduğunu söyler.
- **Model:** `offer.Offer` iki alan kazandı: `min_spend` (TL, 0 = yok) ve `expires_at` (`YYYY-MM-DD`, boş = yok). Aynı `segment_rule` jsonb içinde saklanır (eski kayıtlarda yok = yok). `Equal`, `Describe`, `Mentions`, JSON şeması ve `Validate` buna göre genişledi; `none` teklifte şart olamaz.
- **Brief'ten okuma (`brief.Analyze`):** `N TL üzeri / ve üzeri / 'den fazla / aşan`, `en az N TL'lik`, `N TL'lik` ifadeleri bir *eşik* sayılır. Eşiğin sınıfı bağlamdan belirlenir: (1) hemen ardından alışveriş sözcüğü (`alışveriş, sepet, sipariş, harcama, ürün…`) gelir ve müşteri sözcüğü gelmezse → **teklif şartı** (`min_spend`); (2) önünde/ardında `toplam`, `harcayan`, `harcaması…` varsa → **kitle** (D64); (3) `alışveriş yapan müşterilere` gibi ikisine de yorulabiliyorsa → **sorulur**. Şartın tükettiği alışveriş sözcüğü ("…üzeri **sepete** %10") ayrıca "sepet kitlesi" diye okunmaz. Eşik metni "ikinci teklif" sayılmasın diye çalışma kopyasından silinir.
- **Tarih:** `31.12.2026`, `31/12/2026`, `31-12-2026`, `2026-12-31`, `31 Aralık 2026`. D44 gereği hiçbir tarih sessizce düşmez: imkansız tarih (31.02), yılsız "31 Aralık", birden fazla farklı tarih, ve "son kullanım / geçerli / kadar / bitiş…" gibi bir işaret olmadan yazılmış tarih **sorulur**. Teklif yokken şart/tarih de sorulur ("hangi teklif için?").
- **Mesaj doğrulaması (`content.Quality`):** SMS ve e-posta (konu veya gövde) teklifin yanında alt limiti (`1000 TL`; `1.000 TL` de olur) ve son günü (`31.12.2026`, `31/12/2026`, `31 Aralık 2026`) içermeli; eksikse model repair'e girer, elle düzenlemede uyarı olur (D46). Brief'teki son günden **başka bir tarih** (ya da brief'te tarih yokken herhangi bir tarih) çelişki sayılır. Geri bildirim şartları kelimesi kelimesine yazar. Şablon fallback'i ve mock şartlı teklifler için SMS'i sıkıştırır (yoksa 160 sınırını aşardı) ve testle her teklif türünde `Validate` + `Quality` geçer.
- **Prompt:** segment prompt'u "alışveriş tutarı şartları teklife aittir, kitleye değil" der, örnekle gösterir, `1000 TL üzeri alışverişe` ifadesini `total_spent` yapmayı yasaklar; mesaj prompt'u alt limit ve `dd.mm.yyyy` biçimli son günün SMS'te ve e-postada yazılmasını ister.
- **Doğrulayıcı:** model alt limiti `total_spent` yaparsa geri bildirim "Brief'teki 1000 TL alışveriş tutarı şartı teklife aittir: offer.min_spend" der ve düzeltir (testli, 6 senaryo).
- **Bilinen sınırlar:** son kullanım tarihi *bugünle karşılaştırılmaz* (geçmiş tarihli kupon fark edilmez; testler saate bağımlı olmasın diye); yalnız TL; "bin lira", "bin TL" gibi yazımlar tanınmaz (sözlük sonlu, D44/D51); tek alt limit ve tek tarih.

## D63 — "Herkese": boş kural değil, açık `all_customers` koşulu · `Kesin (kullanıcı kararı)`
- Brief açıkça "herkese / tüm müşterilere / bütün kullanıcılara…" diyorsa kural `all_customers eq true` içerir. Bu, segment alan beyaz listesinde bir alandır (`WHERE TRUE`); boş kural yine reddedilir. **Tek başına** olmalıdır (başka koşulla birleşemez) ve yalnız `true` olabilir. İzin/iletişim filtresi her zamanki gibi sistem tarafından uygulanır: "tüm müşteri" = iletişim izni olanlar (testte tüm kullanıcı sayılır ama kuyruğa yalnız ulaşılabilirler girer).
- "Herkese" sözcüğü başka bir kitle koşuluyla (şehir, sepet, harcama…) birlikteyse yok sayılır: daha belirgin olan kazanır.
- Brief hiçbir kitle söylemiyorsa cevap **kodla** `needs_clarification` olur ve model hiç çağrılmaz: "Bu kampanya kime gönderilsin? Tüm müşterilere mi?". Anlaşılan teklif (şartlarıyla) soruyla birlikte saklanıp ekranda gösterilir.
- **Düğme:** soru kartında "Tüm müşterilere gönder". Düğme gizli bir bayrak göndermez; brief'e "Tüm müşterilere gönder." cümlesini ekleyip kampanyayı baştan kurar (brief tek doğruluk kaynağı kalır, geçmişte ve dashboard'da görünür). Öneri `Rule.Suggestion` ile (modelin değil kodun kararı) `segment_rule`'a yazılır.
- **Doğrulayıcı:** model "herkese" denilmediği halde `all_customers` uydurursa ya da "herkese" denildiği halde unutursa geri bildirimle düzeltir.
- **Bedeli (dürüst):** kodun bilmediği bir kitle sıfatı ("VIP müşterilere %10") artık modele gitmeden "kime gönderilsin?" sorusu alır; eskiden gerçek model bunu `unsupported` diye cevaplayabilirdi. Kullanıcı "Tüm müşterilere gönder"e basarsa bu, VIP'i bırakma kararını kendisinin vermesi demektir. Kısmi çözüm: harcama sözcükleri (`harcayan`, `toplam harcaması…`) bu yola düşmez (D64).

## D64 — Toplam harcama kitlesi artık destekleniyor (`total_spent`) · `Kesin (uygulama gereği)`
- Eski eşik mesajı "toplam harcaması 500 TL'den fazla" yazmayı öneriyordu, ama bu ifade hiç desteklenmiyordu (tutar "ikinci teklif" sayılıp reddediliyordu): öneri boştu. D62'nin ayrımı (şart mı, kitle mi) çalışsın diye açık toplam-harcama ifadesi gerçek bir kitle koşulu oldu: `toplam harcaması N TL üzeri / 'den fazla` → `total_spent gt N`, `N TL ve üzeri` → `gte`, `N TL altında / 'den az` → `lt`. `harcayan müşteriler` de aynı. Şehir gibi koşullarla birleşir.
- Doğrulayıcı: bu ifade varken kuralda aynı koşul olmalı; yokken `total_spent` olmamalı.
- Hâlâ sorulanlar: "1500 TL üzeri alışveriş yapan müşterilere" (tek alışveriş mi toplam mı belli değil) ve tutarsız "çok harcayan müşteriler". Sorunun örneği brief'teki sayıyı kullanır (D65).

## D65 — `needs_clarification` örnekleri brief'in kendi sayısını kullanır · `Kesin (kullanıcı kararı)`
- Tutar eşiği sorusu: "toplam harcaması **1500** TL'den fazla" (brief 1500 yazdıysa), teklif şartı örneği de aynı sayıyla.
- Saat/dakika: "son 48 saat" → örnek "son **2** gün" (saat gün'e çevrilir, en az 1); "son 3 saat" → "son 1 gün".
- Belirsiz süre içinde rakam varsa ("son 45+ gün") örnek "son **45** gün". Rakam yoksa ("son birkaç gün") genel örnek kalır; yılsız tarih sorusu brief'in kendi ifadesini kullanır ("31 Aralık" → "31 Aralık 2026").

## D66 — Test seti ve `make test-live` · `Kesin (kullanıcı kararı)`
- `brief_test.go` tablosu bu brief'i ("kime?" + anlaşılan teklif; "Tüm müşterilere gönder." eklenince `all_customers` + şartlar), "herkese %10 indirim" ve benzerlerini, şart/tarih hatalarını, tutar eşiği ve harcama kitlesi satırlarını içerir. Model hata yolları: eşiğin `total_spent` yapılması, şartların unutulması/yanlış yazılması, `all_customers` uydurulması/unutulması (scripted mock). Servis düzeyinde: gerçek DB'de "herkese" kitlesi kullanıcı sayısına ve erişilebilir sayılara eşit; onay yalnız erişilebilirleri kuyruğa alır.
- `make test-live`: `TestLiveBriefsWithTermsAndEveryone` (6 brief, tüm hat: kod okur, gerçek model cevaplar, kod doğrular) ve `TestLiveMessagesStateTheTermsOfTheOffer`.
- Mock bir modeli taklit eder: kodun kararı olan `suggestion` alanını çıktısından çıkarır (katı ayrıştırıcı model çıktısında bu alanı reddeder; test bunu yakaladı).

## D67 — Kullanıcı profili: dil, cinsiyet, doğum tarihi · `Kesin (kullanıcı kararı)`
- **Bonus başlangıcı:** "kullanıcının diline göre içerik" ve "zamanlanmış gönderim" bonusları kullanıcı isteğiyle kapsama alındı (önceki "bonuslar dışarıda" kararı bu iki bonus için kalktı; A/B ve CI hâlâ dışarıda). Üç faz: 1 profil alanları, 2 dil, 3 zamanlama.
- **Şema (`00006_user_profile.sql`):** `users.language` (`tr|en|de`, `NOT NULL DEFAULT 'tr'`, CHECK), `users.gender` (`female|male|other`, NULL = belirtmemiş), `users.birth_date` (`DATE`, NULL = belirtmemiş). Dil için varsayılan, mevcut satırların geçerli kalmasını sağlar; liste veritabanında CHECK ile de korunur (uygulama kodu atlansa bile `fr` girilemez).
- **Seed dağılımı (tam sayılar, rastgele yerleşim):** dil %60 tr / %25 en / %15 de (5000 kişide 3000/1250/750); cinsiyet %48 kadın / %48 erkek / %2 other / %2 belirtmemiş; doğum tarihi 18–75 yaş arasında düzgün dağılmış, %3'ü boş. Her alan karıştırılmış bir listeden dağıtılır (rıza alanlarındaki gibi): toplamlar şansa bırakılmaz, ama kişiler arasında şehirle, rızayla veya brief kohortuyla ilişki yoktur. Testle: örnek brief kohortunda üç dil de ve iki cinsiyet de bulunur.
- **Ayrı rastgele akış:** profil alanları kendi `rand` akışından çekilir. Böylece mevcut veri (isimler, şehirler, rızalar, siparişler) ve "örnek brief kohortu tam 300 kişi" garantisi değişmez; eski testlerin hepsi dokunulmadan geçer.
- **Kapsam:** (Güncelleme: D83 ile cinsiyet, brief açıkça isterse segment koşulu oldu.) cinsiyet ve doğum tarihi şimdilik yalnız saklanır; segment alanı ya da kişiselleştirme değişkeni değildir (istenirse `segment/fields.go` ve `content/variables.go` tek yerden genişler). Dil, Faz 2'de içerik seçimini yönetir.
- **Mevcut veritabanı:** seed "users boş değilse atla" kuralıyla çalıştığı için eski bir volume'da yeni alanlar boş kalır (dil hep `tr`). Yeniden seed: `make clean` (`docker compose down -v`).

## D68 — Diller: `tr`, `en`, `de`; tek kayıt yeri, varsayılan = referans · `Kesin (kullanıcı kararı)`
- **Liste tek yerde:** `content.Languages` (kod, İngilizce ad, kendi adı). Değişken fallback'leri, yerleşik şablonlar, mock metinleri ve `users.language` CHECK'i bu listeyle birlikte genişler; yeni dil eklemek için gereken adımlar `lang.go` başında yazılı. `/api/info` listeyi arayüze verir.
- **`tr` varsayılan ve referanstır.** Dili boş ya da bilinmeyen biri Türkçe alır. Diğer diller referansın söylediğini söylemek zorundadır (D70).
- **Şema (`00007_content_languages.sql`):** `campaign_contents` PK'si `(campaign_id, language)`; `llm_generations.language` (denetim kaydı hangi dilin denemesi olduğunu bilir); `campaigns.language_counts` (kitlenin dil dağılımı). Mevcut satırlar `tr` olur. Dil eklenmeden önce üretilmiş bir kampanya tüm diller olmadan onaylanamaz; mesajlar yeniden üretilmelidir (onay açık bir hata verir).
- **Metin sayımı:** SMS 160 / konu 80 sınırı üç dilde de karakter (rune) sayısıyla uygulanır (CASE.md'deki limitler aynen). Almanca metinler daha uzun olmaya meyillidir; bu yüzden prompt ve yerleşik şablonlar kısa tutulur.

## D69 — Üretim: önce referans, sonra diğerleri paralel; hepsi ya da hiçbiri · `Kesin (kullanıcı kararı)`
- Her kampanya **üç dilde birden** üretilir (hemen ya da zamanlı gönderimde hangi dillerin lazım olacağı onay anında bilinemeyebilir). Önce `tr` yazılır ve doğrulanır; sonra `en` ve `de` **paralel** yazılır. Çevirilere referans metin prompt'ta gösterilir; "kelimesi kelimesine çevirme, doğal yaz, aynı sayıları ve değişkenleri kullan" denir.
- Her dil kendi `ask → validate → repair` döngüsünde (en çok 3 deneme) çalışır; çevirinin ek doğrulaması referansla tutarlılıktır (D70) ve sebep modele geri bildirim olur. Denetim kaydı dil başına tutulur.
- **Hiçbir şey, tüm diller hazır olana dek saklanmaz** (tek transaction). **Bir dil üretilemezse (3 geçersiz deneme, ret, zaman aşımı) üç dil de yerleşik şablona düşer:** model metniyle şablon metni karışırsa birbirlerini çelişkili bırakabilir. Şablonlar yan yana üretilir: ya hepsi kişiselleştirilir ya hiçbiri, böylece değişkenleri de aynıdır. Bedeli: tek dil başarısızsa iyi üretilmiş diğerleri de atılır; kişi sonradan düzenleyebilir.
- Arayüze akış: SSE olaylarının hepsi `language` taşır, olaylar sunucuda tek tek (mutex ile) gönderilir. Kimlik doğrulama hatası şablonun arkasına saklanmaz (öncekiyle aynı).
- Mock: `NewScripted` betiği referans dili anlatır; diğer diller `WithMessages(dil, ...)` ile ayrıca betiklenir, yoksa akıllı mock cevap verir.

## D70 — Diller aynı şeyi söylemeli: sert doğrulama · `Kesin (kullanıcı kararı)`
- `content.Consistency`: referansla her dil karşılaştırılır. **SMS** ve **e-posta (konu+metin birlikte)** için ayrı ayrı: indirim oranları, TL tutarları, tarihler, "ücretsiz kargo" vaadi ve `{{değişkenler}}` kümeleri aynı olmalı. Küme karşılaştırması: kaç kez ve hangi sırayla geçtiği değil, **hangi değerlerin** geçtiği önemlidir.
- Sayılar dilden bağımsız okuyucularla bulunur (`%30`, `30% off`, `30 % Rabatt` hepsi 30); bu yüzden tüm dillerde sayılar, `1000 TL` ve `dd.mm.yyyy` aynı yazılır (prompt ister). Ücretsiz kargo ifadeleri üç dilde tanınır.
- **Üç yerde zorunlu:** (1) üretimde çeviri tutarsızsa model düzeltmeye gönderilir (mesajda iki dil, fark ve değerler yazar); (2) elle kayıtta (`PUT /content` `{"languages": {...}}`; tek dil gönderilse bile bütün set kontrol edilir) → 422 `invalid_content`, her sorun `language` taşır; (3) onayda bütün diller yeniden doğrulanır. Bu, **uyarı değil hata**dır; eski "teklif eksik" uyarıları (D46) olduğu gibi yumuşak kalır.
- Bir tek dili düzenleyip teklifi değiştirmek bu yüzden reddedilir; kişi teklifi değiştirmek istiyorsa üç dili birlikte kaydeder (arayüz hepsini birlikte gönderir).

## D71 — Gönderimde dil · `Kesin (uygulama)`
- Alıcının dili **gönderim anında** `users.language`'dan okunur (kuyruk satırında dil yoktur): onaydan sonra dilini değiştiren kişi yeni dilindeki metni alır. Metin `(kampanya, dil)` anahtarıyla önbelleğe alınır.
- Kişinin dilinin metni yoksa başka dile **düşülmez**; teslimat bekler ve yeniden denenir (testli). Onay bunu zaten engeller; bu, bozuk veriye karşı ikinci savunmadır.
- Ad/şehir boşsa ya da bütçeyi aşıyorsa yerine **alıcının dilindeki** sözcük yazılır ("değerli müşterimiz" / "valued customer" / "geschätzter Kunde"). Fallback sözcüklerinin hepsi bütçeye sığmak zorundadır; sığmayan bütçe başlangıçta reddedilir.
- Onay ekranı kitlenin dil dağılımını gösterir (segment adımında da); toplam ulaşılabilir kişi sayısına eşittir (testli).

## D72 — Zamanlanmış gönderim: onay yalnız kararı kaydeder, kitle gönderim anında çıkar · `Kesin (kullanıcı kararı)`
- **Akış:** `content_ready → scheduled` (onay) `→ approved` (zaman gelince, worker) `→ sending → completed`. Yan dallar: `cancelled` (kişi, başlamadan önce) ve `failed` (kampanya hiç gönderilemez). Hemen gönderimde akış **aynen** durur (`approved` doğrudan, kuyruk aynı transaction'da); iki yol aynı `enqueueAudience` fonksiyonunu kullanır, mevcut testlerin hiçbiri değişmeden geçer.
- **Neden kitle gönderim anında:** "son 30 gün", izin ve alışveriş durumu yarın bugünkünden farklıdır; onay anında sabitlenen bir liste, izni kaldırmış ya da o arada alışveriş yapmış kişiye mesaj gönderebilirdi. İzin ayrıca gönderim anında da yeniden kontrol edilir (öncekiyle aynı). Bedeli: onay ekranındaki sayı **tahmindir**; ekran bunu açıkça söyler ("kişi sayısı gönderim anında değişebilir"), kampanya başlayınca gerçek sayılar yazılır.
- **Neden yeni durum (`scheduled`):** `Finish`, "bekleyen teslimatı olmayan `approved/sending`" kampanyayı tamamlar. Zamanlı kampanyanın onayda hiç teslimatı olmadığı için `approved` kullansaydık saniyeler içinde yanlışlıkla `completed` olurdu. `scheduled` o sorgunun dışındadır.
- **Şema (`00008_scheduled_send.sql`):** `scheduled_at`, `materialized_at`, `cancelled_at`, `schedule_error`; durum CHECK'i genişledi; `status='scheduled'` için kısmi indeks.
- **Zaman sınırları:** en az 1 dakika (kimse bakamadan geçmesin), en fazla 30 gün (yanlış yıl yazımı). Tekrar onayda ilk karar korunur ve `already_approved` döner; "hemen gönder" demek zamanlanmış kampanyayı göndermez.
- **Zamanlanmış kampanya donar:** metin düzenlenemez, yeniden üretilemez, yeniden zamanlanamaz. Yanlışsa iptal edip yenisi kurulur (yeniden zamanlama bilerek yok).

## D73 — Zamanlayıcı: tek transaction, `FOR UPDATE SKIP LOCKED`, insansız · `Kesin (kullanıcı kararı)`
- Worker içinde ayrı bir döngü (`campaign.Scheduler`, `clock.Clock` ile test edilir) her saniye bir kampanya alır: `SELECT … WHERE status='scheduled' AND scheduled_at <= now ORDER BY scheduled_at LIMIT 1 FOR UPDATE SKIP LOCKED`. **Aynı transaction'da** kayıtlı kural yeniden doğrulanır (`segment.Parse`), o anın kitlesi sayılır, teslimatlar oluşturulur, sayılar ve dil dağılımı güncellenir, durum `approved` olur. Hepsi ya da hiçbiri: çökerse transaction geri alınır ve kampanya bir sonraki turda yeniden denenir; commit'ten sonra durum `approved` olduğu için ikinci kez seçilemez. `UNIQUE(campaign_id, user_id, channel)` + `ON CONFLICT DO NOTHING` ikinci savunmadır.
- Birden çok worker çalışsa da her kampanya bir kez başlar (8 eşzamanlı zamanlayıcıyla testli: toplam "işlendi" sayısı kampanya sayısına eşit).
- **Kalıcı hata** (kural bozuk, bir dil metni eksik, diller uyuşmuyor, teklif sona ermiş) → `failed` + `schedule_error`; kimse gelip bakmadan döngüye girmez. **Geçici hata** (veritabanı) → kampanya `scheduled` kalır, sonraki turda yeniden denenir.
- **Worker kapalıyken** zamanı gelen kampanya, worker açılınca hemen başlar (`scheduled_at <= now`). Teslimatların `created_at` değeri zamanlayıcının "şimdi"sidir; böylece yeniden deneme yaşı (en çok 10 dk) onaydan değil başlangıçtan sayılır ve saatler önce onaylanan kampanyanın ilk geçici hatası "çok eski" diye düşmez.

## D74 — Gönderim zamanı teklifin son gününden sonra olamaz · `Kesin (kullanıcı kararı)`
- **Kural:** `scheduled_at`, teklifin `expires_at` gününün sonundan (Türkiye saatiyle 23:59:59.999, UTC+3, sabit dilim: DST yok, tzdata gerekmez) sonra olamaz; **son günün kendisi serbesttir**. Backend reddeder (400 `invalid_input`, mesaj tarihi söyler), arayüz de aynı kuralı uygular: zaman seçici `max` ile sınırlıdır, geçersiz zaman için "Zamanla" düğmesi kapanır ve neden yazılır (`utils/schedule.ts` backend'in aynasıdır; sunucu yine karar verendir).
- **Tarayıcı saati:** `datetime-local` kişinin kendi saat dilimindedir; seçilen an UTC'ye çevrilip gönderilir, son gün ise hep Türkiye saatiyle okunur. Türkiye dışından kullananlar için sınır birkaç saat kayabilir; sunucu kuralı değişmez.
- **Gönderim anında da denetlenir:** kampanya beklerken ya da worker kapalıyken teklif sona ererse mesaj **gönderilmez**, kampanya `failed` olur ("teklifin son kullanma tarihi geçti"). Süresi dolmuş bir indirimi duyurmak, onayın zamanlamasından daha kötü bir hatadır.

## D75 — İptal · `Kesin (kullanıcı kararı)`
- `POST /api/campaigns/{id}/cancel` yalnız `status='scheduled'` iken çalışır: `UPDATE … WHERE id=$1 AND status='scheduled'`. Zamanlayıcı kampanya satırını kilitleyerek başlatır; iptal UPDATE'i o kilidi bekler ve kilit kalkınca koşulu yeniden değerlendirir. **Tam biri kazanır:** ya kampanya iptal edilir ve hiçbir teslimat oluşmaz, ya başlamıştır ve iptal 409 ile reddedilir (başladıktan sonra iptal yok). İptal kazanırsa zamanlayıcı satırı `SKIP LOCKED` ile atlar ya da `cancelled` olduğunu görür.
- Test iptali ve zamanlayıcıyı aynı anda 20 kez yarıştırır; her turda sonuç ya "iptal edildi + 0 teslimat" ya "reddedildi + başlamış" olmalıdır, başka sonuç testi düşürür. Bir çalıştırmada iki sonuç da gözlendi (18 iptal / 2 başlatma): yarış gerçekten yaşanıyor.
- İptal tekrarında hata yok (idempotent); `cancelled`/`failed` kampanya yeniden onaylanamaz. Dashboard'da ve gönderim adımında iki adımlı "İptal et" düğmesi vardır.

## D76 — Dil değişikliği ve zamanlama · `Kesin (kullanıcı kararı)`
- Teslimatlar başlangıçta oluştuğu ve dil gönderim anında okunduğu (D71) için iki durum da doğru çalışır ve testlidir: (a) kişi **zamanlamadan sonra, başlamadan önce** dilini değiştirirse kitle zaten yeni haliyle çıkar; (b) **başladıktan sonra, gönderilmeden önce** değiştirirse mesaj yine yeni dilde gider. Her iki durumda sağlayıcıya o kişiye bir SMS ve bir e-posta gider, yeni dilde, iki kez değil.
- Dil dağılımı (`language_counts`) kampanya başlarken yeniden sayılır; onay ve kampanya ekranları o anki gerçek sayıyı gösterir.

## D77 — Elle denemeden gelen düzeltmeler · `Kesin (kullanıcı kararı)`
- **"Yeni kampanya" temiz açılır.** Sekme tıklaması adresten `?c=` parametresini siler ve `ComposeView`'ı yeni anahtarla yeniden kurar (state sıfırlanır); dashboard'dan `#/` ile dönüş de aynı yolu izler. Sebep: aynı bileşen yeniden kullanılıyordu ve açılışta adresteki `?c=` eski kampanyayı yüklüyordu (`utils/route.ts`, testli).
- **Kampanya yüklenemezse adres korunur.** Yalnız 404 (kampanya yok) adresi temizler; sunucuya geçici ulaşılamama bir hata gösterir ve yenilemeyle kampanya yine açılabilir. Önceden her hata sessizce boş bir Brief açıp adresi siliyordu.
- **Dil kırılımı.** `deliveries` satırında dil tutulmaz; gönderim ekranı ve kartlar `users.language` ile birleştirip sayar (`DeliveryView.languages`: toplam ve gönderilen, SMS+e-posta birlikte). Sınırı: gönderildikten sonra dilini değiştiren kişi yeni dilinde sayılır. Gönderilirken gerçek dil satıra yazılsaydı bu da çözülürdü; şimdilik gereksiz görüldü. Gönderim sürerken "TR 120/171", bitince "TR 171"; başlamadan önce kitle dilleri "tahmini" etiketiyle.
- **Kart özeti.** Liste her kampanya için kitle cümlesini (`explanation`), kişi sayısını, kitle dillerini ve varsa planlanan zamanı verir. Zamanlanmış kampanya başladıktan sonra da zamanını korur ("Zamanlanan gönderim").
- **Görünüm.** Salt okunur alan gri, yeniden boyutlanamaz, metin imleçsiz; aktif adım dolu nokta + kalın etiket + renkli hap; dashboard bloğu ve kartları ortalı. Bu kurallar stil dosyasından okunup testle sabitlenir (görsel doğrulama yerine kuralın varlığı).

## D78 — Ortak tasarım sistemi ve yeniden düzenlenen ekranlar · `Kesin (kullanıcı kararı)`
- **Yön:** Linear/Stripe gibi sade ve ciddi: soğuk gri yapı, tek vurgu rengi (koyu mavi `#2457D6`), durum için yeşil/kehribar/kırmızı, gölge yok (kenar 1 px çizgi), iki köşe yarıçapı (kontrol 6, panel 10), tek yazı ailesi **Geist** (paketlenmiş, CDN yok), sayılar tablo hizalı. Açık ve koyu tema aynı token'lardan (`src/styles/tokens.css`).
- **Bileşenler** (`components/ui/`): Button, Card, StatTile, Badge, SegmentedControl, DateTimeField, ProgressBar, Modal; ikonlar `lucide-vue-next`. **Tarayıcının çıplak radio, tarih ve açılır kutuları hiçbir yerde kullanılmaz**; bunu `designSystem.test.ts` tüm `.vue` dosyalarını tarayarak korur. DateTimeField kendi takvimi ve saat sütunlarıdır (teklifin son günü sonrası seçilemez, "1 saat sonra / Bu akşam / Yarın" kısayolları var); tarih mantığı `utils/calendar.ts`'te saf fonksiyonlardır.
- **Tek sütun:** başlık, sihirbaz ve dashboard aynı `.container` (1040 px, `--page` token'ı) içindedir; hiçbir ekran kendi genişliğini koymaz.
- **Ekranlar:** Onay = Hemen/Zamanla anahtarı + defter satırları (teklif, kitle, alıcılar, diller) ve tek cümlelik not. Gönderim = tek ilerleme çubuğu, tek kanal tablosu, tek dil satırı (aynı sayı bir kez). Dashboard = istatistik şeridi + kampanya **tablosu** (başlık brief'in tek satıra kısaltılmış, baş harfi büyük hâli; tam metin üzerine gelince), iptal onayı **modalda**; dar ekranda satırlar bloklara döner.
- **Kampanya adı yok** (kullanıcı kararı): tablo başlığı brief'ten türetilir, ayrı bir ad alanı veya türetme kuralı yoktur.
- **Bileşen testleri:** `@vue/test-utils` + `jsdom` eklendi; her bileşen dosya başına `// @vitest-environment jsdom` ile test edilir, saf mantık Node'da kalır. Backend'e dokunulmadı.
- **Yan düzeltme:** dashboard'da reddedilen iptalin hata mesajı, hemen ardından gelen liste yenilemesi tarafından siliniyordu; artık ayrı bir alanda kalır.

## D79 — "Aynı kişiye iki kez gitmez" adres düzeyinde de geçerli · `Kesin (kullanıcı kararı)`
- **Sorun:** `UNIQUE(campaign_id, user_id, channel)` kayıt düzeyinde koruyordu. Aynı kişinin iki müşteri kaydı varsa (aynı telefon ya da aynı e-posta) adres iki mesaj alırdı; `users` tablosunda adres tekil değil (gerçek veride de çoğu zaman değildir).
- **Karar:** kuyruğa alma kanal başına **adres** başına tek teslimat oluşturur (`SELECT DISTINCT ON (adres) … ORDER BY adres, id`: en küçük id'li kayıt alıcı olur). Adres normalleştirilir: telefon `btrim(phone)`, e-posta `lower(btrim(email))` (`segment.SMSAddressSQL` / `EmailAddressSQL`, tek yerde).
- Ekrandaki "SMS / e-posta ile ulaşılabilir" sayıları da `count(DISTINCT adres)`: gösterilen sayı = kuyruğa giren sayı. "Eşleşen" ve "iki kanaldan ulaşılabilir" kayıt sayısı olarak kalır.
- Hem "hemen gönder" hem zamanlanmış başlatma aynı `enqueueAudience`'ı kullanır. Test: bir kişinin iki kopya kaydı (biri büyük harfli, boşluklu e-postayla) eklenince kuyruğa giren sayı değişmez, kişiye kanal başına bir teslimat düşer.
- **Sınır:** tekilleştirme kuyruğa alma anındaki adrese göre yapılır; worker gönderim anında kişinin güncel adresini okur (D55). Onaydan sonra iki kaydın adresi aynı hale getirilirse ikisi de gider. Kabul edildi.

## D80 — Uzunluk: en kötü durum bütçesi + göndermeden önce son ölçüm · `Kesin (kullanıcı kararı)`
- Değişkenler doğrulamada en kötü durum bütçesiyle sayılır (önceki kural, değişmedi). **Worker da aynı bütçeyi kullanır:** `VARIABLE_BUDGET_*` artık compose'da worker'a da verilir (önce yalnız api'deydi; api'de bütçe düşürülünce worker varsayılan 20 ile render eder, 18'e göre doğrulanmış SMS 160'ı aşabilirdi). Worker açılışta kullandığı bütçeleri log'a yazar.
- **Son ölçüm:** worker render ettiği metni sağlayıcıya göndermeden önce bir kez daha sayar (`content.RenderedProblem`: SMS ≤160, konu ≤80 ve tek satır, gövde ≤5000). Aşan mesaj **gönderilmez**, teslimat deneme harcamadan kalıcı `failed` olur, sebep `last_error`'da ("message too long: sms is 162 characters…"). Dashboard bunu `too_long` koduyla "Kişiselleştirme sonrası karakter sınırını aştığı için gönderilmedi" diye gösterir. Kısaltma yapılmaz: kesilmiş bir metin teklifi ya da kodu düşürebilir.
- Test: api bütçesi 18, worker bütçesi 20 iken 20 harfli isim → SMS 162 / konu 82 olurdu; sağlayıcıya hiç istek gitmez, iki teslimat `failed` + sebep; kısa isimli alıcının mesajları gider. Sınır testleri: 160/161, 80/81, satır sonu.

## D81 — Hız sınırı tek worker varsayar · `Kesin (kullanıcı kararı: kodlanmıyor)`
- Kanal başına 100/sn sınırı worker sürecindeki pacer ile uygulanır (D54, varsayılan 90/sn). Pacer süreç içidir: **iki worker çalışırsa** her biri 90/sn gönderir ve kanal toplamı 180/sn olur. Mock sağlayıcı yine 100/sn'de 429 döner, mesaj kaybolmaz ama sınır istemci tarafında artık garanti değildir.
- **Varsayım:** compose tek worker çalıştırır; ölçeklemek kapsam dışı. Çoklu worker gerekirse sınır paylaşılan bir yerde tutulmalı (ör. Postgres'te kanal başına saniyelik kota satırı ya da Redis token bucket) ve pacer oradan slot almalı. Kuyruk (`SKIP LOCKED`) ve idempotency zaten çoklu worker'a hazır; tek eksik bu.
- README'deki "worker tek replica (hız sınırı süreç içi)" notu bu kararın kısa hâlidir.

## D82 — Özel gün kitlenin değil, mesajın konusudur · `Kesin (kullanıcı kararı)`
- **Sorun:** "Kadınlar Günü'ne özel indirim" brief'i `unsupported` dönüyordu: "kadın" kelimesi cinsiyet sözlüğüne takılıyordu. "8 Mart" yılsız tarih diye soruluyordu.
- **Karar:** özel gün ifadeleri (`brief/occasion.go`: Kadınlar Günü / 8 Mart, Anneler, Babalar, Sevgililer / 14 Şubat, Öğretmenler / 24 Kasım, 23 Nisan, Yılbaşı, Black Friday, Ramazan / Kurban Bayramı, bayram) brief okunurken **ilk** bulunur ve metinden çıkarılır. Sonraki okuyucular (cinsiyet, tarih, zaman aralığı) onları görmez. "8 Mart 2027'ye kadar" yılı olduğu için son kullanım tarihidir, özel gün değildir.
- Özel günlü brief kitle yazmıyorsa **varsayılan herkes** (`all_customers`), soru sorulmaz. Kural `note` alanında "Kitle belirtilmediği için tüm müşteriler seçildi." taşır, kitle adımı bunu gösterir. Not kodla konur: modelin yazdığı not silinir, kodun koyduğu not modelden bağımsızdır. Özel gün yoksa eski kural geçerli: kitle yazılmadıysa "Kime gönderilsin?" (D63).
- Claude'a aynı kural prompt'ta söylenir. `Verify` özel günde `all_customers` yoksa ya da özel gün yüzünden `gender` eklenmişse cevabı gerekçesiyle geri gönderir (testli).

## D83 — Cinsiyet yalnız açıkça istenirse kitle koşuludur · `Kesin (kullanıcı kararı)`
- Segment beyaz listesine `gender eq female|male` eklendi (`users.gender`, D67). "other" ve boş (NULL) hiçbir cinsiyet koşuluna girmez.
- Açık kalıplar: "kadın/bayan müşteri/kullanıcı/üye…", "kadınlara", "kadınlarımıza", "kadınlar için", "erkeklere", "erkek müşterilerimize", "cinsiyeti kadın/erkek olan". "Kadın ve erkek müşterilere" herkestir. Hem kadınlara hem erkeklere ayrı ayrı hitap eden brief sorulur. Kitleyi anlatmayan cinsiyet kelimesi ("kadın koleksiyonu") koşul üretmez ve engel de olmaz.
- "Anneler Günü" anne, "Babalar Günü" baba demek değildir: ebeveynlik verisi yok. Bu yüzden ikisi de özel gün olarak herkese gider, açık cinsiyet kalıbı varsa o uygulanır.

## D84 — Birleşik teklif: farklı türden en fazla 3 parça · `Kesin (kullanıcı kararı)`
- `offer.extras: [{kind, value}]` eklendi. Ana parça `kind/value`, diğerleri `extras`. Türler `percent`, `amount`, `free_shipping`, her tür en fazla bir kez. Kupon kodu ve şartlar (`code`, `min_spend`, `expires_at`) tüm teklife aittir. `Normalize` parçaları sabit sıraya koyar (yüzde, tutar, kargo): "kargo + %20" ile "%20 + kargo" aynı tekliftir, modelin sırası önemsizdir.
- Aynı türden iki değer (%20 ve %30) ya da "ya da / veya" ile seçenek olarak verilen teklifler **çelişkidir** ve sorulur.
- Mesaj kontrolü (`Mentions`) her parçayı SMS'te ve e-postada arar. Diller arası tutarlılık zaten sayıları, kargo vaadini ve değişkenleri karşılaştırdığı için tüm parçaları kapsar. Yerleşik şablonlar da tüm parçaları yazar, sığmazsa üçüncü "en sade" biçime düşer (teklif + şartlar).

## D85 — Kademeli indirim ve indirim tavanı teklifin yapısıdır · `Kesin (kullanıcı kararı)`
- `offer.step_spend` (her N TL'de) ve `offer.max_discount` (toplam / en fazla) eklendi. "Her 1000 TL'lik alışverişe 100 TL, toplamda en fazla 300 TL" → `amount 100, step_spend 1000, max_discount 300`. Kademe ve tavan, eşik ve tarih okunmadan önce okunur. Böylece "1000 TL'lik" alt limit, "300 TL" ikinci teklif sanılmaz.
- Kurallar: kademe yalnız TL indiriminde olur, yüzdeyle birleşmez. İndirim kademe tutarından küçük olmalıdır. Tavan yüzdede "en çok şu kadar TL" demektir, kademede toplam üst sınırdır. Sabit bir TL indirimine tavan anlamsızdır, sorulur.
- Mesaj üç sayıyı da yazmak zorundadır. Ayrıca **teklifin kendi tutarları dışındaki her TL tutarı çelişkidir** (`Conflicts`). Gerçek Claude ile kademeli brief'te "Örneğin 3000 TL'lik alışverişte 300 TL" gibi uydurulmuş bir örnek hesap görüldü. Artık düzelttirilir, eski yüzde kuralının TL karşılığı budur.
- Eski kayıtlar etkilenmez: yeni alanlar boşsa 0 / boş sayılır, migration gerekmez (teklif `segment_rule` JSON'unda).

## D86 — Metin: koddan tema, mock'ta doğrulanan aday seçimi · `Kesin (kullanıcı kararı)`
- **Tema** (`brief.ThemeOf`): özel gün, konu (kitleden: sepet, geri kazanma, hoş geldin, teşekkür, genel), ton (samimi, resmi, heyecanlı, kısa, zarif, acil). Mesaj isteğinde `Theme` olarak hem Claude'a (prompt'ta tek satır, "kitleyi değil metni etkiler") hem mock'a gider.
- **Claude prompt'u:** temaya özgü başlık ve açılış, kalıp cümlelerden kaçınma, tonun anlamı, özel günde okura cinsiyet/aile varsayımıyla hitap etmeme (örnekle), birleşik teklifin her parçası, kademede üç sayı.
- **Mock:** üç dilde ifade bankaları (`llm/mock/kits.go`). Bankalar sırayla denenir: önce özel gün, sonra konu, sonra genel. Başlangıç noktası brief'in hash'idir. Backend'in kendi kontrolünden (Validate + Quality, alan bazında) geçen ilk aday seçilir. Sonuç: aynı brief aynı metni, farklı brief farklı metni alır. 160/80, teklifin her parçası, kelime tekrarı yasağı ve diller arası tutarlılık korunur. Yer kalmazsa şartlar kısa biçimle yazılır ("Kod:", "1500 TL üzeri", "Son gün …"). Bu bir şablon değil, kurallı bir arama: metin kalitesi sınırlı ama hiçbir zaman kural dışı değil.
- **Sınır:** tema sözlüğü kodda sabittir. Üslup (ör. "Kadınlar Günün kutlu olsun" demek) Claude için prompt kuralıdır, kodla denetlenmez.

## D87 — Gerçek Claude modunda brief'i Claude anlar; kod yalnız nesnel kontrol yapar · `Kesin (kullanıcı kararı)`
- **Sorun:** Kural okuyucusu (`brief.Analyze`) her iki modda da Claude'dan **önce** çalışıyor ve sözlüğünde olmayan ya da belirsiz bulduğu her brief'i durduruyordu. "1000 tl ye varan siparişte 300 tl indirim" Claude'a hiç gitmeden "aynı türden iki teklif" sorusuyla kaldı. Claude'un cevabı da yine bu okuyucuya göre (`Verify`) reddediliyordu. Dilin zenginliğini anlayan model, sınırlı bir regex sözlüğünün altında çalışıyordu. Sözlüğün tanımadığı her yazım bir engel ya da yanlış bir "düzeltme" oluyordu.
- **Karar (gerçek Claude modu, `brief.Grounded`):** brief doğrudan Claude'a gider. Kitleyi Claude çıkarır, `ok` / `needs_clarification` / `unsupported` kararını da Claude verir. `Analyze` engellemez, `Verify` çalışmaz. Kod yalnız nesnel kontrol yapar:
  1. `segment.Parse`: şema, beyaz liste, tipler ve aralıklar. Yalnız bu kural SQL'e derlenebilir.
  2. `Offer.Validate` (Parse'ın içinde): teklif kendi içinde mantıklı mı (yüzde 1–100, kademede indirim < kademe, tavan yalnız yüzdede ya da kademede…).
  3. `brief.Ground`, **dayanak** kontrolü: Claude'un yazdığı her şehir, gün sayısı, sipariş sayısı, tutar, yüzde, kupon kodu ve tarih brief'te geçiyor mu. Gün sayıları brief'in kendi biriminden çevrilebilir ("2 hafta" = 14, "bir aydır" = 30, "48 saat" = 2, "bu ay" = 30). 0 ve 1 her zaman serbesttir. Cinsiyet koşulu ancak brief'te bir cinsiyet kelimesi varsa kabul edilir; bir özel günün adı ("Kadınlar Günü") sayılmaz. Ücretsiz kargo ancak brief kargodan söz ediyorsa kabul edilir.
- Bir kontrol tutmazsa sorun Claude'a geri bildirim olarak gider (3 deneme). Yine tutmazsa netleştirme sorulur ve soru neyin tutmadığını söyler. Kitle yorumuna (ör. "almayanlar" koşulunu eklemesi gerekir miydi) kod karışmaz.
- Claude yalnız `all_customers` seçtiyse ve brief "herkese / tüm müşterilere" demiyorsa (ör. özel gün), kitle adımı "Kitle belirtilmediği için tüm müşteriler seçildi." notunu gösterir. Claude "Kime gönderilsin?" diye sorarsa "Tüm müşterilere gönder" düğmesi çıkar. İkisi de koddan gelir; arayüz değişmedi.
- **Mock modu değişmez (`brief.Strict`):** mock "kusursuz model" olarak kural okuyucusunun kendisidir. Engelleme, `Verify` ve D44 bugünkü gibi çalışır; testler ve anahtarsız demo bu yoldan geçer. Mod `cmd/api/main.go`'da seçilir: `LLM_MODE` claude ise `Grounded`, değilse `Strict`. Testler `Grounded` yolunu betikli mock'la anahtarsız sınar.
- **Bedeli (dürüst):** D44'ün koruması ("hiçbir koşul sessizce atlanmaz") gerçek Claude modunda kod garantisi olmaktan çıkar. Claude bir koşulu düşürürse kampanya o kuralla ilerler. Bunu pazarlamacı yalnız kitle adımındaki koşul rozetlerinden ve kişi sayısından görür. Kitle okuması yeniden Claude'un sorumluluğundadır (prompt kuralları yerinde). Kod artık Claude'un **uydurmasını** yakalar, **yorumunu** değil. Ayrıca brief'te birden çok sayı varsa, Claude'un yanlış sayıyı yanlış alana koymasını dayanak kontrolü yakalamaz (iki sayı da brief'te geçer).
- Mesaj adımı iki modda aynıdır: `Validate`, `Quality`/`Conflicts`, diller arası tutarlılık. Mesajın teması (`ThemeOf`) hâlâ kural okuyucusundan gelir; yalnız üslubu etkiler, kitleyi değil.

## D88 — "N TL'ye varan sipariş" alt limittir · `Kesin (kullanıcı kararı)`
- "1000 TL'ye varan/ulaşan **siparişte/alışverişte/sepete** 300 TL indirim": 1000 TL'ye ulaşan sepet, yani `min_spend 1000`. "300 TL'ye varan **indirim**" ise tavandır (`max_discount`), D85'teki gibi. Ayrım, tutarın arkasından gelen kelimeye göre yapılır. "tl ye" gibi boşluklu yazım da tanınır. "1000 TL'ye **kadar** alışverişte" alışverişin üst sınırı gibi okunduğu için sorulur (teklif modelinde üst sepet sınırı yok).
- Kural okuyucusundadır, yani mock modunda geçerlidir. Gerçek Claude modunda Claude kendisi okur; dayanak kontrolü yalnız 1000 ve 300'ün brief'te geçtiğine bakar.

## D89 — Claude modunda brief'teki her tutar ve yüzde kuralda bir yere oturmalı · `Kesin (kullanıcı kararı)`
- **Sorun:** D87'nin dayanak kontrolü tek yönlüydü (Claude'un yazdığı sayı brief'te var mı). Canlı denemede Claude "1000 tl ye varan siparişte 300 tl indirim" brief'ini düz "300 TL indirim" okudu. 1000 TL kayboldu; kuralda uydurma olmadığı için hiçbir kontrol bunu görmedi. Mesajlar alt limiti söylemeyecekti.
- **Karar:** ters yön de nesnel olarak denetlenir (`brief.UnusedValues`). Brief'teki her TL tutarı ve her yüzde, teklifte (value, extras, min_spend, step_spend, max_discount) ya da bir `total_spent` koşulunda yer almalı. Yer almıyorsa Claude'a geri bildirim gider. Geri bildirim tutarın nereye konabileceğini sayar ama hangisi olduğunu dayatmaz; kararı Claude verir. Onarımdan sonra hâlâ kullanılmıyorsa pazarlamacıya seçenekli bir soru sorulur: "Brief'teki 1000 TL ne için: minimum sepet tutarı mı, indirimin üst sınırı mı, yoksa müşterinin toplam harcaması mı?" Örnekler brief'in kendi sayısıyla yazılır.
- **Sayılmayanlar:** tarihler ("31.12.2026", "31 Aralık", "31 Mart 2027") ve özel gün ifadeleri ("23 Nisan", "8 Mart", "14 Şubat") kontrolden önce metinden çıkarılır. Gün sayıları (TL ya da % işareti taşımadıkları için) bu kontrole girmez; onlar D87'nin dayanak kontrolündedir.
- **Prompt:** minimum sepetin gündelik yazımları örneklendi ("1000 TL ve üzeri siparişlerde", "1000 TL'ye varan siparişte", "sepet tutarı 1000 TL'yi geçince", "en az 1000 TL'lik alışverişte"). "Her tutar bir yere oturmalı, değilse seçenekli sor" kuralı eklendi. Canlıda örnek brief ilk denemede `min_spend 1000` ile doğru geldi. İki belirsiz brief'te ("2000 tl 250 tl indirim" ve iki ayrı tutarlı bir teklif) Claude kendisi seçenekli soru sordu. Onarım döngüsünün bu yolu betikli testlerle sınanıyor.
- Mock modu değişmez: kural okuyucusu her tutarı zaten bir terime bağlıyor ya da soruyor.

---

## Açık / onay bekleyenler
- Yazım hatalı "satın almama" ifadesi (D51): bilinen sınır, çözülecek.
- Başka yok. D3, D19, D20, D21, D24 ve PLAN §9 varsayımları kullanıcı tarafından onaylandı.
