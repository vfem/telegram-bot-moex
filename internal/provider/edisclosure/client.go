package edisclosure

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"telegram-bot-moex/internal/domain"
)

// ErrNoFactsFound is returned when no disclosures or news items are found for the given INN.
var ErrNoFactsFound = errors.New("no material facts found for issuer")

var (
	mskLoc       = time.FixedZone("MSK", 3*3600)
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
)

// Client fetches material facts and corporate disclosures from MOEX ISS (bondization + sitenews) and e-disclosure feeds.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new disclosure feed client with SSRF and timeout protections.
func NewClient(customBaseURL *string, customHTTPClient ...*http.Client) *Client {
	base := "https://iss.moex.com/iss"
	if customBaseURL != nil && *customBaseURL != "" {
		base = strings.TrimRight(*customBaseURL, "/")
	}

	var hc *http.Client
	if len(customHTTPClient) > 0 && customHTTPClient[0] != nil {
		hc = customHTTPClient[0]
	} else {
		hc = &http.Client{
			Timeout:       8 * time.Second,
			CheckRedirect: checkSafeRedirect,
		}
	}

	return &Client{
		baseURL:    base,
		httpClient: hc,
	}
}

func checkSafeRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("stopped after 5 redirects")
	}
	host := req.URL.Hostname()
	if isPrivateOrLocalHost(host) {
		return fmt.Errorf("security: redirect to private/internal host blocked: %s", host)
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("security: redirect to non-https scheme blocked: %s", req.URL.Scheme)
	}
	return nil
}

func isPrivateOrLocalHost(host string) bool {
	if host == "localhost" || host == "metadata.google.internal" || host == "169.254.169.254" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate()
}

// rssFeed models standard RSS 2.0 XML feeds from e-disclosure.ru.
type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
}

// jsonFactItem models JSON disclosure items.
type jsonFactItem struct {
	ID          string `json:"id"`
	INN         string `json:"inn"`
	ISIN        string `json:"isin"`
	IssuerName  string `json:"issuer_name"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	PublishedAt string `json:"published_at"`
	Date        string `json:"date"`
	URL         string `json:"url"`
}

type moexTableBlock struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

// moexCombinedResponse models MOEX ISS tables (securities, description, bondization, sitenews, news).
type moexCombinedResponse struct {
	Securities    moexTableBlock `json:"securities"`
	Description   moexTableBlock `json:"description"`
	Offers        moexTableBlock `json:"offers"`
	Coupons       moexTableBlock `json:"coupons"`
	Amortizations moexTableBlock `json:"amortizations"`
	SiteNews      moexTableBlock `json:"sitenews"`
	News          moexTableBlock `json:"news"`
}

type securityRef struct {
	secID       string
	isin        string
	name        string
	shortName   string
	issuerTitle string
	isTraded    bool
}

// GetRecentFacts retrieves and classifies the latest material facts for an issuer INN.
func (c *Client) GetRecentFacts(ctx context.Context, inn string, limit int) ([]domain.MaterialFact, error) {
	cleanINN := strings.TrimSpace(inn)
	if cleanINN == "" {
		return nil, errors.New("empty INN provided")
	}
	if limit <= 0 {
		limit = 5
	}

	reqURL := fmt.Sprintf("%s/securities.json?q=%s&iss.meta=off&group_by=group&group_by_filter=stock_bonds&limit=10", c.baseURL, url.QueryEscape(cleanINN))
	rawBody, err := c.doGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(rawBody)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: INN %s", ErrNoFactsFound, cleanINN)
	}

	// Support direct RSS XML or JSON array feeds (e.g., custom disclosure feeds or test servers)
	if trimmed[0] == '<' || trimmed[0] == '[' {
		facts, parseErr := parseDisclosurePayload(trimmed, cleanINN, nil)
		if parseErr != nil {
			return nil, parseErr
		}
		if len(facts) == 0 {
			return nil, fmt.Errorf("%w: INN %s", ErrNoFactsFound, cleanINN)
		}
		return sortAndTrimFacts(facts, limit), nil
	}

	var root moexCombinedResponse
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return nil, fmt.Errorf("failed to decode disclosure JSON object: %w", err)
	}

	// If response contains direct sitenews/news without securities table (e.g. direct news endpoint)
	if len(root.Securities.Columns) == 0 && (len(root.SiteNews.Columns) > 0 || len(root.News.Columns) > 0) {
		facts, parseErr := parseDisclosurePayload(trimmed, cleanINN, nil)
		if parseErr != nil {
			return nil, parseErr
		}
		if len(facts) == 0 {
			return nil, fmt.Errorf("%w: INN %s", ErrNoFactsFound, cleanINN)
		}
		return sortAndTrimFacts(facts, limit), nil
	}

	// Extract securities matching cleanINN from MOEX ISS securities search
	secRefs := extractIssuerSecurities(root.Securities, cleanINN)
	if len(secRefs) == 0 {
		return nil, fmt.Errorf("%w: INN %s", ErrNoFactsFound, cleanINN)
	}

	issuerName := secRefs[0].issuerTitle
	filterKeywords := buildFilterKeywords(secRefs)

	var allFacts []domain.MaterialFact

	// Inspect up to 2 primary securities for default status and bondization events (offers, coupons, amortizations)
	maxSec := len(secRefs)
	if maxSec > 2 {
		maxSec = 2
	}
	now := time.Now().In(mskLoc)

	for i := 0; i < maxSec; i++ {
		sec := secRefs[i]

		// 1. Check security description for HASDEFAULT / HASTECHNICALDEFAULT
		secURL := fmt.Sprintf("%s/securities/%s.json?iss.meta=off", c.baseURL, url.PathEscape(sec.secID))
		if secBody, secErr := c.doGet(ctx, secURL); secErr == nil {
			var secResp moexCombinedResponse
			if json.Unmarshal(secBody, &secResp) == nil {
				defaultFacts := extractDefaultFacts(secResp.Description, cleanINN, issuerName, sec, now)
				allFacts = append(allFacts, defaultFacts...)
			}
		}

		// 2. Fetch bondization events (offers, coupons, amortizations)
		bondURL := fmt.Sprintf("%s/securities/%s/bondization.json?iss.meta=off&limit=100", c.baseURL, url.PathEscape(sec.secID))
		if bondBody, bondErr := c.doGet(ctx, bondURL); bondErr == nil {
			var bondResp moexCombinedResponse
			if json.Unmarshal(bondBody, &bondResp) == nil {
				bondFacts := extractBondizationFacts(bondResp, cleanINN, issuerName, sec, now)
				allFacts = append(allFacts, bondFacts...)
			}
		}
	}

	// 3. Fetch MOEX sitenews and filter strictly by issuer keywords / tickers / ISINs
	newsURL := fmt.Sprintf("%s/sitenews.json?iss.meta=off", c.baseURL)
	if newsBody, newsErr := c.doGet(ctx, newsURL); newsErr == nil && len(filterKeywords) > 0 {
		if newsFacts, parseErr := parseDisclosurePayload(newsBody, cleanINN, filterKeywords); parseErr == nil {
			for idx := range newsFacts {
				if newsFacts[idx].IssuerName == "" {
					newsFacts[idx].IssuerName = issuerName
				}
			}
			allFacts = append(allFacts, newsFacts...)
		}
	}

	if len(allFacts) == 0 {
		return nil, fmt.Errorf("%w: INN %s", ErrNoFactsFound, cleanINN)
	}

	return sortAndTrimFacts(allFacts, limit), nil
}

func (c *Client) doGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "BondsBot/1.0 (Mozilla/5.0 compatible)")
	req.Header.Set("Accept", "application/json, application/rss+xml, application/xml, text/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("disclosure feed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoFactsFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("disclosure feed returned status %d", resp.StatusCode)
	}

	// Security: limit response body to 4 MB to prevent OOM
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed reading disclosure feed: %w", err)
	}
	return rawBody, nil
}

func extractIssuerSecurities(block moexTableBlock, targetINN string) []securityRef {
	colIdx := indexColumns(block.Columns)
	_, hasINNCol := colIdx["emitent_inn"]
	var traded, untraded []securityRef

	for _, row := range block.Data {
		emitentINN := strings.TrimSpace(cellString(row, colIdx, "emitent_inn"))
		if hasINNCol {
			if emitentINN != targetINN {
				continue
			}
		} else if emitentINN != "" && emitentINN != targetINN {
			continue
		}
		secID := strings.TrimSpace(cellString(row, colIdx, "secid"))
		isin := strings.TrimSpace(cellString(row, colIdx, "isin"))
		if secID == "" && isin == "" {
			continue
		}
		if secID == "" {
			secID = isin
		}
		if isin == "" {
			isin = secID
		}
		name := cleanText(cellString(row, colIdx, "name"))
		shortName := cleanText(cellString(row, colIdx, "shortname"))
		if name == "" {
			name = shortName
		}
		issuerTitle := cleanText(cellString(row, colIdx, "emitent_title"))
		isTraded := cellString(row, colIdx, "is_traded") == "1"

		ref := securityRef{
			secID:       secID,
			isin:        isin,
			name:        name,
			shortName:   shortName,
			issuerTitle: issuerTitle,
			isTraded:    isTraded,
		}
		if isTraded {
			traded = append(traded, ref)
		} else {
			untraded = append(untraded, ref)
		}
	}

	return append(traded, untraded...)
}

func buildFilterKeywords(refs []securityRef) []string {
	seen := make(map[string]bool)
	var keywords []string
	addKw := func(s string) {
		k := strings.ToLower(strings.TrimSpace(s))
		if len([]rune(k)) < 3 || seen[k] {
			return
		}
		seen[k] = true
		keywords = append(keywords, k)
	}

	for _, r := range refs {
		addKw(r.isin)
		addKw(r.secID)
		addKw(r.shortName)
		if r.issuerTitle != "" {
			// Extract quoted brand name if present, e.g. Публичное акционерное общество "МТС-Банк" -> МТС-Банк
			if start := strings.Index(r.issuerTitle, `"`); start >= 0 {
				rest := r.issuerTitle[start+1:]
				if end := strings.Index(rest, `"`); end > 0 {
					addKw(rest[:end])
				}
			}
			if start := strings.Index(r.issuerTitle, "«"); start >= 0 {
				rest := r.issuerTitle[start+len("«"):]
				if end := strings.Index(rest, "»"); end > 0 {
					addKw(rest[:end])
				}
			}
		}
	}
	return keywords
}

func extractDefaultFacts(desc moexTableBlock, inn, issuerName string, sec securityRef, now time.Time) []domain.MaterialFact {
	colIdx := indexColumns(desc.Columns)
	hasDefault := false
	hasTechDefault := false

	for _, row := range desc.Data {
		name := strings.ToUpper(strings.TrimSpace(cellString(row, colIdx, "name")))
		val := strings.TrimSpace(cellString(row, colIdx, "value"))
		if name == "HASDEFAULT" && val == "1" {
			hasDefault = true
		}
		if name == "HASTECHNICALDEFAULT" && val == "1" {
			hasTechDefault = true
		}
	}

	var facts []domain.MaterialFact
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	issueURL := fmt.Sprintf("https://www.moex.com/ru/issue.aspx?code=%s", url.QueryEscape(sec.secID))
	if hasDefault {
		f := domain.MaterialFact{
			ID:          fmt.Sprintf("%s-default", sec.secID),
			INN:         inn,
			ISIN:        sec.isin,
			IssuerName:  issuerName,
			Title:       fmt.Sprintf("Неисполнение обязательств эмитента (дефолт) по облигациям %s (%s)", sec.name, sec.isin),
			Category:    domain.FactDefault,
			Severity:    domain.SeverityCritical,
			PublishedAt: today,
			URL:         issueURL,
			Source:      "MOEX ISS",
		}
		facts = append(facts, f)
	} else if hasTechDefault {
		f := domain.MaterialFact{
			ID:          fmt.Sprintf("%s-techdefault", sec.secID),
			INN:         inn,
			ISIN:        sec.isin,
			IssuerName:  issuerName,
			Title:       fmt.Sprintf("Технический дефолт по облигациям %s (%s)", sec.name, sec.isin),
			Category:    domain.FactDefault,
			Severity:    domain.SeverityCritical,
			PublishedAt: today,
			URL:         issueURL,
			Source:      "MOEX ISS",
		}
		facts = append(facts, f)
	}
	return facts
}

func extractBondizationFacts(resp moexCombinedResponse, inn, issuerName string, sec securityRef, now time.Time) []domain.MaterialFact {
	var facts []domain.MaterialFact
	issueURL := fmt.Sprintf("https://www.moex.com/ru/issue.aspx?code=%s", url.QueryEscape(sec.secID))

	// 1. Offers
	offerCols := indexColumns(resp.Offers.Columns)
	for _, row := range resp.Offers.Data {
		offerDateStr := cellString(row, offerCols, "offerdate")
		offerTime := parseFlexibleDate(offerDateStr)
		if offerTime.IsZero() {
			continue
		}
		// Include past offers within 365 days or upcoming offers within 180 days
		if offerTime.After(now.AddDate(0, 6, 0)) || offerTime.Before(now.AddDate(-1, 0, 0)) {
			continue
		}
		offerType := cleanText(cellString(row, offerCols, "offertype"))
		if offerType == "" {
			offerType = "оферта"
		}
		title := fmt.Sprintf("Оферта (%s) по облигациям %s: дата выкупа %s", offerType, sec.name, offerTime.Format("02.01.2006"))
		f := domain.MaterialFact{
			ID:          fmt.Sprintf("%s-offer-%s", sec.secID, offerDateStr),
			INN:         inn,
			ISIN:        sec.isin,
			IssuerName:  issuerName,
			Title:       title,
			PublishedAt: offerTime,
			URL:         issueURL,
			Source:      "MOEX ISS",
		}
		f.Classify()
		facts = append(facts, f)
	}

	// 2. Coupons (recent paid coupons within the last year)
	couponCols := indexColumns(resp.Coupons.Columns)
	var paidCoupons []domain.MaterialFact
	for _, row := range resp.Coupons.Data {
		cDateStr := cellString(row, couponCols, "coupondate")
		cTime := parseFlexibleDate(cDateStr)
		if cTime.IsZero() || cTime.After(now) || cTime.Before(now.AddDate(-1, 0, 0)) {
			continue
		}
		valStr := cellString(row, couponCols, "value")
		if valStr == "" {
			continue
		}
		unit := cellString(row, couponCols, "faceunit")
		if unit == "" || unit == "SUR" {
			unit = "RUB"
		}

		title := fmt.Sprintf("Выплата купонного дохода по облигациям %s в полном объеме (%s %s)", sec.name, valStr, unit)
		f := domain.MaterialFact{
			ID:          fmt.Sprintf("%s-coupon-%s", sec.secID, cDateStr),
			INN:         inn,
			ISIN:        sec.isin,
			IssuerName:  issuerName,
			Title:       title,
			PublishedAt: cTime,
			URL:         issueURL,
			Source:      "MOEX ISS",
		}
		f.Classify()
		paidCoupons = append(paidCoupons, f)
	}
	// Keep up to 3 most recent paid coupons per security
	if len(paidCoupons) > 3 {
		paidCoupons = paidCoupons[len(paidCoupons)-3:]
	}
	facts = append(facts, paidCoupons...)

	// 3. Amortizations (recent completed redemptions/amortizations)
	amortCols := indexColumns(resp.Amortizations.Columns)
	for _, row := range resp.Amortizations.Data {
		aDateStr := cellString(row, amortCols, "amortdate")
		aTime := parseFlexibleDate(aDateStr)
		if aTime.IsZero() || aTime.After(now) || aTime.Before(now.AddDate(-1, 0, 0)) {
			continue
		}
		valStr := cellString(row, amortCols, "value")
		unit := cellString(row, amortCols, "faceunit")
		if unit == "" || unit == "SUR" {
			unit = "RUB"
		}
		title := fmt.Sprintf("Погашение облигаций (амортизация номинала) %s (%s %s)", sec.name, valStr, unit)
		f := domain.MaterialFact{
			ID:          fmt.Sprintf("%s-amort-%s", sec.secID, aDateStr),
			INN:         inn,
			ISIN:        sec.isin,
			IssuerName:  issuerName,
			Title:       title,
			PublishedAt: aTime,
			URL:         issueURL,
			Source:      "MOEX ISS",
		}
		f.Classify()
		facts = append(facts, f)
	}

	return facts
}

// parseDisclosurePayload parses RSS XML, JSON array of facts, or MOEX ISS JSON news blocks.
func parseDisclosurePayload(raw []byte, defaultINN string, filterKeywords []string) ([]domain.MaterialFact, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: empty payload", ErrNoFactsFound)
	}

	// 1. Check if XML / RSS feed
	if trimmed[0] == '<' {
		var feed rssFeed
		if err := xml.Unmarshal(trimmed, &feed); err != nil {
			return nil, fmt.Errorf("failed to parse RSS XML feed: %w", err)
		}
		var facts []domain.MaterialFact
		for idx, item := range feed.Channel.Items {
			title := cleanText(item.Title)
			summary := cleanText(item.Description)
			if title == "" {
				continue
			}
			pubTime := parseFlexibleDate(item.PubDate)
			id := strings.TrimSpace(item.GUID)
			if id == "" {
				id = fmt.Sprintf("%s-rss-%d", defaultINN, idx+1)
			}
			fact := domain.MaterialFact{
				ID:          id,
				INN:         defaultINN,
				IssuerName:  cleanText(feed.Channel.Title),
				Title:       title,
				Summary:     summary,
				PublishedAt: pubTime,
				URL:         SanitizeURL(item.Link),
				Source:      "e-disclosure.ru",
			}
			fact.Classify()
			facts = append(facts, fact)
		}
		return facts, nil
	}

	// 2. Check if JSON array of facts
	if trimmed[0] == '[' {
		var items []jsonFactItem
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, fmt.Errorf("failed to decode JSON facts array: %w", err)
		}
		var facts []domain.MaterialFact
		for idx, item := range items {
			title := cleanText(item.Title)
			if title == "" {
				continue
			}
			dateStr := item.PublishedAt
			if dateStr == "" {
				dateStr = item.Date
			}
			inn := strings.TrimSpace(item.INN)
			if inn == "" {
				inn = defaultINN
			}
			id := strings.TrimSpace(item.ID)
			if id == "" {
				id = fmt.Sprintf("%s-json-%d", inn, idx+1)
			}
			fact := domain.MaterialFact{
				ID:          id,
				INN:         inn,
				ISIN:        strings.TrimSpace(item.ISIN),
				IssuerName:  cleanText(item.IssuerName),
				Title:       title,
				Summary:     cleanText(item.Summary),
				PublishedAt: parseFlexibleDate(dateStr),
				URL:         SanitizeURL(item.URL),
				Source:      "e-disclosure.ru",
			}
			fact.Classify()
			facts = append(facts, fact)
		}
		return facts, nil
	}

	// 3. Check if JSON object (MOEX ISS sitenews / news block)
	var moexResp moexCombinedResponse
	if err := json.Unmarshal(trimmed, &moexResp); err != nil {
		return nil, fmt.Errorf("failed to decode disclosure JSON object: %w", err)
	}

	columns := moexResp.News.Columns
	data := moexResp.News.Data
	if len(data) == 0 {
		columns = moexResp.SiteNews.Columns
		data = moexResp.SiteNews.Data
	}

	colIdx := indexColumns(columns)

	var facts []domain.MaterialFact
	for rowIdx, row := range data {
		title := cleanText(cellString(row, colIdx, "title"))
		if title == "" {
			continue
		}

		if len(filterKeywords) > 0 && !matchesAnyKeyword(title, filterKeywords) {
			continue
		}

		id := cellString(row, colIdx, "id")
		if id == "" {
			id = fmt.Sprintf("%s-moex-%d", defaultINN, rowIdx+1)
		}
		pubStr := cellString(row, colIdx, "published_at")
		if pubStr == "" {
			pubStr = cellString(row, colIdx, "modified_at")
		}

		itemURL := SanitizeURL(cellString(row, colIdx, "url"))
		if itemURL == "" && isDigitsOnly(id) {
			itemURL = "https://www.moex.com/n" + id
		}

		fact := domain.MaterialFact{
			ID:          id,
			INN:         defaultINN,
			Title:       title,
			PublishedAt: parseFlexibleDate(pubStr),
			URL:         itemURL,
			Source:      "MOEX News",
		}
		fact.Classify()
		facts = append(facts, fact)
	}

	return facts, nil
}

func sortAndTrimFacts(facts []domain.MaterialFact, limit int) []domain.MaterialFact {
	sort.Slice(facts, func(i, j int) bool {
		return facts[i].PublishedAt.After(facts[j].PublishedAt)
	})
	if limit > 0 && len(facts) > limit {
		facts = facts[:limit]
	}
	return facts
}

func indexColumns(columns []string) map[string]int {
	idx := make(map[string]int, len(columns))
	for i, c := range columns {
		idx[strings.ToLower(c)] = i
	}
	return idx
}

func cellString(row []interface{}, colIdx map[string]int, col string) string {
	i, ok := colIdx[col]
	if !ok || i >= len(row) || row[i] == nil {
		return ""
	}
	switch v := row[i].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func isDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func matchesAnyKeyword(title string, keywords []string) bool {
	lower := strings.ToLower(title)
	for _, kw := range keywords {
		if kw != "" && strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// SanitizeURL ensures only valid http:// or https:// URLs are kept.
func SanitizeURL(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

func cleanText(s string) string {
	unescaped := html.UnescapeString(s)
	noTags := htmlTagRegex.ReplaceAllString(unescaped, " ")
	return strings.Join(strings.Fields(html.UnescapeString(noTags)), " ")
}

func parseFlexibleDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// Explicit timezone layouts
	tzLayouts := []string{
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
	}
	for _, layout := range tzLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}

	// Timezone-less Russian/MOEX timestamps are in Europe/Moscow (MSK, UTC+3)
	localLayouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"02.01.2006 15:04:05",
		"02.01.2006 15:04",
		"02.01.2006",
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, mskLoc); err == nil {
			return t
		}
	}
	return time.Time{}
}
