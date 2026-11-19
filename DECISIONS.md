# DECISIONS.md

Her karar: **Karar** (ne yapıldı) / **Neden** (gerekçe). Kurulum ve mimari özeti: [README.md](README.md).

## Proje gereksinimleri


**Karar:** Tek komutla kurulum (`docker compose up`) ve tek komutla test (`make test`); her şey Docker içinde çalışır.
**Neden:** Değerlendiren kişinin host'ta Go/Node kurmadan, API key olmadan sistemi çalıştırıp test edebilmesi gerekiyor.

## Backend dili, DB ve kuyruk

**Karar:** Backend Go 1.25; tek modül, dört binary (`api`, `worker`, `mockprovider`, `tool`), tek multi-stage Dockerfile.
**Neden:** Eşzamanlı gönderim (goroutine, context, zaman aşımı) için doğal bir dil; tek statik binary küçük image verir.

**Karar:** Veritabanı PostgreSQL 16, migration'lar goose ile.
**Neden:** Segment sorguları, transaction'lar ve kuyruk aynı yerde; ilişkisel veri ve `UNIQUE` kısıtları idempotency
için hazır araç.

**Karar:** Kuyruk ayrı bir broker değil, Postgres `deliveries` tablosu; worker satırları `FOR UPDATE SKIP LOCKED` ile
süreli bir kira (lease) alarak çeker, kirası dolan satır başka worker'a geçer.
**Neden:** Onay ve kuyruğa yazma tek transaction'da olur (mesaj kaybı ya da yarım kuyruk olmaz); ek servis gerekmez.
Kuyruk `queue.Queue` arayüzünün arkasında olduğu için ileride değiştirilebilir.

## LLM çıktısının güvenilir hale getirilmesi

**Karar:** Claude'dan JSON şemalı structured output istenir (segment, mesaj ve çeviri için ayrı şemalar); çıktı backend'de
ayrıca bilinmeyen alanlara izin vermeyen bir parser ve iş kurallarıyla (alan/operatör, tip, aralık, uzunluk, teklif
tutarlılığı) doğrulanır.
**Neden:** Şema biçimi garanti eder ama anlamı garanti etmez; son söz her zaman backend'in kontrolündedir.

**Karar (LLM retry stratejisi):** İki katman. (1) Anthropic SDK, tek bir çağrının içinde 429/5xx ve bağlantı
hatalarını kendi backoff'uyla yeniden dener. (2) Uygulama katmanında geri bildirimli onarım döngüsü (`llm.Repair`):
geçersiz çıktı (bozuk JSON, kural ihlali, limit aşımı, `max_tokens` ile kesilme) reddedilen metin ve sorun listesiyle
modele geri gönderilir. SDK retry'larından sonra hâlâ hata alan çağrı geri bildirimsiz tekrarlanır. Refusal, auth hatası
ve iptal döngüyü hemen durdurur. Toplam en fazla 3 deneme yapılır.
**Neden:** Taşıma hatası ile içerik hatası farklı ilaç ister. Model çoğu hatayı neyin yanlış olduğunu görünce tek
denemede düzeltir; refusal'ı ya da yanlış key'i tekrar denemek işe yaramaz.

**Karar:** Gerçek Claude modunda brief'i yalnız Claude yorumlar: kod Claude'un kuralını brief'le karşılaştırmaz, yalnız
teknik doğrulama yapar (şema, beyaz liste, tip/aralık, iş kuralları). Anlamsal güvence insan onayıdır: kitle özeti,
koşul rozetleri ve kişi sayısı onaydan önce gösterilir, onaysız hiçbir şey kuyruğa girmez. Mock modunda ise brief'i
koddaki kural okuyucusu çözer ve modelin cevabının brief'teki her parçayı içerdiği kontrol edilir.
**Neden:** Yanlış yorumun zararı, mesaj
gönderilince ortaya çıkar. Onay ekranı kuralın sonucunu (kim, hangi koşulla, kaç kişi) herkesin anlayacağı
biçimde gösterdiği için bu hata gönderimden önce yakalanır.

**Karar:** LLM'e müşteri verisi gitmez: modele yalnız brief ve pazarlamacının soru cevapları gönderilir. Ad, telefon,
e-posta gibi kişisel veriler hiçbir prompt'a girmez; kişiselleştirme gönderim anında `{{first_name}}` gibi
değişkenlerle worker'da yapılır.
**Neden:** Kişisel verinin üçüncü taraf bir modele çıkmaması gizlilik açısından en güvenli seçim; ayrıca tek bir şablon
tüm kitleye yeter, kişi başına LLM çağrısı gerekmez.

**Karar:** Denemeler tükenirse: mesajlarda limitlere uyan hazır bir şablona düşülür (ekranda belirtilir); kitlede tahmin
yapılmaz, kullanıcıya hata ya da soru gösterilir.
**Neden:** Yanlış bir kitleye gönderim, eksik bir metinden çok daha pahalıdır.

**Karar:** LLM bir arayüzün (`llm.Client`) arkasında; iki mock var: brief'i sözlükle okuyan deterministik "akıllı" mock
(geliştirme ve demo) ve testlerde bozuk JSON, kural ihlali, refusal, API hatası üreten senaryolu mock.
**Neden:** Testler gerçek key ve internet olmadan çalışmalı, retry ve fallback yolları kesin olarak sınanabilmeli.

**Karar:** `LLM_MODE=auto`: key varsa gerçek Claude, yoksa mock. `make test` her zaman mock kullanır.
**Neden:** Kod değişikliği olmadan iki mod arasında geçiş; testler `.env`'deki key'den etkilenmez.

## Segment kuralının SQL'e dönüşümü

**Karar:** LLM SQL üretmez. Düz bir kural üretir (`{field, op, value}` koşulları, AND/OR); alanlar ve her alanın izinli
operatörleri, tipi ve aralığı tek bir beyaz listede (`segment/fields.go`) tanımlıdır.
**Neden:** LLM çıktısı doğrudan SQL olarak çalıştırılmamalı; beyaz liste dışındaki her şey reddedilir ve modele geri
bildirilir. Prompt ve JSON şeması da aynı listeden üretilir, böylece ikisi ayrışmaz.

**Karar:** Derleyici SQL metnini yalnız koddaki sabit parçalardan kurar; kuraldan gelen her değer bind parametresi (`$n`)
olarak geçer.
**Neden:** SQL injection'a yer bırakmaz ve derlenen sorgu birim testlerinde birebir karşılaştırılabilir.

**Karar:** Kitle belirsizse model tahmin etmez, soru sorar (`needs_clarification`). Cevap aynı kampanyaya gider; kitle
brief + tüm cevaplarla yeniden çözülür (en fazla 5 cevap). Kitle hiç yazılmadıysa "Tüm müşterilere gönder" tek tıkla
cevap olarak sunulur.
**Neden:** Yanlış tahmin edilmiş bir kitle sessizce yanlış kişilere gönderim demektir; soru sormak ucuzdur. Cevaplar
kampanyada saklandığı için brief ve kararın geçmişi kaybolmaz.

## Kanal izni kontrolü

**Karar:** Bir kişi bir kanaldan yalnız o kanala izni varsa ve adresi (telefon/e-posta) kayıtlıysa kuyruğa alınır;
worker göndermeden hemen önce izni tekrar okur, onaydan sonra izni çekilen kişiyi `skipped` yapar.
**Neden:** İzin onay ile gönderim arasında değişebilir; ekranda gösterilen sayılar da aynı filtreyle hesaplanır.

## Karakter limitleri ve değişkenler

**Karar:** SMS en fazla 160, e-posta konusu en fazla 80 karakter (rune), konu tek satır. Aşan metin kesilmez; modele
düzeltme için geri gönderilir, elle düzenlemede kaydetme reddedilir.
**Neden:** Kesilmiş bir kampanya metni anlamını kaybeder; limit her yerde aynı fonksiyonla ölçülür. Not: gerçek
ağlarda Türkçe karakterli SMS 70 karakterdir; case 160 dediği için karakter sayılır.

**Karar:** `{{first_name}}` ve `{{city}}` limit hesabında sabit bir pay olarak sayılır (20 ve 15 karakter,
`VARIABLE_BUDGET_*`). Kişinin değeri boşsa ya da payı aşıyorsa kısa bir yedek sözcük yazılır; worker kişiselleştirilmiş
mesajı göndermeden önce bir kez daha ölçer, yine aşarsa göndermez.
**Neden:** Gerçek isim uzunluğu onay anında bilinmez; en kötü durum bütçesi limitin hiçbir alıcıda aşılmamasını garanti eder.

## İçeriğin düzenlenmesi

**Karar:** Pazarlamacı metni yalnız onaydan önce düzenleyebilir. Düzenleme LLM çıktısıyla aynı doğrulamadan geçer:
limit, tek satır konu ve değişken ihlali kaydı reddeder (422 `invalid_content`); teklifin bir parçasının eksik olması
gibi kalite sorunları yalnız uyarıdır ve onay ekranında tekrar gösterilir.
**Neden:** Son karar insanın, bu yüzden üslup ve teklif tercihi engellenmez; ama kanal limitleri pazarlık konusu değildir.
Aynı kuralın modele hata, insana uyarı olması modelin uydurmasını engellerken insanın elini bağlamaz.

**Karar:** Elle yalnız kaynak metin (tr-TR) düzenlenir; çeviriler salt okunurdur (422 `translation_read_only`). Kaynak
değişince çeviriler "güncel değil" olur ve yeniden çevrilmeden onay verilmez (409).
**Neden:** Tek doğruluk kaynağı olursa diller birbirinden ayrışmaz; her çeviri yine kaynakla aynı kontrollerden geçer.

**Karar:** Onaydan sonra kitle ve metin değiştirilemez (409 `invalid_state`); onay anında gösterilen kişi sayıları
kampanyaya kaydedilir.
**Neden:** Kuyruğa giren mesajlar tam olarak onaylanan metin olmalı; worker da metni donmuş kabul edip cache'ler.

## Rate limit

**Karar:** Worker'da kanal başına bir pacer: gönderim slotları eşit aralıklıdır (patlama yok, boşta biriken kredi yok),
varsayılan 97 msg/sn (`WORKER_RATE_PER_SEC`).
**Neden:** Eşit aralık, herhangi bir 1 saniyelik pencerede 100'ü aşmamayı garanti eder; 100 yerine 97, provider'ın kayan
penceresinin kenarlarında 429 almamak için pay bırakır. `make bench` bunu gerçek saatle ölçer.

**Karar:** Tek bir 429 yalnız kendi mesajını bekletir; 1 saniyede 10 ve daha fazla 429 gelirse kanal `Retry-After`
kadar durur.
**Neden:** Mock rastgele %3 429 üretir; her 429'da kanalı durdurmak hızı çok düşürüyordu, art arda 429 ise gerçek bir
sınırlamadır.

## Retry stratejisi

**Karar:** 429: `Retry-After` kadar beklenir (yoksa 1 sn), deneme hakkı harcanmaz. 5xx, 408, ağ hatası ve zaman aşımı:
full-jitter üstel backoff (taban 2 sn, tavan 60 sn), en fazla 5 başarısız deneme. Diğer 4xx (ör. geçersiz alıcı)
yeniden denenmez, `failed` olur. 10 dakikadan eski bir teslimat yeniden denenmez.
**Neden:** 429 mesajın değil hızın sorunudur; 5xx geçici olabilir; 4xx tekrar denemekle düzelmez. Jitter, aynı anda
hata alan mesajların aynı anda geri dönmesini önler.

## Duplicate gönderim önleme

**Karar:** Üç katman: (1) `UNIQUE (campaign_id, user_id, channel)` + `ON CONFLICT DO NOTHING`; (2) onay tek transaction'da
durum kontrolüyle (compare-and-set) yapılır, aynı onay iki kez gelse de bir kez kuyruğa yazılır; (3) her provider
çağrısı teslimat ID'sini `Idempotency-Key` olarak taşır, provider aynı key'e ikinci teslimat yapmaz. Aynı telefonu ya da
e-postayı paylaşan kayıtlar o adrese tek mesaj alır.
**Neden:** Retry'lar "provider kabul etti ama cevap kayboldu" durumunda bile güvenli olmalı; tekillik hem kişi hem adres
düzeyinde garanti edilir.

## Stream yöntemi

**Karar:** SSE (WebSocket değil). Mesaj üretimi `POST .../messages/stream` ile başlar; tarayıcıda `fetch` + elle yazılmış
bir SSE parser kullanılır. Model JSON üretirken string alanlar parça parça ayrıştırılıp ekrana akıtılır; son metin tam
çıktıdan doğrulanır.
**Neden:** Akış tek yönlü (sunucu → istemci) ve nginx proxy'den rahat geçer. `EventSource` POST desteklemediği için
istemci elle yazıldı. Stream yalnız önizlemedir, doğrulamayı atlatamaz.

## Mock provider

CASE: "Gerçek bir servisi taklit eden basit bir servis; retry ve rate limit mantığını gerçekten test edebilmek için
zaman zaman hata, gecikme ve 429 döndürmeli."

**Karar (sözleşme):** Ayrı bir Go servisi (`mock-provider`, :8090): `POST /v1/sms/send`, `POST /v1/email/send`
(`{to, subject, body}`), zorunlu `Idempotency-Key` header'ı ve `GET /healthz`. Her istek sırayla dört adımdan geçer:
doğrulama → gerçek rate limit → idempotency → kaos.
**Neden:** Worker, gerçek bir sağlayıcıya konuşur gibi HTTP üzerinden konuşur; mock'u değiştirmek worker'ı değiştirmez.
Sıra her cevabın anlamını gerçekçi tutar: bozuk istek limit harcamaz, tekrar eden key kaosa hiç girmez.

**Karar (gerçek rate limit):** Kanal başına 100 istek / 1 sn (`PROVIDER_RATE_LIMIT_PER_SEC`). Token bucket değil, katı
bir kayan pencere: herhangi bir 1 saniyelik pencerede 100'den fazla istek kabul edilmez, aşan istek 429 ve
`Retry-After` (en eski isteğin pencereden çıkmasına kalan süre) alır. Görülen en yüksek pencere sayısı tutulur.
**Neden:** Token bucket pencere sınırlarında kısa patlamalara izin verir ve "limit aşıldı mı" sorusunu bulanıklaştırır.
Katı pencere, worker'ın limiti hiçbir saniyede aşmadığını kanıtlamak için kullanılır (testler ve `make bench`).

**Karar (kaos, varsayılanlar):** Her istekte bir dizi rastgele çekiliş yapılır:

| Davranış | Varsayılan | Neyi test eder |
|---|---|---|
| Gecikme | 50 ms + 0–100 ms jitter | Gönderimin eşzamanlı olması gerekir (kanal başına 64 gönderici) |
| 500/502/503, mesaj kabul edilmeden | %5 | Full-jitter backoff, en fazla 5 deneme |
| Rastgele 429, `Retry-After` 1–3 sn | %3 | 429 deneme hakkı harcamaz; tek 429 kanalı durdurmaz |
| Zaman aşımı: 10 sn sonra 504 | %1 | Worker'ın 5 sn istemci zaman aşımı ve yeniden deneme |
| Kabul edip 5xx dönme | %2 | Idempotency: retry çift teslimat yapmamalı |

İlk denemelerin ~%11'i hata alır. Her oran `PROVIDER_*` ile değiştirilebilir, hepsi 0 yapılırsa provider temiz çalışır.
**Neden:** Oranlar, her retry dalının bir demoda gözle görülmesi için yeterince yüksek, kampanyanın yine de tamamlanması
için yeterince düşük seçildi. Zaman aşımı süresi bilerek worker'ın zaman aşımından uzun: istemci tarafındaki zaman aşımı
yolu da çalışsın diye.

**Karar (idempotency):** Provider her key için kanal bazında bir kayıt tutar (bellekte). Aynı key ve aynı içerik tekrar
gelirse ikinci teslimat yapılmaz: 200, aynı `provider_message_id` ve `Idempotent-Replay: true` döner. Aynı key farklı
içerikle gelirse 422. Aynı key'li eşzamanlı istekler sıraya sokulur.
**Neden:** "Kabul ettim ama hata döndüm" senaryosunda worker'ın retry'ının güvenli olduğu provider tarafında sayılarak
doğrulanabilir (tekil teslimat ve replay sayaçları). Kayıt bellekte tutulur, provider yeniden başlarsa silinir; bir
mock için bilinçli sadelik.

**Karar (kalıcı hatalar):** Eksik/uzun key, bozuk JSON, eksik alan, `+90` ve 10 haneden oluşmayan telefon
(`+905551234567` biçimi) ya da geçersiz e-posta 400 alır.
**Neden:** Tekrar denemekle düzelmeyen hatalar; seed'deki bozuk adresler worker'ın bunları yeniden denemeyip `failed`
yaptığını gösterir.

**Karar (tekrar üretilebilirlik):** Rastgelelik `PROVIDER_SEED` ile tohumlanır (aynı tohum, aynı hata dizisi). Testlerde
oranlar yalnız 0 ya da 1'dir ve sahte saat kullanılır. Her 429, 5xx ve replay nedeniyle birlikte loglanır.
**Neden:** Demo tekrar edilebilir olmalı, testler rastgeleliğe ve gerçek zamana bağlı olmamalı.

## Gözlemlenebilirlik

**Karar:** `/metrics` (Prometheus metin formatı) sayıları veritabanından (`deliveries`, `llm_generations`, `campaigns`)
okur. Gönderim tarafında kanal başına durum sayıları, kuyruk uzunluğu, retry/429 sayısı, anlık hız ve provider yanıt
süresi; LLM tarafında ilk denemede geçerli çıktı, onarım denemesi, fallback, refusal, süre ve token sayıları var.
Veritabanı okunamazsa scrape 503 döner.
**Neden:** api ve worker ayrı süreçler; süreç içi sayaçlar yeniden başlatmada sıfırlanır ve birbirini görmez. LLM
metrikleri "model ne sıklıkla ilk seferde doğru üretiyor" sorusunu ölçülebilir kılar.

**Karar:** `/healthz` veritabanı bağlantısını da kontrol eder (bozuksa 503) ve Docker healthcheck'i bunu kullanır;
worker'ın kendi `/healthz`'ı vardır, `/api/system` api, veritabanı ve worker durumunu birlikte verir.
**Neden:** Veritabanına erişemeyen bir api "sağlıklı" görünmemeli; arayüzdeki sistem rozeti gerçek durumu göstermeli.

## Seed verisi

CASE: "Birkaç bin kullanıcılık bir seed. Hangi alanlara ihtiyaç olduğuna sen karar ver."

**Karar (üretim):** Seed Go ile yazıldı ve `migrate` servisi migration'lardan sonra çalıştırır. Varsayılan 5.000
kullanıcı (`SEED_USERS`, en az 200). Deterministiktir (`SEED=42`, aynı tohum aynı veri) ve idempotenttir (`users`
doluysa atlanır). Tüm tarihler seed'in çalıştığı ana göre üretilir.
**Neden:** Testler ve demo her temiz kurulumda aynı sayıları görür. Göreli tarihler "son 30 gün" gibi brief'leri her
zaman anlamlı kılar. Bedeli: veritabanı eskidikçe pencereler kayar, `make clean` ile yeniden yüklenir.

**Karar (alanlar):** Her alan bir gereksinimden geliyor:

| Alan | Neden |
|---|---|
| `first_name`, `last_name` | `{{first_name}}` kişiselleştirmesi |
| `email`, `phone` (boş olabilir) | İki kanal; izinden bağımsız, "izin var ama adres yok" durumu için |
| `sms_consent`, `email_consent` | Kanal başına ayrı izin (CASE: izni olmayana o kanaldan gönderilmez) |
| `city` | Örnek brief'teki "İstanbul'daki" ve `{{city}}` |
| `cart_events`, `orders` tabloları | Alışveriş geçmişi: sepete ekleme ve sipariş olayları |
| `last_cart_add_at`, `last_order_at`, `order_count`, `total_spent` | Olaylardan türetilen kolonlar; segment sorguları join'siz ve indeksli çalışır |
| `locale`, `gender`, `birth_date` | Dile göre içerik bonusu; cinsiyet ve yaş koşulları |

**Neden:** Örnek brief'i ("son 30 günde sepete ekleyip alışveriş yapmayan İstanbul'daki kullanıcılar") ve beyaz listedeki
her segment alanını karşılayan en küçük alan kümesi. Ham olaylar tutulduğu için türev kolonlar her zaman yeniden
hesaplanabilir.

**Karar (dağılımlar):** Oranlar karıştırılmış bir listeden tam sayıyla dağıtılır, toplamlar rastgeleliğe bırakılmaz:
- **Şehir:** 19 şehir, İstanbul %35, Ankara %12, İzmir %10, diğerleri %1–5.
- **İzin:** %60 iki kanal, %12 yalnız SMS, %18 yalnız e-posta, %10 hiç izin yok.
- **Locale:** tr-TR %40; 11 locale %4–8.
- **Cinsiyet:** kadın %48, erkek %48, diğer %2, belirtilmemiş %2.
- **Yaş:** 18–75; %16'sı 18–24, %28'i 25–34, %24'ü 35–44, %16'sı 45–54, %10'u 55–64, %6'sı 65–75; %3'ünün doğum tarihi boş.
- **Alışveriş:** sipariş tutarı 50–3.000 TL. Diğer kullanıcıların %35'inin hiç aktivitesi yok, %25'inin yalnız siparişi,
  %15'inin yalnız sepeti, %25'inin ikisi de var.

**Neden:** Bir e-ticaret müşteri tabanına benzer oranlar, ama kesin toplamlarla: testler "tam şu kadar kişi" bekleyebilir.
Profil alanları (locale, cinsiyet, doğum tarihi) ayrı bir rastgele akıştan gelir, böylece sonradan eklenmeleri mevcut
verinin tek bir değerini değiştirmedi.

**Karar (örnek brief kohortu ve tuzaklar):** Örnek brief'in kitlesi tam 300 kişidir (%6): İstanbul, son 1–25 gün içinde
sepete ekleme, sonrasında sipariş yok. Yanına 150'şer kişilik üç tuzak grup kurulur, her biri tek bir koşulu kaçırır:
İstanbul'da ama son sepeti 36+ gün önce; İstanbul'da, sepet yeni ama sonrasında sipariş vermiş; sepet yeni ve sipariş yok
ama başka şehirde. Tarihler bilerek 26–35 gün aralığına konmaz. Rastgele bir İstanbul kullanıcısı kohorta düşerse ona
sepetten sonra bir sipariş eklenir.
**Neden:** Segment kuralının herhangi bir koşulu (şehir, 30 günlük pencere, satın almama) yanlış uygulanırsa sayı
300'den sapar; kitle doğruluğu tek bir sayıyla doğrulanır. Boş bırakılan 26–35 gün aralığı, seed'in günün hangi
saatinde çalıştığının sonucu değiştirmemesini sağlar.

**Karar (kenar durumlar):** Bilerek eklendi: %3 telefonu boş, %3 e-postası boş (izinden bağımsız), %0,5 bozuk telefon ve
%0,5 bozuk e-posta (her biri kişiye özgü), %0,6 30–45 karakterlik ad, %10 hiç izni olmayan.
**Neden:** Her biri bir kod yolunu sınar: izin var ama adres yok (kuyruğa girmez); bozuk adres (provider kalıcı 400 döner,
yeniden denenmez); uzun ad (`{{first_name}}` payını aşar, yedek sözcük yazılır); izinsiz kişi (hiçbir kanaldan gönderilmez).

## Bonuslar

**Karar (zamanlanmış gönderim):** Onay yalnız kararı kaydeder; zamanı gelince worker'daki zamanlayıcı kuralı yeniden
çalıştırıp o anki kitleyi kuyruğa yazar (`SKIP LOCKED` ile tek kez). Gönderim zamanı 1 dakika–30 gün arası olmalı ve
teklifin son gününden sonra olamaz; başlamadan iptal edilebilir.
**Neden:** Kitle gönderim anında güncel olmalı; süresi geçmiş bir teklif gönderilmemeli.

**Karar (A/B):** Yalnız A ve B; her varyant sabit bir listeden farklı bir yaklaşım (aciliyet, duygusal, fayda, merak,
kişisel) alır, teklif ve rakamlar aynı kalır, B A'nın aynısı olamaz. Bölünme `md5(kampanya:kişi)` ile %50/%50; bir kişinin
iki kanalı aynı varyanttadır.
**Neden:** Deterministik bölünme tekrar çalıştırmada aynı sonucu verir ve tekillik kısıtını bozmaz. Açılma/tıklama
ölçülmediği için kazanan seçimi kapsam dışında bırakıldı.

**Karar (dile göre içerik):** Locale’ler bir tabloda tutulur (12 locale). Kaynak metin tr-TR’dir ve kitlede kişisi olan her locale’e çevrilir. Çeviriler kaynakla aynı kontrollerden geçer: limitler, değişkenler ve rakamlar kaynakla aynı olmalı. Kaynak değişince eski çeviriler “güncel değil” olarak işaretlenir ve yeni kaynaktan yeniden çevrilir. Güncellenen çeviri kontrolleri geçene kadar o içerik onaylanamaz; onay isteği 409 ile reddedilir.

Neden: Yeni locale eklemek kod değil, veri değişikliğidir. Kaynak değişince çeviriler de onunla birlikte güncellenir. Böylece her kişi yalnız kendi dilinde, kaynağın son haliyle tutarlı metin alır.

## CI pipeline

**Karar:** GitHub Actions, her push ve PR'da üç paralel job: `backend` (gofmt, go vet, build), `frontend` (npm ci,
vue-tsc, build, vitest) ve `docker` (`make test` + `docker compose build`). Go modülleri ve npm cache'i
`setup-go`/`setup-node` ile cache'lenir.
**Neden:** Hızlı lint/build geri bildirimi native araçlardan gelir; `make test` yerelde çalışan komutun aynısıdır.
Hiçbir adım secret istemez: testler mock LLM ile, `ANTHROPIC_API_KEY` boş çalışır.
