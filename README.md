# Smart Campaign Composer

[![CI](https://github.com/d-nazli/smart_campaign_composer/actions/workflows/ci.yml/badge.svg)](https://github.com/d-nazli/smart_campaign_composer/actions/workflows/ci.yml)

Pazarlamacının doğal dille yazdığı kampanya brief'ini Claude ile bir hedef kitle kuralına ve SMS/e-posta
metinlerine çeviren sistem. Kural backend'de doğrulanıp parametreli SQL'e derlenir, metinler ekrana stream
edilir, insan onayından sonra mesajlar kuyruk üzerinden mock provider'a gönderilir ve durum canlı izlenir.

Kararlar ve nedenleri: [DECISIONS.md](DECISIONS.md)

## Teknolojiler

| Katman   | Seçim |
|----------|-------|
| Frontend | Vue 3 + TypeScript, Vite; nginx statik dosyaları sunar ve `/api`'yi backend'e yönlendirir |
| Backend  | Go 1.25 (`api`, `worker`, `mockprovider`, `tool` binary'leri tek modülde) |
| DB       | PostgreSQL 16 (goose migration'ları, deterministik seed) |
| Kuyruk   | Postgres `deliveries` tablosu, `FOR UPDATE SKIP LOCKED` ile alınır |
| LLM      | Claude (Anthropic Go SDK, structured outputs); anahtar yoksa yerleşik mock LLM |

## Kurulum

Gereken tek şey Docker (host'ta Go/Node gerekmez).

```bash
cp .env.example .env        # isteğe bağlı: her değerin varsayılanı var
docker compose up --build
```

| Servis | Adres |
|--------|-------|
| Arayüz | http://localhost:8080 |
| API | http://localhost:8081 |
| Health | http://localhost:8081/healthz (veritabanını da kontrol eder; ayrıca `/readyz`, `/api/system`) |
| Metrics | http://localhost:8081/metrics (Prometheus metin formatı) |
| Mock provider | http://localhost:8090 (`POST /v1/sms/send`, `POST /v1/email/send`, `GET /healthz`) |

`ANTHROPIC_API_KEY` boşsa mock LLM, doluysa gerçek Claude kullanılır (`LLM_MODE=auto`). `migrate` servisi
şemayı kurar ve ~5.000 sahte kullanıcıyı yükler; `make clean` veritabanını silip seed'i baştan yükletir.

## Testler

```bash
make test
```

Backend (Go, Postgres'li entegrasyon testleri dahil) ve frontend (vue-tsc + vitest) testlerini Docker içinde
çalıştırır. **Her zaman mock LLM kullanır: gerçek API key ve internet gerekmez**, `.env`'de key olsa bile test
container'larına verilmez. Ek hedefler: `make test-race` (race detector), `make bench` (gönderim hızı ölçümü),
`make test-live` (gerçek Claude ile duman testi, key gerekir).

## Kullanıcı akışı

1. **Brief:** pazarlamacı isteği doğal dille yazar.
2. **Segment:** Claude brief'ten JSON bir kitle kuralı (ve teklif) üretir; backend şemayı, alan/operatör
   beyaz listesini ve değerleri doğrular, kuralı parametreli SQL'e derler ve kaç kişiye gideceğini gösterir.
   Kitle belirsizse model soru sorar, kullanıcı aynı kampanyada cevaplar.
3. **Stream içerik:** SMS ve e-posta metinleri SSE ile ekrana akar; son metin limit ve tutarlılık
   kontrollerinden geçer.
4. **Onay:** pazarlamacı önizler, gerekirse düzenler (aynı kontroller tekrar çalışır) ve onaylar.
5. **Kuyruk:** onay, izinli ve iletişim bilgisi olan herkesi kanal başına bir kez `deliveries` tablosuna yazar;
   `worker` kanal başına 97 msg/sn hızla mock provider'a gönderir, hataları yeniden dener.
6. **Dashboard:** kampanya listesi ve gönderim ekranı gönderildi / başarısız / atlandı / bekliyor sayılarını
   ve kanal hızlarını periyodik olarak yeniler.

## Mimari

```
  Tarayıcı (Vue 3 SPA)
        │  HTTP + SSE
        ▼
  ┌──────────────┐  /api   ┌──────────────┐  JSON şema   ┌─────────────────────┐
  │ web (nginx)  │────────▶│  api (Go)    │─────────────▶│ Claude / mock LLM   │
  │ :8080        │         │  :8081       │◀── stream ───└─────────────────────┘
  └──────────────┘         └──────┬───────┘
                                  │ kural → parametreli SQL, onay → deliveries
                                  ▼
  ┌──────────────┐         ┌──────────────┐  SKIP LOCKED  ┌──────────────┐  HTTP + Idempotency-Key  ┌────────────────────┐
  │ migrate      │────────▶│ PostgreSQL   │◀─────────────│ worker (Go)  │────────────────────────▶│ mock-provider (Go) │
  │ şema + seed  │         │ users,       │               │ pacer, retry,│◀─ 2xx / 429 / 5xx ──────│ :8090              │
  └──────────────┘         │ campaigns,   │               │ zamanlayıcı  │                          └────────────────────┘
                           │ deliveries   │               └──────────────┘
                           └──────────────┘
```

- **web:** Vue SPA'yı sunar, `/api` isteklerini `api`'ye proxy'ler (CORS yok).
- **api:** kampanya API'si, LLM çağrıları (doğrula → hata varsa geri bildirimle yeniden dene, en fazla 3),
  segment derleme, SSE stream, onay, `/healthz` ve `/metrics`.
- **worker:** kuyruktan teslimat alır, kanal başına sabit hızda gönderir, 429/5xx'i ayrı ele alır, zamanı
  gelen kampanyaları başlatır.
- **mock-provider:** gecikme, 5xx, 429 (`Retry-After`), zaman aşımı ve "kabul edip hata dönme" davranışı
  olan sahte SMS/e-posta servisi; idempotency key ile tekrar teslimatı engeller.
- **migrate:** tek seferlik; migration'ları uygular ve seed'i yükler. `api` ve `worker` onun bitmesini bekler.

## Bonuslar

- [x] Zamanlanmış gönderim (kitle gönderim anında yeniden çıkarılır, başlamadan iptal edilebilir)
- [x] A/B varyantları (yalnız A/B, %50/%50 hash ile bölünme)
- [x] Kullanıcının diline göre içerik (12 locale, kaynak tr-TR, çeviriler doğrulanır)
- [x] CI pipeline ([.github/workflows/ci.yml](.github/workflows/ci.yml))

