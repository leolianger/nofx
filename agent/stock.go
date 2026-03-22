package agent

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// StockQuote holds real-time stock data from Sina Finance API.
type StockQuote struct {
	Name      string
	Code      string
	Open      float64
	PrevClose float64
	Price     float64
	High      float64
	Low       float64
	Volume    float64 // shares
	Turnover  float64 // CNY
	Date      string
	Time      string

	Change    float64 // Price - PrevClose
	ChangePct float64 // Change / PrevClose * 100
}

// knownStocks maps Chinese names / common terms to stock codes.
var knownStocks = map[string]string{
	"拓维信息": "sz002261", "比亚迪": "sz002594", "宁德时代": "sz300750",
	"贵州茅台": "sh600519", "中国平安": "sh601318", "招商银行": "sh600036",
	"腾讯": "hk00700", "阿里巴巴": "hk09988", "美团": "hk03690",
	"小米": "hk01810", "京东": "hk09618",
	"苹果": "usr_aapl", "特斯拉": "usr_tsla", "英伟达": "usr_nvda",
	"微软": "usr_msft", "谷歌": "usr_googl", "亚马逊": "usr_amzn",
}

// resolveStockCode tries to find a stock code from user text.
func resolveStockCode(text string) (string, string) {
	// Direct match known stocks
	for name, code := range knownStocks {
		if strings.Contains(text, name) {
			return code, name
		}
	}

	// Try to extract a stock code like 002261, 600519, etc.
	words := strings.Fields(text)
	for _, w := range words {
		w = strings.TrimSpace(w)
		if len(w) == 6 {
			if _, err := strconv.Atoi(w); err == nil {
				prefix := "sz"
				if w[0] == '6' {
					prefix = "sh"
				} else if w[0] == '3' {
					prefix = "sz"
				} else if w[0] == '0' {
					prefix = "sz"
				}
				return prefix + w, w
			}
		}
	}

	return "", ""
}

// fetchStockQuote gets real-time A-share data from Sina Finance.
func fetchStockQuote(code string) (*StockQuote, error) {
	if !strings.HasPrefix(code, "sh") && !strings.HasPrefix(code, "sz") {
		return nil, fmt.Errorf("unsupported stock code: %s", code)
	}

	url := fmt.Sprintf("https://hq.sinajs.cn/list=%s", code)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Referer", "https://finance.sina.com.cn")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Sina returns GBK encoding
	reader := transform.NewReader(resp.Body, simplifiedchinese.GBK.NewDecoder())
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	line := string(body)
	// Parse: var hq_str_sz002261="name,open,prev_close,price,high,low,..."
	start := strings.Index(line, "\"")
	end := strings.LastIndex(line, "\"")
	if start == -1 || end <= start {
		return nil, fmt.Errorf("invalid response for %s", code)
	}

	fields := strings.Split(line[start+1:end], ",")
	if len(fields) < 32 {
		return nil, fmt.Errorf("not enough fields for %s", code)
	}

	q := &StockQuote{
		Name: fields[0],
		Code: code,
		Date: fields[30],
		Time: fields[31],
	}
	q.Open, _ = strconv.ParseFloat(fields[1], 64)
	q.PrevClose, _ = strconv.ParseFloat(fields[2], 64)
	q.Price, _ = strconv.ParseFloat(fields[3], 64)
	q.High, _ = strconv.ParseFloat(fields[4], 64)
	q.Low, _ = strconv.ParseFloat(fields[5], 64)
	q.Volume, _ = strconv.ParseFloat(fields[8], 64)
	q.Turnover, _ = strconv.ParseFloat(fields[9], 64)

	if q.PrevClose > 0 {
		q.Change = q.Price - q.PrevClose
		q.ChangePct = (q.Change / q.PrevClose) * 100
	}

	return q, nil
}

// formatStockQuote formats a quote for display.
func formatStockQuote(q *StockQuote) string {
	emoji := "🟢"
	if q.ChangePct < 0 {
		emoji = "🔴"
	}
	volStr := fmt.Sprintf("%.0f万手", q.Volume/1000000)
	turnStr := fmt.Sprintf("%.2f亿", q.Turnover/100000000)

	return fmt.Sprintf(`%s *%s* (%s)

💰 现价: ¥%.2f (%+.2f%%)
📊 今开: ¥%.2f | 昨收: ¥%.2f
📈 最高: ¥%.2f | 最低: ¥%.2f
📦 成交量: %s | 成交额: %s
🕐 数据时间: %s %s`,
		emoji, q.Name, q.Code,
		q.Price, q.ChangePct,
		q.Open, q.PrevClose,
		q.High, q.Low,
		volStr, turnStr,
		q.Date, q.Time)
}
