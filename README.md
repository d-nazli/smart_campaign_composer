# Smart Campaign Composer

Insider One AI Developer case study. Pazarlamacının doğal dille yazdığı brief'i Claude ile
hedef kitleye ve SMS/email mesajlarına çeviren, onaya sunan, kuyruk üzerinden mock provider'a
gönderen ve durumu canlı gösteren sistem.

> **Durum:** geliştirme sürüyor (Aşama 5 tamam: brief → kitle → mesajlar → onay → gönderim → dashboard/metrics; cilalama sırada). Plan: [PLAN.md](PLAN.md), kararlar: [DECISIONS.md](DECISIONS.md).
> Bu README son aşamada tamamlanacak (mimari özet, yapılmayanlar).

## Kurulum

Gereken: Docker Desktop (host'ta Go/Node gerekmez). `make` için Windows'ta `choco install make` (veya winget ile bir GNU Make paketi); `make` olmadan Makefile'daki `docker compose ...` komutları doğrudan çalıştırılabilir.

```powershell
docker compose up --build      # veya: make up
```

- Uygulama: http://localhost:8080 — **Dashboard:** http://localhost:8080/#/dashboard
- API: http://localhost:8081 — sağlık: **http://localhost:8081/healthz** (veritabanını da kontrol eder, bozuksa 503),
  metrikler: **http://localhost:8081/metrics** (Prometheus formatı), ayrıca `/readyz`, `/api/info`
- Mock provider: http://localhost:8090

> ### ⚡ Hızlı demo için
> Mock provider varsayılan olarak **kasıtlı olarak bozuktur** (%5 5xx, %3 rastgele 429, %1 zaman aşımı, %2 "kabul ettim ama
> hata döndüm"). Her 429 gönderimi 1–3 sn durdurduğu için bir kampanya **1–2 dakika** sürer. Anında bitmesini istiyorsanız
> `.env` dosyasına **`PROVIDER_429_RATE=0`** yazıp `docker compose up -d mock-provider` çalıştırın (tüm kaosu kapatmak için
> `PROVIDER_ERROR_RATE=0 PROVIDER_TIMEOUT_RATE=0 PROVIDER_FAIL_AFTER_ACCEPT_RATE=0` da ekleyin).

`.env` zorunlu değil; ayarlar için `copy .env.example .env`. `ANTHROPIC_API_KEY` boşsa mock LLM,
doluysa gerçek Claude kullanılır (`LLM_MODE=auto`).

```powershell
make test        # tüm testler, Docker içinde, API key/internet gerektirmez (her zaman mock LLM)
make test-race   # backend testleri Go race detector ile (Docker, CGO); yavaş (~2 dk), `make test`'e dahil değil
make dev         # geliştirme: compose watch + air + Vite HMR
make clean       # her şeyi durdur ve veritabanı volume'unu sil (seed baştan yüklenir)
```

## LLM modu (mock / Claude)

- `ANTHROPIC_API_KEY` boş → **mock LLM** (varsayılan; anahtar gerekmez). Anahtarı `.env`'ye yazıp
  `docker compose up -d api` ile yeniden başlatırsanız kod değişikliği olmadan **gerçek Claude** kullanılır
  (`CLAUDE_MODEL`, varsayılan `claude-sonnet-5-5`). Aktif mod açılış logunda ve arayüzün sağ üstündeki rozette görünür.
- `make test` **her zaman mock** ile çalışır; `.env`'de anahtar olsa bile okunmaz, internet gerekmez.
- `make test-live` gerçek Claude'a duman testi yapar (yalnız anahtar varsa, birkaç sent tutar; `make test`'e dahil değil).

## Mesaj limitleri

- SMS en fazla **160**, e-posta konusu en fazla **80** karakter ve konu **tek satır**. Aşan metin asla kesilmez:
  model otomatik düzeltmeye zorlanır (3 deneme), olmazsa limite uyan hazır şablon gelir; elle düzenlemede de aynı kontrol
  çalışır.
- Değişkenler (`{{first_name}}`, `{{city}}`) limitlere **sabit pay** olarak sayılır (20 ve 15 karakter; `.env`'den
  ayarlanır). Kişinin değeri boşsa veya payı aşıyorsa mesajda kısa bir yedek sözcük yazılır, böylece limit hiçbir
  alıcı için aşılamaz.
- Not: gerçek SMS ağlarında Türkçe karakterli (ş, ğ, ı …) bir mesaj 70 karakterle sınırlıdır; case 160 dediği için burada
  karakter sayılır.

## Onay ve gönderim

1. Kitleyi çıkarın, mesajları üretin (gerekirse düzenleyip kaydedin), **Onayla ve gönder**'e basın. Onay iki adımlıdır ve
   uyarıları (ör. düzenlemede silinen teklif) tekrar gösterir. Onaydan sonra kitle ve metin değiştirilemez.
2. Onay, izin vermiş ve telefon/e-postası kayıtlı herkesi kanal başına bir kez kuyruğa alır (`deliveries`). Aynı onayı iki
   kez (hatta aynı anda) göndermek çift kayıt üretmez.
3. `worker` kuyruğu kanal başına **90 mesaj/sn** (`WORKER_RATE_PER_SEC`) hızla boşaltır. Provider'ın 429/5xx/zaman aşımı
   cevaplarında yeniden dener (429'da `Retry-After`'e uyar ve kanalı bekletir; 5xx'te üstel bekleme + jitter, en fazla 5
   deneme). Aynı mesaj aynı `Idempotency-Key` ile gittiği için provider "kabul ettim ama hata döndüm" durumunda bile
   tek teslimat yapar. Geçersiz telefon/e-posta (`invalid_recipient`) yeniden denenmez, `failed` olur.
4. Sayfa her saniye ilerlemeyi günceller (gönderildi / başarısız / atlandı / bekliyor); onaydan sonra izni çeken kişi `atlandı`.

Varsayılan provider kaosu %3 rastgele 429 enjekte eder ve her 429 kanalı 1–3 sn durdurur; bu yüzden bir kampanya
yaklaşık 1–2 dakika sürer. Hızlı demo için `.env`'de `PROVIDER_429_RATE=0` yapıp `docker compose up -d mock-provider` deyin.

## İzleme: dashboard, `/metrics`, `/healthz`

- **Dashboard** (`/#/dashboard`): tüm kampanyalar, kanal bazlı ilerleme çubuğu, gönderildi / başarısız / tekrar / 429 sayıları;
  her 2 sn'de kendini yeniler. Sade bir ilk sürümdür, arayüz sonra yeniden tasarlanacak.
- **`GET :8081/metrics`** (Prometheus metin formatı). Hepsi veritabanından (`deliveries`, `llm_generations`, `campaigns`)
  okunur; api ve worker ayrı süreçlerdir ve sayılar yeniden başlatmada kaybolmaz. Veritabanı okunamazsa scrape 503 döner
  (sıfırlarla dolu sayfa dönmez).
  - Gönderim (`channel`: `sms`/`email`): `smartcampaign_deliveries_total{status=sent|failed|skipped}`,
    `smartcampaign_delivery_queue_length`, `smartcampaign_delivery_retries_total`, `smartcampaign_delivery_rate_limited_total`
    (retry'ların kaçı 429), `smartcampaign_send_rate_per_second` (son 10 sn ortalaması),
    `smartcampaign_provider_response_seconds_sum/_count` (ortalama = sum/count).
  - LLM (`kind`: `segment`/`messages`): `smartcampaign_llm_generations_total`, `…_first_attempt_valid_total`,
    `…_repair_attempts_total`, `…_fallbacks_total`, `…_refusals_total`, `smartcampaign_llm_attempt_seconds_sum/_count`,
    `smartcampaign_llm_tokens_total{direction=input|output}`, `smartcampaign_segment_outcomes_total{status=ok|needs_clarification|unsupported}`.
  - Mock LLM token harcamaz, token sayaçları yalnız gerçek Claude ile artar.
- **`GET :8081/healthz`**: api ve veritabanı bağlantısı (200 `{"status":"ok"}` / 503). docker-compose healthcheck'i bunu kullanır.
  Worker'ın `:9091/healthz`'ı da aynı veritabanı kontrolünü yapar.

**Gönderim kapsamı dışında:** worker tek replica (hız sınırı süreç içi); e-posta gövdesi düz metin; "delivered" bildirimi
yok (`sent` = provider kabul etti). Metriklerde histogram/yüzdelik yok, yalnız toplam ve adet (ortalama hesaplanır).

## Bilinen sınırlamalar
- **Teklifin şartları kitle değildir.** "1000 TL üzeri alışverişe 100 TL indirim, son kullanım 31.12.2026" brief'inde
  "1000 TL" ve tarih *teklifin* şartlarıdır (SMS ve e-postada yazılır); kimin mesaj alacağını belirlemez. Bu yüzden
  kitleyi de yazın: "Tüm müşterilere …" ya da "İstanbul'daki … müşterilere …". Kitle yazılmazsa sistem sorar: "Bu kampanya
  kime gönderilsin? Tüm müşterilere mi?" ve "Tüm müşterilere gönder" düğmesi sunar (iletişim izni olmayanlara yine de gitmez).
  Müşterinin *toplam harcamasına* göre kitle için "toplam harcaması 5000 TL'den fazla olan müşteriler" yazın.
- Son kullanım tarihi bugünle karşılaştırılmaz (geçmiş tarihli bir kupon fark edilmez); yalnız `gün.ay.yıl`, `gün/ay/yıl`,
  `yıl-ay-gün` ve "31 Aralık 2026" yazımları tanınır, yılsız tarih sorulur. Kodun tanımadığı bir kitle sıfatı ("VIP
  müşteriler") "kime gönderilsin?" sorusu alır. Ayrıntı: DECISIONS.md D62–D65.

- **Yazım hatası "satın almama" koşulunu düşürebilir.** Brief'teki kitle ifadeleri (ör. "sipariş oluşturmamış") bilinen
  kalıplarla okunur ve model kuralı bunlara karşı doğrulanır. Sözlükte olmayan bir yazım ("oluşturmmaış") tanınmaz;
  gerçek Claude ile bu durumda `purchased=false` koşulunun kuraldan düştüğü görüldü. Kampanyayı onaylamadan önce
  ekrandaki kitle kartındaki koşulları kontrol edin. Açık iş olarak PLAN.md'de, gerekçesiyle DECISIONS.md D51'de.
- Desteklenmeyen kısıtlar (yaş, cinsiyet, ürün kategorisi, doğum günü) ve takvime bağlı süreler ("bu ay", "geçen hafta")
  sessizce atlanmaz; ekranda `unsupported` / `needs_clarification` olarak açıkça gösterilir (DECISIONS.md D44).
  Sözlükte hiç olmayan bir kısıt ise mock'ta fark edilmeyebilir.

## Seed verisi hakkında notlar

- `docker compose up` sırasında `migrate` servisi şemayı kurar ve ~5.000 sahte kullanıcıyı yükler.
  Seed **deterministik** (`SEED=42`) ve **idempotent**: `users` doluysa atlanır.
- **Tarihler seed'in çalıştığı ana göre üretilir.** "Son 30 günde sepete ekleyen" kohortu (örnek
  brief: İstanbul, sepete ekledi, satın almadı) seed anında tam **300 kullanıcıdır**. Veritabanı
  eskidikçe sepet tarihleri 30 günlük pencerenin dışına kayar ve kohort sayısı **birkaç gün
  sonra azalmaya başlar**. Güncel sayı için `make clean` ile seed'i yeniden yükleyin. Testler her
  seferinde taze veritabanı kurduğu için bundan etkilenmez.
- Seed kodunu veya `SEED`/`SEED_USERS` değerini değiştirirseniz de `make clean` gerekir (seed yalnız
  boş veritabanına yazar).
- Bilerek eklenen kenar durumlar: izinli ama telefonu/emaili boş kullanıcılar, hiç izni olmayanlar,
  çok uzun isimler, bozuk telefon/email (mock provider kalıcı 400 döner).

## Mock provider

`POST /v1/sms/send`, `POST /v1/email/send` (zorunlu `Idempotency-Key` header'ı) ve `GET /healthz`.
Gecikme, 5xx, 429 (`Retry-After`), kanal başına 100/sn limit ve kabul-sonrası-hata davranışı
`.env.example` içindeki `PROVIDER_*` değişkenleriyle ayarlanır. Her 429/5xx ve idempotent replay
sebebiyle loglanır: `docker compose logs -f mock-provider`.
