package seed

// Name pools are plain data. They deliberately contain Turkish letters
// (ş, ğ, ı, İ, ö, ü, ç) so rune-vs-byte length bugs show up in tests.

var firstNames = []string{
	"Ahmet", "Mehmet", "Mustafa", "Ali", "Hüseyin", "İbrahim", "İsmail", "Ömer", "Yusuf", "Emre",
	"Burak", "Can", "Cem", "Deniz", "Eren", "Fatih", "Gökhan", "Hakan", "Kaan", "Murat",
	"Onur", "Serkan", "Tolga", "Uğur", "Volkan", "Şükrü", "Çağlar", "Barış", "Berkay", "Oğuz",
	"Ayşe", "Fatma", "Emine", "Hatice", "Zeynep", "Elif", "Merve", "Büşra", "Özlem", "Selin",
	"Gözde", "Işıl", "Şeyma", "Ceren", "Derya", "Esra", "Gizem", "Hande", "İpek", "Kübra",
	"Melis", "Naz", "Pınar", "Sevgi", "Tuğçe", "Yağmur", "Ebru", "Damla", "Aslı", "Buse",
}

var lastNames = []string{
	"Yılmaz", "Kaya", "Demir", "Şahin", "Çelik", "Yıldız", "Yıldırım", "Öztürk", "Aydın", "Özdemir",
	"Arslan", "Doğan", "Kılıç", "Aslan", "Çetin", "Kara", "Koç", "Kurt", "Özkan", "Şimşek",
	"Polat", "Korkmaz", "Özer", "Çakır", "Erdoğan", "Yavuz", "Can", "Acar", "Güneş", "Aksoy",
	"Aktaş", "Bulut", "Tekin", "Ünal", "Güler", "Bozkurt", "Karaca", "Duman", "Soylu", "Taş",
	"Işık", "Tuna", "Sönmez", "Avcı", "Kaplan", "Er", "Bayram", "Turan", "Uçar", "Ateş",
	"Çiftçi", "Önal", "Gündüz", "Eren", "Başaran", "Ergün", "Karakaya", "Yalçın", "Öz", "Sezer",
}

// Used (joined) to build the deliberately very long first names.
var longNameParts = []string{
	"Muhammed", "Abdurrahman", "Selahattin", "Şükrüye", "Gülşah", "Kemalettin", "Nureddin", "Hayrünnisa",
}

var products = []string{
	"Spor ayakkabı", "Kablosuz kulaklık", "Akıllı saat", "Sırt çantası", "Kahve makinesi",
	"Yoga matı", "Termos", "Bluetooth hoparlör", "Deri cüzdan", "Güneş gözlüğü",
	"Mont", "Kot pantolon", "Parfüm", "Powerbank", "Laptop standı",
	"Kitap seti", "Nevresim takımı", "Hava fritözü", "Koşu saati", "Telefon kılıfı",
}

type cityShare struct {
	name    string
	percent int
}

// Percentages add up to 100. Any rounding remainder goes to İstanbul.
var cityShares = []cityShare{
	{"İstanbul", 35}, {"Ankara", 12}, {"İzmir", 10}, {"Bursa", 5}, {"Antalya", 5},
	{"Adana", 4}, {"Konya", 3}, {"Gaziantep", 3}, {"Kocaeli", 3}, {"Mersin", 3},
	{"Eskişehir", 2}, {"Trabzon", 2}, {"Samsun", 2}, {"Kayseri", 2}, {"Diyarbakır", 2},
	{"Muğla", 2}, {"Denizli", 2}, {"Tekirdağ", 2}, {"Balıkesir", 1},
}

// Malformed contact values: the mock provider rejects them with a permanent 400.
var invalidPhones = []string{"+90000", "0555-INVALID", "12345"}
var invalidEmails = []string{"not-an-email", "user@", "@example.com"}
