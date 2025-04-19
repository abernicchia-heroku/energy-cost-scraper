package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

const DebugEnv string = "ECS_ENERGYCOSTSCRAPER_DEBUG"

const EnergyCostSiteUrlEnv string = "ECS_ENERGYCOSTSCRAPER_SITEURL"
const EnergyCostSiteUrlDefault string = "https://www.acea.it/tariffe-indici"

const EnergyCostPunXpathEnv string = "ECS_ENERGYCOSTSCRAPER_PUN_XPATH"
const PunTableFullXpathDefault string = "/html/body/div[2]/div[2]/section[3]/div/div/div/div[2]/div[2]/div/div[1]/div/div[2]/div/div/div/div/div/div/div/table/tbody"

const EnergyCostPsvXpathEnv string = "ECS_ENERGYCOSTSCRAPER_PSV_XPATH"
const PsvTableFullXpathDefault string = "/html/body/div[2]/div[2]/section[3]/div/div/div/div[2]/div[2]/div/div[2]/div/div[2]/div/div/div/div/div/div/div/table/tbody"

const ReferencePunEnergyCostEnv string = "ECS_ENERGYCOSTSCRAPER_PUN_REFERENCE_COST"
const ReferencePunEnergyCostDefault string = "0.11"

const ReferencePsvEnergyCostEnv string = "ECS_ENERGYCOSTSCRAPER_PSV_REFERENCE_COST"
const ReferencePsvEnergyCostDefault string = "0.39"

const MailSmtpUrlEnv string = "ECS_ENERGYCOSTSCRAPER_SMTP_URL"
const MailSmtpUrlDefault string = "smtp://user:password@hostname:port?starttls=true"

const MailgunUserEnv string = "MAILGUN_SMTP_LOGIN"
const MailgunUserDefault string = "mailgunuser"

const MailgunPasswordEnv string = "MAILGUN_SMTP_PASSWORD"
const MailgunPasswordDefault string = "mailgunpasswd"

const MailgunSmtpPortEnv string = "MAILGUN_SMTP_PORT"
const MailgunSmtpPortDefault string = "587"

const MailgunSmtpHostnameEnv string = "MAILGUN_SMTP_SERVER"
const MailgunSmtpHostnameDefault string = "mailgunhostname"

const MailFromEnv string = "ECS_ENERGYCOSTSCRAPER_MAIL_FROM"
const MailFromDefault string = "bot@example.com"

const MailToEnv string = "ECS_ENERGYCOSTSCRAPER_MAIL_TO"
const MailToDefault string = "tmp@example.com"

const mailMessageTemplate string = "" +
	"To: {{.MailTo}}\r\n" +
	"From: {{.MailFrom}}\r\n" +
	"Subject: Energy Cost Scraper - {{.EnergyType}} market price getting lower!\r\n" +
	"\r\n" +
	"{{.EnergyType}} market price {{.MarketPrice}} EUR is lower than your current bill price {{.BillPrice}} EUR with a discount of {{.PercentageDiscount}}%\nPlease, look for alternative energy providers!"

type MailInfo struct {
	MailTo             string
	MailFrom           string
	EnergyType         string
	MarketPrice        string
	BillPrice          string
	PercentageDiscount string
}

type EnergyCostEntryType int

const (
	EnergyCostEntryType_PUN EnergyCostEntryType = 1
	EnergyCostEntryType_PSV EnergyCostEntryType = 2
)

func (s EnergyCostEntryType) String() string {
	switch s {
	case 1:
		return "pun"
	case 2:
		return "psv"
	}
	return "unknown"
}

type energyCostEntry struct {
	date time.Time
	cost float64
}

var month2Number = map[string]int{"gen": 1, "feb": 2, "mar": 3, "apr": 4, "mag": 5, "giu": 6, "lug": 7, "ago": 8, "set": 9, "ott": 10, "nov": 11, "dic": 12}

func main() {

	fmt.Println("Loading URL: ", getEnv(EnergyCostSiteUrlEnv, EnergyCostSiteUrlDefault))

	htmldoc, err := htmlquery.LoadURL(getEnv(EnergyCostSiteUrlEnv, EnergyCostSiteUrlDefault))
	if err != nil {
		panic(err)
	}

	fmt.Println("PUN - energy")
	scrapeEnergyCost(htmldoc, EnergyCostEntryType_PUN)

	fmt.Println("PSV - gas")
	scrapeEnergyCost(htmldoc, EnergyCostEntryType_PSV)
}

func scrapeEnergyCost(htmldoc *html.Node, energyCostEntryType EnergyCostEntryType) ([]energyCostEntry, time.Time) {
	var tableFullXPath string

	if energyCostEntryType == EnergyCostEntryType_PUN {
		tableFullXPath = getEnv(EnergyCostPunXpathEnv, PunTableFullXpathDefault)
	} else if energyCostEntryType == EnergyCostEntryType_PSV {
		tableFullXPath = getEnv(EnergyCostPsvXpathEnv, PsvTableFullXpathDefault)
	}

	costEntries := scrapeEnergyCostEntries(htmldoc, tableFullXPath)

	var t time.Time
	var err error

	t, err = maxTimeSelect(energyCostEntryType)
	if err != nil {
		fmt.Printf("Not able to retrieve the latest energy cost entry from %s: %v\n", energyCostEntryType.String(), err)
	}

	if isEnvGreaterThan(DebugEnv, 1000) {
		fmt.Printf("[main.go:scrapeEnergyCost] latest cost entry found: %v\n", t)
	}

	var newEntriesFound = false
	for _, ce := range costEntries {
		if ce.date.After(t) {
			newEntriesFound = true
			costEntryInsert(energyCostEntryType, ce.date, ce.cost)
		}
	}

	if newEntriesFound {
		fmt.Printf("[main.go:scrapeEnergyCost] new cost entries found\n")

		var refEnergyCost float64
		if energyCostEntryType == EnergyCostEntryType_PUN {
			refEnergyCost, _ = strconv.ParseFloat(getEnv(ReferencePunEnergyCostEnv, ReferencePunEnergyCostDefault), 64)
		} else if energyCostEntryType == EnergyCostEntryType_PSV {
			refEnergyCost, _ = strconv.ParseFloat(getEnv(ReferencePsvEnergyCostEnv, ReferencePsvEnergyCostDefault), 64)
		}

		// assuming that new inserted entries are greater (more recent) than those added previously (older)
		earliestEnergyCostEntry := slices.MaxFunc(costEntries, func(a, b energyCostEntry) int {
			return a.date.Compare(b.date)
		})

		// check if the earliest market price is lower than the reference paid price
		if earliestEnergyCostEntry.cost < refEnergyCost {
			sendMail(energyCostEntryType, earliestEnergyCostEntry, refEnergyCost)
		}
	} else {
		fmt.Printf("[main.go:scrapeEnergyCost] no new cost entries found\n")
	}

	return costEntries, t
}

func sendMail(energyCostEntryType EnergyCostEntryType, earliestEnergyCostEntry energyCostEntry, refEnergyCost float64) {
	// by default it's expected to work with Mailgun add-on, but it's possible to override the smtp URL with the MailSmtpUrlEnv

	var mailSmtpUrl string
	if isEnv(MailSmtpUrlEnv) {
		fmt.Printf("Using external SMTP service to send emails\n")

		mailSmtpUrl = getEnv(MailSmtpUrlEnv, MailSmtpUrlDefault)
	} else {
		fmt.Printf("Using Mailgun to send emails\n")

		mailSmtpUrl = fmt.Sprintf("smtp://%s:%s@%s:%s?starttls=true", getEnv(MailgunUserEnv, MailgunUserDefault), getEnv(MailgunPasswordEnv, MailgunPasswordDefault), getEnv(MailgunSmtpHostnameEnv, MailgunSmtpHostnameDefault), getEnv(MailgunSmtpPortEnv, MailgunSmtpPortDefault))
	}

	u, err := url.Parse(mailSmtpUrl)
	if err != nil {
		fmt.Printf("Invalid SMTP URL: %v\n", err)
	} else {
		// https://docs.cloudmailin.com/outbound/examples/send_email_with_golang/

		// hostname is used by PlainAuth to validate the TLS certificate.
		hostname := u.Hostname()
		passwd, _ := u.User.Password()
		auth := smtp.PlainAuth("", u.User.Username(), passwd, hostname)

		mailInfo := MailInfo{getEnv(MailToEnv, MailToDefault), getEnv(MailFromEnv, MailFromDefault), strings.ToUpper(energyCostEntryType.String()), fmt.Sprint(earliestEnergyCostEntry.cost), fmt.Sprint(refEnergyCost), fmt.Sprint((refEnergyCost - earliestEnergyCostEntry.cost) * 100 / earliestEnergyCostEntry.cost)}

		tmpl, err := template.New("mailTemplate").Parse(mailMessageTemplate)
		if err != nil {
			panic(err)
		}

		var mailMsg bytes.Buffer
		err = tmpl.Execute(&mailMsg, mailInfo)
		if err != nil {
			panic(err)
		}

		fmt.Printf("[main.go:scrapeEnergyCost] market price [%v EUR] is lower than current paid price [%v EUR], discount [%v%%], sending email alert ...\n", earliestEnergyCostEntry.cost, refEnergyCost, mailInfo.PercentageDiscount)
		err = smtp.SendMail(hostname+":"+u.Port(), auth, mailInfo.MailFrom, []string{mailInfo.MailTo}, mailMsg.Bytes())
		if err != nil {
			fmt.Printf("Error sending email: %v\n", err)
		}
	}
}

func scrapeEnergyCostEntries(htmldoc *html.Node, xpath string) []energyCostEntry {
	table, err := htmlquery.Query(htmldoc, xpath)
	if err != nil {
		panic(err)
	}

	if table == nil {
		fmt.Printf("Table not found: %s\n", xpath)
		panic("xpath not found, check if the source page was modified")
	}

	rows, err := htmlquery.QueryAll(table, "//tr")
	if err != nil {
		panic(err)
	}

	energyCostEntries := make([]energyCostEntry, len(rows)-1)

	for rowIdx, row := range rows {
		if rowIdx != 0 {

			columns, err := htmlquery.QueryAll(row, "//td")
			if err != nil {
				panic(err)
			}

			for colIdx, column := range columns {
				if colIdx == 0 {
					energyCostEntries[rowIdx-1].date = parseTime(htmlquery.InnerText(column))
				} else if colIdx == 1 {
					energyCostEntries[rowIdx-1].cost, _ = strconv.ParseFloat(strings.Replace(string(htmlquery.InnerText(column)), ",", ".", -1), 64)
				}
			}
		}
	}

	for _, energyCostEntry := range energyCostEntries {
		fmt.Printf("%s %f\n", energyCostEntry.date, energyCostEntry.cost)
	}

	return energyCostEntries
}

// mag 24 ==> 2024-05-01
func parseTime(timeString string) time.Time {

	year, _ := strconv.ParseInt(strings.Fields(timeString)[1], 10, 64)

	dateValue := fmt.Sprintf("20%02d-%02d-01", year, month2Number[strings.Fields(timeString)[0]])

	t, _ := time.Parse("2006-01-02", dateValue)

	return t
}

func isEnv(key string) bool {
	if _, ok := os.LookupEnv(key); ok {
		return true
	}
	return false
}

func isEnvGreaterThan(key string, val int64) bool {
	if v, ok := os.LookupEnv(key); ok {
		intval, _ := strconv.ParseInt(string(v), 10, 64)

		return intval > val
	}

	return false
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
