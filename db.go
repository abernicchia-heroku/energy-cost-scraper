// reference(s):
// 	https://github.com/heroku/sql-drain
// 	debug test bug - https://github.com/golang/vscode-go/issues/2953

package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const DbUrlEnv string = "DATABASE_URL"

var db *sql.DB
var punEntryInsertStmt *sql.Stmt
var psvEntryInsertStmt *sql.Stmt

const createTableTemplate string = `
	CREATE TABLE IF NOT EXISTS ${TABLENAME} (
		id SERIAL,
		time timestamp UNIQUE NOT NULL,
		cost float NOT NULL,
		PRIMARY KEY (id)
  );
  	CREATE INDEX IF NOT EXISTS ${TABLENAME}_time_idx ON ${TABLENAME}(time);
`

const selectLastEnergyCostTimeTemplate string = "SELECT max(time) FROM ${TABLENAME} HAVING max(time) IS NOT NULL;"

const insertEnergyCostTemplate string = "INSERT into ${TABLENAME}(time, cost) VALUES ($1, $2);"

func costEntryInsert(costType EnergyCostEntryType, time time.Time, cost float64) error {
	if isEnvGreaterThan(DebugEnv, 1000) {
		fmt.Printf("[db.go:costEntryInsert] inserting cost entry type[%s] date[%v] cost[%v]\n", costType.String(), time, cost)
	}

	var entryInsertStmt *sql.Stmt

	if costType == EnergyCostEntryType_PUN {
		entryInsertStmt = punEntryInsertStmt
	} else if costType == EnergyCostEntryType_PSV {
		entryInsertStmt = psvEntryInsertStmt
	}

	_, err := entryInsertStmt.Exec(
		time,
		cost)
	if err != nil {
		fmt.Printf("[db.go:costEntryInsert] DB error: %v\n", err)
	}

	return err
}

func maxTimeSelect(costType EnergyCostEntryType) (time.Time, error) {
	// in this way the first time the table is created all the cost entries are created and a time is always returned
	var t time.Time = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.Local)

	err := db.QueryRow(strings.Replace(selectLastEnergyCostTimeTemplate, "${TABLENAME}", costType.String(), -1)).Scan(&t)
	if err != nil && err != sql.ErrNoRows {
		fmt.Printf("DB error: %v\n", err)

		return t, err
	}

	return t, nil // max or the default time is returned
}

// called when the class is loaded
func init() {
	// Connect to postgresql
	var err error

	dburl := os.Getenv(DbUrlEnv) + "?sslmode=require&application_name=energy-cost-scraper"

	u, err := url.Parse(dburl)
	if err != nil {
		fmt.Printf("Invalid DB URL: %v\n", err)
	}

	if isEnv(DebugEnv) {
		fmt.Printf("[db.go:init] db url %v\n", u.Redacted())
	}

	db, err = sql.Open("postgres", dburl)
	if err != nil {
		fmt.Printf("Open DB error: %v\n", err)
	}

	err = db.Ping()
	if err != nil {
		fmt.Printf("Unable to ping DB: %v\n", err)
	}

	fmt.Printf("Initializing db tables ...\n")

	_, err = db.Exec(strings.Replace(createTableTemplate, "${TABLENAME}", "pun", -1))
	if err != nil {
		fmt.Printf("Unable to create [pun] table: %v\n", err)
	}

	_, err = db.Exec(strings.Replace(createTableTemplate, "${TABLENAME}", "psv", -1))
	if err != nil {
		fmt.Printf("Unable to create [psv] table: %v\n", err)
	}

	// tables need to be created before the prepared statements are created as they depend on them
	fmt.Printf("Initializing prepared statements ...\n")

	punEntryInsertStmt, err = db.Prepare(strings.Replace(insertEnergyCostTemplate, "${TABLENAME}", "pun", -1))
	if err != nil {
		fmt.Printf("Unable to create prepared stmt: %v\n", err)
	}

	psvEntryInsertStmt, err = db.Prepare(strings.Replace(insertEnergyCostTemplate, "${TABLENAME}", "psv", -1))
	if err != nil {
		fmt.Printf("Unable to create prepared stmt: %v\n", err)
	}
}
