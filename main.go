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

// TODO: env var
const EnergyCostSiteUrl string = "https://www.acea.it/tariffe-indici"

// TODO: env vars
const PunTableFullXpath string = "/html/body/div[2]/div[2]/section[3]/div/div/div/div[2]/div[2]/div/div[1]/div/div[2]/div/div/div/div/div/div/div[2]/table/tbody"
const PsvTableFullXpath string = "/html/body/div[2]/div[2]/section[3]/div/div/div/div[2]/div[2]/div/div[2]/div/div[2]/div/div/div/div/div/div/div/table/tbody"

const (
	EnergyCostEntryType_PUN = iota
	EnergyCostEntryType_PSV
)

type energyCostEntry struct {
	date time.Time
	cost float64
}

var month2Number = map[string]int{"gen": 1, "feb": 2, "mar": 3, "apr": 4, "mag": 5, "giu": 6, "lug": 7, "ago": 8, "set": 9, "ott": 10, "nov": 11, "dic": 12}

func main() {

	fmt.Println("Loading URL: ", EnergyCostSiteUrl)

	htmldoc, err := htmlquery.LoadURL(EnergyCostSiteUrl)
	if err != nil {
		panic(err)
	}

	fmt.Println("PUN - energy")
	scrapeEnergyCost(htmldoc, EnergyCostEntryType_PUN)

	fmt.Println("PSV - gas")
	scrapeEnergyCost(htmldoc, EnergyCostEntryType_PSV)
}

func scrapeEnergyCost(htmldoc *html.Node, energyCostEntryType int) ([]energyCostEntry, time.Time) {
	var tableFullXPath string

	if energyCostEntryType == EnergyCostEntryType_PUN {
		tableFullXPath = PunTableFullXpath
	} else if energyCostEntryType == EnergyCostEntryType_PSV {
		tableFullXPath = PsvTableFullXpath
	}

	costEntries := scrapeEnergyCostEntries(htmldoc, tableFullXPath)

	var t time.Time
	var err error

	if energyCostEntryType == EnergyCostEntryType_PUN {
		t, err = maxPunTimeSelect()
		if err != nil {
			fmt.Printf("Not able to retrieve the latest energy cost entry from PUN: %v\n", err)
		}
	} else if energyCostEntryType == EnergyCostEntryType_PSV {
		t, err = maxPsvTimeSelect()
		if err != nil {
			fmt.Printf("Not able to retrieve the latest energy cost entry from PSV: %v\n", err)
		}
	}

	for _, ce := range costEntries {
		if ce.date.After(t) {
			if energyCostEntryType == EnergyCostEntryType_PUN {
				punEntryInsert(t, ce.cost)
			} else if energyCostEntryType == EnergyCostEntryType_PSV {
				psvEntryInsert(t, ce.cost)
			}
		}
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
