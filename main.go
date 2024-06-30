package main

import (
	"fmt"
	"os"
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
const PunTableFullXpathDefault string = "/html/body/div[2]/div[2]/section[3]/div/div/div/div[2]/div[2]/div/div[1]/div/div[2]/div/div/div/div/div/div/div[2]/table/tbody"

const EnergyCostPsvXpathEnv string = "ECS_ENERGYCOSTSCRAPER_PSV_XPATH"
const PsvTableFullXpathDefault string = "/html/body/div[2]/div[2]/section[3]/div/div/div/div[2]/div[2]/div/div[2]/div/div[2]/div/div/div/div/div/div/div/table/tbody"

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

	if !newEntriesFound {
		fmt.Printf("[main.go:scrapeEnergyCost] no new cost entries found\n")
	}

	return costEntries, t
}

func scrapeEnergyCostEntries(htmldoc *html.Node, xpath string) []energyCostEntry {
	table, err := htmlquery.Query(htmldoc, xpath)
	if err != nil {
		panic(err)
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
					//fmt.Printf("row(%d) column(%d) %s\n", rowIdx, colIdx, htmlquery.InnerText(column))
					energyCostEntries[rowIdx-1].date = parseTime(htmlquery.InnerText(column))
				} else if colIdx == 1 {
					//fmt.Printf("row(%d) column(%d) %s\n", rowIdx, colIdx, htmlquery.InnerText(column))
					energyCostEntries[rowIdx-1].cost, _ = strconv.ParseFloat(strings.Replace(string(htmlquery.InnerText(column)), ",", ".", -1), 64)
				}
			}
		}
	}

	for _, energyCostEntry := range energyCostEntries {
		fmt.Printf("%s %f\n", energyCostEntry.date, energyCostEntry.cost)
	}

	//fmt.Printf("%#v\n", energyCostEntries)

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
