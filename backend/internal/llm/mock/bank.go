package mock

import "smartcampaign/internal/brief"

// The sentence bank of the mock. The mock writes the Turkish source only from
// these lines, and every line carries the same sentence in the eight other
// languages, so the mock translator can translate any text made of them,
// including a text the user edited by hand (see translate.go).
//
// A line has slots for what may differ from campaign to campaign:
//
//	{1} {2}          a number (an amount, a percentage)
//	{d}              a date (31.12.2026)
//	{c}              a coupon code
//	{s}              the offer's scope ("kış ürünleri"), copied as written
//	{o}              the offer phrase, itself made of bank lines
//	{{first_name}}   a placeholder (any {{variable}})
//
// The translation takes the slot values from the source text, so a number, a
// date, a code or a placeholder changed in the Turkish text changes in every
// language. A trailing ".", "!" or "," is not part of a line: it is the
// source's own punctuation and is kept as it is.
//
// Hooks lead the SMS and the subject ("{{first_name}}, <hook>: <offer>!");
// openings start the email body. Nothing here invents a fact: no product,
// price, stock level or date that the brief did not give.

// bankLangs are the languages of a line's translations, in this order.
var bankLangs = [8]string{"en", "de", "fr", "es", "it", "nl", "pt", "ar"}

// tx is one line in the eight languages of bankLangs.
type tx [8]string

// line is one entry of the bank: the Turkish sentence in the informal and the
// formal register, and its translations. f is the formal register's own
// translation, for the few lines whose two registers say different things
// ("Görüşmek üzere!" / "Saygılarımızla").
type line struct {
	tr pair
	t  tx
	f  *tx
}

func (l line) in(formal bool) string { return l.tr.in(formal) }

func L(tr pair, t tx) line     { return line{tr: tr, t: t} }
func LF(tr pair, t, f tx) line { return line{tr: tr, t: t, f: &f} }

// Lines used under more than one key.
var (
	happyHoliday = L(pair{"bayramın kutlu olsun", "bayramınız kutlu olsun"}, tx{"happy holidays", "frohe Feiertage", "joyeuses fêtes", "felices fiestas", "buone feste", "fijne feestdagen", "boas festas", "كل عام وأنتم بخير"})
	goodNewsHook = L(pair{"bugün sana güzel bir haberimiz var", "bugün size güzel bir haberimiz var"}, goodNewsToday)
	goodNewsOpen = L(pair{"Bugün sana güzel bir haberimiz var.", "Bugün size güzel bir haberimiz var."}, goodNewsToday)

	greetHello = L(same("Merhaba {{first_name}},"), tx{"Hello {{first_name}},", "Hallo {{first_name}},", "Bonjour {{first_name}},", "Hola {{first_name}},", "Ciao {{first_name}},", "Hallo {{first_name}},", "Olá {{first_name}},", "مرحباً {{first_name}},"})
	greetDear  = tx{"Dear {{first_name}},", "Guten Tag {{first_name}},", "Bonjour {{first_name}},", "Hola {{first_name}},", "Gentile {{first_name}},", "Beste {{first_name}},", "Olá {{first_name}},", "مرحباً {{first_name}},"}
	nameOnly   = tx{"{{first_name}},", "{{first_name}},", "{{first_name}},", "{{first_name}},", "{{first_name}},", "{{first_name}},", "{{first_name}},", "{{first_name}},"}
)

var goodNewsToday = tx{"we have some good news for you today", "heute haben wir gute Nachrichten für Sie", "aujourd'hui, nous avons une bonne nouvelle pour vous", "hoy tenemos una buena noticia para ti", "oggi abbiamo una bella notizia per te", "vandaag hebben we goed nieuws voor je", "hoje temos uma ótima notícia para você", "لدينا اليوم خبر سار لكم"}

var turkishKit = kit{
	lang: "tr",
	greetSMS: []line{
		LF(pair{"{{first_name}},", "Sayın {{first_name}},"}, nameOnly, greetDear),
		greetHello,
	},
	greetBody: []line{
		LF(pair{"Merhaba {{first_name}},", "Sayın {{first_name}},"}, greetHello.t, greetDear),
		LF(pair{"Selam {{first_name}},", "Merhaba {{first_name}},"},
			tx{"Hi {{first_name}},", "Hallo {{first_name}},", "Salut {{first_name}},", "Hola {{first_name}},", "Ciao {{first_name}},", "Hoi {{first_name}},", "Oi {{first_name}},", "أهلاً {{first_name}},"}, greetHello.t),
	},
	hooks: map[string][]line{
		brief.OccasionWomensDay: {
			L(same("Kadınlar Günü'ne özel"), tx{"Women's Day special", "zum Weltfrauentag", "spécial Journée de la femme", "especial Día de la Mujer", "speciale Festa della donna", "speciaal voor Vrouwendag", "especial Dia da Mulher", "خاص بيوم المرأة"}),
			L(pair{"8 Mart'ı birlikte kutlayalım", "8 Mart'ı birlikte kutluyoruz"}, tx{"let's celebrate 8 March together", "feiern wir gemeinsam den 8. März", "fêtons ensemble le 8 mars", "celebremos juntos el 8 de marzo", "festeggiamo insieme l'8 marzo", "laten we samen 8 maart vieren", "vamos celebrar juntos o 8 de março", "لنحتفل معاً بيوم 8 مارس"}),
			L(same("Kadınlar Günü kutlu olsun"), tx{"happy Women's Day", "alles Gute zum Weltfrauentag", "bonne Journée de la femme", "feliz Día de la Mujer", "buona Festa della donna", "fijne Vrouwendag", "feliz Dia da Mulher", "يوم مرأة سعيد"}),
		},
		brief.OccasionMothersDay: {
			L(same("Anneler Günü'ne özel"), tx{"Mother's Day special", "zum Muttertag", "spécial fête des Mères", "especial Día de la Madre", "speciale Festa della mamma", "speciaal voor Moederdag", "especial Dia das Mães", "خاص بعيد الأم"}),
			L(pair{"annene en güzel hediye için", "annenize en güzel hediye için"}, tx{"for the perfect gift for your mum", "für das schönste Geschenk für Ihre Mutter", "pour le plus beau cadeau pour votre maman", "para el mejor regalo para tu madre", "per il regalo più bello per la tua mamma", "voor het mooiste cadeau voor je moeder", "para o presente perfeito para sua mãe", "لأجمل هدية لوالدتكم"}),
			L(same("Anneler Günü yaklaşıyor"), tx{"Mother's Day is coming", "bald ist Muttertag", "la fête des Mères approche", "se acerca el Día de la Madre", "la Festa della mamma si avvicina", "Moederdag komt eraan", "o Dia das Mães está chegando", "عيد الأم يقترب"}),
		},
		brief.OccasionFathersDay: {
			L(same("Babalar Günü'ne özel"), tx{"Father's Day special", "zum Vatertag", "spécial fête des Pères", "especial Día del Padre", "speciale Festa del papà", "speciaal voor Vaderdag", "especial Dia dos Pais", "خاص بعيد الأب"}),
			L(pair{"babana sürpriz yapmanın tam zamanı", "babanıza sürpriz yapmanın tam zamanı"}, tx{"the perfect time to surprise your dad", "die perfekte Zeit, Ihren Vater zu überraschen", "le moment idéal pour surprendre votre papa", "el momento perfecto para sorprender a tu padre", "il momento giusto per sorprendere il tuo papà", "het perfecte moment om je vader te verrassen", "o momento certo para surpreender seu pai", "الوقت المثالي لمفاجأة والدكم"}),
			L(same("Babalar Günü yaklaşıyor"), tx{"Father's Day is coming", "bald ist Vatertag", "la fête des Pères approche", "se acerca el Día del Padre", "la Festa del papà si avvicina", "Vaderdag komt eraan", "o Dia dos Pais está chegando", "عيد الأب يقترب"}),
		},
		brief.OccasionValentines: {
			L(same("Sevgililer Günü'ne özel"), tx{"Valentine's Day special", "zum Valentinstag", "spécial Saint-Valentin", "especial San Valentín", "speciale San Valentino", "speciaal voor Valentijnsdag", "especial Dia dos Namorados", "خاص بعيد الحب"}),
			L(pair{"sevdiklerini mutlu etmenin tam zamanı", "sevdiklerinizi mutlu etmenin tam zamanı"}, tx{"the perfect time to make your loved ones happy", "die perfekte Zeit, Ihre Liebsten glücklich zu machen", "le moment idéal pour faire plaisir à ceux que vous aimez", "el momento perfecto para hacer felices a tus seres queridos", "il momento giusto per rendere felici le persone che ami", "het perfecte moment om je dierbaren blij te maken", "o momento certo para fazer feliz quem você ama", "الوقت المثالي لإسعاد أحبائكم"}),
			L(same("14 Şubat'a özel"), tx{"a 14 February special", "zum 14. Februar", "spécial 14 février", "especial 14 de febrero", "speciale 14 febbraio", "speciaal voor 14 februari", "especial 14 de fevereiro", "خاص بيوم 14 فبراير"}),
		},
		brief.OccasionTeachersDay: {
			L(same("Öğretmenler Günü'ne özel"), tx{"Teachers' Day special", "zum Lehrertag", "spécial Journée des enseignants", "especial Día del Maestro", "speciale Giornata degli insegnanti", "speciaal voor de Dag van de Leraar", "especial Dia do Professor", "خاص بيوم المعلم"}),
			L(pair{"öğretmenine teşekkür etmenin en güzel yolu", "öğretmeninize teşekkür etmenin en güzel yolu"}, tx{"the nicest way to thank your teacher", "die schönste Art, Ihrer Lehrkraft zu danken", "la plus belle façon de dire merci à vos enseignants", "la mejor forma de dar las gracias a tu docente", "il modo più bello per ringraziare chi ti ha insegnato", "de mooiste manier om je leraar te bedanken", "a melhor forma de agradecer a quem te ensinou", "أجمل طريقة لشكر معلميكم"}),
		},
		brief.OccasionChildrensDay: {
			L(same("23 Nisan'a özel"), tx{"a 23 April special", "zum 23. April", "spécial 23 avril", "especial 23 de abril", "speciale 23 aprile", "speciaal voor 23 april", "especial 23 de abril", "خاص بيوم 23 أبريل"}),
			L(same("Çocuk Bayramı neşesiyle"), tx{"with the joy of Children's Day", "mit der Freude des Kindertags", "avec la joie de la fête des enfants", "con la alegría del Día del Niño", "con la gioia della Festa dei bambini", "met de vreugde van Kinderdag", "com a alegria do Dia das Crianças", "بفرحة يوم الطفل"}),
		},
		brief.OccasionNewYear: {
			L(same("yeni yıla özel"), tx{"New Year special", "zum Jahreswechsel", "spécial Nouvel An", "especial Año Nuevo", "speciale Capodanno", "speciaal voor het nieuwe jaar", "especial de Ano-Novo", "خاص بالعام الجديد"}),
			L(same("yılbaşı heyecanı başladı"), tx{"the New Year excitement has begun", "die Vorfreude auf Silvester hat begonnen", "l'effervescence du Nouvel An a commencé", "ya empezó la emoción de Año Nuevo", "è iniziata l'emozione di Capodanno", "de spanning van oud en nieuw is begonnen", "a emoção do Ano-Novo começou", "بدأت حماسة رأس السنة"}),
			L(same("yeni yıla güzel bir başlangıç"), tx{"a great start to the new year", "ein schöner Start ins neue Jahr", "un bon départ pour la nouvelle année", "un buen comienzo para el año nuevo", "un bell'inizio per il nuovo anno", "een mooi begin van het nieuwe jaar", "um ótimo começo de ano", "بداية جميلة للعام الجديد"}),
		},
		brief.OccasionBlackFriday: {
			L(same("Black Friday başladı"), tx{"Black Friday is here", "Black Friday ist da", "le Black Friday est là", "llegó el Black Friday", "è arrivato il Black Friday", "Black Friday is begonnen", "a Black Friday chegou", "انطلقت الجمعة البيضاء"}),
			L(same("Black Friday'e özel"), tx{"Black Friday special", "Black-Friday-Special", "spécial Black Friday", "especial Black Friday", "speciale Black Friday", "Black Friday-aanbieding", "especial Black Friday", "خاص بالجمعة البيضاء"}),
			L(same("yılın en çok beklenen indirim günü geldi"), tx{"the most awaited sale day of the year is here", "der beliebteste Rabatttag des Jahres ist da", "le jour de soldes le plus attendu de l'année est arrivé", "llegó el día de descuentos más esperado del año", "è arrivato il giorno di sconti più atteso dell'anno", "de meest verwachte kortingsdag van het jaar is er", "chegou o dia de descontos mais esperado do ano", "وصل يوم التخفيضات الأكثر انتظاراً في العام"}),
		},
		brief.OccasionRamadanFeast: {
			L(same("Ramazan Bayramı'na özel"), tx{"Eid al-Fitr special", "zum Zuckerfest", "spécial Aïd el-Fitr", "especial Eid al-Fitr", "speciale Eid al-Fitr", "speciaal voor het Suikerfeest", "especial Eid al-Fitr", "خاص بعيد الفطر"}),
			happyHoliday,
		},
		brief.OccasionEidAlAdha: {
			L(same("Kurban Bayramı'na özel"), tx{"Eid al-Adha special", "zum Opferfest", "spécial Aïd al-Adha", "especial Eid al-Adha", "speciale Eid al-Adha", "speciaal voor het Offerfeest", "especial Eid al-Adha", "خاص بعيد الأضحى"}),
			happyHoliday,
		},
		brief.OccasionHoliday: {
			L(same("bayrama özel"), tx{"holiday special", "zu den Feiertagen", "spécial fêtes", "especial fiestas", "speciale feste", "speciaal voor de feestdagen", "especial de festas", "خاص بالعيد"}),
			happyHoliday,
		},

		"cart": {
			L(pair{"sepetindekiler seni bekliyor", "sepetinizdekiler sizi bekliyor"}, tx{"your cart is waiting for you", "Ihr Warenkorb wartet auf Sie", "votre panier vous attend", "tu carrito te espera", "il tuo carrello ti aspetta", "je winkelwagen wacht op je", "seu carrinho está esperando por você", "سلة مشترياتكم بانتظاركم"}),
			L(pair{"sepetinde unuttukların hâlâ burada", "sepetinizde unuttuklarınız hâlâ burada"}, tx{"what you left in your cart is still here", "Ihre vergessenen Artikel liegen noch im Warenkorb", "ce que vous avez laissé dans votre panier est toujours là", "lo que dejaste en tu carrito sigue aquí", "quello che hai lasciato nel carrello è ancora qui", "wat je in je winkelwagen liet staan, is er nog", "o que você deixou no carrinho ainda está aqui", "ما تركتموه في السلة لا يزال هنا"}),
			L(pair{"sepetini tamamlamanın tam zamanı", "sepetinizi tamamlamanın tam zamanı"}, tx{"the perfect time to complete your order", "der perfekte Moment, Ihre Bestellung abzuschließen", "le moment idéal pour finaliser votre panier", "el momento perfecto para completar tu compra", "il momento giusto per completare il tuo ordine", "het perfecte moment om je bestelling af te ronden", "o momento certo para finalizar sua compra", "الوقت المثالي لإتمام طلبكم"}),
		},
		"winback": {
			L(pair{"seni özledik", "sizi özledik"}, tx{"we miss you", "wir vermissen Sie", "vous nous manquez", "te echamos de menos", "ci manchi", "we missen je", "sentimos sua falta", "اشتقنا إليكم"}),
			L(same("uzun zamandır görüşemedik"), tx{"it's been a while", "lange nicht gesehen", "cela fait longtemps", "hace tiempo que no nos vemos", "è passato un po' di tempo", "het is alweer even geleden", "faz tempo que não nos vemos", "مر وقت طويل منذ آخر زيارة"}),
			L(pair{"seni yeniden aramızda görmek istiyoruz", "sizi yeniden aramızda görmek istiyoruz"}, tx{"we'd love to see you back", "wir würden uns freuen, Sie wiederzusehen", "nous serions ravis de vous revoir", "nos encantaría volver a verte", "ci farebbe piacere rivederti", "we zien je graag terug", "queremos ver você de volta", "نتطلع لرؤيتكم مجدداً"}),
		},
		"welcome": {
			L(pair{"aramıza hoş geldin", "aramıza hoş geldiniz"}, tx{"welcome aboard", "willkommen bei uns", "bienvenue parmi nous", "te damos la bienvenida", "ti diamo il benvenuto", "welkom bij ons", "boas-vindas", "أهلاً بكم معنا"}),
			L(pair{"ilk alışverişine özel", "ilk alışverişinize özel"}, tx{"for your first order", "zu Ihrer ersten Bestellung", "pour votre première commande", "para tu primera compra", "per il tuo primo acquisto", "voor je eerste bestelling", "para sua primeira compra", "لطلبكم الأول"}),
		},
		"loyal": {
			L(pair{"bizi tercih ettiğin için teşekkürler", "bizi tercih ettiğiniz için teşekkürler"}, tx{"thanks for choosing us", "danke, dass Sie uns wählen", "merci de nous avoir choisis", "gracias por elegirnos", "grazie per averci scelto", "bedankt dat je voor ons kiest", "obrigado por nos escolher", "شكراً لاختياركم لنا"}),
			L(same("sadık müşterilerimize özel"), tx{"for our loyal customers", "für unsere treuen Kunden", "pour nos clients fidèles", "para nuestros clientes fieles", "per i nostri clienti fedeli", "voor onze trouwe klanten", "para nossos clientes fiéis", "لعملائنا الأوفياء"}),
		},
		"general": {
			L(pair{"sana özel", "size özel"}, tx{"just for you", "nur für Sie", "rien que pour vous", "solo para ti", "solo per te", "speciaal voor jou", "só para você", "خصيصاً لكم"}),
			L(same("kaçırılmayacak bir fırsat"), tx{"an offer not to miss", "ein Angebot, das Sie nicht verpassen sollten", "une offre à ne pas manquer", "una oferta que no te puedes perder", "un'offerta da non perdere", "een aanbieding die je niet wilt missen", "uma oferta imperdível", "عرض لا يفوَّت"}),
			goodNewsHook,
		},

		"tone:excited": {
			L(same("harika bir haberimiz var"), tx{"we have great news", "wir haben tolle Neuigkeiten", "nous avons une super nouvelle", "tenemos una gran noticia", "abbiamo una notizia fantastica", "we hebben geweldig nieuws", "temos uma notícia incrível", "لدينا خبر رائع"}),
			L(same("müjde"), tx{"good news", "gute Nachrichten", "bonne nouvelle", "buenas noticias", "bella notizia", "goed nieuws", "boa notícia", "بشرى سارة"}),
		},
		"tone:elegant": {
			L(pair{"zarif bir sürpriz hazırladık", "sizin için zarif bir sürpriz hazırladık"}, tx{"we have prepared an elegant surprise", "wir haben eine elegante Überraschung vorbereitet", "nous vous avons préparé une surprise élégante", "te hemos preparado una sorpresa elegante", "abbiamo preparato una sorpresa elegante", "we hebben een elegante verrassing voorbereid", "preparamos uma surpresa elegante", "أعددنا لكم مفاجأة أنيقة"}),
		},
		"tone:urgent": {
			L(same("son fırsat"), tx{"last chance", "letzte Chance", "dernière chance", "última oportunidad", "ultima occasione", "laatste kans", "última chance", "الفرصة الأخيرة"}),
			L(same("acele et, süre doluyor"), tx{"hurry, time is running out", "beeilen Sie sich, die Zeit läuft ab", "faites vite, le temps presse", "date prisa, se acaba el tiempo", "affrettati, il tempo sta per scadere", "wees er snel bij, de tijd dringt", "corra, o tempo está acabando", "أسرعوا، الوقت ينفد"}),
		},
	},
	opens: map[string][]line{
		brief.OccasionWomensDay: {
			L(same("8 Mart Dünya Kadınlar Günü'nü kutluyoruz."), tx{"We are celebrating International Women's Day on 8 March.", "Wir feiern den Weltfrauentag am 8. März.", "Nous célébrons la Journée internationale des droits des femmes le 8 mars.", "Celebramos el Día Internacional de la Mujer el 8 de marzo.", "Celebriamo la Giornata internazionale della donna l'8 marzo.", "We vieren op 8 maart Internationale Vrouwendag.", "Celebramos o Dia Internacional da Mulher em 8 de março.", "نحتفل باليوم العالمي للمرأة في 8 مارس."}),
			L(pair{"Bu Kadınlar Günü'nde sana küçük bir sürprizimiz var.", "Bu Kadınlar Günü'nde size küçük bir sürprizimiz var."}, tx{"This Women's Day we have a little surprise for you.", "Zum Weltfrauentag haben wir eine kleine Überraschung für Sie.", "Pour la Journée de la femme, nous avons une petite surprise pour vous.", "Este Día de la Mujer tenemos una pequeña sorpresa para ti.", "Per la Festa della donna abbiamo una piccola sorpresa per te.", "Voor Vrouwendag hebben we een kleine verrassing voor je.", "Neste Dia da Mulher temos uma pequena surpresa para você.", "في يوم المرأة لدينا مفاجأة صغيرة لكم."}),
		},
		brief.OccasionMothersDay: {
			L(pair{"Anneler Günü'nde annene kendini özel hissettirmenin tam zamanı.", "Anneler Günü'nde annenize kendini özel hissettirmenin tam zamanı."}, tx{"Mother's Day is the perfect moment to make your mum feel special.", "Der Muttertag ist der perfekte Anlass, Ihrer Mutter eine Freude zu machen.", "La fête des Mères est le moment idéal pour gâter votre maman.", "El Día de la Madre es el momento perfecto para que tu madre se sienta especial.", "La Festa della mamma è il momento giusto per far sentire speciale la tua mamma.", "Moederdag is het perfecte moment om je moeder in het zonnetje te zetten.", "O Dia das Mães é o momento certo para fazer sua mãe se sentir especial.", "عيد الأم هو الوقت المثالي لإسعاد والدتكم."}),
			L(pair{"Anneler Günü yaklaşırken sana bir sürprizimiz var.", "Anneler Günü yaklaşırken size bir sürprizimiz var."}, tx{"With Mother's Day coming up, we have a surprise for you.", "Bald ist Muttertag, und wir haben eine Überraschung für Sie.", "À l'approche de la fête des Mères, nous avons une surprise pour vous.", "Se acerca el Día de la Madre y tenemos una sorpresa para ti.", "Con la Festa della mamma alle porte abbiamo una sorpresa per te.", "Nu Moederdag eraan komt, hebben we een verrassing voor je.", "Com a chegada do Dia das Mães, temos uma surpresa para você.", "مع اقتراب عيد الأم لدينا مفاجأة لكم."}),
		},
		brief.OccasionFathersDay: {
			L(pair{"Babalar Günü'nde babanı mutlu etmenin tam zamanı.", "Babalar Günü'nde babanızı mutlu etmenin tam zamanı."}, tx{"Father's Day is the perfect moment to make your dad happy.", "Der Vatertag ist der perfekte Anlass, Ihrem Vater eine Freude zu machen.", "La fête des Pères est le moment idéal pour faire plaisir à votre papa.", "El Día del Padre es el momento perfecto para hacer feliz a tu padre.", "La Festa del papà è il momento giusto per rendere felice il tuo papà.", "Vaderdag is het perfecte moment om je vader blij te maken.", "O Dia dos Pais é o momento certo para fazer seu pai feliz.", "عيد الأب هو الوقت المثالي لإسعاد والدكم."}),
			L(pair{"Babalar Günü yaklaşırken sana bir sürprizimiz var.", "Babalar Günü yaklaşırken size bir sürprizimiz var."}, tx{"With Father's Day coming up, we have a surprise for you.", "Bald ist Vatertag, und wir haben eine Überraschung für Sie.", "À l'approche de la fête des Pères, nous avons une surprise pour vous.", "Se acerca el Día del Padre y tenemos una sorpresa para ti.", "Con la Festa del papà alle porte abbiamo una sorpresa per te.", "Nu Vaderdag eraan komt, hebben we een verrassing voor je.", "Com a chegada do Dia dos Pais, temos uma surpresa para você.", "مع اقتراب عيد الأب لدينا مفاجأة لكم."}),
		},
		brief.OccasionValentines: {
			L(pair{"Sevgililer Günü'nde sevdiklerine sürpriz yapmaya ne dersin?", "Sevgililer Günü'nde sevdiklerinize sürpriz yapmaya ne dersiniz?"}, tx{"How about surprising your loved ones this Valentine's Day?", "Wie wäre es mit einer Überraschung für Ihre Liebsten zum Valentinstag?", "Et si vous surpreniez ceux que vous aimez pour la Saint-Valentin ?", "¿Qué tal sorprender a tus seres queridos este San Valentín?", "Che ne dici di sorprendere le persone che ami a San Valentino?", "Zin om je dierbaren te verrassen op Valentijnsdag?", "Que tal surpreender quem você ama neste Dia dos Namorados?", "ما رأيكم بمفاجأة أحبائكم في عيد الحب؟"}),
			L(same("Aşkın en güzel günü yaklaşıyor."), tx{"The most romantic day of the year is almost here.", "Der schönste Tag der Liebe steht vor der Tür.", "Le plus beau jour de l'amour approche.", "Se acerca el día más bonito del amor.", "Il giorno più bello dell'amore si avvicina.", "De mooiste dag van de liefde komt eraan.", "O dia mais bonito do amor está chegando.", "يقترب أجمل أيام الحب."}),
		},
		brief.OccasionTeachersDay: {
			L(same("Öğretmenler Günü'nde emeğe teşekkür etmenin tam zamanı."), tx{"Teachers' Day is the perfect time to say thank you.", "Der Lehrertag ist der perfekte Anlass, Danke zu sagen.", "La Journée des enseignants est le moment idéal pour dire merci.", "El Día del Maestro es el momento perfecto para dar las gracias.", "La Giornata degli insegnanti è il momento giusto per dire grazie.", "De Dag van de Leraar is het perfecte moment om dank je wel te zeggen.", "O Dia do Professor é o momento certo para agradecer.", "يوم المعلم هو الوقت المثالي لتقديم الشكر."}),
			L(same("Öğretmenlerimizin günü kutlu olsun."), tx{"Happy Teachers' Day to all our teachers.", "Alles Gute zum Lehrertag an alle Lehrkräfte.", "Bonne Journée à tous les enseignants.", "Feliz Día del Maestro a todos los docentes.", "Buona Giornata a tutti gli insegnanti.", "Fijne Dag van de Leraar aan alle leraren.", "Feliz Dia do Professor a todos os docentes.", "كل عام ومعلمونا بخير."}),
		},
		brief.OccasionChildrensDay: {
			L(same("23 Nisan Ulusal Egemenlik ve Çocuk Bayramı kutlu olsun!"), tx{"Happy 23 April National Sovereignty and Children's Day!", "Alles Gute zum Tag der nationalen Souveränität und des Kindes am 23. April!", "Bonne fête de la souveraineté nationale et des enfants du 23 avril !", "¡Feliz Día de la Soberanía Nacional y del Niño, 23 de abril!", "Buona Festa della sovranità nazionale e dei bambini del 23 aprile!", "Fijne Dag van de Nationale Soevereiniteit en het Kind op 23 april!", "Feliz Dia da Soberania Nacional e das Crianças, 23 de abril!", "عيد سيادة وطنية ويوم طفل سعيد في 23 أبريل!"}),
			L(same("Çocukların bayramını birlikte kutluyoruz."), tx{"We are celebrating Children's Day together.", "Wir feiern gemeinsam den Kindertag.", "Nous fêtons ensemble la fête des enfants.", "Celebramos juntos el Día del Niño.", "Festeggiamo insieme la Festa dei bambini.", "We vieren samen Kinderdag.", "Celebramos juntos o Dia das Crianças.", "نحتفل معاً بيوم الطفل."}),
		},
		brief.OccasionNewYear: {
			L(pair{"Yeni yıla girerken sana bir hediyemiz var.", "Yeni yıla girerken size bir hediyemiz var."}, tx{"We have a gift for you as the new year begins.", "Zum neuen Jahr haben wir ein Geschenk für Sie.", "Pour la nouvelle année, nous avons un cadeau pour vous.", "Para empezar el año nuevo, tenemos un regalo para ti.", "Per il nuovo anno abbiamo un regalo per te.", "Voor het nieuwe jaar hebben we een cadeau voor je.", "Para começar o ano novo, temos um presente para você.", "مع بداية العام الجديد لدينا هدية لكم."}),
			L(pair{"Yılbaşı alışverişin için hazırız.", "Yılbaşı alışverişiniz için hazırız."}, tx{"We are ready for your New Year shopping.", "Wir sind bereit für Ihren Neujahrseinkauf.", "Nous sommes prêts pour vos achats du Nouvel An.", "Estamos listos para tus compras de Año Nuevo.", "Siamo pronti per i tuoi acquisti di Capodanno.", "We zijn klaar voor je inkopen voor oud en nieuw.", "Estamos prontos para suas compras de Ano-Novo.", "نحن جاهزون لتسوقكم في رأس السنة."}),
		},
		brief.OccasionBlackFriday: {
			L(same("Black Friday indirimleri başladı!"), tx{"Black Friday deals have started!", "Die Black-Friday-Angebote haben begonnen!", "Les offres du Black Friday ont commencé !", "¡Han empezado las ofertas del Black Friday!", "Le offerte del Black Friday sono iniziate!", "De Black Friday-aanbiedingen zijn begonnen!", "As ofertas da Black Friday começaram!", "بدأت عروض الجمعة البيضاء!"}),
			L(same("Yılın en çok beklenen indirim dönemi geldi."), tx{"The most awaited sale season of the year is here.", "Die meisterwartete Rabattzeit des Jahres ist da.", "La période de réductions la plus attendue de l'année est arrivée.", "Llegó la temporada de descuentos más esperada del año.", "È arrivato il periodo di sconti più atteso dell'anno.", "De meest verwachte kortingsperiode van het jaar is er.", "Chegou a temporada de descontos mais esperada do ano.", "حان موسم التخفيضات الأكثر انتظاراً في العام."}),
		},
		brief.OccasionRamadanFeast: {
			L(pair{"Ramazan Bayramını en içten dileklerimizle kutlarız.", "Ramazan Bayramınızı en içten dileklerimizle kutlarız."}, tx{"We wish you a happy Eid al-Fitr.", "Wir wünschen Ihnen ein frohes Zuckerfest.", "Nous vous souhaitons une bonne fête de l'Aïd el-Fitr.", "Te deseamos un feliz Eid al-Fitr.", "Ti auguriamo un felice Eid al-Fitr.", "We wensen je een fijn Suikerfeest.", "Desejamos a você um feliz Eid al-Fitr.", "نتمنى لكم عيد فطر سعيداً."}),
		},
		brief.OccasionEidAlAdha: {
			L(pair{"Kurban Bayramını en içten dileklerimizle kutlarız.", "Kurban Bayramınızı en içten dileklerimizle kutlarız."}, tx{"We wish you a happy Eid al-Adha.", "Wir wünschen Ihnen ein frohes Opferfest.", "Nous vous souhaitons une bonne fête de l'Aïd al-Adha.", "Te deseamos un feliz Eid al-Adha.", "Ti auguriamo un felice Eid al-Adha.", "We wensen je een fijn Offerfeest.", "Desejamos a você um feliz Eid al-Adha.", "نتمنى لكم عيد أضحى سعيداً."}),
		},
		brief.OccasionHoliday: {
			L(pair{"Bayramını en içten dileklerimizle kutlarız.", "Bayramınızı en içten dileklerimizle kutlarız."}, tx{"We wish you happy holidays.", "Wir wünschen Ihnen schöne Feiertage.", "Nous vous souhaitons de joyeuses fêtes.", "Te deseamos felices fiestas.", "Ti auguriamo buone feste.", "We wensen je fijne feestdagen.", "Desejamos a você boas festas.", "نتمنى لكم عيداً سعيداً."}),
		},

		"cart": {
			L(pair{"Sepetine eklediğin ürünler hâlâ seni bekliyor.", "Sepetinize eklediğiniz ürünler hâlâ sizi bekliyor."}, tx{"The items you added to your cart are still waiting for you.", "Die Artikel in Ihrem Warenkorb warten noch auf Sie.", "Les articles de votre panier vous attendent toujours.", "Los productos que añadiste a tu carrito todavía te esperan.", "I prodotti nel tuo carrello ti stanno ancora aspettando.", "De artikelen in je winkelwagen wachten nog op je.", "Os produtos que você colocou no carrinho ainda estão esperando por você.", "المنتجات التي أضفتموها إلى السلة لا تزال بانتظاركم."}),
			L(pair{"Sepetindeki ürünler seni unutmadı.", "Sepetinizdeki ürünler sizi unutmadı."}, tx{"Your cart has not forgotten you.", "Ihr Warenkorb hat Sie nicht vergessen.", "Votre panier ne vous a pas oublié.", "Tu carrito no te ha olvidado.", "Il tuo carrello non ti ha dimenticato.", "Je winkelwagen is je niet vergeten.", "Seu carrinho não esqueceu você.", "سلتكم لم تنسَكم."}),
		},
		"winback": {
			L(pair{"Bir süredir seni göremedik ve özledik.", "Bir süredir sizi göremedik ve özledik."}, tx{"We have not seen you for a while and we miss you.", "Wir haben Sie eine Weile nicht gesehen und vermissen Sie.", "Cela fait un moment que nous ne vous avons pas vu, et vous nous manquez.", "Hace tiempo que no te vemos y te echamos de menos.", "È da un po' che non ti vediamo e ci manchi.", "We hebben je al een tijdje niet gezien en we missen je.", "Faz tempo que não vemos você e sentimos sua falta.", "لم نركم منذ فترة واشتقنا إليكم."}),
			L(pair{"Seni yeniden görmek bizi çok mutlu eder.", "Sizi yeniden görmek bizi çok mutlu eder."}, tx{"We would be very happy to see you again.", "Wir würden uns sehr freuen, Sie wiederzusehen.", "Nous serions très heureux de vous revoir.", "Nos encantaría volver a verte.", "Saremmo felici di rivederti.", "We zouden het heel fijn vinden je weer te zien.", "Ficaríamos muito felizes em ver você de novo.", "سيسعدنا كثيراً أن نراكم مجدداً."}),
		},
		"welcome": {
			L(pair{"Aramıza katıldığın için çok mutluyuz.", "Aramıza katıldığınız için çok mutluyuz."}, tx{"We are delighted to have you with us.", "Schön, dass Sie dabei sind.", "Nous sommes ravis de vous compter parmi nous.", "Nos alegra mucho que te hayas unido.", "Siamo felici che tu sia con noi.", "We zijn blij dat je erbij bent.", "Estamos muito felizes por ter você com a gente.", "يسعدنا انضمامكم إلينا."}),
		},
		"loyal": {
			L(pair{"Bizi tercih ettiğin için teşekkür ederiz.", "Bizi tercih ettiğiniz için teşekkür ederiz."}, tx{"Thank you for choosing us.", "Vielen Dank, dass Sie bei uns einkaufen.", "Merci de nous faire confiance.", "Gracias por elegirnos.", "Grazie per averci scelto.", "Bedankt dat je voor ons kiest.", "Obrigado por escolher a gente.", "شكراً لاختياركم لنا."}),
		},
		"general": {
			L(pair{"Sana özel bir kampanyamız var.", "Size özel bir kampanyamız var."}, tx{"We have a special campaign for you.", "Wir haben eine besondere Aktion für Sie.", "Nous avons une offre spéciale pour vous.", "Tenemos una campaña especial para ti.", "Abbiamo una promozione speciale per te.", "We hebben een speciale actie voor je.", "Temos uma campanha especial para você.", "لدينا حملة خاصة لكم."}),
			goodNewsOpen,
		},
	},
	ctas: map[string][]line{
		"general": {
			L(pair{"Hemen keşfet.", "Hemen keşfedin."}, tx{"Discover it now.", "Jetzt entdecken.", "Découvrez-la maintenant.", "Descúbrelo ahora.", "Scoprilo ora.", "Ontdek het nu.", "Descubra agora.", "اكتشفوا الآن."}),
			L(pair{"Fırsatı kaçırma!", "Fırsatı kaçırmayın."}, tx{"Don't miss out!", "Nicht verpassen!", "Ne la manquez pas !", "¡No te lo pierdas!", "Non perdertela!", "Mis het niet!", "Não perca!", "لا تفوّتوا الفرصة!"}),
			L(pair{"Şimdi göz at.", "Şimdi göz atın."}, tx{"Take a look now.", "Schauen Sie jetzt vorbei.", "Jetez-y un œil maintenant.", "Échale un vistazo ahora.", "Dai un'occhiata ora.", "Neem nu een kijkje.", "Dê uma olhada agora.", "ألقوا نظرة الآن."}),
			L(pair{"Alışverişe başla.", "Alışverişe başlayın."}, tx{"Start shopping.", "Jetzt einkaufen.", "Commencez vos achats.", "Empieza a comprar.", "Inizia lo shopping.", "Begin met winkelen.", "Comece a comprar.", "ابدأوا التسوق."}),
		},
		"cart": {
			L(pair{"Sepetini tamamla.", "Sepetinizi tamamlayın."}, tx{"Complete your order.", "Schließen Sie Ihre Bestellung ab.", "Finalisez votre panier.", "Completa tu compra.", "Completa il tuo ordine.", "Rond je bestelling af.", "Finalize sua compra.", "أكملوا طلبكم."}),
			L(pair{"Sepetine dön.", "Sepetinize dönün."}, tx{"Back to your cart.", "Zurück zum Warenkorb.", "Retournez à votre panier.", "Vuelve a tu carrito.", "Torna al tuo carrello.", "Terug naar je winkelwagen.", "Volte ao seu carrinho.", "عودوا إلى سلتكم."}),
		},
		"gift": {
			L(pair{"Hediyeni şimdi seç.", "Hediyenizi şimdi seçin."}, tx{"Pick your gift now.", "Wählen Sie jetzt Ihr Geschenk.", "Choisissez votre cadeau dès maintenant.", "Elige tu regalo ahora.", "Scegli ora il tuo regalo.", "Kies nu je cadeau.", "Escolha seu presente agora.", "اختاروا هديتكم الآن."}),
		},
	},
	// {o} is the offer with its minimum.
	offerSentence: []line{
		L(pair{"{o} seni bekliyor.", "{o} sizi bekliyor."}, tx{"{o} is waiting for you.", "{o} wartet auf Sie.", "{o} vous attend.", "{o} te espera.", "{o} ti aspetta.", "{o} wacht op je.", "{o} está esperando por você.", "{o} بانتظاركم."}),
		L(pair{"Şimdi {o} fırsatından yararlanabilirsin.", "Şimdi {o} fırsatından yararlanabilirsiniz."}, tx{"You can now enjoy {o}.", "Profitieren Sie jetzt von {o}.", "Profitez dès maintenant de {o}.", "Ahora puedes disfrutar de {o}.", "Ora puoi approfittare di {o}.", "Profiteer nu van {o}.", "Agora você pode aproveitar {o}.", "يمكنكم الآن الاستفادة من {o}."}),
		L(pair{"{o} ile alışverişin daha keyifli.", "{o} ile alışverişiniz daha keyifli."}, tx{"Shopping is more fun with {o}.", "Mit {o} macht Einkaufen noch mehr Spaß.", "Vos achats sont encore plus agréables avec {o}.", "Comprar es más divertido con {o}.", "Lo shopping è più piacevole con {o}.", "Winkelen is leuker met {o}.", "Comprar fica mais gostoso com {o}.", "التسوق أمتع مع {o}."}),
	},
	// A subject around the main part of the offer ({o}).
	subjectOffer: []line{
		L(pair{"{{first_name}}, {o} seni bekliyor", "{{first_name}}, {o} sizi bekliyor"}, tx{"{{first_name}}, {o} is waiting for you", "{{first_name}}, {o} wartet auf Sie", "{{first_name}}, {o} vous attend", "{{first_name}}, {o} te espera", "{{first_name}}, {o} ti aspetta", "{{first_name}}, {o} wacht op je", "{{first_name}}, {o} está esperando por você", "{{first_name}}، {o} بانتظاركم"}),
		L(same("{o} fırsatı başladı"), tx{"{o} starts now", "Jetzt: {o}", "C'est parti pour {o}", "Ya empieza {o}", "Al via {o}", "Nu: {o}", "Começou: {o}", "بدأ عرض {o}"}),
	},
	signoff: []line{
		LF(pair{"Görüşmek üzere!", "Saygılarımızla"},
			tx{"See you soon!", "Bis bald!", "À bientôt !", "¡Hasta pronto!", "A presto!", "Tot snel!", "Até logo!", "نراكم قريباً!"},
			tx{"Kind regards", "Mit freundlichen Grüßen", "Cordialement", "Saludos cordiales", "Cordiali saluti", "Met vriendelijke groet", "Atenciosamente", "مع أطيب التحيات"}),
		LF(pair{"Sevgiler", "İyi alışverişler dileriz"},
			tx{"Best wishes", "Herzliche Grüße", "Bien à vous", "Un abrazo", "Un caro saluto", "Hartelijke groet", "Abraços", "مع خالص المودة"},
			tx{"Happy shopping", "Viel Spaß beim Einkaufen", "Bon shopping", "Felices compras", "Buono shopping", "Veel winkelplezier", "Boas compras", "نتمنى لكم تسوقاً ممتعاً"}),
	},
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

// The lines of the offer and its terms. Their Turkish is exactly how package
// offer writes the source (offer.WhatIn("tr"), MinSpendPhraseIn, ...), so a
// text that states the offer is translated part by part. short names the
// shorter form a translation may switch to when it would not fit 160 / 80.
var termLines = []struct {
	kind  kind
	l     line
	short string
}{
	{kindPart, L(same("%{1} indirim"), tx{"{1}% off", "{1} % Rabatt", "{1} % de réduction", "{1} % de descuento", "{1}% di sconto", "{1}% korting", "{1}% de desconto", "خصم {1}%"}), ""},
	{kindPart, L(same("{1} TL indirim"), tx{"{1} TL off", "{1} TL Rabatt", "{1} TL de réduction", "{1} TL de descuento", "{1} TL di sconto", "{1} TL korting", "{1} TL de desconto", "خصم {1} TL"}), ""},
	{kindPart, L(same("ekstra {1} TL indirim"), tx{"an extra {1} TL off", "zusätzlich {1} TL Rabatt", "{1} TL de réduction en plus", "{1} TL de descuento extra", "ulteriori {1} TL di sconto", "{1} TL extra korting", "mais {1} TL de desconto", "خصم إضافي {1} TL"}), ""},
	{kindPart, L(same("her {1} TL alışverişe {2} TL indirim"), tx{"{2} TL off every {1} TL", "{2} TL Rabatt je {1} TL Einkaufswert", "{2} TL de réduction par tranche de {1} TL", "{2} TL de descuento por cada {1} TL", "{2} TL di sconto ogni {1} TL di spesa", "{2} TL korting per {1} TL", "{2} TL de desconto a cada {1} TL", "خصم {2} TL لكل {1} TL"}), ""},
	{kindPart, L(same("ücretsiz kargo"), tx{"free shipping", "kostenloser Versand", "livraison gratuite", "envío gratis", "spedizione gratuita", "gratis verzending", "frete grátis", "شحن مجاني"}), ""},
	{kindPart, L(same("{c} kodlu indirim"), tx{"a discount with code {c}", "Rabatt mit dem Code {c}", "une réduction avec le code {c}", "un descuento con el código {c}", "uno sconto con il codice {c}", "korting met de code {c}", "um desconto com o código {c}", "خصم بالرمز {c}"}), ""},
	{kindPart, L(same("Kupon kodu {c}"), tx{"Coupon code {c}", "Gutscheincode {c}", "Code promo {c}", "Código de cupón {c}", "Codice coupon {c}", "Kortingscode {c}", "Código do cupom {c}", "رمز القسيمة {c}"}), ""},
	{kindCode, L(same("Kupon kodu: {c}"), tx{"Coupon code: {c}", "Gutscheincode: {c}", "Code promo : {c}", "Código de cupón: {c}", "Codice coupon: {c}", "Kortingscode: {c}", "Código do cupom: {c}", "رمز القسيمة: {c}"}), "Kod: {c}"},
	{kindCode, L(same("Kod: {c}"), tx{"Code: {c}", "Code: {c}", "Code : {c}", "Código: {c}", "Codice: {c}", "Code: {c}", "Código: {c}", "الرمز: {c}"}), ""},
	{kindCap, L(same("toplamda en fazla {1} TL"), tx{"up to {1} TL in total", "insgesamt höchstens {1} TL", "jusqu'à {1} TL au total", "hasta {1} TL en total", "fino a {1} TL in totale", "in totaal maximaal {1} TL", "até {1} TL no total", "بحد أقصى {1} TL إجمالاً"}), ""},
	{kindCap, L(same("en fazla {1} TL"), tx{"up to {1} TL", "höchstens {1} TL", "jusqu'à {1} TL", "hasta {1} TL", "fino a {1} TL", "maximaal {1} TL", "até {1} TL", "بحد أقصى {1} TL"}), ""},
	{kindMin, L(same("{1} TL ve üzeri alışverişte"), tx{"on orders of {1} TL and over", "ab einem Einkaufswert von {1} TL", "dès {1} TL d'achat", "en compras desde {1} TL", "per una spesa minima di {1} TL", "bij besteding vanaf {1} TL", "em compras a partir de {1} TL", "على الطلبات بقيمة {1} TL فأكثر"}), "{1} TL üzeri"},
	{kindMin, L(same("{1} TL üzeri"), tx{"over {1} TL", "ab {1} TL", "dès {1} TL", "desde {1} TL", "da {1} TL", "vanaf {1} TL", "acima de {1} TL", "فوق {1} TL"}), ""},
	{kindTerm, L(same("Geçerli ürünler: {s}"), tx{"Applies to: {s}", "Gilt für: {s}", "Valable sur : {s}", "Válido en: {s}", "Valido su: {s}", "Geldig op: {s}", "Válido em: {s}", "يسري على: {s}"}), ""},
	{kindTerm, L(same("Son kullanım: {d}"), tx{"Valid until: {d}", "Gültig bis: {d}", "Valable jusqu'au {d}", "Válido hasta el {d}", "Valido fino al {d}", "Geldig tot {d}", "Válido até {d}", "صالح حتى {d}"}), "Son gün {d}"},
	{kindTerm, L(same("Son gün {d}"), tx{"Until {d}", "Bis {d}", "Jusqu'au {d}", "Hasta el {d}", "Fino al {d}", "Tot {d}", "Até {d}", "حتى {d}"}), ""},

	// The built-in template of package content (content.Fallback), so that a
	// campaign that fell back to it is translated too.
	{kindOther, L(same("Merhaba"), tx{"Hello", "Hallo", "Bonjour", "Hola", "Ciao", "Hallo", "Olá", "مرحباً"}), ""},
	{kindOther, L(pair{"sana özel {o} fırsatı var", "size özel {o} fırsatı var"}, tx{"a special offer awaits you: {o}", "für Sie gibt es ein besonderes Angebot: {o}", "une offre spéciale vous attend : {o}", "tienes una oferta especial: {o}", "c'è un'offerta speciale per te: {o}", "er is een speciale aanbieding voor je: {o}", "há uma oferta especial para você: {o}", "لديكم عرض خاص: {o}"}), ""},
	{kindOther, L(pair{"sana özel {o} fırsatı", "size özel {o} fırsatı"}, tx{"a special offer for you: {o}", "ein besonderes Angebot für Sie: {o}", "une offre spéciale pour vous : {o}", "una oferta especial para ti: {o}", "un'offerta speciale per te: {o}", "een speciale aanbieding voor je: {o}", "uma oferta especial para você: {o}", "عرض خاص لكم: {o}"}), ""},
	{kindOther, L(pair{"sana özel bir fırsatımız var", "size özel bir fırsatımız var"}, tx{"we have a special offer for you", "wir haben ein besonderes Angebot für Sie", "nous avons une offre spéciale pour vous", "tenemos una oferta especial para ti", "abbiamo un'offerta speciale per te", "we hebben een speciale aanbieding voor je", "temos uma oferta especial para você", "لدينا عرض خاص لكم"}), ""},
	{kindOther, L(pair{"sana özel bir fırsat var", "size özel bir fırsat var"}, tx{"a special offer for you", "ein besonderes Angebot für Sie", "une offre spéciale pour vous", "una oferta especial para ti", "un'offerta speciale per te", "een speciale aanbieding voor je", "uma oferta especial para você", "عرض خاص لكم"}), ""},
	{kindOther, L(pair{"Detaylar için bizi ziyaret et", "Detaylar için bizi ziyaret edin"}, tx{"Visit us for details", "Besuchen Sie uns für alle Details", "Rendez-nous visite pour en savoir plus", "Visítanos para más detalles", "Visitaci per i dettagli", "Bezoek ons voor meer informatie", "Visite-nos para mais detalhes", "تفضلوا بزيارتنا لمعرفة التفاصيل"}), ""},
	{kindOther, L(pair{"Fırsatı kaçırmamak için hemen göz at", "Fırsatı kaçırmamak için hemen göz atın"}, tx{"Take a look now so you don't miss out", "Schauen Sie gleich vorbei, um nichts zu verpassen", "Jetez-y un œil vite pour ne rien manquer", "Échale un vistazo ya para no perdértelo", "Dai subito un'occhiata per non perdertela", "Neem snel een kijkje zodat je niets mist", "Dê uma olhada agora para não perder", "ألقوا نظرة الآن حتى لا تفوتكم الفرصة"}), ""},
	{kindOther, L(same("{1} TL ve üzeri alışverişte geçerli"), tx{"valid on orders of {1} TL and over", "gültig ab einem Einkaufswert von {1} TL", "valable dès {1} TL d'achat", "válido en compras desde {1} TL", "valido per una spesa minima di {1} TL", "geldig bij besteding vanaf {1} TL", "válido em compras a partir de {1} TL", "صالح على الطلبات بقيمة {1} TL فأكثر"}), ""},
}
