package mock

import "smartcampaign/internal/brief"

// Phrase banks of the mock copywriter (see copy.go). Hooks lead the SMS and the
// subject ("{{first_name}}, <hook>: <offer>!"); openings start the email body.
// Banks are keyed by special day, by topic and by tone. Nothing here invents a
// fact: no product, price, stock level or date that the brief did not give.

var turkishKit = kit{
	lang:      "tr",
	greetSMS:  []pair{{"{{first_name}},", "Sayın {{first_name}},"}, {"Merhaba {{first_name}},", "Merhaba {{first_name}},"}},
	greetBody: []pair{{"Merhaba {{first_name}},", "Sayın {{first_name}},"}, {"Selam {{first_name}},", "Merhaba {{first_name}},"}},
	hooks: map[string][]pair{
		brief.OccasionWomensDay:    {same("Kadınlar Günü'ne özel"), {"8 Mart'ı birlikte kutlayalım", "8 Mart'ı birlikte kutluyoruz"}, same("Kadınlar Günü kutlu olsun")},
		brief.OccasionMothersDay:   {same("Anneler Günü'ne özel"), {"annene en güzel hediye için", "annenize en güzel hediye için"}, same("Anneler Günü yaklaşıyor")},
		brief.OccasionFathersDay:   {same("Babalar Günü'ne özel"), {"babana sürpriz yapmanın tam zamanı", "babanıza sürpriz yapmanın tam zamanı"}, same("Babalar Günü yaklaşıyor")},
		brief.OccasionValentines:   {same("Sevgililer Günü'ne özel"), {"sevdiklerini mutlu etmenin tam zamanı", "sevdiklerinizi mutlu etmenin tam zamanı"}, same("14 Şubat'a özel")},
		brief.OccasionTeachersDay:  {same("Öğretmenler Günü'ne özel"), {"öğretmenine teşekkür etmenin en güzel yolu", "öğretmeninize teşekkür etmenin en güzel yolu"}},
		brief.OccasionChildrensDay: {same("23 Nisan'a özel"), same("Çocuk Bayramı neşesiyle")},
		brief.OccasionNewYear:      {same("yeni yıla özel"), same("yılbaşı heyecanı başladı"), same("yeni yıla güzel bir başlangıç")},
		brief.OccasionBlackFriday:  {same("Black Friday başladı"), same("Black Friday'e özel"), same("yılın en çok beklenen indirim günü geldi")},
		brief.OccasionRamadanFeast: {same("Ramazan Bayramı'na özel"), {"bayramın kutlu olsun", "bayramınız kutlu olsun"}},
		brief.OccasionEidAlAdha:    {same("Kurban Bayramı'na özel"), {"bayramın kutlu olsun", "bayramınız kutlu olsun"}},
		brief.OccasionHoliday:      {same("bayrama özel"), {"bayramın kutlu olsun", "bayramınız kutlu olsun"}},

		"cart":    {{"sepetindekiler seni bekliyor", "sepetinizdekiler sizi bekliyor"}, {"sepetinde unuttukların hâlâ burada", "sepetinizde unuttuklarınız hâlâ burada"}, {"sepetini tamamlamanın tam zamanı", "sepetinizi tamamlamanın tam zamanı"}},
		"winback": {{"seni özledik", "sizi özledik"}, same("uzun zamandır görüşemedik"), {"seni yeniden aramızda görmek istiyoruz", "sizi yeniden aramızda görmek istiyoruz"}},
		"welcome": {{"aramıza hoş geldin", "aramıza hoş geldiniz"}, {"ilk alışverişine özel", "ilk alışverişinize özel"}},
		"loyal":   {{"bizi tercih ettiğin için teşekkürler", "bizi tercih ettiğiniz için teşekkürler"}, same("sadık müşterilerimize özel")},
		"general": {{"sana özel", "size özel"}, same("kaçırılmayacak bir fırsat"), {"bugün sana güzel bir haberimiz var", "bugün size güzel bir haberimiz var"}},

		"tone:excited": {same("harika bir haberimiz var"), same("müjde")},
		"tone:elegant": {{"zarif bir sürpriz hazırladık", "sizin için zarif bir sürpriz hazırladık"}},
		"tone:urgent":  {same("son fırsat"), same("acele et, süre doluyor")},
	},
	opens: map[string][]pair{
		brief.OccasionWomensDay:    {same("8 Mart Dünya Kadınlar Günü'nü kutluyoruz."), {"Bu Kadınlar Günü'nde sana küçük bir sürprizimiz var.", "Bu Kadınlar Günü'nde size küçük bir sürprizimiz var."}},
		brief.OccasionMothersDay:   {{"Anneler Günü'nde annene kendini özel hissettirmenin tam zamanı.", "Anneler Günü'nde annenize kendini özel hissettirmenin tam zamanı."}, {"Anneler Günü yaklaşırken sana bir sürprizimiz var.", "Anneler Günü yaklaşırken size bir sürprizimiz var."}},
		brief.OccasionFathersDay:   {{"Babalar Günü'nde babanı mutlu etmenin tam zamanı.", "Babalar Günü'nde babanızı mutlu etmenin tam zamanı."}, {"Babalar Günü yaklaşırken sana bir sürprizimiz var.", "Babalar Günü yaklaşırken size bir sürprizimiz var."}},
		brief.OccasionValentines:   {{"Sevgililer Günü'nde sevdiklerine sürpriz yapmaya ne dersin?", "Sevgililer Günü'nde sevdiklerinize sürpriz yapmaya ne dersiniz?"}, same("Aşkın en güzel günü yaklaşıyor.")},
		brief.OccasionTeachersDay:  {same("Öğretmenler Günü'nde emeğe teşekkür etmenin tam zamanı."), same("Öğretmenlerimizin günü kutlu olsun.")},
		brief.OccasionChildrensDay: {same("23 Nisan Ulusal Egemenlik ve Çocuk Bayramı kutlu olsun!"), same("Çocukların bayramını birlikte kutluyoruz.")},
		brief.OccasionNewYear:      {{"Yeni yıla girerken sana bir hediyemiz var.", "Yeni yıla girerken size bir hediyemiz var."}, {"Yılbaşı alışverişin için hazırız.", "Yılbaşı alışverişiniz için hazırız."}},
		brief.OccasionBlackFriday:  {same("Black Friday indirimleri başladı!"), same("Yılın en çok beklenen indirim dönemi geldi.")},
		brief.OccasionRamadanFeast: {{"Ramazan Bayramını en içten dileklerimizle kutlarız.", "Ramazan Bayramınızı en içten dileklerimizle kutlarız."}},
		brief.OccasionEidAlAdha:    {{"Kurban Bayramını en içten dileklerimizle kutlarız.", "Kurban Bayramınızı en içten dileklerimizle kutlarız."}},
		brief.OccasionHoliday:      {{"Bayramını en içten dileklerimizle kutlarız.", "Bayramınızı en içten dileklerimizle kutlarız."}},

		"cart":    {{"Sepetine eklediğin ürünler hâlâ seni bekliyor.", "Sepetinize eklediğiniz ürünler hâlâ sizi bekliyor."}, {"Sepetindeki ürünler seni unutmadı.", "Sepetinizdeki ürünler sizi unutmadı."}},
		"winback": {{"Bir süredir seni göremedik ve özledik.", "Bir süredir sizi göremedik ve özledik."}, {"Seni yeniden görmek bizi çok mutlu eder.", "Sizi yeniden görmek bizi çok mutlu eder."}},
		"welcome": {{"Aramıza katıldığın için çok mutluyuz.", "Aramıza katıldığınız için çok mutluyuz."}},
		"loyal":   {{"Bizi tercih ettiğin için teşekkür ederiz.", "Bizi tercih ettiğiniz için teşekkür ederiz."}},
		"general": {{"Sana özel bir kampanyamız var.", "Size özel bir kampanyamız var."}, {"Bugün sana güzel bir haberimiz var.", "Bugün size güzel bir haberimiz var."}},
	},
	ctas: map[string][]pair{
		"general": {{"Hemen keşfet.", "Hemen keşfedin."}, {"Fırsatı kaçırma!", "Fırsatı kaçırmayın."}, {"Şimdi göz at.", "Şimdi göz atın."}, {"Alışverişe başla.", "Alışverişe başlayın."}},
		"cart":    {{"Sepetini tamamla.", "Sepetinizi tamamlayın."}, {"Sepetine dön.", "Sepetinize dönün."}},
		"gift":    {{"Hediyeni şimdi seç.", "Hediyenizi şimdi seçin."}},
	},
	offerSentence: []pair{{"%s seni bekliyor.", "%s sizi bekliyor."}, {"Şimdi %s fırsatından yararlanabilirsin.", "Şimdi %s fırsatından yararlanabilirsiniz."}, {"%s ile alışverişin daha keyifli.", "%s ile alışverişiniz daha keyifli."}},
	subjectOffer:  []pair{{"{{first_name}}, %s seni bekliyor", "{{first_name}}, %s sizi bekliyor"}, {"%s fırsatı başladı", "%s fırsatı başladı"}},
	signoff:       []pair{{"Görüşmek üzere!", "Saygılarımızla"}, {"Sevgiler", "İyi alışverişler dileriz"}},
	join: func(what, minP string) string {
		if minP == "" {
			return what
		}
		return minP + " " + what // "1000 TL ve üzeri alışverişte %20 indirim"
	},
	shortCode:  "Kod",
	shortMin:   func(n string) string { return n + " TL üzeri" },
	shortUntil: func(d string) string { return "Son gün " + d },
}

var englishKit = kit{
	lang:      "en",
	greetSMS:  []pair{same("Hi {{first_name}},"), same("Hello {{first_name}},")},
	greetBody: []pair{same("Hi {{first_name}},"), same("Hello {{first_name}},")},
	hooks: map[string][]pair{
		brief.OccasionWomensDay:    {same("Women's Day special"), same("celebrating Women's Day")},
		brief.OccasionMothersDay:   {same("Mother's Day special"), same("spoil your mum this Mother's Day")},
		brief.OccasionFathersDay:   {same("Father's Day special"), same("surprise your dad this Father's Day")},
		brief.OccasionValentines:   {same("Valentine's Day special"), same("show some love this Valentine's Day")},
		brief.OccasionTeachersDay:  {same("Teachers' Day special"), same("say thank you this Teachers' Day")},
		brief.OccasionChildrensDay: {same("Children's Day special"), same("happy Children's Day")},
		brief.OccasionNewYear:      {same("New Year special"), same("start the new year in style")},
		brief.OccasionBlackFriday:  {same("Black Friday is here"), same("Black Friday special")},
		brief.OccasionRamadanFeast: {same("Eid al-Fitr special"), same("happy Eid")},
		brief.OccasionEidAlAdha:    {same("Eid al-Adha special"), same("happy Eid")},
		brief.OccasionHoliday:      {same("holiday special"), same("happy holidays")},
		"cart":                     {same("your cart is waiting"), same("you left something in your cart")},
		"winback":                  {same("we miss you"), same("it's been a while")},
		"welcome":                  {same("welcome aboard"), same("for your first order")},
		"loyal":                    {same("thanks for shopping with us"), same("a thank-you just for you")},
		"general":                  {same("something special for you"), same("good news")},
		"tone:excited":             {same("big news")},
		"tone:elegant":             {same("a little treat for you")},
		"tone:urgent":              {same("last chance")},
	},
	opens: map[string][]pair{
		brief.OccasionWomensDay:    {same("Happy International Women's Day!"), same("This Women's Day we have a little surprise for you.")},
		brief.OccasionMothersDay:   {same("Mother's Day is the perfect moment to make mum feel special."), same("Mother's Day is coming up.")},
		brief.OccasionFathersDay:   {same("Father's Day is the perfect moment to treat dad."), same("Father's Day is coming up.")},
		brief.OccasionValentines:   {same("Valentine's Day is almost here."), same("Surprise someone you love this Valentine's Day.")},
		brief.OccasionTeachersDay:  {same("Teachers' Day is a time to say thank you.")},
		brief.OccasionChildrensDay: {same("Happy Children's Day!")},
		brief.OccasionNewYear:      {same("We have a gift for you as the new year begins."), same("Your New Year shopping starts here.")},
		brief.OccasionBlackFriday:  {same("Black Friday has started!")},
		brief.OccasionRamadanFeast: {same("We wish you a happy Eid al-Fitr.")},
		brief.OccasionEidAlAdha:    {same("We wish you a happy Eid al-Adha.")},
		brief.OccasionHoliday:      {same("We wish you happy holidays.")},
		"cart":                     {same("The items you added to your cart are still waiting for you."), same("Your cart has not forgotten you.")},
		"winback":                  {same("We have not seen you for a while and we miss you.")},
		"welcome":                  {same("We are delighted to have you with us.")},
		"loyal":                    {same("Thank you for choosing us.")},
		"general":                  {same("We have a special campaign for you."), same("We have some good news today.")},
	},
	ctas: map[string][]pair{
		"general": {same("Shop now."), same("Take a look."), same("Don't miss out!")},
		"cart":    {same("Complete your order.")},
		"gift":    {same("Pick a gift today.")},
	},
	offerSentence: []pair{same("Enjoy %s."), same("Treat yourself to %s.")},
	subjectOffer:  []pair{same("{{first_name}}, enjoy %s"), same("%s starts now")},
	signoff:       []pair{same("Best regards"), same("See you soon")},
	join: func(what, minP string) string {
		if minP == "" {
			return what
		}
		return what + " " + minP // "20% off on orders of 1000 TL and over"
	},
	shortCode:  "Code",
	shortMin:   func(n string) string { return "over " + n + " TL" },
	shortUntil: func(d string) string { return "Until " + d },
}

var germanKit = kit{
	lang:      "de",
	greetSMS:  []pair{same("Hallo {{first_name}},"), same("Guten Tag {{first_name}},")},
	greetBody: []pair{same("Hallo {{first_name}},"), same("Guten Tag {{first_name}},")},
	hooks: map[string][]pair{
		brief.OccasionWomensDay:    {same("zum Weltfrauentag"), same("Weltfrauentag-Special")},
		brief.OccasionMothersDay:   {same("zum Muttertag"), same("Muttertag-Special")},
		brief.OccasionFathersDay:   {same("zum Vatertag"), same("Vatertag-Special")},
		brief.OccasionValentines:   {same("zum Valentinstag"), same("Valentinstag-Special")},
		brief.OccasionTeachersDay:  {same("zum Lehrertag")},
		brief.OccasionChildrensDay: {same("zum Kindertag")},
		brief.OccasionNewYear:      {same("zum Jahreswechsel"), same("Neujahrs-Special")},
		brief.OccasionBlackFriday:  {same("Black Friday ist da"), same("Black-Friday-Special")},
		brief.OccasionRamadanFeast: {same("zum Zuckerfest")},
		brief.OccasionEidAlAdha:    {same("zum Opferfest")},
		brief.OccasionHoliday:      {same("zu den Feiertagen")},
		"cart":                     {same("Ihr Warenkorb wartet auf Sie"), same("Sie haben etwas im Warenkorb vergessen")},
		"winback":                  {same("wir vermissen Sie"), same("lange nicht gesehen")},
		"welcome":                  {same("willkommen bei uns"), same("zu Ihrer ersten Bestellung")},
		"loyal":                    {same("danke für Ihre Treue")},
		"general":                  {same("etwas Besonderes für Sie"), same("gute Nachrichten")},
		"tone:excited":             {same("tolle Neuigkeiten")},
		"tone:elegant":             {same("eine kleine Aufmerksamkeit")},
		"tone:urgent":              {same("letzte Chance")},
	},
	opens: map[string][]pair{
		brief.OccasionWomensDay:    {same("Wir feiern den Weltfrauentag."), same("Zum Weltfrauentag haben wir eine kleine Überraschung für Sie.")},
		brief.OccasionMothersDay:   {same("Der Muttertag ist der perfekte Anlass für eine Freude."), same("Bald ist Muttertag.")},
		brief.OccasionFathersDay:   {same("Der Vatertag ist der perfekte Anlass für eine Überraschung."), same("Bald ist Vatertag.")},
		brief.OccasionValentines:   {same("Bald ist Valentinstag."), same("Überraschen Sie einen lieben Menschen zum Valentinstag.")},
		brief.OccasionTeachersDay:  {same("Zum Lehrertag sagen wir Danke.")},
		brief.OccasionChildrensDay: {same("Wir feiern den Kindertag.")},
		brief.OccasionNewYear:      {same("Zum neuen Jahr haben wir ein Geschenk für Sie."), same("Ihr Neujahrseinkauf beginnt hier.")},
		brief.OccasionBlackFriday:  {same("Black Friday hat begonnen!")},
		brief.OccasionRamadanFeast: {same("Wir wünschen Ihnen ein frohes Zuckerfest.")},
		brief.OccasionEidAlAdha:    {same("Wir wünschen Ihnen ein frohes Opferfest.")},
		brief.OccasionHoliday:      {same("Wir wünschen Ihnen schöne Feiertage.")},
		"cart":                     {same("Die Artikel in Ihrem Warenkorb warten noch auf Sie."), same("Ihr Warenkorb hat Sie nicht vergessen.")},
		"winback":                  {same("Wir haben Sie eine Weile nicht gesehen und vermissen Sie.")},
		"welcome":                  {same("Schön, dass Sie dabei sind.")},
		"loyal":                    {same("Vielen Dank, dass Sie bei uns einkaufen.")},
		"general":                  {same("Wir haben eine besondere Aktion für Sie."), same("Heute haben wir gute Nachrichten.")},
	},
	ctas: map[string][]pair{
		"general": {same("Jetzt entdecken."), same("Schauen Sie vorbei."), same("Nicht verpassen!")},
		"cart":    {same("Bestellung jetzt abschließen.")},
		"gift":    {same("Jetzt ein Geschenk aussuchen.")},
	},
	offerSentence: []pair{same("Profitieren Sie von %s."), same("Für Sie: %s.")},
	subjectOffer:  []pair{same("{{first_name}}, Ihr Vorteil: %s"), same("Jetzt: %s")},
	signoff:       []pair{same("Viele Grüße"), same("Herzliche Grüße")},
	join: func(what, minP string) string {
		if minP == "" {
			return what
		}
		return what + " " + minP // "20 % Rabatt ab einem Einkaufswert von 1000 TL"
	},
	shortCode:  "Code",
	shortMin:   func(n string) string { return "ab " + n + " TL" },
	shortUntil: func(d string) string { return "Bis " + d },
}
