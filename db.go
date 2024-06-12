// reference(s):
// 	https://github.com/heroku/sql-drain
// 	debug test bug - https://github.com/golang/vscode-go/issues/2953

package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"

	_ "github.com/lib/pq"
)

const DbUrlEnv string = "DATABASE_URL"

var db *sql.DB
var punEntryInsertStmt *sql.Stmt
var psvEntryInsertStmt *sql.Stmt

const createPunTable string = `
	CREATE TABLE IF NOT EXISTS pun (
		id SERIAL PRIMARY KEY,
		time UNIQUE NOT NULL,
		cost float NOT NULL,
		PRIMARY KEY (id)
  );
  	CREATE INDEX pun_time_idx ON pun(time);
  `

const selectMaxPunTime string = "SELECT max(time) FROM pun ;"

const createPsvTable string = `
	CREATE TABLE IF NOT EXISTS psv (
		id SERIAL PRIMARY KEY,
		time UNIQUE NOT NULL,
		cost float NOT NULL,
		PRIMARY KEY (id)
  );
  	CREATE INDEX psv_time_idx ON psv(time);
  `

const selectMaxPsvTime string = "SELECT max(time) FROM psv ;"

func punEntryInsert(time time.Time, cost float64) error {
	_, err := punEntryInsertStmt.Exec(
		time,
		cost)
	if err != nil {
		fmt.Printf("DB error: %v\n", err)
	}

	return err
}

func maxPunTimeSelect() (time.Time, error) {
	var t time.Time = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.Local) // in this way the first time the table is created all the cost entries are created and a time is always returned
	err := db.QueryRow(selectMaxPunTime).Scan(&t)
	if err == sql.ErrNoRows {
		return t, nil
	} else {
		fmt.Printf("DB error: %v\n", err)

		return t, err
	}
}

func psvEntryInsert(time time.Time, cost float64) error {
	_, err := psvEntryInsertStmt.Exec(
		time,
		cost)
	if err != nil {
		fmt.Printf("DB error: %v\n", err)
	}

	return err
}

func maxPsvTimeSelect() (time.Time, error) {
	var t time.Time = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.Local) // in this way the first time the table is created all the cost entries are created and a time is always returned
	err := db.QueryRow(selectMaxPsvTime).Scan(&t)
	if err == sql.ErrNoRows {
		return t, nil
	} else {
		fmt.Printf("DB error: %v\n", err)

		return t, err
	}
}

// called when the class is loaded
func init() {
	// Connect to postgresql
	var err error

	dburl := os.Getenv(DbUrlEnv) + "?sslmode=require"

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

	punEntryInsertStmt, err = db.Prepare("INSERT into pun(time, cost) VALUES ($1, $2);")
	if err != nil {
		fmt.Printf("Unable to create prepared stmt: %v\n", err)
	}

	psvEntryInsertStmt, err = db.Prepare("INSERT into psv(time, cost) VALUES ($1, $2);")
	if err != nil {
		fmt.Printf("Unable to create prepared stmt: %v\n", err)
	}

	fmt.Printf("Initializing db tables ...\n")
	_, err = db.Exec(createPunTable)
	if err != nil {
		fmt.Printf("Unable to create [pun] table: %v\n", err)
	}

	_, err = db.Exec(createPsvTable)
	if err != nil {
		fmt.Printf("Unable to create [psv] table: %v\n", err)
	}
}
