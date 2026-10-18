package domain

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// FactCategory represents the classification category of a corporate disclosure or material fact.
type FactCategory string

const (
	FactDefault      FactCategory = "Дефолт / Обязательства"
	FactOffer        FactCategory = "Оферта"
	FactCoupon       FactCategory = "Купонные выплаты"
	FactMeeting      FactCategory = "СД и Собрания акционеров"
	FactLitigation   FactCategory = "Судебные споры"
	FactRatingChange FactCategory = "Рейтинговые действия"
	FactOther        FactCategory = "Прочее"
)

// ShortTag returns a compact Russian label for displaying in Telegram lists.
func (c FactCategory) ShortTag() string {
	switch c {
	case FactDefault:
		return "Дефолт"
	case FactOffer:
		return "Оферта"
	case FactCoupon:
		return "Купон"
	case FactMeeting:
		return "СД"
	case FactLitigation:
		return "Суд"
	case FactRatingChange:
		return "Рейтинг"
	default:
		return "Событие"
	}
}

// FactSeverity represents the importance/urgency level of a material fact.
type FactSeverity string

const (
	SeverityCritical FactSeverity = "CRITICAL" // Красный флаг (дефолт, техдефолт, банкротство)
	SeverityWarning  FactSeverity = "WARNING"  // Внимание (оферта, суды, понижение рейтинга)
	SeverityInfo     FactSeverity = "INFO"     // Плановое событие (купон, СД, собрание)
)

// Emoji returns the visual indicator corresponding to the severity level.
func (s FactSeverity) Emoji() string {
	switch s {
	case SeverityCritical:
		return "🚨"
	case SeverityWarning:
		return "⚠️"
	default:
		return "ℹ️"
	}
}

// MaterialFact represents a corporate disclosure event from e-disclosure.ru or MOEX ISS.
type MaterialFact struct {
	ID          string       `json:"id"`
	INN         string       `json:"inn"`
	ISIN        string       `json:"isin,omitempty"`
	IssuerName  string       `json:"issuer_name,omitempty"`
	Title       string       `json:"title"`
	Summary     string       `json:"summary,omitempty"`
	Category    FactCategory `json:"category"`
	Severity    FactSeverity `json:"severity"`
	PublishedAt time.Time    `json:"published_at"`
	URL         string       `json:"url"`
	Source      string       `json:"source,omitempty"`
}

// Classify populates Category and Severity on the MaterialFact based on its Title and Summary.
func (f *MaterialFact) Classify() {
	cat, sev := ClassifyFact(f.Title, f.Summary)
	f.Category = cat
	f.Severity = sev
}

// IsAlertWorthy returns true if the fact should trigger a proactive alert for monitored bonds.
func (f MaterialFact) IsAlertWorthy() bool {
	return f.Severity == SeverityCritical || f.Severity == SeverityWarning
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// containsStem checks whether stem appears in text starting at a word boundary.
func containsStem(text, stem string) bool {
	if stem == "" {
		return false
	}
	offset := 0
	for {
		idx := strings.Index(text[offset:], stem)
		if idx < 0 {
			return false
		}
		absIdx := offset + idx
		leftOK := true
		if absIdx > 0 {
			prevRune, _ := utf8.DecodeLastRuneInString(text[:absIdx])
			if isWordChar(prevRune) {
				leftOK = false
			}
		}
		if leftOK {
			return true
		}
		offset = absIdx + len(stem)
	}
}

// containsWord checks whether word appears in text bounded on both sides by non-word characters.
func containsWord(text, word string) bool {
	if word == "" {
		return false
	}
	offset := 0
	for {
		idx := strings.Index(text[offset:], word)
		if idx < 0 {
			return false
		}
		absIdx := offset + idx
		endIdx := absIdx + len(word)

		leftOK := true
		if absIdx > 0 {
			prevRune, _ := utf8.DecodeLastRuneInString(text[:absIdx])
			if isWordChar(prevRune) {
				leftOK = false
			}
		}
		rightOK := true
		if endIdx < len(text) {
			nextRune, _ := utf8.DecodeRuneInString(text[endIdx:])
			if isWordChar(nextRune) {
				rightOK = false
			}
		}
		if leftOK && rightOK {
			return true
		}
		offset = endIdx
	}
}

// ClassifyFact analyzes the title and optional summary of a disclosure event
// and assigns its FactCategory and FactSeverity.
func ClassifyFact(title, summary string) (FactCategory, FactSeverity) {
	combined := strings.ToLower(strings.TrimSpace(title + " " + summary))

	// 1. CRITICAL: Defaults, non-performance of obligations, bankruptcy, trading/emission suspension, license revocation
	criticalDefaultStems := []string{
		"неисполнени",
		"технический дефолт",
		"технического дефолта",
		"техническом дефолте",
		"техдефолт",
		"дефолт",
		"банкротств",
		"несостоятельност",
		"отзыв лицензии",
		"отзыве лицензии",
		"приостановлении торгов",
		"приостановление торгов",
		"приостановлении эмиссии",
		"приостановление эмиссии",
		"приостановлении размещения",
		"приостановление размещения",
		"приостановлении действия лицензии",
		"просрочк",
		"невыплат",
		"досрочном погашении по требованию",
	}
	for _, stem := range criticalDefaultStems {
		if containsStem(combined, stem) {
			return FactDefault, SeverityCritical
		}
	}

	// 2. WARNING - Offer (Put/Call, buyback, coupon rate setting for next period)
	offerStems := []string{
		"оферт",
		"приобретении ценных бумаг",
		"приобретения ценных бумаг",
		"выкупе ценных бумаг",
		"выкуп ценных бумаг",
		"выкупу ценных бумаг",
		"выкупе облигаций",
		"выкуп облигаций",
		"выкупу облигаций",
		"к выкупу",
		"предъявления бумаг",
		"предъявлении бумаг",
		"предъявлению бумаг",
		"предъявления ценных бумаг",
		"предъявлении ценных бумаг",
		"предъявления облигаций",
		"предъявлении облигаций",
		"предъявлении к выкупу",
		"досрочного предъявления",
		"определение ставки купона",
		"определении ставки купона",
		"определении размера процента",
		"определении размера купона",
	}
	for _, stem := range offerStems {
		if containsStem(combined, stem) {
			return FactOffer, SeverityWarning
		}
	}

	// 3. WARNING - Litigation, court disputes, asset freeze, creditor/lawsuit claims
	litigationStems := []string{
		"судебн",
		"арбитраж",
		"исков",
		"арест",
		"взыскан",
		"исковых требований",
		"искового требования",
		"требований кредиторов",
		"требования кредиторов",
	}
	for _, stem := range litigationStems {
		if containsStem(combined, stem) {
			return FactLitigation, SeverityWarning
		}
	}
	litigationExactWords := []string{
		"иск", "иска", "иску", "иском", "иске", "иски", "искам", "исками", "исках",
		"спор", "спора", "спору", "спором", "споре", "споры", "споров", "спорам", "спорами", "спорах",
		"суд", "суда", "суду", "судом", "суде", "суды", "судов",
	}
	for _, w := range litigationExactWords {
		if containsWord(combined, w) {
			return FactLitigation, SeverityWarning
		}
	}

	// 4. Rating actions (Warning on downgrade/withdrawal/negative, Info otherwise)
	hasRating := containsStem(combined, "рейтинг") ||
		containsStem(combined, "эксперт ра") ||
		containsWord(combined, "акра") ||
		containsWord(combined, "нкр") ||
		containsWord(combined, "нра") ||
		containsWord(combined, "raex") ||
		containsWord(combined, "acra")
	if hasRating {
		if containsStem(combined, "понижени") ||
			containsStem(combined, "отозван") ||
			containsStem(combined, "отзыв") ||
			containsStem(combined, "негативн") {
			return FactRatingChange, SeverityWarning
		}
		return FactRatingChange, SeverityInfo
	}

	// 5. INFO - Coupon payments / fulfillment of obligations / scheduled redemption
	couponStems := []string{
		"выплата купон",
		"выплате купон",
		"выплаченных доходах",
		"выплате дохода",
		"выплата дохода",
		"купонного дохода",
		"купонный доход",
		"исполнении обязательств",
		"исполнение обязательств",
		"погашении ценных бумаг",
		"погашение ценных бумаг",
		"погашении облигаций",
		"погашение облигаций",
		"амортизац",
	}
	for _, stem := range couponStems {
		if containsStem(combined, stem) {
			return FactCoupon, SeverityInfo
		}
	}

	// 6. INFO - Board of Directors / Shareholder meetings (СД / ГОСА / ВОСА)
	meetingStems := []string{
		"совета директоров",
		"совет директоров",
		"собрани",
		"повестк",
		"акционеров",
		"участников общества",
	}
	for _, stem := range meetingStems {
		if containsStem(combined, stem) {
			return FactMeeting, SeverityInfo
		}
	}
	if containsWord(combined, "воса") || containsWord(combined, "госа") {
		return FactMeeting, SeverityInfo
	}

	return FactOther, SeverityInfo
}
