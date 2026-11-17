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

- Uygulama: http://localhost:8080 — **Kampanyalar (dashboard):** http://localhost:8080/#/dashboard
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
- **Brief'i kim anlıyor?** Gerçek Claude modunda brief'i Claude yorumlar ve kitleyi çıkarır. Kod yalnız nesnel kontrol
  yapar: şema/beyaz liste ve SQL güvenliği, teklifin mantıklı olması ve Claude'un yazdığı şehir, sayı, tutar, kod ve
  tarihin brief'te gerçekten geçmesi (uydurma onarıma gönderilir, sürerse sorulur). Ters yönde de: brief'teki her TL
  tutarı ve yüzde kuralda ya da teklifte bir yere oturmalı; oturmazsa Claude'a geri gönderilir, sürerse "1000 TL ne için:
  minimum sepet mi, üst sınır mı…?" diye sorulur (tarihler ve özel gün ifadeleri sayılmaz, D89). Mock modunda brief'i kural okuyucusu
  anlar (`brief.Analyze`): tanımadığı ya da belirsiz bulduğu ifadeyi sorar (DECISIONS.md D87).
- `make test` **her zaman mock** ile çalışır; `.env`'de anahtar olsa bile okunmaz, internet gerekmez.
- `make test-live` gerçek Claude'a duman testi yapar (yalnız anahtar varsa, birkaç sent tutar; `make test`'e dahil değil).

## Mesaj limitleri

- SMS en fazla **160**, e-posta konusu en fazla **80** karakter ve konu **tek satır**. Aşan metin asla kesilmez:
  model otomatik düzeltmeye zorlanır (3 deneme), olmazsa limite uyan hazır şablon gelir; elle düzenlemede de aynı kontrol
  çalışır.
- Değişkenler (`{{first_name}}`, `{{city}}`) limitlere **sabit pay** olarak sayılır (20 ve 15 karakter; `.env`'den
  ayarlanır). Kişinin değeri boşsa veya payı aşıyorsa mesajda kısa bir yedek sözcük yazılır, böylece limit hiçbir
  alıcı için aşılamaz.
- Worker da aynı payları kullanır (`VARIABLE_BUDGET_*` api ve worker'a birlikte verilir) ve **her mesajı göndermeden
  önce son kez ölçer**: kişiselleştirilmiş SMS 160'ı ya da konu 80'i aşarsa gönderilmez, teslimat nedeniyle `failed` olur
  (dashboard: "Kişiselleştirme sonrası karakter sınırını aştığı için gönderilmedi").
- Not: gerçek SMS ağlarında Türkçe karakterli (ş, ğ, ı …) bir mesaj 70 karakterle sınırlıdır; case 160 dediği için burada
  karakter sayılır.

## Onay ve gönderim

1. Kitleyi çıkarın, mesajları üretin (gerekirse düzenleyip kaydedin), **Onayla ve gönder**'e basın. Onay iki adımlıdır ve
   uyarıları (ör. düzenlemede silinen teklif) tekrar gösterir. Onaydan sonra kitle ve metin değiştirilemez.
2. Onay, izin vermiş ve telefon/e-postası kayıtlı herkesi kanal başına bir kez kuyruğa alır (`deliveries`). Aynı onayı iki
   kez (hatta aynı anda) göndermek çift kayıt üretmez. Aynı telefonu ya da e-postayı paylaşan iki müşteri kaydı da o
   adrese **tek mesaj** alır (ekrandaki "ulaşılabilir" sayıları adres sayısıdır).
3. `worker` kuyruğu kanal başına **90 mesaj/sn** (`WORKER_RATE_PER_SEC`) hızla boşaltır. Provider'ın 429/5xx/zaman aşımı
   cevaplarında yeniden dener (429'da `Retry-After`'e uyar ve kanalı bekletir; 5xx'te üstel bekleme + jitter, en fazla 5
   deneme). Aynı mesaj aynı `Idempotency-Key` ile gittiği için provider "kabul ettim ama hata döndüm" durumunda bile
   tek teslimat yapar. Geçersiz telefon/e-posta (`invalid_recipient`) yeniden denenmez, `failed` olur.
4. Sayfa her saniye ilerlemeyi günceller (gönderildi / başarısız / atlandı / bekliyor); onaydan sonra izni çeken kişi `atlandı`.

Varsayılan provider kaosu %3 rastgele 429 enjekte eder ve her 429 kanalı 1–3 sn durdurur; bu yüzden bir kampanya
yaklaşık 1–2 dakika sürer. Hızlı demo için `.env`'de `PROVIDER_429_RATE=0` yapıp `docker compose up -d mock-provider` deyin.

## Çok dilli içerik (bonus)

- Her kullanıcının bir dili vardır (`users.language`: `tr`, `en`, `de`; seed'de %60 / %25 / %15). Her kampanya **üç dilde birden**
  üretilir: önce Türkçe (referans), sonra İngilizce ve Almanca paralel. Mesajlar ekranda dil sekmelerinde akar ve düzenlenir.
- **Üç dil aynı şeyi söylemek zorundadır.** İndirim oranı, TL tutarı, tarih, ücretsiz kargo vaadi ve `{{değişkenler}}`
  dillerde aynı değilse üretim modele geri bildirimle yeniden yazdırılır, elle kayıt **422 ile reddedilir**, onay yapılmaz
  (D70). Bu yüzden bir dili düzenlerken teklifi değiştirirseniz üç dili birlikte güncellemelisiniz; ekran hepsini birlikte kaydeder.
- Bir dil üretilemezse üç dil de yerleşik şablona düşer (karışık metin/şablon olmaz, D69).
- Gönderimde kişinin dili **gönderim anında** okunur: zamanlamadan sonra dilini değiştiren kişi yeni dilindeki mesajı bir kez alır.
  Adı boşsa kendi dilindeki sözcük yazılır ("değerli müşterimiz" / "valued customer" / "geschätzter Kunde").
- Onay ekranı kitlenin dil dağılımını gösterir. Cinsiyet yalnız brief açıkça isterse kitle koşulu olur (aşağıya bakın);
  doğum tarihi şimdilik yalnız saklanır.

## Brief'te özel gün, cinsiyet, birleşik ve kademeli teklif

Aşağıdaki kurallar mock modunda kural okuyucusunun kurallarıdır. Gerçek Claude modunda aynı kurallar Claude'a prompt'ta
söylenir ama kararı Claude verir; kod yalnız uydurmayı yakalar (D82–D88).

- **Özel gün kitleyi kısıtlamaz.** "Kadınlar Günü'ne özel %20 indirim" mesajın konusunu ve tonunu belirler; kitle
  yazılmadıysa **herkes** seçilir ve kitle adımında "Kitle belirtilmediği için tüm müşteriler seçildi." yazar (soru
  sorulmaz). Tanınan günler: Kadınlar Günü / 8 Mart, Anneler, Babalar, Sevgililer (14 Şubat), Öğretmenler, 23 Nisan,
  Yılbaşı, Black Friday, Ramazan / Kurban Bayramı, bayram.
- **Cinsiyet yalnız açıkça istenirse** filtrelenir: "kadın müşterilere özel", "kadınlara", "erkek müşterilerimize",
  "cinsiyeti erkek olan". "Kadın ve erkek müşterilere" herkestir; "kadın koleksiyonu" gibi kitleyi anlatmayan bir kelime
  hiçbir şeyi değiştirmez. Cinsiyetini belirtmemiş kişiler cinsiyet koşuluna girmez.
- **Birleşik teklif:** farklı türden en fazla 3 teklif birlikte verilir ("%20 indirim + ücretsiz kargo", "%10 indirim, 50
  TL hediye çeki ve ücretsiz kargo"); her parça SMS'te ve e-postada geçmek zorundadır. Aynı türden iki değer ("%20 ya da
  %30") ya da seçenek olarak yazılmış teklifler ("%20 indirim ya da ücretsiz kargo") sorulur.
- **Kademeli indirim:** "Her 1000 TL'lik alışverişe 100 TL indirim, toplamda en fazla 300 TL" → teklif `amount 100`,
  `step_spend 1000`, `max_discount 300`; mesajda üç sayı da yazılır. "1000 TL'ye varan siparişte 300 TL indirim"
  alt limittir (`min_spend 1000`), "300 TL'ye varan indirim" ise tavan. Yüzdeye tavan da olur: "%20 indirim, en fazla 300 TL".
  Mesajda teklifte olmayan bir TL tutarı (ör. örnek hesap "3000 TL'lik alışverişte…") çelişki sayılır ve düzelttirilir.
- **Metinler konuya ve tona göre değişir.** Kod brief'ten bir tema çıkarır (özel gün; konu: sepet / geri kazanma / hoş
  geldin / teşekkür; ton: samimi, resmi, heyecanlı, kısa, zarif, acil) ve bunu Claude'a da mock'a da verir. Mock, ifade
  bankalarından adaylar üretip brief'e göre farklı bir noktadan başlar ve backend kontrollerinden geçen ilk adayı seçer:
  aynı brief hep aynı metni, farklı brief farklı metni alır; 160/80 sınırı ve teklifin her parçası korunur.

## Zamanlanmış gönderim (bonus)

- Onay ekranında **Hemen** ya da **İleri bir zamanda** seçilir (en az 1 dakika, en fazla 30 gün sonrası). Hemen gönderimde
  hiçbir şey değişmez. Zamanlanan kampanyada onay yalnızca kararı kaydeder (`scheduled`): **kuyruk ve kitle oluşmaz.**
- Saat gelince `worker` içindeki zamanlayıcı kayıtlı kuralı yeniden çalıştırır, **o andaki** kitleyi çıkarır, teslimatları
  oluşturur ve kampanya normal şekilde (`approved → sending → completed`) ilerler; kimsenin müdahalesi gerekmez. Kampanya
  `FOR UPDATE SKIP LOCKED` ile sahiplenilir: birden çok worker olsa da her kampanya bir kez başlar. Onay ekranı ve kampanya
  kartı "kişi sayısı gönderim anında değişebilir" der; sayılar tahmindir.
- **Gönderim zamanı teklifin son kullanma gününden sonra seçilemez** (arayüz ve sunucu; gün Türkiye saatiyle okunur).
  Teklif, kampanya beklerken (ya da worker kapalıyken) sona ererse mesaj **gönderilmez**, kampanya `failed` olur ve nedeni görünür.
- Başlamadan önce **İptal et** ile (`POST /api/campaigns/{id}/cancel`) iptal edilebilir; başladıysa iptal reddedilir.
  İptal ile başlatma yarışırsa tam biri kazanır (testli). Worker kapalıyken zamanı gelen kampanya, worker açılınca hemen başlar.
- Zamanlanmış bir kampanya düzenlenemez; yanlışsa iptal edip yenisini oluşturun (yeniden zamanlama yok).

## İzleme: dashboard, `/metrics`, `/healthz`

- **Kampanyalar** (`/#/dashboard`, eski adıyla dashboard): tüm kampanyalar, kanal bazlı ilerleme çubuğu, gönderildi / başarısız / tekrar / 429 sayıları;
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

**Gönderim kapsamı dışında:** worker tek replica (hız sınırı süreç içi; iki worker kanal başına 180/sn'ye çıkar, D81); e-posta gövdesi düz metin; "delivered" bildirimi
yok (`sent` = provider kabul etti). Metriklerde histogram/yüzdelik yok, yalnız toplam ve adet (ortalama hesaplanır).

## Bilinen sınırlamalar
- **Teklifin şartları kitle değildir.** "1000 TL üzeri alışverişe 100 TL indirim, son kullanım 31.12.2026" brief'inde
  "1000 TL" ve tarih *teklifin* şartlarıdır (SMS ve e-postada yazılır); kimin mesaj alacağını belirlemez. Bu yüzden
  kitleyi de yazın: "Tüm müşterilere …" ya da "İstanbul'daki … müşterilere …". Kitle yazılmazsa sistem sorar: "Bu kampanya
  kime gönderilsin? Tüm müşterilere mi?" ve "Tüm müşterilere gönder" düğmesi sunar (iletişim izni olmayanlara yine de gitmez).
  Müşterinin *toplam harcamasına* göre kitle için "toplam harcaması 5000 TL'den fazla olan müşteriler" yazın.
- Özel gün sözlüğü sınırlıdır: listede olmayan bir gün ("Dünya Kedi Günü") mock'ta tema olarak tanınmaz ve kitle yazılmamışsa
  "kime gönderilsin?" sorulur. Gerçek Claude metinde temayı brief'ten yine okur. Kodla denetlenmeyen tek şey üslup: Claude'un
  özel günde okura kadın/anne gibi hitap etmemesi prompt kuralıdır, kontrol değildir.
- Son kullanım tarihi bugünle karşılaştırılmaz (geçmiş tarihli bir kupon fark edilmez); yalnız `gün.ay.yıl`, `gün/ay/yıl`,
  `yıl-ay-gün` ve "31 Aralık 2026" yazımları tanınır, yılsız tarih sorulur. Kodun tanımadığı bir kitle sıfatı ("VIP
  müşteriler") "kime gönderilsin?" sorusu alır. Ayrıntı: DECISIONS.md D62–D65.

- **Yazım hatası "satın almama" koşulunu düşürebilir.** Brief'teki kitle ifadeleri (ör. "sipariş oluşturmamış") bilinen
  kalıplarla okunur ve model kuralı bunlara karşı doğrulanır. Sözlükte olmayan bir yazım ("oluşturmmaış") tanınmaz;
  gerçek Claude ile bu durumda `purchased=false` koşulunun kuraldan düştüğü görüldü. Kampanyayı onaylamadan önce
  ekrandaki kitle kartındaki koşulları kontrol edin. Açık iş olarak PLAN.md'de, gerekçesiyle DECISIONS.md D51'de.
- Mock modunda desteklenmeyen kısıtlar (yaş, ürün kategorisi, doğum günü) ve takvime bağlı süreler ("bu ay", "geçen
  hafta") sessizce atlanmaz; ekranda `unsupported` / `needs_clarification` olarak açıkça gösterilir (DECISIONS.md D44).
  Sözlükte hiç olmayan bir kısıt ise mock'ta fark edilmeyebilir.
- **Gerçek Claude modunda kitle yorumu Claude'a aittir** (D87). Kod Claude'un brief'te olmayan bir şehir, sayı, tutar,
  kod ya da tarih uydurmasını yakalar, ama bir koşulu yanlış anlamasını ya da düşürmesini yakalamaz. Onaydan önce kitle
  kartındaki koşul rozetlerini ve kişi sayısını kontrol edin.

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
- Her kullanıcının ayrıca **dil** (`tr`/`en`/`de`), **cinsiyet** ve **doğum tarihi** vardır (bir kısmı boş bırakılır). Bu
  alanlar ayrı bir rastgele akıştan gelir; mevcut verinin hiçbir değeri ve "kohort tam 300" garantisi değişmedi. Bu
  sürüme güncelleyince eski bir veritabanında dil hep `tr` kalır: `make clean` ile seed'i yeniden yükleyin.
- Bilerek eklenen kenar durumlar: izinli ama telefonu/emaili boş kullanıcılar, hiç izni olmayanlar,
  çok uzun isimler, bozuk telefon/email (mock provider kalıcı 400 döner).

## Mock provider

`POST /v1/sms/send`, `POST /v1/email/send` (zorunlu `Idempotency-Key` header'ı) ve `GET /healthz`.
Gecikme, 5xx, 429 (`Retry-After`), kanal başına 100/sn limit ve kabul-sonrası-hata davranışı
`.env.example` içindeki `PROVIDER_*` değişkenleriyle ayarlanır. Her 429/5xx ve idempotent replay
sebebiyle loglanır: `docker compose logs -f mock-provider`.
