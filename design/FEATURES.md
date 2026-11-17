# Frontend özellik envanteri (taşıma sonrası kontrol listesi)

Kaynak: `frontend/src` (Vue 3 + TypeScript + Vite). vue-router ve Pinia yok; durum bileşenlerde/composable'larda tutuluyor. Bu belge, mevcut kullanıcıya görünen her davranışı listeler; taşıma (migration) sonrası her satırın aynı şekilde çalıştığı doğrulanmalıdır.

## 1. Giriş

### Ekranlar
- **Kabuk (App shell)** — `src/App.vue`: üst bar (marka, sayfa sekmeleri, LLM rozeti) ve `<main>` içinde tek bir görünüm.
- **Kampanya oluşturma sihirbazı (Compose)** — `src/views/ComposeView.vue`: 5 adımlı sihirbaz: **Brief → Kitle → Mesajlar → Onay → Gönderim**. Altta yapışık değil, içeriğin hemen altında duran bir eylem çubuğu (Geri + ana buton).
- **Dashboard** — `src/views/DashboardView.vue`: özet kutucukları, kampanya tablosu, sistem sağlığı, iptal modali.

### Gezinme
- **Hash**: `#/dashboard` → Dashboard; diğer her şey (`#/`, boş, bilinmeyen) → Compose (`pageFromHash`, `src/utils/dashboard.ts`). `hashchange` dinlenir.
- **Sorgu parametresi `?c=<id>`**: Compose'da üzerinde çalışılan kampanyayı adlandırır (`src/utils/route.ts`: `campaignIdFromUrl`, `urlForCampaign`, `urlWithoutCampaign`). Yalnızca pozitif tam sayılar geçerlidir. Kampanya oluşturulunca `history.replaceState` ile yazılır; "Yeni kampanya" `history.pushState` ile `?c=`'yi silip `#/`'e gider.
- Dashboard'dan `#/` linkiyle Compose'a dönmek her zaman **yeni (temiz) kampanya** demektir (ComposeView `key` artırılarak yeniden kurulur).
- Dashboard satırına tıklamak `?c=<id>` ile tam sayfa gezinmesi yapar (`window.location.href`), sihirbaz kampanyanın bulunduğu adımda açılır.
- Sihirbaz içindeki adım değişimi URL'ye yansımaz (yalnızca bileşen durumu `view`).

### Genel kabuk öğeleri
- Marka logosu + "Campaign Composer" başlığı, `nav` ("Yeni kampanya", "Dashboard"), sağda LLM modu rozeti veya "API ulaşılamıyor" rozeti.
- Tarayıcı sekme başlığı: "Smart Campaign Composer" (`frontend/index.html`).
- Kalıcı istemci depolaması **yok** (localStorage/sessionStorage/IndexedDB kullanılmıyor); kalıcılık yalnızca URL (`?c=`) ve sunucu üzerinden.
- Açık/koyu tema `prefers-color-scheme` ile otomatik (`src/styles/tokens.css`), `prefers-reduced-motion` tüm animasyonları kapatır (`src/style.css`). Yazı tipi: Geist Variable (`main.ts`).
- Vite geliştirme sunucusu `/api` isteklerini backend'e proxy'ler (`vite.config.ts`, `API_PROXY_TARGET` veya `http://localhost:8081`).

## 2. Özellik tabloları

### 2.1 Genel kabuk (App)

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F01 | Marka başlığı | Tümü | `src/App.vue` | — | Görüntüleme | SVG logo (accent renkli kare + "M" çizgisi) ve "Campaign Composer" metni (`h1`). |
| F02 | "Yeni kampanya" sekmesi | Tümü | `src/App.vue` | `urlWithoutCampaign` (`src/utils/route.ts`), `history.pushState`, `composeKey` | Tıklama | `?c=` silinir, hash `#/` olur, ComposeView yeni `key` ile sıfırdan kurulur (eski kampanyadan hiçbir şey kalmaz). Compose'dayken tıklamak da sıfırlar. `aria-current="page"` aktif sayfada. |
| F03 | "Dashboard" sekmesi | Tümü | `src/App.vue` | hash `#/dashboard` | Tıklama | Normal link; `aria-current="page"` aktifken. |
| F04 | Hash tabanlı sayfa seçimi | Tümü | `src/App.vue`, `src/utils/dashboard.ts` (`pageFromHash`) | `hashchange` olayı | Adres çubuğu / geri-ileri | `#/dashboard` dışındaki her hash Compose açar. Dashboard → Compose geçişi her zaman yeni kampanya (F02 davranışı). |
| F05 | LLM modu rozeti | Tümü | `src/App.vue`, `src/components/ui/Badge.vue` | `fetchInfo` → `GET /api/info`, `llmLabel` (`src/api/info.ts`) | Görüntüleme / hover | Mock modunda "Mock modu" (warn tonu, noktalı) ve tooltip "Yanıtlar gerçek model yerine sahte (mock) modelden geliyor". Claude modunda "Claude (model-adı)" veya "Claude" (ok tonu). Bilgi gelene kadar rozet yok. |
| F06 | "API ulaşılamıyor" rozeti | Tümü | `src/App.vue` | `fetchInfo` hatası | Görüntüleme | `/api/info` başarısızsa danger tonlu, noktalı rozet. Uygulama yine çalışır (fallback değerler). |
| F07 | `/api/info` gelmeden önceki varsayılanlar | Compose, Dashboard | `src/utils/languages.ts` (`FALLBACK_LANGUAGES`, `DEFAULT_LANGUAGE`) | `info` prop'u | — | Diller varsayılan **tr / en / de** (Türkçe, English, Deutsch); referans dil `tr`. `limits` gelmeden mesaj editörü gösterilmez. Değişken listesi boş olur. |
| F08 | Açık/koyu tema | Tümü | `src/styles/tokens.css`, `src/style.css` | CSS `prefers-color-scheme` | Sistem ayarı | Koyu modda tüm renk token'ları değişir; manuel tema anahtarı yok. |
| F09 | Hareket azaltma | Tümü | `src/style.css`, `AnimatedNumber.vue`, `LanguageTabs.vue` | `prefers-reduced-motion` | Sistem ayarı | Animasyon/geçişler ~0 ms; sayılar animasyonsuz atlar; yanıp sönen noktalar durur. |
| F10 | Mobil yerleşim | Tümü | `src/style.css`, view stilleri | CSS `@media (max-width: 640px)` | — | Nav üst barın altına iner; sihirbazın ana butonu tam genişlik; olgu listeleri (dl) tek sütun; dashboard'da Kitle/Diller sütunları gizlenir (`hide-sm`). |
| F11 | Sekme başlığı | Tümü | `frontend/index.html` | — | — | "Smart Campaign Composer". |

### 2.2 Compose — sihirbazın ortak davranışı

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F12 | URL'deki kampanyayı açma (`?c=`) | Compose | `src/views/ComposeView.vue` | `campaignIdFromUrl`, `getCampaign` → `GET /api/campaigns/{id}` | Sayfayı `?c=12` ile açma / yenileme | Yüklenene kadar hiçbir panel gösterilmez (yanlış adım yanıp sönmez; `ready`). Brief, kampanya ve kayıtlı metinler (tüm diller) yüklenir; sihirbaz `currentStep` ile kampanyanın bulunduğu adımda açılır. |
| F13 | Kampanya bulunamadı / yüklenemedi | Compose | `src/views/ComposeView.vue` | `ApiError.status` | — | 404 → `?c=` sessizce silinir, boş brief açılır. Diğer hata → üstte `role=alert` uyarı (`data-testid=load-error`): sunucu mesajı veya "Kampanya yüklenemedi. Sayfayı yenileyin."; URL korunur (yenileme tekrar dener). |
| F14 | Adım çubuğu (StepBar) | Compose | `src/components/StepBar.vue`, `src/utils/steps.ts`, `src/utils/wizard.ts` | `currentStep`, `reachableSteps` | Adıma tıklama | 5 adım: Brief, Kitle, Mesajlar, Onay, Gönderim. Tamamlanan adımlarda onay işareti (+ ekran okuyucu için "(tamamlandı)"), diğerlerinde sıra numarası. Görünen adım `aria-current="step"`. Ulaşılmamış adımlar devre dışı (ileri atlama yok); görüntülenen adımın düğmesi de devre dışı. İş sürerken (oluşturma/üretim/kaydetme/onay) tıklama yok sayılır. Tamamlanmış kampanyada son adım da "tamamlandı". |
| F15 | İlerleme kuralı (hangi adıma kadar açılabilir) | Compose | `src/utils/steps.ts` (`currentStep`) | `campaign.status`, `segment.status`, akış, kaydedilmemiş değişiklik | — | Kampanya yok → Brief. Segment `ok` değil veya `draft` → Kitle. `scheduled/approved/sending/completed/cancelled/failed` → Gönderim. Akış sürüyor veya kaydedilmemiş düzenleme → Mesajlar. `content_ready` → Onay. Aksi halde içerik varsa Mesajlar, yoksa Kitle. |
| F16 | Adımlar arası kayma animasyonu | Compose | `src/views/ComposeView.vue` (`<Transition>`), `direction` | `slide` | İleri/geri gitme | İleri → panel sola kayar, geri → sağa (`slide-left` / `slide-right`, out-in). Her geçişte sayfa en üste kaydırılır; onay sorusu (`confirming`) sıfırlanır. Adım verileri ana bileşende tutulduğu için gidip gelince kaybolmaz. |
| F17 | Geri / Vazgeç butonu | Compose | `src/views/ComposeView.vue`, `Button.vue` | `prevStep` | Tıklama | Brief adımında gizli. İş sürerken devre dışı. Onay sorusu açıkken etiketi "Vazgeç" ve yalnızca soruyu kapatır; değilse "Geri" bir önceki adıma gider. `data-testid=back`. |
| F18 | Ana eylem butonu (alt çubuk) | Compose | `src/views/ComposeView.vue`, `src/utils/wizard.ts` (`barAction`) | `bar`, `barLabel`, `barDisabled` | Tıklama | Adıma göre: Brief → "Kitleyi çıkar" (kampanya yoksa veya brief değiştiyse ve dondurulmamışsa) yoksa "İleri"; Kitle → "Mesajları üret" (içerik yoksa ve dondurulmamışsa) yoksa "İleri"; Mesajlar → "Onayla" (dondurulmuşsa "İleri"); Onay → "Gönder"/"Zamanla", sonra "Evet, gönder"/"Evet, zamanla" (dondurulmuşsa "İleri"); Gönderim → "Dashboard'a git". Yükleniyor etiketleri: "Kitle çıkarılıyor…", "Üretiliyor…", "Kaydediliyor…", "Gönderim başlıyor…"/"Zamanlanıyor…". `data-testid`: `approve` (ask), `confirm-approve` (send). "İleri" Brief'te kampanya yokken veya iş sürerken devre dışı. |
| F19 | Dondurma (onay sonrası salt okunur) | Compose | `src/views/ComposeView.vue` | `frozen` (status ∈ scheduled/approved/sending/completed/cancelled/failed) | Önceki adımlara dönme | Onaydan sonra tüm önceki adımlar açılabilir ama değiştirilemez; ana buton yalnızca "İleri". Brief/Mesajlar/Onay kartlarında "Onaylandı" rozeti. |

### 2.3 Compose — Adım 1: Brief

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F20 | Brief metin alanı | Brief | `src/views/ComposeView.vue`, `Card.vue` | `brief` ref | Yazma | 6 satır, `maxlength=2000`, yer tutucu "Kime, ne zaman, ne göndermek istediğinizi doğal dille yazın…". Erişilebilir gizli etiket "Kampanya brief'i". Oluşturma sırasında `disabled`, dondurulmuşsa `readonly`. |
| F21 | "Örnek brief" butonu | Brief | `src/views/ComposeView.vue` | `EXAMPLE_BRIEF` | Tıklama | Alanı örnek metinle doldurur ("Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."). Kart başlığında ghost/sm, `FileText` ikonu; oluşturma sırasında devre dışı; dondurulmuşsa yerine "Onaylandı" rozeti. |
| F22 | Ctrl+Enter ile gönderme | Brief | `src/views/ComposeView.vue` | `@keydown.ctrl.enter` | Klavye | Yalnızca ana aksiyon "Kitleyi çıkar" ve buton etkinken çalışır. |
| F23 | "Kitleyi çıkar" (kampanya oluşturma) | Brief | `src/views/ComposeView.vue` | `createCampaign` → `POST /api/campaigns` `{brief}` | Ana buton | En az 5 karakter (trim) gerekir, aksi halde devre dışı. Önceki kampanya, akış, onay sonucu, taslaklar ve aktif dil (`tr`) sıfırlanır. Başarıda `?c=<id>` adrese yazılır (`replaceState`) ve Kitle adımına geçilir. Hata durumunda `?c=` silinir, Brief'te kalınır. |
| F24 | Brief değişti uyarısı | Brief | `src/views/ComposeView.vue` | `briefChanged` | Yazma | Kampanya varken metin (trim) farklıysa: "Brief değişti: kitle ve mesajlar yeniden oluşturulacak." Değişmemiş brief'e dönmek kampanyayı yeniden kurmaz (buton "İleri" olur). |
| F25 | Onaylı brief notu | Brief | `src/views/ComposeView.vue` | `frozen` | — | "Onaylanan kampanyanın brief'i değiştirilemez." |
| F26 | Oluşturma hatası + ayrıntılar | Brief | `src/views/ComposeView.vue` | `createError: ApiError`, `details.attempts` | `<details>` açma | `role=alert` hata metni; sunucu deneme listesi döndürdüyse "Ayrıntılar" açılır listesi: "Deneme N: sorun1; sorun2". Ağ hatası metni: "Sunucuya ulaşılamadı. Bağlantınızı kontrol edin." |

### 2.4 Compose — Adım 2: Kitle

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F27 | Kitle yükleniyor durumu | Kitle | `src/views/ComposeView.vue` | `campaign.segment` yok | — | "Kitle çıkarılıyor…" kartı (`role=status`). |
| F28 | Kitle açıklaması ve not | Kitle | `src/components/SegmentCard.vue` | `segment.explanation`, `segment.note`, `sentenceCase` | Görüntüleme | Açıklama Türkçe kurallarıyla büyük harfle başlar. `note` varsa bilgi kutusu (`data-testid=audience-note`, ör. neden herkesin seçildiği). Kart `aria-live="polite"`. |
| F29 | Kural koşulları çipleri | Kitle | `SegmentCard.vue`, `src/utils/formatRule.ts` (`describeCondition`) | `segment.conditions`, `segment.match` | Görüntüleme | Her koşul bir rozet; aralarında "ve" (all) / "veya" (any). Desteklenen alanlar: şehir (dahil/dışında), son sepete ekleme günü, son sipariş günü, "hiç sipariş vermemişler dahil", cinsiyet (kadın/erkek), tüm müşteriler, sipariş sayısı, toplam harcama (TL), sepete eklemeden sonra satın alma; bilinmeyen alan ham gösterilir. |
| F30 | Kitle sayıları (stat kutuları) | Kitle | `SegmentCard.vue`, `ui/StatTile.vue`, `AnimatedNumber.vue`, `src/utils/funnel.ts` | `segment.audience` | Görüntüleme | "Eşleşen kişi" (ipucu: "N kişiye ulaşılamıyor" / "Hepsine ulaşılabiliyor"), "SMS ile ulaşılabilir", "E-posta ile ulaşılabilir". Sayılar animasyonlu. Ekran okuyucular için gizli özet cümlesi (eşleşen, SMS, e-posta, ulaşılamayan; test id'leri `matched`, `sms-reachable`, `email-reachable`, `unreachable`). |
| F31 | Kanal dağılımı çubuğu ve lejant | Kitle | `SegmentCard.vue`, `ui/ProgressBar.vue` | `funnel()` | Görüntüleme | Çok parçalı çubuk: iki kanal (accent), yalnız SMS, yalnız e-posta; maksimum = eşleşen. Lejant: "İki kanaldan N", "Yalnız SMS N", "Yalnız e-posta N", varsa "Ulaşılamayan N". `aria-label="Kanallara göre ulaşılabilirlik"`. |
| F32 | Teklif satırı | Kitle | `SegmentCard.vue`, `src/utils/formatOffer.ts` (`describeOffer`) | `segment.offer` | Görüntüleme | Yalnızca teklif varsa (`kind != none`). Biçimler: "%N indirim", "N TL indirim", kademeli "Her X TL alışverişe N TL indirim", "Ücretsiz kargo", "Kupon kodu KOD", üst sınır "(en fazla N TL)" / "(toplamda en fazla N TL)", birleşik teklifler "+ ücretsiz kargo / + ekstra N TL indirim", koşullar parantezde: kupon kodu, "N TL ve üzeri alışverişte", "son kullanım GG.AA.YYYY". Sayılar tr-TR biçiminde. |
| F33 | Mesaj dili dağılımı | Kitle | `SegmentCard.vue`, `languageSummary` | `segment.languages` | Görüntüleme | "Mesaj dili" satırında dil başına rozet "Türkçe 120"; kimsenin konuşmadığı diller atlanır, dil sırası `/api/info` sırasıdır. |
| F34 | Ulaşılamayan kişi ipucu ve sıfır eşleşme uyarısı | Kitle | `SegmentCard.vue` | `funnel.unreachable`, `audience.matched` | — | Ulaşılamayan > 0: "İzin vermeyen ya da telefonu / e-postası olmayan kişilere mesaj gönderilmez." Eşleşen 0: uyarı "Bu kurala uyan kimse yok. Brief'i değiştirmeyi deneyin." |
| F35 | Netleştirme sorusu (needs_clarification) | Kitle | `SegmentCard.vue` | `segment.question`, `segment.offer` | Görüntüleme | "Biraz daha bilgi lazım" kutusu (`role=status`, soru ikonu), soru metni, teklif anlaşıldıysa "Anladığım teklif" rozeti + açıklama, alt ipucu "Ya da brief'i kime göndereceğinizi yazarak güncelleyip tekrar deneyin." Sihirbaz Kitle adımında bekler (Mesajlar'a geçilemez). |
| F36 | "Tüm müşterilere gönder" tek tık cevabı | Kitle | `SegmentCard.vue`, `src/utils/sendToAll.ts`, `ComposeView.vue` (`sendToAll`) | `segment.suggestion === 'all_customers'`, `createCampaign` | Tıklama | Brief'in sonuna "Tüm müşterilere gönder." cümlesi eklenir (zaten varsa tekrar eklenmez) ve kampanya yeniden oluşturulur. İpucu: "İzin vermemiş olanlara yine de gönderilmez." Oluşturma sırasında veya dondurulmuşken devre dışı. `data-testid=send-to-all`. |
| F37 | Desteklenmeyen brief | Kitle | `SegmentCard.vue` | `segment.status === 'unsupported'` | Görüntüleme | Kırmızı kutu "Bu brief desteklenmiyor" (`role=alert`) + açıklama. |
| F38 | "Mesajları üret" (ana buton) | Kitle | `ComposeView.vue` (`generate`) | `useGenerationStream` | Ana buton | Yalnız segment hazırsa (`ok` ve status ≠ draft) etkin. Mesajlar adımına geçer ve SSE akışını başlatır (F39). İçerik zaten varsa buton "İleri" olur. |

### 2.5 Compose — Adım 3: Mesajlar

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F39 | Canlı mesaj üretimi (SSE) | Mesajlar | `src/composables/useGenerationStream.ts`, `src/composables/streamState.ts`, `src/api/sse.ts` | `streamPost` → `POST /api/campaigns/{id}/messages/stream` (Accept: text/event-stream); olaylar `attempt`, `delta`, `retry`, `validated`, `fallback`, `done`, `error` | Üret / Yeniden üret | Diller paralel yazılır; her dilin sekmesi canlı taslağı gösterir (harf harf). Yeni deneme taslağı sıfırlar. Tüm diller sonuçlanınca doğrulanmış (veya şablon) metinler düzenlenebilir olur, sunucu sorunları/kayıt hatası temizlenir, kampanya tekrar çekilir (`content_ready`). Yeni üretim önceki akışı iptal eder (AbortController); bileşen kapanınca akış iptal edilir. Akış final olay olmadan kapanırsa: "Bağlantı beklenmedik şekilde kapandı. Tekrar deneyin." |
| F40 | Deneme rozeti | Mesajlar | `ComposeView.vue`, `src/utils/preview.ts` (`attemptLabel`) | `stream.langs[active].attempt` | — | Akış sürerken kart başlığında: "<Dil adı>: üretiliyor 1/3" (accent) veya "düzeltiliyor 2/3" (warn). `role=status`, `data-testid=attempt`. |
| F41 | Yeniden deneme notları | Mesajlar | `ComposeView.vue`, `retryNotes` | `retry` olayı | — | Uyarı kutusu: "Önceki deneme sınırlara uymadığı için model düzeltiyor:" + dil başına madde "• Türkçe: sorun". Dil sonuçlanınca o dilin notları kalkar. |
| F42 | Şablon (fallback) bildirimi | Mesajlar | `ComposeView.vue` | `stream.fallbackReason` | — | `role=alert` uyarı: neden (`llm_refused` → "Model bu isteği yanıtlamayı reddetti.", `llm_unavailable` → "Model servisine ulaşılamadı.", diğer → "Model 3 denemede sınırlara uyan bir metin üretemedi.") + "Hazır bir şablon yerleştirildi; lütfen metni düzenleyin. Şablon da tüm sınırlara uyar." |
| F43 | Akış hata bildirimi | Mesajlar | `ComposeView.vue`, `useGenerationStream.ts` | `stream.error` | — | `role=alert` hata; ağ hatası "Sunucuya ulaşılamadı…", akış desteklenmiyor "Tarayıcı akış okumayı desteklemiyor.", beklenmeyen "Beklenmeyen bir hata oluştu.", sunucunun JSON hata mesajı. |
| F44 | Dil sekmeleri | Mesajlar | `src/components/LanguageTabs.vue` | `activeLang`, `tabStatus` | Sekmeye tıklama | `role=tablist` "Mesaj dili"; her sekme kod (TR/EN/DE) + yerel ad, `aria-selected`, `data-testid=lang-<code>`. Durum noktası (öncelik sırası): yazılıyor (yanıp sönen, "Yazılıyor"), N sorun (kırmızı, "N sorun"), diğer dillerle uyuşmuyor (kırmızı), kaydedilmemiş düzenleme (ayrı renk). Sekme değişince editör o dilin taslağını gösterir. |
| F45 | Diller arası tutarsızlık uyarısı | Mesajlar | `ComposeView.vue` | `campaign.mismatches` + kayıt reddindeki `language_mismatch` sorunları | — | `role=alert`, `data-testid=mismatch`: "Diller aynı şeyi söylemiyor. İndirim oranı, tutar, tarih ve {{değişkenler}} her dilde aynı olmalı:" + madde listesi. Varsa onay engellenir. |
| F46 | SMS alanı + halka sayaç | Mesajlar | `src/components/MessageEditor.vue`, `src/components/CharCounter.vue` | `limits.sms_max` (backend: 160) | Yazma | 4 satırlık textarea; sayaç halka biçiminde "kullanılan / 160". Uzunluk değişkenler sabit paylarıyla sayılır (`effectiveLength`), karakter = Unicode kod noktası. |
| F47 | E-posta konusu alanı + çubuk sayaç | Mesajlar | `MessageEditor.vue`, `CharCounter.vue` | `limits.subject_max` (backend: 80) | Yazma | Tek satır `input`; çubuk sayaç "kullanılan / 80". Satır sonu içeremez. |
| F48 | E-posta metni alanı | Mesajlar | `MessageEditor.vue` | `limits.email_body_max` (backend: 5000) | Yazma | 9 satırlık textarea; görünür sayaç yok, sınır yalnızca doğrulamada. |
| F49 | Sayaç renk seviyeleri | Mesajlar | `CharCounter.vue`, `meterLevel` | — | — | Normal < %90, sarı ≥ %90, kırmızı > sınır. `role=status`, `aria-label` "N / M karakter" veya "…, sınır aşıldı". |
| F50 | Anlık istemci doğrulaması | Mesajlar | `MessageEditor.vue`, `src/utils/limits.ts` (`checkContent`) | yerel | Yazma | Alan başına mesajlar (her dil için ayrı): "X boş olamaz.", "E-posta konusu tek satır olmalı; satır sonu içeremez.", "X: bilinmeyen değişken {{ad}}. Kullanılabilir: …", "X: hatalı süslü parantez…", "X en fazla N karakter olmalı; değişkenler paylarıyla sayıldığında M karakter (K fazla)." Hatalı alanda `aria-invalid`. Herhangi bir dilde sorun varsa kaydetme engellenir. Aynı mesaj tekrar gösterilmez. |
| F51 | Sunucu doğrulama sorunları | Mesajlar | `MessageEditor.vue`, `ComposeView.vue` (`save`) | `ApiError.details.report.issues` | Kaydetme reddi | Alan altında gösterilir (görünen dile ait olanlar); ilk sorunlu dile otomatik geçilir; sekmede sorun sayısına eklenir. |
| F52 | Kalite uyarıları (engellemeyen) | Mesajlar | `MessageEditor.vue` | `content.report.warnings` (kayıtlı metin) | — | Alan altında "⚠ mesaj" (`role=status`), ör. teklif eksik, tekrarlanan kelime. Akış sürerken gösterilmez. |
| F53 | Kişiselleştirme değişkenleri listesi | Mesajlar | `MessageEditor.vue` | `info.variables` (`name`, `budget`, `fallback(s)`) | Hover | "Kişiselleştirme (karakter sınırlarına sabit pay olarak sayılır):" + `{{first_name}} = 10` gibi kodlar; tooltip: "N karakter pay ayrılır; kişinin değeri bundan uzunsa veya boşsa “<dilin yedek kelimesi>” yazılır". |
| F54 | SMS telefon önizlemesi | Mesajlar | `src/components/PhonePreview.vue`, `fillSample` | taslak | — | Telefon ekranı (09:41, 5G, "MARKA" gönderen, "şimdi"); `{{first_name}}` → "Elif", `{{city}}` → "İstanbul"; bilinmeyen değişkenler aynen. Model yazarken yanıp sönen imleç; boşken "Mesaj üretilince burada görünür." Önizleme yapışkan (sticky). |
| F55 | E-posta önizlemesi | Mesajlar | `src/components/EmailPreview.vue` | taslak | — | "Gelen kutusu" penceresi; konu (boşken "Konu burada görünür"), gönderen "Marka <merhaba@marka.example>", "Alıcı: Elif", gövde (boşken "E-posta metni üretilince burada görünür."). Yazılan alanda (konu veya gövde) imleç. Üstte not: "Önizleme: kişi adı yerine örnek olarak “Elif” kullanılır." |
| F56 | Yazma sırasında salt okunur editör | Mesajlar | `MessageEditor.vue` | `readonly = streaming || frozen` | — | Akış sürerken veya onaydan sonra alanlar düzenlenemez. İmleç, son metin alan alana (`activeField`) göre konur. |
| F57 | "Yeniden üret" butonu | Mesajlar | `ComposeView.vue` | `generate` | Tıklama | Kart başlığında (ghost/sm, `RefreshCw`); yalnız içerik varken, akış yokken, segment hazırken ve dondurulmamışken. Mevcut taslakların üzerine yeni üretim yapar. |
| F58 | Boş durum | Mesajlar | `ComposeView.vue` | `hasContent=false`, akış yok | — | "Henüz mesaj yok." + (segment hazırsa) "Mesajları üret" birincil butonu. `limits` yoksa da bu durum görünür. |
| F59 | Kaynak ve kaydedilmemiş not | Mesajlar | `ComposeView.vue` | `source` (`llm`/`fallback`/`edited`), `unsaved` | — | "Kaynak: model / hazır şablon / elle düzenlendi." + kaydedilmemiş düzenleme varsa "Kaydedilmemiş düzenleme var; “Onayla” kaydeder." |
| F60 | "Onayla" (kaydet ve Onay'a geç) | Mesajlar | `ComposeView.vue` (`save`) | `saveContent` → `PUT /api/campaigns/{id}/content` `{languages:{tr,en,de}}` | Ana buton | Tüm diller birlikte kaydedilir. Kaydedilmemiş değişiklik yoksa yalnızca Onay'a geçer. Devre dışı: iş sürerken, içerik yokken, kaydedilmemiş + yerel sorun varken, ya da değişiklik yok ve status ≠ `content_ready`. Etiket "Kaydediliyor…". Başarıda akış durumu sıfırlanır. Hata: `role=alert` mesaj (kart altında), Onay'a geçilmez. |
| F61 | Onaylı mesajlar rozeti | Mesajlar | `ComposeView.vue` | `frozen` | — | Akış yoksa ve dondurulmuşsa "Onaylandı" rozeti. |

### 2.6 Compose — Adım 4: Onay

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F62 | Onay için ön koşul boş durumu | Onay | `ComposeView.vue` | status ≠ `content_ready` ve dondurulmamış | — | "Onaya geçmek için önce mesajları üretip kaydedin." |
| F63 | Gönderim zamanı seçimi (Hemen / Zamanla) | Onay | `src/components/ApprovalCard.vue`, `src/components/ui/SegmentedControl.vue` | `sendMode` | Tıklama / ok tuşları | `role=radiogroup` "Gönderim zamanı"; seçenekler "Hemen" (`Send` ikonu, `mode-now`) ve "Zamanla" (`Clock`, `mode-later`). Klavye: ←/→/↑/↓ seçimi döngüsel değiştirir, Home/End; yalnız seçili seçenek Tab ile odaklanır. Onay sorusu açıkken devre dışı. Dondurulmuşsa bölüm gizli. |
| F64 | Tarih-saat seçici | Onay | `src/components/ui/DateTimeField.vue`, `src/utils/calendar.ts` | `sendWhen` ("YYYY-MM-DDTHH:mm", yerel saat) | Tıklama / klavye | Yalnız "Zamanla"da. Tetik düğmesi "GG.AA.YYYY SS:dd" veya "Tarih ve saat seçin" (`data-testid=send-at`, `aria-haspopup=dialog`, `aria-expanded`, geçersizse `aria-invalid` + kırmızı). Açılır pencere: ay başlığı ("Kasım 2026", `aria-live`), önceki/sonraki ay okları (aralık dışına çıkılamaz), Pzt'den başlayan 6 haftalık 42 hücre, bugün işaretli, seçili gün `aria-pressed`, aralık dışı günler devre dışı; saat (00–23) ve dakika (5'er dakika + mevcut değer) sütunları, aralık dışındakiler devre dışı, açılınca seçili değere kaydırılır; gün seçince saat korunur (yoksa 09:00), değer min/max'a sıkıştırılır. "Tamam" (değer yoksa devre dışı) kapatır. Esc ve dışarı tıklama kapatır; odak tetiğe döner. |
| F65 | Hızlı zaman kısayolları | Onay | `DateTimeField.vue`, `src/utils/schedule.ts` (`quickTimes`) | — | Tıklama | "1 saat sonra" (5 dakikaya yuvarlanır), "Bu akşam 18:00" (yalnız ≥1 saat varsa), "Yarın 09:00". Aralık dışındaki kısayol devre dışı. Seçince pencere kapanır. |
| F66 | Varsayılan zaman | Onay | `ComposeView.vue` (`defaultSendTime`) | — | — | Bir sonraki tam saat, en az 1 saat sonrası. |
| F67 | Zaman sınırı ipucu | Onay | `ApprovalCard.vue`, `scheduleBounds` | `segment.offer.expires_at` | — | "En fazla 30 gün sonrası" veya teklif daha erken bitiyorsa "En geç GG.AA.YYYY (teklifin son günü)". Sınırlar 30 sn'de bir saate göre güncellenir; en erken = şimdi + 1 dk. |
| F68 | Zaman doğrulaması | Onay | `ComposeView.vue`, `checkSchedule` | `clockNow` (15 sn'de bir güncellenir) | Zaman seçme | `role=alert` (`data-testid=schedule-error`): "Bir gönderim zamanı seçin.", "Gönderim zamanı en az 1 dakika sonrası olmalı.", "Gönderim zamanı teklifin son kullanma tarihinden (GG.AA.YYYY) sonra olamaz." (son gün İstanbul saatiyle günün sonu), "Gönderim zamanı en fazla 30 gün sonrası olabilir." Hata varsa onay butonu devre dışı. |
| F69 | Onay özeti | Onay | `ApprovalCard.vue`, `AnimatedNumber.vue` | `segment` | — | Teklif (teklif yoksa "Brief’te teklif yok; mesajlarda indirim yazılmayacak"), Kitle açıklaması, Alıcılar: benzersiz kişi sayısı (SMS + e-posta − ikisi; sıfırdan sayarak animasyon) "kişi", "SMS N", "E-posta N"; Diller rozetleri (`approval-languages`). |
| F70 | Tahmini sayılar notu (zamanlanmış) | Onay | `ApprovalCard.vue` | `mode === 'later'` | — | Başlıkta "Tahmini sayılar" rozeti ve not (`later-note`): "Kitle gönderim anında yeniden belirlenir; kişi sayısı değişebilir." |
| F71 | Uyarı listesi | Onay | `ApprovalCard.vue` | tüm dillerin `report.warnings` | — | "Uyarılar" listesi; her madde dil kodu (TR/EN/DE) kalın + mesaj; engellemez. |
| F72 | İki adımlı onay | Onay | `ApprovalCard.vue`, `ComposeView.vue` (`approve`) | `confirming`, `canApprove`, `approveCampaign` → `POST /api/campaigns/{id}/approve` (gövde yok = hemen; `{scheduled_at}` RFC3339 = zamanlı) | "Gönder"/"Zamanla" → "Evet, gönder"/"Evet, zamanla" | İlk tıklama soruyu açar (fade animasyonu, `role=alertdialog`): "Emin misiniz?" + hemen: "Mesajlar hemen gönderilir. Onaydan sonra metin ve kitle değişmez." / zamanlı: "Mesajlar seçtiğiniz zamanda kendiliğinden gönderilir. Başlamadan önce iptal edebilirsiniz." "Vazgeç" (Geri butonu) soruyu kapatır. `canApprove`: status `content_ready`, zaman geçerli, tüm dillerin kayıtlı metni var, tutarsızlık yok, akış/kaydetme/onay sürmüyor, kaydedilmemiş değişiklik yok. Başarıda kampanya tekrar çekilir ve Gönderim adımına geçilir. |
| F73 | Onay hatası | Onay | `ApprovalCard.vue` | `approveError` | — | `role=alert` hata metni kartın altında. |
| F74 | Onaylanmış salt okunur kart | Onay | `ApprovalCard.vue` | `frozen` | — | "Onaylandı" rozeti; zaman seçimi gizli; özet görünür. |

### 2.7 Compose — Adım 5: Gönderim

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F75 | Durum rozeti | Gönderim | `ComposeView.vue`, `statusLabel` | `campaign.status` | — | Metin: Kitle hazır / Onay bekliyor / Zamanlandı / Onaylandı / İptal edildi / Başlatılamadı / Gönderiliyor / Tamamlandı (taslakta Desteklenmiyor / Soru bekliyor / Taslak). Ton: completed ok, failed danger, cancelled neutral, diğerleri accent. |
| F76 | Zamanlanmış kampanya bilgileri | Gönderim | `ComposeView.vue`, `formatWhen`, `untilText`, `countsLine` | `campaign.scheduled_at`, `segment.audience`, `segment.languages` | — | `data-testid=scheduled`: "Gönderim zamanı" (tr-TR kısa tarih-saat) + geri sayım ("2 sa 5 dk sonra", "3 gün 4 sa sonra", "şimdi başlıyor"; 15 sn'de bir yenilenir), "Tahmini alıcı: N SMS, M e-posta", "Tahmini diller: TR 171 · EN 72 · DE 42". İpucu: "Kitle gönderim anında belirlenir; kimsenin burada olması gerekmez." |
| F77 | Zamanlanmış gönderimi iptal (sihirbaz) | Gönderim | `ComposeView.vue`, `ui/Modal.vue` | `cancelCampaign` → `POST /api/campaigns/{id}/cancel` | "Gönderimi iptal et" → modal | Buton (secondary, `XCircle`, `data-testid=cancel`) modal açar: başlık "Zamanlanmış gönderim iptal edilsin mi?", açıklama "Kampanya <zaman> için zamanlanmıştı. İptal edersen hiçbir mesaj gönderilmez." Butonlar "Vazgeç" (ghost) ve "Gönderimi iptal et" (danger, yükleniyor spinner, `confirm-cancel`). Hata (ör. 409 başlamış) → `role=alert` mesaj, kampanya yeniden çekilir, modal kapanır. |
| F78 | İptal edildi bildirimi | Gönderim | `ComposeView.vue` | status `cancelled` | — | `data-testid=cancelled`: "Zamanlanmış gönderim iptal edildi; hiçbir mesaj gönderilmedi." |
| F79 | Başlatılamadı bildirimi | Gönderim | `ComposeView.vue` | status `failed`, `schedule_error` | — | `data-testid=failed`: "Zamanlanmış gönderim başlatılamadı: <neden | bilinmeyen neden>. Hiçbir mesaj gönderilmedi." |
| F80 | Onay anı sayı farkı bildirimi | Gönderim | `ComposeView.vue`, `approvalDifference` | `approved.queued` vs `segment.audience` | — | Farklıysa: "Ekranda X SMS gösterilmişti; onay anında Y SMS kuyruğa alındı. … (veriler değişmiş olabilir)." Yalnız bu oturumda onaylandıysa. |
| F81 | Genel ilerleme çubuğu ve başlık | Gönderim | `src/components/DeliveryProgress.vue`, `ProgressBar.vue` | `campaign.delivery` | — | Çubuk: gönderilen (yeşil) + başarısız/atlanan (kırmızı) / toplam; toplam 0 ise tam görünür. Başlık (`role=status`, `delivery-caption`): "N / M mesaj işlendi" + bitmediyse "– bu sayfa kendiliğinden güncellenir". |
| F82 | Kanal tablosu | Gönderim | `DeliveryProgress.vue` | `delivery.sms`, `delivery.email` | — | Satırlar SMS / E-posta (renk işaretli); sütunlar Gönderilen, Başarısız (>0 ise kırmızı), Atlanan, Bekleyen. `delivery-table`, `delivery-sms`, `delivery-email`. |
| F83 | Dile göre gönderilen | Gönderim | `DeliveryProgress.vue` | `delivery.languages` | — | "Dile göre gönderilen" + rozetler "TR 120/171" (bekleyen varken) veya "TR 171"; kimseye gitmeyen diller yok. |
| F84 | Başarısızlık nedenleri | Gönderim | `DeliveryProgress.vue`, `src/utils/failures.ts` | `delivery.failures` (kanallar birleştirilip büyükten küçüğe) | — | Kodlar → Türkçe: rejected, gave_up, too_old, too_long, other (bilinmeyen kod = other). |
| F85 | Tamamlandı mesajı | Gönderim | `DeliveryProgress.vue` | status `completed` | — | Onay ikonu + "Gönderim tamamlandı." (`role=status`). |
| F86 | Canlı ilerleme yoklaması | Gönderim | `ComposeView.vue` (`startPolling`), `shouldPoll` | `getCampaign` her 1 sn | — | Durum `scheduled`/`approved`/`sending` iken saniyede bir; `completed` (veya diğer) olunca durur. Zamanlanmış kampanya başladığında ekran kendiliğinden ilerleme görünümüne geçer. Kaçan tik yok sayılır. Sayfadan çıkınca durur. |
| F87 | Henüz onaylanmadı boş durumu | Gönderim | `ComposeView.vue` | `delivery` yok | — | "Kampanya henüz onaylanmadı." |
| F88 | "Dashboard'a git" | Gönderim | `ComposeView.vue` | hash `#/dashboard` | Ana buton | Gönderim adımında ana buton. |

### 2.8 Dashboard

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F89 | Başlık ve "Yeni kampanya" butonu | Dashboard | `src/views/DashboardView.vue` | hash `#/` | Tıklama | "Kampanyalar" başlığı; birincil "Yeni kampanya" (`Plus`, `data-testid=new-campaign`) → temiz Compose. |
| F90 | Liste yoklaması | Dashboard | `DashboardView.vue` | `listCampaigns` → `GET /api/campaigns?limit=100` her 2 sn | — | Yeni → eski sıralı. Altta "Sayfa her 2 saniyede güncellenir." Sayfadan çıkınca durur. |
| F91 | Liste güncellenemedi uyarısı | Dashboard | `DashboardView.vue` | `error` | — | `role=alert`: "Liste güncellenemedi (<hata>). Gösterilen bilgi eski olabilir." Son başarılı liste ekranda kalır; sonraki başarılı yoklamada kalkar. |
| F92 | Özet kutucukları | Dashboard | `DashboardView.vue`, `ui/StatTile.vue`, `overallTotals` | tüm kampanyaların `delivery` toplamı | — | "Gönderilen" (ok tonu, "SMS ve e-posta birlikte"), "Başarısız" (>0 ise danger + "Nedenleri satırlarda", değilse "Sorun yok"), "Kuyrukta" ("Gönderilmeyi bekliyor"), "Hız" (1 ondalık, "/sn", "Son 10 saniye"; yalnız kuyrukta mesaj varken, aksi 0). Sayılar animasyonla değişir. `data-testid`: summary, total-sent, total-failed, total-pending, rate. |
| F93 | Gönderim hızı hesabı | Dashboard | `src/utils/rate.ts` | yoklama örnekleri (son 10 sn penceresi) | — | (son gönderilen − ilk) / saniye; <2 örnek veya azalma → 0. |
| F94 | Yükleniyor durumu | Dashboard | `DashboardView.vue` | `campaigns === null` | — | "Yükleniyor…" (hata yoksa). |
| F95 | Boş durum | Dashboard | `DashboardView.vue` | liste boş | Tıklama | `data-testid=empty`: gelen kutusu ikonu, "Henüz kampanya yok", "Bir brief yazarak ilk kampanyayı oluşturun.", "Yeni kampanya" butonu. |
| F96 | Kampanya tablosu ve satır açma | Dashboard | `src/components/CampaignTable.vue` | `urlForCampaign` | Satıra / başlığa tıklama | `data-testid=campaign-table`, satır `row-<id>`. Sütunlar: Kampanya, Durum, Kitle, Diller, İlerleme, Zaman, İşlemler (gizli başlık). Satırın herhangi bir yerine tıklama `?c=<id>` adresine gider (içteki link/buton kendi işini yapar). Başlık gerçek link (yeni sekmede açılabilir). |
| F97 | Kampanya başlığı ve kitle alt satırı | Dashboard | `CampaignTable.vue`, `briefTitle`, `sentenceCase` | `brief`, `explanation` | Hover | Brief tek satıra indirilir, kelimede kesilip "…" (en fazla 64 karakter), Türkçe büyük harfle başlar; tooltip tam brief. Altta kitle açıklaması (`card-audience`, tooltip tam metin). |
| F98 | Durum rozeti (satır) | Dashboard | `CampaignTable.vue`, `statusBadge` | `status`, `segment_status` | — | Tamamlandı (ok), Gönderiliyor/Onaylandı/Zamanlandı (accent), İptal edildi (neutral), Başlatılamadı (danger), Desteklenmiyor/Soru bekliyor (warn), segment_ready/content_ready/draft → Taslak (neutral). Gönderiliyor ve Zamanlandı'da nokta. |
| F99 | Kitle sütunu | Dashboard | `CampaignTable.vue` | `matched` | — | tr-TR binlik biçim; yoksa "–". Mobilde gizli. |
| F100 | Diller sütunu | Dashboard | `CampaignTable.vue`, `languageCells` | `delivery.languages` veya `audience_languages` | — | Gönderim başladıysa dil başına gönderilen ("TR 120/171", bitince "TR 171"), öncesinde kitle tahmini; boşsa "–". Mobilde gizli. `card-languages`. |
| F101 | İlerleme sütunu | Dashboard | `CampaignTable.vue`, `rowProgress`, `progressTitle`, `ProgressBar.vue` | `delivery` | Hover | Toplam > 0: küçük çubuk (gönderilen yeşil, başarısız+atlanan kırmızı), "N / M" (`sent`), başarısız > 0 ise ", K başarısız" (`failed`); tooltip "N gönderildi, K başarısız, A atlandı, B bekliyor" (+ tekrar varsa "R tekrar, X adet 429"). Başlatılamadıysa kırmızı `schedule_error` veya "Başlatılamadı". Aksi "–". |
| F102 | Zaman sütunu | Dashboard | `CampaignTable.vue`, `formatWhen`, `untilText` | `scheduled_at`, `created_at` | — | Zamanlanmışsa saat ikonu + tarih-saat ve "… sonra" (scheduled) veya "zamanlanmıştı"; değilse oluşturma zamanı + "oluşturuldu". `card-planned`. Not: tablodaki geri sayımın referans zamanı (`now`) bileşen kurulurken bir kez alınır ve sonra güncellenmez (mevcut davranış; liste yoklaması bunu yenilemez). |
| F103 | Satırdan zamanlanmış gönderimi iptal | Dashboard | `CampaignTable.vue`, `DashboardView.vue`, `ui/Modal.vue` | `cancelCampaign` → `POST /api/campaigns/{id}/cancel`, ardından `listCampaigns` | İkon butonu → modal | Yalnız `scheduled` satırlarda ikon buton (`XCircle`, title "Gönderimi iptal et", `aria-label="#<id> zamanlı gönderimi iptal et"`, `data-testid=cancel`). Modal: "Zamanlanmış gönderim iptal edilsin mi?" + "“<brief, 60 karakter>” <zaman> için zamanlanmıştı. İptal edersen hiçbir mesaj gönderilmez." "Vazgeç" / "Gönderimi iptal et" (danger, spinner). Kapatma (Esc/arka plan/X/Vazgeç) hiçbir şey yapmaz. Onay sonrası liste yenilenir. |
| F104 | İptal hatası (dashboard) | Dashboard | `DashboardView.vue` | `cancelError` | — | `data-testid=cancel-error`: "Gönderim iptal edilemedi: <hata>" (ör. başlamış kampanya, 409). Liste hatasından ayrı tutulur; sonraki yoklamada silinmez. Modal kapanır. |
| F105 | Sistem sağlığı paneli | Dashboard | `src/components/SystemHealth.vue` | `fetchSystem` → `GET /api/system` her 5 sn; `info.llm_mode` | — | `role=group` "Sistem sağlığı". Işıklar: API, Worker, Veritabanı (çalışıyor / yanıt vermiyor / bilinmiyor / kontrol ediliyor) + LLM (Claude ise yeşil, mock ise ayrı renk, etiket `llmLabel` veya "…"). İstek başarısızsa API "yanıt vermiyor", diğerleri "bilinmiyor". Hepsi ok ise başlık "Sistem sağlıklı", değilse "Sistem durumu". `health-api/worker/db/llm`. |

### 2.9 Ortak UI davranışları

| ID | Özellik | Ekran | Bileşen(ler) | API / state | Kullanıcı aksiyonu | Notlar |
|---|---|---|---|---|---|---|
| F106 | Modal erişilebilirliği | Compose (Gönderim), Dashboard | `src/components/ui/Modal.vue` | `v-model` | Esc / arka plan / X / Tab | `body`'ye teleport, `role=dialog`, `aria-modal`, başlıkla etiketli. Esc, arka plan tıklama ve X ("Kapat") kapatır (`persistent` değilse). Tab odağı içeride tutar; açılınca odak alt bilgideki ilk etkin butona (Vazgeç) gider; kapanınca açan öğeye döner. Açıkken sayfa kaydırması kilitlenir. Giriş/çıkış animasyonu. |
| F107 | Buton yükleniyor durumu | Tümü | `src/components/ui/Button.vue` | `loading` | — | Dönen `Loader2` ikonu, buton devre dışı, `aria-busy`. Varyantlar: primary, secondary, ghost, danger; boyut md/sm. |
| F108 | Animasyonlu sayılar | Kitle, Onay, Dashboard | `src/components/AnimatedNumber.vue` | — | — | ~280 ms ease-out ile yeni değere; `fromZero` ilk görünüşte sıfırdan sayar; tr-TR biçimi; ekran okuyucu yalnız son değeri okur. |
| F109 | İlerleme çubuğu erişilebilirliği | Kitle, Gönderim, Dashboard | `src/components/ui/ProgressBar.vue` | — | — | `role=progressbar`, `aria-valuenow` yüzde; max 0 → %100 (boş kuyruk tamam sayılır). |
| F110 | Genel API hata biçimi | Tümü | `src/api/http.ts` | `{"error":{code,message,details}}` → `ApiError` | — | JSON olmayan hata: "İstek başarısız oldu (<status>)". Ağ hatası: "Sunucuya ulaşılamadı. Bağlantınızı kontrol edin." |

## 3. Kullanılan API uç noktaları

| Yöntem | Yol | Çağıran fonksiyon | Özellikler |
|---|---|---|---|
| GET | `/api/info` | `fetchInfo` (`src/api/info.ts`) | F05, F06, F07, F46–F48, F53, F105 (diller, varsayılan dil, limitler, değişkenler, LLM modu/modeli) |
| POST | `/api/campaigns` (gövde `{brief}`) | `createCampaign` (`src/api/campaigns.ts`) | F23, F26, F36 |
| GET | `/api/campaigns/{id}` | `getCampaign` | F12, F13, F39 (üretim sonrası), F72 (onay sonrası), F77 (iptal hatası sonrası), F86 (1 sn yoklama) |
| PUT | `/api/campaigns/{id}/content` (gövde `{languages:{<kod>:{sms_body,email_subject,email_body}}}`) | `saveContent` | F51, F60 |
| POST | `/api/campaigns/{id}/messages/stream` (SSE, `Accept: text/event-stream`) | `streamPost` (`src/api/sse.ts`) via `useGenerationStream` | F39–F43 |
| POST | `/api/campaigns/{id}/approve` (gövde yok veya `{scheduled_at}`) | `approveCampaign` | F72, F73, F80 |
| POST | `/api/campaigns/{id}/cancel` | `cancelCampaign` | F77, F103, F104 |
| GET | `/api/campaigns?limit=100` | `listCampaigns` | F90–F104 |
| GET | `/api/system` | `fetchSystem` (`src/api/system.ts`) | F105 |

SSE olayları (`src/composables/streamState.ts`): `attempt {language,n,max}`, `delta {language,field,text}`, `retry {language,problems[]}`, `validated {language,content,report}`, `fallback {language,reason,content,report}`, `done`, `error {code,message}`. `language` yoksa `tr` kabul edilir. Yorum satırları (`: keepalive`) yok sayılır. EventSource kullanılamaz (POST), akış `fetch` + `ReadableStream` ile okunur.

## 4. Ekranda gösterilen veri tipleri (`src/types.ts`)

- **MessageContent**: `sms_body`, `email_subject`, `email_body` — editör ve önizlemeler.
- **LanguageInfo**: `code`, `name`, `display` — dil sekmeleri, rozetler.
- **Issue**: `field`, `code`, `message`, `length?`, `limit?`, `language?` — alan hataları, uyarılar, tutarsızlıklar.
- **Report**: `ok`, `issues`, `warnings?`, `sms_length`, `subject_length` — kayıtlı metnin doğrulaması.
- **Condition / Segment**: `status` (`ok` / `needs_clarification` / `unsupported`), `match`, `conditions`, `explanation`, `question?`, `audience?`, `offer?`, `suggestion?`, `note?`, `languages?` — Kitle adımı.
- **Audience**: `matched`, `sms_reachable`, `email_reachable`, `both_reachable` — kitle sayıları, onay özeti.
- **Offer / OfferPart**: `kind` (none/percent/amount/free_shipping/coupon_code), `value`, `code`, `min_spend?`, `expires_at?`, `step_spend?`, `max_discount?`, `extras?` — teklif metni, zaman sınırı.
- **StoredContent**: MessageContent + `language`, `source` (llm/fallback/edited), `report`, `updated_at`.
- **CampaignStatus**: draft, segment_ready, content_ready, scheduled, approved, sending, completed, cancelled, failed.
- **ChannelProgress**: `total`, `pending`, `sent`, `failed`, `skipped`, `retries`, `rate_limited`.
- **FailureReason**: `channel`, `reason`, `count`.
- **DeliveryProgress**: `sms`, `email`, `done`, `failures?`, `languages?` — Gönderim adımı, dashboard satırları.
- **ApproveResult**: `id`, `status`, `already_approved`, `queued {sms,email}`, `scheduled_at?`, `audience?`, `languages?`, `warnings`.
- **Campaign**: `id`, `brief`, `status`, `segment?`, `content?`, `translations?`, `mismatches?`, `delivery?`, `scheduled_at?`, `schedule_error?`, `created_at`.
- **VariableInfo**: `name`, `budget`, `fallback`, `fallbacks?`.
- **Limits**: `sms_max` (160), `subject_max` (80), `email_body_max` (5000).
- **CampaignSummary**: `id`, `brief`, `status`, `segment_status?`, `matched?`, `explanation?`, `audience_languages?`, `delivery?`, `scheduled_at?`, `schedule_error?`, `created_at` — dashboard satırı.
- **AppInfo** (`src/api/info.ts`): `llm_mode` (mock/claude), `model?`, `limits?`, `variables?`, `languages?`, `default_language?`.
- **SystemHealth** (`src/api/system.ts`): `api`, `database`, `worker` ∈ ok/down/unknown.

## 5. Kontrol listesi

Insider tarzı arayüze geçişten sonra (17.11.2026) her madde gerçek uygulamada denendi:
- Bileşen testleri ve tip kontrolü (270 test).
- Örnek kampanyalarla Playwright ekran görüntüleri: 12 ekran × 1920 / 1366 / 1024 / 768 / 390 px; hiçbir ekranda yatay taşma yok.

Notu olmayan maddeler olduğu gibi çalışıyor; notlu maddeler yeni yerini ya da kaldırılan açıklama cümlesini söylüyor.

- [x] F01 Marka başlığı — sol rayda nötr "CC" işareti, üst barda "Campaign Composer"
- [x] F02 "Yeni kampanya" sekmesi (temiz sıfırlama, `?c=` silme) — sol rayda "Yeni", telefonda hamburger menüde "Yeni kampanya"; aynı `newCampaign()`
- [x] F03 "Dashboard" sekmesi — adı "Kampanyalar" oldu (karar 6); sol ray ve hamburger menü
- [x] F04 Hash tabanlı sayfa seçimi
- [x] F05 LLM modu rozeti (Mock modu / Claude (model)) — üst barın sağında
- [x] F06 "API ulaşılamıyor" rozeti — üst barın sağında
- [x] F07 `/api/info` öncesi varsayılanlar (tr/en/de)
- [x] F08 Açık/koyu tema — yeni renklerle açık ve koyu tema; telefon ve gelen kutusu önizlemeleri gerçek ekran gibi açık kalır
- [x] F09 Hareket azaltma
- [x] F10 Mobil yerleşim — kırılımlar: 1280 / 1024 / 768 px (DESIGN.md §8); 5 genişlikte taşma yok
- [x] F11 Sekme başlığı
- [x] F12 URL'deki kampanyayı açma (`?c=`) — akış barı kampanyanın adımında açılır
- [x] F13 Kampanya bulunamadı / yüklenemedi — akış barının altında hata bandı; olmayan kampanyada adres temizlenir
- [x] F14 Adım çubuğu — akış barı "← Kampanyalar | Brief › Kitle › …"; telefonda "3/5 · Mesajlar"
- [x] F15 İlerleme kuralı
- [x] F16 Adımlar arası kayma animasyonu
- [x] F17 Geri / Vazgeç butonu — kutusuz, sol kenarda çerçeveli "← Geri"
- [x] F18 Ana eylem butonu — kutusuz, sağ kenarda; telefonda tam genişlik ve üstte
- [x] F19 Onay sonrası dondurma — akış barında "Onaylandı" rozeti
- [x] F20 Brief metin alanı — AI istem kartı
- [x] F21 "Örnek brief" butonu
- [x] F22 Ctrl+Enter ile gönderme
- [x] F23 "Kitleyi çıkar" — çalışırken istem kutusunda gradyan kenar ve "Kitle çıkarılıyor…"
- [x] F24 Brief değişti uyarısı — uyarı bandı olarak kaldı
- [x] F25 Onaylı brief notu — açıklama cümlesi kaldırıldı; alan salt okunur ve akış barında "Onaylandı"
- [x] F26 Oluşturma hatası + ayrıntılar
- [x] F27 Kitle yükleniyor durumu
- [x] F28 Kitle açıklaması ve not — not ("Kitle belirtilmediği için…") bilgi bandında kaldı (gerçek karar)
- [x] F29 Kural koşulları çipleri
- [x] F30 Kitle sayıları — sağ rayda "Tahmini kitle" kartı; SMS / e-posta sayıları ana kartta
- [x] F31 Kanal dağılımı çubuğu ve lejant — kanal satırları ve sağ raydaki yığılmış çubuk + noktalı satırlar
- [x] F32 Teklif satırı
- [x] F33 Mesaj dili dağılımı
- [x] F34 Ulaşılamayan ipucu / sıfır eşleşme uyarısı — ipucu cümlesi kaldırıldı; ulaşılamayan sayısı sağ rayda; sıfır eşleşme uyarısı kısaldı
- [x] F35 Netleştirme sorusu — AI yüzeyli soru kartı
- [x] F36 "Tüm müşterilere gönder" — iki yardım cümlesi kaldırıldı
- [x] F37 Desteklenmeyen brief
- [x] F38 "Mesajları üret"
- [x] F39 Canlı mesaj üretimi (SSE)
- [x] F40 Deneme rozeti — editörün üstünde AI çalışma şeridi
- [x] F41 Yeniden deneme notları — şeritte liste; giriş cümlesi kaldırıldı
- [x] F42 Şablon (fallback) bildirimi — uyarı kısaldı ("Hazır bir şablon yerleştirildi.")
- [x] F43 Akış hata bildirimi
- [x] F44 Dil sekmeleri ve durum noktaları — alt çizgili sekmeler; noktalar aynı
- [x] F45 Diller arası tutarsızlık uyarısı — hata kısaldı ("Diller aynı şeyi söylemiyor:" + liste)
- [x] F46 SMS alanı + halka sayaç (160)
- [x] F47 E-posta konusu + çubuk sayaç (80)
- [x] F48 E-posta metni alanı (5000)
- [x] F49 Sayaç renk seviyeleri
- [x] F50 Anlık istemci doğrulaması
- [x] F51 Sunucu doğrulama sorunları
- [x] F52 Kalite uyarıları
- [x] F53 Kişiselleştirme değişkenleri listesi — "Kişiselleştirme" etiketi + `{{değişken}} = pay` çipleri; açıklama tooltip
- [x] F54 SMS telefon önizlemesi — sağda yapışık önizleme kartı (%35), en fazla 300px; "Elif" cümlesi kaldırıldı
- [x] F55 E-posta önizlemesi — aynı kartta "Gelen kutusu", en fazla 420px
- [x] F56 Yazma sırasında salt okunur editör
- [x] F57 "Yeniden üret" — kart başlığında çerçeveli buton
- [x] F58 Mesajlar boş durumu
- [x] F59 Kaynak ve kaydedilmemiş not — cümle yerine rozetler: "Model metni / Hazır şablon / Elle düzenlendi" ve "Kaydedilmedi"; sekmede nokta
- [x] F60 "Onayla" (kaydet ve geç)
- [x] F61 Onaylı mesajlar rozeti — akış barındaki "Onaylandı" rozeti
- [x] F62 Onay ön koşul boş durumu — "Mesajlar henüz kaydedilmedi."
- [x] F63 Hemen / Zamanla seçimi — seçenek satırları (karar 4); `role="radio"`
- [x] F64 Tarih-saat seçici — geçmiş ve 30 günden sonraki günler seçilemez (backend de 422 döner)
- [x] F65 Hızlı zaman kısayolları
- [x] F66 Varsayılan zaman — değişmedi
- [x] F67 Zaman sınırı ipucu — ipucu cümlesi kaldırıldı; sınırı seçici uygular, aşılırsa F68 mesajı ve backend 422
- [x] F68 Zaman doğrulaması
- [x] F69 Onay özeti — ana kartta Teklif / Kitle / Diller, sağ rayda Alıcılar
- [x] F70 Tahmini sayılar notu — cümle yerine "Tahmini sayılar" rozeti
- [x] F71 Uyarı listesi — sağ rayda turuncu "Uyarılar" kartı
- [x] F72 İki adımlı onay
- [x] F73 Onay hatası
- [x] F74 Onaylanmış salt okunur kart — seçenekler gizlenir, "Onaylandı" rozeti
- [x] F75 Gönderim durum rozeti — rapor başlığında durum hapı ("Zamanlandı" mor)
- [x] F76 Zamanlanmış kampanya bilgileri — tarih ve geri sayım başlıkta, tahmini sayılar KPI kartında; açıklama cümlesi kaldırıldı
- [x] F77 Zamanlanmış gönderimi iptal (sihirbaz) — başlıktaki "Gönderimi iptal et" + aynı Modal
- [x] F78 İptal edildi bildirimi
- [x] F79 Başlatılamadı bildirimi
- [x] F80 Onay anı sayı farkı bildirimi
- [x] F81 Genel ilerleme çubuğu ve başlık — "bu sayfa kendiliğinden güncellenir" kaldırıldı
- [x] F82 Kanal tablosu — tablo yerine kanal başına KPI grubu (gönderilen; başarısız / atlanan / bekleyen)
- [x] F83 Dile göre gönderilen — "Dile göre gönderilen" kartı
- [x] F84 Başarısızlık nedenleri — yarım halka + noktalı lejant (karar 3)
- [x] F85 Tamamlandı mesajı
- [x] F86 Canlı ilerleme yoklaması (1 sn)
- [x] F87 Henüz onaylanmadı boş durumu
- [x] F88 "Dashboard'a git" — etiketi "Kampanyalara git" (karar 6)
- [x] F89 Dashboard başlığı ve "Yeni kampanya" — H1 "Kampanyalar" + "+ YENİ KAMPANYA"
- [x] F90 Liste yoklaması (2 sn) — "Sayfa her 2 saniyede güncellenir" kaldırıldı; yoklama aynı
- [x] F91 Liste güncellenemedi uyarısı
- [x] F92 Özet kutucukları — tek KPI kartı; açıklamalar (?) tooltip
- [x] F93 Gönderim hızı hesabı
- [x] F94 Dashboard yükleniyor
- [x] F95 Dashboard boş durumu — yardım cümlesi kaldırıldı
- [x] F96 Kampanya tablosu ve satır açma — 1024px altında satırlar kart (yalnız CSS)
- [x] F97 Kampanya başlığı ve kitle alt satırı
- [x] F98 Satır durum rozeti — zamanlanmış: "Zamanlandı · 24 Kas 14:00", mor
- [x] F99 Kitle sütunu
- [x] F100 Diller sütunu
- [x] F101 İlerleme sütunu
- [x] F102 Zaman sütunu — geri sayımın donması aynen duruyor (ayrı iş, DESIGN.md §7)
- [x] F103 Satırdan zamanlanmış gönderimi iptal — satırdaki iptal ikonu + Modal (Escape ile kapanır)
- [x] F104 İptal hatası (dashboard)
- [x] F105 Sistem sağlığı paneli (5 sn) — sayfa altında "Sistem durumu" kartı
- [x] F106 Modal erişilebilirliği
- [x] F107 Buton yükleniyor durumu
- [x] F108 Animasyonlu sayılar
- [x] F109 İlerleme çubuğu erişilebilirliği
- [x] F110 Genel API hata biçimi
