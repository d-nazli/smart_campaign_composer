# Insider One tarzı arayüz: eşleştirme ve plan

Durum: **uygulandı** (17.11.2026, "ONAYLIYORUM" sonrası). Kontrol listesi: [FEATURES.md](FEATURES.md) §5, 110/110. Kararlar: DECISIONS.md D90, D91. `/preview` sayfası ve önizlemeye özel kod kaldırıldı.
`design/` altındaki iki belge ve `/preview` sayfasının yeni dosyaları eklendi (bkz. §6).

Kaynaklar: [FEATURES.md](FEATURES.md) (F01–F110) ve academy.insiderone.com'daki panel ekran görüntüleri
(kampanya listeleri, oluşturma akışı, AI ajan, editör, analytics sayfaları). Insider logosu, adı ve görselleri
kullanılmaz. Alınan şey yalnızca yerleşim ve görsel dildir.

## 1. Insider panelinden çıkan desenler (özet)

| Desen | Panelde görülen | Bizde karşılığı |
|---|---|---|
| Kabuk | Koyu (≈#0F1221) dar ikon rayı. Aktif öğe dolgun mavi kare. İnce beyaz üst bar. | Sol ray: Kampanyalar, Yeni kampanya. Üst bar: içerik haritası (breadcrumb) ve LLM rozeti. |
| Sayfa başlığı | Ev ikonlu breadcrumb, büyük ve ince H1, sağ üstte büyük harfli "+ CREATE" (maviden mora gradyan). | "⌂ › Kampanyalar", H1 ve "+ YENİ KAMPANYA". |
| Liste | Beyaz çerçeveli tam genişlik tablo. Başlıklar büyük harf, harf aralıklı, gri. Durum hapı (pill), satır ikon aksiyonları. | Mevcut kampanya tablosu, aynı sütunlarla. |
| KPI | Tek beyaz kart dikey ayraçlarla bölünür. Etiket büyük harf, (?) simgeli. Altında büyük değer ve noktalı çizgiyle (dotted leader) alt satırlar. | Dashboard'daki dört kutucuk ve Gönderim adımının sayıları. |
| Oluşturma akışı | Üst beyaz şerit: "← Geri" bağlantısı ve oklarla ayrılmış adım adları (aktif adım mavi ve kalın, numara dairesi yok). Gövde iki kolon: ⅔ beyaz kart, ⅓ sağ ray. Altta yapışık beyaz bar: solda "← Geri", sağda dolgun mavi ana buton. | Beş adımlı sihirbaz. |
| Gönderime hazırlık | "Ready to Go Live?" başlığı. Seçenekler satır satır (Send Now / Send Later). Sağ rayda üst kenarı renkli özet kartları (turuncu uyarı, mavi bilgi, yeşil hazır). | Onay adımı. |
| Kitle | Sağ rayda yarım halka (half-donut) göstergesi, "TAHMİNİ KİTLE" etiketi ve büyük sayı. Kanal bazında ulaşılabilirlik satırları ince ilerleme çubuğuyla. | Kitle adımı. |
| AI ajan | Pastel pembe ve lavanta yüzey. Çalışırken pembeden mora gradyan kenarlı istem (prompt) kutusu. "✦ N görev üzerinde çalışılıyor" satırı. Öneri kartları. | Brief kutusu ve mesaj üretimi sırasında görünen durum. |
| Editör | Üstte sekmeler (alt çizgili, aktif olan mavi). Ortada içerik, sağda önizleme paneli. Satır, etiket ve kontrol şeklinde sağ panel. | Mesajlar adımı. |
| Tekil kampanya analitiği | Breadcrumb ve kampanya adı. Durum hapı içeren meta şeridi. KPI grubu. Başarısız mesajlar için yarım halka ve noktalı lejant ("37,7% (40.000)"). | Gönderim adımı. |

## 2. Özellik → Insider tarzı ekran tablosu

Ekran kodları:
- **K** = kabuk.
- **L** = Kampanyalar listesi (eski Dashboard).
- **A** = oluşturma akışı. Alt ekranları: **A1** Brief, **A2** Kitle, **A3** Mesajlar, **A4** Onay, **A5** Gönderim.
- **O** = ortak bileşen.

"Yeni bileşen" sütunundaki adlar onaydan sonra eklenecek, yalnızca görünüm taşıyan bileşenlerdir.

| ID | Özellik | Yeni ekran | Yeni yer / bileşen |
|---|---|---|---|
| F01 | Marka başlığı | K | Sol rayın üstünde nötr monogram ve üst barda ürün adı "Campaign Composer". Insider logosu yok. `SideNav` |
| F02 | "Yeni kampanya" sekmesi (`?c=` temizler, yeniden kurar) | K | Sol rayda "Yeni kampanya" ikonu. Aynı `newCampaign()` çağrılır. `SideNav` |
| F03 | "Dashboard" sekmesi | K | Sol rayda "Kampanyalar" ikonu (`#/dashboard`). `SideNav` |
| F04 | Hash ile sayfa seçimi | K | Değişmez (`pageFromHash`). Rayda aktif öğe `aria-current` ile işaretlenir. |
| F05 | LLM modu rozeti | K | Üst barın sağı, aynı `Badge` mantığıyla. |
| F06 | "API ulaşılamıyor" rozeti | K | Üst barın sağı. |
| F07 | `/api/info` gelmeden varsayılanlar | K/A3 | Değişmez. |
| F08 | Açık/koyu tema | O | Yeni token setinin koyu karşılığı tanımlanır (§3). |
| F09 | Azaltılmış hareket | O | Değişmez (`style.css` kuralı korunur). |
| F10 | Mobil yerleşim | O | §8'deki kırılım noktaları. |
| F11 | Sekme başlığı | K | Değişmez (`index.html`). |
| F12 | `?c=` ile kampanya açma | A | Değişmez. Akış barında kampanya başlığı görünür. |
| F13 | 404 / yükleme hatası | A | Akış barının altında kırmızı bant (`notice error`). |
| F14 | StepBar | A | Üst beyaz akış barı: "Brief › Kitle › Mesajlar › Onay › Gönderim". Tamamlanan adımda ✓ işareti, aktif adım mavi ve kalın. `FlowBar` (StepBar'ın yeni görünümü) |
| F15 | Hangi adımlar açılabilir kuralı | A | Değişmez (`reachable`). Açılamayan adım gri ve tıklanamaz. |
| F16 | Adımlar arası kayma animasyonu | A | Ana kolonda korunur. |
| F17 | Geri / Vazgeç | A | Kutusuz alt buton satırı: "← Geri" ya da "Vazgeç" içerik kolonunun sol kenarında, sayfa zemininde. |
| F18 | Ana bar butonu (etiketler, yükleniyor) | A | Aynı satırda, içerik kolonunun sağ kenarında dolgun mavi buton. Etiketler aynı kalır. |
| F19 | Onaydan sonra salt okunur | A | Değişmez. Akış barında "Onaylandı" hapı. |
| F20 | Brief alanı (2000) | A1 | AI istem kartı: lavanta zemin, gradyan kenarlı büyük metin kutusu. `PromptCard` |
| F21 | "Örnek brief" | A1 | İstem kutusunun altında tek bir öneri kartı. Davranış aynı. |
| F22 | Ctrl+Enter | A1 | Değişmez. Kutunun altında "Ctrl + Enter" ipucu. |
| F23 | "Kitleyi çıkar" | A1 | Alt bar ana butonu (değişmez). Çalışırken istem kutusunda gradyan kenar ve "✦ Kitle çıkarılıyor…". |
| F24 | "Brief değişti" ipucu | A1 | İstem kutusunun altı. |
| F25 | Onaylı brief değişmez notu | A1 | İstem kutusunun altı. Kutu salt okunur. |
| F26 | Oluşturma hatası ve "Ayrıntılar" | A1 | Kartın içinde kırmızı bant ve açılır ayrıntılar. |
| F27 | Kitle yükleniyor | A2 | Ana kartta iskelet (skeleton) satırları ve "Kitle çıkarılıyor…". |
| F28 | Açıklama ve not | A2 | Kartın başında açıklama. Not mavi bilgi bandında. |
| F29 | Koşul çipleri (ve/veya) | A2 | "KURAL" bölüm başlığı altında çipler. |
| F30 | Kitle kutucukları | A2 | Sağ rayda düz "TAHMİNİ KİTLE" kartı: büyük eşleşen kişi sayısı, yığılmış çubuk, noktalı satırlarla dört sayı. Ana kartta SMS / e-posta satırları. Yarım halka yok (karar 3). |
| F31 | Kanal çubuğu ve lejant | A2 | "Kanallara göre ulaşılabilirlik" satırları: kanal adı, sayı, ince çubuk ve yüzde. Aynı dört sayı (iki kanal, yalnız SMS, yalnız e-posta, ulaşılamayan). |
| F32 | Teklif satırı | A2 | Ana kartta "TEKLİF" satırı. |
| F33 | Mesaj dili sayıları | A2 | Ana kartta "MESAJ DİLİ" satırı, çiplerle. |
| F34 | Ulaşılamayan ipucu / sıfır eşleşme uyarısı | A2 | Bilgi ipucu ve turuncu uyarı bandı. |
| F35 | Netleştirme sorusu | A2 | AI soru kartı: "Biraz daha bilgi lazım", soru metni ve "Anladığım teklif". |
| F36 | "Tüm müşterilere gönder" | A2 | Soru kartında seçenek satırı ve buton. Davranış aynı. |
| F37 | Desteklenmeyen brief | A2 | Kırmızı kenarlı uyarı kartı. |
| F38 | "Mesajları üret" | A2/A3 | Alt bar ana butonu (değişmez) ve A3 boş durumdaki buton. |
| F39 | Canlı SSE üretimi | A3 | Değişmez (`useGenerationStream`). Kartın üstünde AI çalışma şeridi. |
| F40 | Deneme rozeti | A3 | AI şeridinde: "✦ Türkçe: üretiliyor" / "düzeltiliyor 2/3". `AgentStatus` |
| F41 | Yeniden deneme notları | A3 | AI şeridinin altında açılır "Ayrıntılar" listesi. |
| F42 | Hazır şablon bildirimi | A3 | Turuncu bant. |
| F43 | Akış hatası | A3 | Kırmızı bant. |
| F44 | Locale sütunu ve durum noktaları | A3 | Mesajlar kartında solda dar locale sütunu; 1280px altında açılır düğme (D96). |
| F45 | Diller arası uyuşmazlık | A3 | Sekmelerin altında kırmızı bant. |
| F46 | SMS alanı ve halka sayaç (160) | A3 | "SMS" bölümü: başlık satırında halka sayaç. |
| F47 | E-posta konusu ve çubuk sayaç (80) | A3 | "E-POSTA KONUSU" satırı. |
| F48 | E-posta metni (5000) | A3 | "E-POSTA METNİ" satırı. |
| F49 | Sayaç renk seviyeleri | A3 | Aynı eşikler, yeni renk tokenları. |
| F50 | Canlı istemci doğrulaması | A3 | Değişmez. Alan altında kırmızı metin. |
| F51 | Sunucu doğrulama sorunları | A3 | Değişmez (ilk sorunlu dile atlama dahil). |
| F52 | Engellemeyen kalite uyarıları | A3 | Alan altında turuncu metin. |
| F53 | Kişiselleştirme değişkenleri | A3 | Editörün üstünde "Kişiselleştirme" satırında değişken çipleri (`{{first_name}} = 12`). |
| F54 | SMS telefon önizlemesi | A3 | Sağ önizleme panelinde (%35, yapışık) mevcut `PhonePreview` telefon mockup'ı, en fazla 300px ve ortalı. Renkleri token'a çevrilir. Seçili locale'i izler; ar-SA sağdan sola. |
| F55 | E-posta önizlemesi | A3 | Aynı panelde mevcut `EmailPreview` "Gelen kutusu" penceresi, en fazla 420px ve ortalı. Renkleri token'a çevrilir. Telefonun altında, ikisi birlikte görünür. |
| F56 | Akış ya da onay sırasında salt okunur | A3 | Değişmez. |
| F57 | "Yeniden üret" | A3 | Kart başlığının sağında ikincil buton. |
| F58 | Boş durum | A3 | Ortalanmış boş durum ve "Mesajları üret". |
| F59 | Kaynak ve kaydedilmemiş not | A3 | Kartın altında gri ipucu satırı. |
| F60 | "Onayla" (PUT content) | A3 | Alt bar ana butonu (değişmez). |
| F61 | Onaylandı rozeti | A3 | Kart başlığında yeşil hap. |
| F62 | Önkoşul boş durumu | A4 | Ana kartta boş durum metni. |
| F63 | Hemen / Zamanla | A4 | "Göndermeye hazır mısınız?" başlığı altında iki seçenek satırı (simge, başlık, açıklama). Seçili satır mavi kenarlı. Erişilebilirlik için `role="radio"`, tarayıcının yerel radyosu kullanılmaz. `OptionRows` (SegmentedControl'ün yeni görünümü) |
| F64 | DateTimeField | A4 | "Zamanla" satırı seçilince satırın içinde açılır. |
| F65 | Hızlı zaman kısayolları | A4 | Tarih alanının yanında çipler. |
| F66 | Varsayılan gönderim zamanı | A4 | Değişmez. |
| F67 | Zaman sınırı ipucu | A4 | Seçenek satırının altında. |
| F68 | Zamanlama doğrulaması | A4 | Seçenek satırının altında kırmızı metin. |
| F69 | Onay özeti | A4 | Ana kartta "ÖZET" listesi (Teklif, Kitle, Alıcılar, Diller). Sağ rayda mavi "Hedef kitle: N kişi" kartı. `RailCard` |
| F70 | Tahmini sayı notu | A4 | Sağ rayda bilgi kartının içinde. |
| F71 | Uyarı listesi | A4 | Sağ rayda turuncu kenarlı "Göndermeden önce" kartı. Uyarı yoksa yeşil "Mesajlar gönderime hazır" kartı. |
| F72 | İki adımlı onay | A4 | Değişmez. "Emin misiniz?" bandı ana kartın altında. Alt bar butonu ikinci basışta onaylar. |
| F73 | Onay hatası | A4 | Kırmızı bant. |
| F74 | Onaylanmış salt okunur kart | A4 | Seçenek satırları gizlenir, "Onaylandı" hapı görünür. |
| F75 | Durum rozeti | A5 | Meta şeridinde durum hapı. |
| F76 | Zamanlanmış kampanya ayrıntıları ve geri sayım | A5 | Meta şeridinde "Gönderim zamanı" ve geri sayım. KPI grubunda tahmini alıcı ve diller. |
| F77 | Zamanlanmışı iptal et (modal) | A5 | Meta şeridinin sağında ikincil "Gönderimi iptal et" ve aynı Modal. |
| F78 | İptal edildi bildirimi | A5 | Mavi bilgi bandı. |
| F79 | Başlatılamadı bildirimi | A5 | Kırmızı bant. |
| F80 | Onay anı sayı farkı | A5 | Mavi bilgi bandı. |
| F81 | Genel ilerleme çubuğu ve açıklama | A5 | KPI grubunun altında tam genişlik çubuk ve "N / M mesaj işlendi". `KpiGroup` |
| F82 | Kanal tablosu | A5 | KPI grup kartı: "SMS" ve "E-POSTA" sütunları, her birinde Gönderilen değeri ve noktalı satırlarla Başarısız, Atlanan, Bekleyen. Aynı sayılar. |
| F83 | Dile göre gönderilen | A5 | "DİLE GÖRE GÖNDERİLEN" kartı, çiplerle. |
| F84 | Başarısızlık nedenleri | A5 | "BAŞARISIZ MESAJLAR" kartı: yarım halka ve noktalı lejant (sayı ve yüzde). Aynı nedenler ve sıralama. `FailureGauge` |
| F85 | Tamamlandı mesajı | A5 | Yeşil bant. |
| F86 | 1 sn ilerleme yoklaması | A5 | Değişmez. |
| F87 | Henüz onaylanmadı boş durumu | A5 | Boş durum metni. |
| F88 | "Dashboard'a git" | A5 | Alt bar ana butonu, yeni etiketiyle "Kampanyalara git" (karar 6). |
| F89 | Başlık ve "Yeni kampanya" | L | Sayfa başlığı: "⌂ › Kampanyalar", H1 "Kampanyalar", sağda "+ YENİ KAMPANYA". |
| F90 | 2 sn liste yoklaması | L | Değişmez. |
| F91 | Eski liste hatası | L | Başlığın altında kırmızı bant. |
| F92 | Dört özet kutucuğu | L | Tek KPI grup kartı: GÖNDERİLEN, BAŞARISIZ, KUYRUKTA, HIZ. Her birinde (?) ipucu ve aynı alt metin. `KpiGroup` |
| F93 | Hız hesabı (son 10 sn) | L | Değişmez. |
| F94 | Yükleniyor | L | Tablo iskeleti. |
| F95 | Boş durum | L | Tablo kartının içinde ortalanmış boş durum ve "+ YENİ KAMPANYA". |
| F96 | Tablo ve satır açma (`?c=`) | L | Tam genişlik tablo kartı. Satır tıklaması değişmez. |
| F97 | Başlık (64) ve kitle alt satırı | L | KAMPANYA sütunu: ad ve gri alt satır (Insider'daki kanal alt satırı gibi). |
| F98 | Satır durum rozeti | L | DURUM sütunu, hap. Renk eşlemesi §3'te. |
| F99 | Kitle sütunu | L | KİTLE sütunu (sağa yaslı). |
| F100 | Diller sütunu | L | DİLLER sütunu. |
| F101 | İlerleme sütunu ve ipucu | L | İLERLEME sütunu: ince çubuk ve "N / M". |
| F102 | Zaman sütunu | L | ZAMAN sütunu. Geri sayımın donma tuhaflığı bu işte korunur; ayrı iş olarak §7'de. |
| F103 | Zamanlanmış satırı iptal et | L | İŞLEMLER sütununda mavi çizgi ikon ve aynı Modal. |
| F104 | İptal hatası | L | Başlığın altında kırmızı bant. |
| F105 | Sistem sağlığı (5 sn) | L | Sayfanın altında "SİSTEM DURUMU" kartı: nokta, ad, durum satırları. Aynı veri. |
| F106 | Modal erişilebilirliği | O | Değişmez. Görünüm: başlık ve ×, altta solda "Vazgeç", sağda ana buton. |
| F107 | Buton yükleniyor | O | Değişmez. Yeni renkler. |
| F108 | Animasyonlu sayılar | O | KPI değerlerinde ve onay özetinde korunur. |
| F109 | İlerleme çubuğu erişilebilirliği | O | Değişmez. |
| F110 | Genel API hata biçimi | O | Değişmez (`api/http.ts`). |

Kontrol: tabloda F01'den F110'a kadar 110 satır var. Boşta kalan özellik yok.

**Bilerek eklenmeyenler.** Insider ekranlarında olup bizde olmayan öğeler, yeni özellik sayılacağı için
eklenmez: filtre ve arama, etiketler, sayfalama, kebab menüsü (Çoğalt, Sabitle…), tarih aralığı seçici,
dışa aktarma, Gün/Hafta/Ay grafiği, açılma, tıklama ve gelir metrikleri, test mesajı, chat geçmişi, sürüm
listesi, masaüstü/mobil önizleme anahtarı.

## 3. Yeni design token'ları

Mevcut adlar (`--accent`, `--surface`…) korunur, değerleri değişir. Böylece bileşenlerin çoğunda yalnızca
stil dokunuşu gerekir. Yeni adlar `+` ile işaretli. Hex değerleri ekran görüntülerinden alınmış tahminlerdir.

| Token | Şimdi | Yeni (açık) | Yeni (koyu) | Not |
|---|---|---|---|---|
| `--bg` | #f7f8fa | **#f1f2f5** | #0e1015 | Panel zemini |
| `--surface` | #fff | #ffffff | #161922 | Kartlar |
| `--surface-2` | #f0f2f5 | **#f6f7f9** | #1c2029 | Tablo başlığı |
| `--fg` | #15181e | **#1f2433** | #e8eaee | |
| `--muted` | #5e6573 | **#6b7280** | #98a0ae | |
| `--border` | #e3e6eb | **#e2e4e9** | #272c37 | |
| `--accent` | #2457d6 | **#1f33cc** | #7a8cff | Ana mavi |
| `--accent-hover` | #1c47b4 | **#1828a8** | #94a3ff | |
| `--accent-soft` | #e8eefc | **#e6ebff** | #1d2347 | Seçili satır, aktif sayfa |
| `+--cta-gradient` | – | `linear-gradient(90deg,#2233cc,#5b2bb5)` | aynı | Yalnız "+ YENİ KAMPANYA" |
| `+--nav-bg` | – | **#0f1221** | #0a0c14 | Sol ray |
| `+--nav-active` | – | **#1f33cc** | #3a4bd8 | |
| `+--ai-surface` | – | `linear-gradient(135deg,#fbeef4,#efedfb 55%,#e9ecfb)` | `#1d1a2b → #171a2b` | AI kartları |
| `+--ai-ring` | – | `linear-gradient(90deg,#e86fa8,#7b5cf0)` | aynı | Çalışırken kenar |
| `+--ai-chip-bg` / `+--ai-chip-fg` | – | #ede7fb / #6b3fd1 | #2b2347 / #c5b3ff | |
| `--ok` / `--ok-bg` | #1b7f4b / #e4f4eb | **#2e7d32 / #e8f6ec** | #6fd69a / #12301f | Tamamlandı hapı |
| `--warn` / `--warn-bg` | #8f5a00 / #fff4db | **#b45f00 / #fff3e3**, kenar #e07b00 | #f0c25a / #33270c | Uyarı kartı |
| `--error` / `--error-bg` | #b3261e / #fdecea | **#c62828 / #fdecec** | #ff9a8f / #3a1613 | |
| `--info-bg` | accent-soft | **#dceef8** | #10283a | Bilgi bandı |
| `--sms` / `--email` | #0e7c70 / #7b4aa5 | #0e7c70 / **#e07b00** | #4fc9ba / #f0a24a | Panelde e-posta turuncu. SMS yeşilimsi kalır. |
| `+--chart-1…6` | – | #1530c8, #4a90d9, #c97a2b, #4a1f80, #3d8a4a, #e0ac45 | açık tonları | Yarım halka (başarısızlık nedenleri) |
| `--r-sm` / `--r-md` | 6 / 10px | **4 / 6px** | | Panel köşeleri küçük |
| `+--shadow-card` | – | `0 1px 2px rgba(16,24,40,.04)` | none | Neredeyse görünmez |
| `+--label` | – | 11px, 600, `letter-spacing:.08em`, büyük harf | | KPI ve tablo başlıkları |
| `--text-base` | 14px | **16px** | | Temel yazı boyutu; ölçeğin geri kalanı orantılı: xs 13, sm 14.5, md 18, lg 22 |
| `--text-xl` | 28px | **34px**, ağırlık 400 | | H1 ince |
| `+--text-label` | – | 13px | | Büyük harfli etiketler |
| `+--control-h` | – | 44px | | Buton ve tek satırlı input yüksekliği; textarea iç boşluğu 12×16px |
| `+--device-*` | – | frame #15181e, screen #f1f2f4, bubble #dcdfe4, line #dcdfe4, chrome #eceef1, dot #aeb4be, surface #fff, avatar #1f33cc | koyu temada da açık kalır (gerçek ekran gibi) | Telefon ve gelen kutusu mockup'ları |
| `+--rail-w` / `--header-h` | – / 56px | 64px / **52px** | | |
| `--page` | 1040px | **1600px** | | Akış barı, kartlar ve alt butonlar aynı kolonda, aynı kenarda |
| `+--gutter` | – | 40px (≥1280), 24px (768–1279), 16px (<768) | | Kolonun yan boşluğu |
| `+--rail-col` | – | 360px | | Kitle ve Onay adımlarının sağ rayı. Mesajlar adımı 65 / 35 bölünür. |

Durum hapı eşlemesi:

| Durum | Hap |
|---|---|
| Taslak | Gri |
| Kitle hazır / Mesajlar hazır | Mavi açık |
| Zamanlandı | Mor açık ve nokta |
| Gönderiliyor | Mavi ve nokta |
| Tamamlandı | Yeşil |
| İptal | Gri |
| Başlatılamadı | Kırmızı |

Yazı tipi: Geist Variable kalır, yeni paket gerekmez (karar 1).

## 4. Değişecek dosyalar (onaydan sonra)

**Yalnızca görünüm ya da şablon yapısı değişecek dosyalar.** Mantık satırları korunur.
- `src/styles/tokens.css`, `src/style.css`: yeni tokenlar, kabuk, `.notice`, tablo, etiket stilleri.
- `src/App.vue`: üst bar yerine sol ray ve üst bar. `newCampaign`, `onHash`, `fetchInfo` aynen kalır.
- `src/views/DashboardView.vue`: sayfa başlığı, KPI grubu, tablo kartı, sistem kartı yerleşimi.
- `src/views/ComposeView.vue`:
  - Akış barı, iki kolon ve yapışık alt bar.
  - Adım içerikleri yeni bileşenlere taşınır.
  - `<script>` bloğuna dokunulmaz. Gerekirse yalnızca sağ raya veri geçirmek için `computed` okumaları eklenir.
- `src/components/`: StepBar, SegmentCard, MessageEditor, ApprovalCard, DeliveryProgress, CampaignTable, LocaleList, CharCounter, PhonePreview, EmailPreview.
- `src/components/ui/`: Badge, Button, Card, StatTile, SegmentedControl, ProgressBar, Modal, DateTimeField.

**Yeni, yalnızca görünüm taşıyan bileşenler.** Veri prop olarak gelir, API çağrısı yoktur.
- `components/shell/SideNav.vue`, `TopBar.vue`
- `components/ui/KpiGroup.vue`, `RailCard.vue`, `HalfDonut.vue`, `OptionRows.vue`, `PageHeader.vue`
- `components/AgentStatus.vue`, `PromptCard.vue`

**Güncellenmesi gereken testler.** `src/designSystem.test.ts` bugünkü tasarımı sabitliyor:
- `--accent:#2457d6`
- `--page:1040px`
- `App.vue` içinde `container topbar-in`
- Kart gölgesi yasağı

Yeni tasarımla bu dört kontrol güncellenmeli. Diğer bileşen testleri `data-testid` ile çalışıyor, test kimlikleri korunacak.

**Dokunulmayacaklar:** `src/api/*`, `src/composables/*`, `src/utils/*`, `src/types.ts`, backend.

## 5. Kararlar (17.11.2026)

1. **Yazı tipi:** Geist kalır.
2. **Koyu tema:** Korunur, yeni renklerle uyarlanır (§3).
3. **Yarım halka:** Yalnızca başarısızlık nedenlerinde (F84). Kitle sayıları düz kart ve çubukla gösterilir (F30, F31).
4. **Hemen / Zamanla:** Seçenek satırları (F63).
5. **Donan geri sayım (F102):** Bu işte dokunulmaz. Ayrı iş olarak §7'de not edildi.
6. **Adlandırma:** "Dashboard" yerine "Kampanyalar" ("Kampanyalara git").
7. **`/preview`:** Dönüşüm bitince silinir (`frontend/preview.html` ve `src/views/preview/`).
8. **`designSystem.test.ts`:** Onaydan sonra dört kontrol yeni tasarıma göre güncellenebilir (accent, sayfa genişliği, kabuk sınıfları, kart gölgesi).

## 6. Önizleme (kaldırıldı)

`/preview`, onay öncesinde yeni yerleşimi örnek veriyle göstermek için vardı. Dönüşüm bitince, karar 7 gereği şunlar silindi:
- `frontend/preview.html`
- `src/views/preview/`
- Önizlemeye özel kod: sarı şerit, `Fxx` etiketleri, `?check=1` taşma kontrolü.

## 7. Sonraki işler (bu dönüşümün dışında)

- **F102, tablodaki geri sayım donuyor.** `CampaignTable` geri sayımı, tablonun ilk gösterildiği andaki `now` ile hesaplıyor ve sonra güncellemiyor. Sihirbazdaki geri sayım ise 15 saniyede bir güncelleniyor. Ayrı bir iş olarak düzeltilecek.

## 8. Responsive kurallar

| Genişlik | Kabuk | Akış | Liste | Butonlar |
|---|---|---|---|---|
| ≥1280px | Sol koyu ray, kolon en fazla 1600px, yan boşluk 40px | İki sütun: içerik ve yapışık sağ ray / önizleme (Mesajlar 65/35) | Tablo | Geri solda, ana buton sağda, kutusuz |
| 1024–1279px | Aynı, yan boşluk 24px | Tek sütun: ray / önizleme içeriğin altında | Tablo | Aynı |
| 768–1023px | Aynı | Tek sütun. KPI'lar 2×2. Gönderim kartları alt alta. | Kart listesi (tablo yatay kaymasın diye) | Aynı |
| <768px | Sol ray yok; üst barda hamburger menü. Breadcrumb yalnızca ev › bulunulan sayfa. Yan boşluk 16px. | Akış barı "← Kampanyalar · 3/5 · Mesajlar". KPI'lar tek sütun. Etiket–değer satırları alt alta. | Kart listesi | Tam genişlik, alt alta, ana buton üstte |

Kontrol (17.11.2026): 6 ekran × 1920 / 1366 / 1024 / 768 / 390px.
- Her sayfa, tam o genişlikte bir iframe içinde açıldı.
- Görünür alanın dışına taşan elemanlar, önizlemedeki `?check=1` betiğiyle sayıldı. Sonuç: hiçbir ekranda taşma ya da yatay kaydırma yok.
- Ekran görüntüleri gözle incelendi.

## 9. Kaldırılan açıklama cümleleri

Kullanıcıya bir şey anlatan yardımcı cümleler kaldırıldı. Sayılar, hata ve uyarı mesajları kaldı.

| Ekran | Kaldırılan cümle | Yerine |
|---|---|---|
| Brief | "Onaylanan kampanyanın brief'i değiştirilemez." | Salt okunur alan ve akış barında "Onaylandı" |
| Kitle | "İzin vermeyen ya da telefonu / e-postası olmayan kişilere mesaj gönderilmez." | Ulaşılamayan sayısı (sağ ray) |
| Kitle | "Bu kurala uyan kimse yok. **Brief'i değiştirmeyi deneyin.**" | Uyarı kalır: "Bu kurala uyan kimse yok." |
| Kitle (soru) | "İzin vermemiş olanlara yine de gönderilmez." | – |
| Kitle (soru) | "Ya da brief'i kime göndereceğinizi yazarak güncelleyip tekrar deneyin." | – |
| Mesajlar | "Kaynak: model. Kaydedilmemiş düzenleme var; “Onayla” kaydeder." | "Model metni / Hazır şablon / Elle düzenlendi" rozeti, "Kaydedilmedi" rozeti, sekmede nokta |
| Mesajlar | "Önizleme: kişi adı yerine örnek olarak “Elif” kullanılır." | – |
| Mesajlar | "Kişiselleştirme (karakter sınırlarına sabit pay olarak sayılır):" | "Kişiselleştirme" etiketi; açıklama çiplerin tooltip'inde |
| Mesajlar | "Önceki deneme sınırlara uymadığı için model düzeltiyor:" | AI şeridinde "düzeltiliyor 2/3" ve sorun listesi |
| Mesajlar | "… lütfen metni düzenleyin. Şablon da tüm sınırlara uyar." | Uyarı kalır: "… Hazır bir şablon yerleştirildi." |
| Mesajlar | "İndirim oranı, tutar, tarih ve {{değişkenler}} her dilde aynı olmalı:" | Hata kalır: "Diller aynı şeyi söylemiyor:" ve liste |
| Onay | "En fazla 30 gün sonrası" / "En geç … (teklifin son günü)" | Seçici geçmişi ve sınır sonrasını kapatır; aşımda doğrulama mesajı ve 422 |
| Onay | "Kitle gönderim anında yeniden belirlenir; kişi sayısı değişebilir." | "Tahmini sayılar" rozeti |
| Onay | "Onaya geçmek için önce mesajları üretip kaydedin." | "Mesajlar henüz kaydedilmedi." |
| Gönderim | "– bu sayfa kendiliğinden güncellenir" | – |
| Gönderim (zamanlı) | "Kitle gönderim anında belirlenir; kimsenin burada olması gerekmez." | Tahmini alıcı kartındaki (?) tooltip |
| Kampanyalar | "Sayfa her 2 saniyede güncellenir." | – |
| Kampanyalar | KPI altındaki "SMS ve e-posta birlikte", "Nedenleri satırlarda" / "Sorun yok", "Gönderilmeyi bekliyor", "Son 10 saniye" | (?) tooltip |
| Kampanyalar (boş) | "Bir brief yazarak ilk kampanyayı oluşturun." | "Henüz kampanya yok" ve buton |

Kalanlar:
- Bütün hata ve uyarı mesajları. Örnekler: limit aşımı, geçersiz LLM çıktısı ("Ayrıntılar"), dil uyuşmazlığı, zaman doğrulaması, "Brief değişti: kitle ve mesajlar yeniden oluşturulacak.".
- İki adımlı onayın "Emin misiniz?" sorusu ve iptal penceresinin metni.
- Kitle kararının notu ("Kitle belirtilmediği için tüm müşteriler seçildi.").
- Durum bildirimleri (iptal edildi, başlatılamadı, onay anı sayı farkı, tamamlandı).
