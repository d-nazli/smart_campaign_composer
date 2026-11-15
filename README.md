# Smart Campaign Composer

Insider One AI Developer case study. Pazarlamacının doğal dille yazdığı brief'i Claude ile
hedef kitleye ve SMS/email mesajlarına çeviren, onaya sunan, kuyruk üzerinden mock provider'a
gönderen ve durumu canlı gösteren sistem.

> **Durum:** geliştirme sürüyor (Aşama 1/6). Plan: [PLAN.md](PLAN.md), kararlar: [DECISIONS.md](DECISIONS.md).
> Bu README son aşamada tamamlanacak (mimari özet, yapılmayanlar).

## Kurulum

Gereken: Docker Desktop (host'ta Go/Node gerekmez). `make` için Windows'ta `choco install make` (veya winget ile bir GNU Make paketi); `make` olmadan Makefile'daki `docker compose ...` komutları doğrudan çalıştırılabilir.

```powershell
docker compose up --build      # veya: make up
```

- Uygulama: http://localhost:8080
- API: http://localhost:8081 (`/healthz`, `/readyz`, `/api/info`)
- Mock provider: http://localhost:8090

`.env` zorunlu değil; ayarlar için `copy .env.example .env`. `ANTHROPIC_API_KEY` boşsa mock LLM,
doluysa gerçek Claude kullanılır (`LLM_MODE=auto`).

```powershell
make test        # tüm testler, Docker içinde, API key/internet gerektirmez (her zaman mock LLM)
make dev         # geliştirme: compose watch + air + Vite HMR
make clean       # her şeyi durdur ve veritabanı volume'unu sil (seed baştan yüklenir)
```

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
