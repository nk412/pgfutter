package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/lib/pq"
)

func parseColumns(c *cli.Context, reader *csv.Reader) []string {
	var err error
	var columns []string
	if c.Bool("skip-header") {
		columns = strings.Split(c.String("fields"), ",")
	} else {
		columns, err = reader.Read()
		failOnError(err, "Could not read header row")
	}

	for i, column := range columns {
		columns[i] = postgresify(column)
	}

	return columns
}

func importCsv(c *cli.Context) {
	filename := c.Args().First()
	if filename == "" {
		fmt.Println("Please provide name of file to import")
		os.Exit(1)
	}

	schema := c.GlobalString("schema")
	tableName := parseTableName(c, filename)

	db, err := connect(createConnStr(c), schema)
	failOnError(err, "Could not connect to db")
	defer db.Close()

	file, err := os.Open(filename)
	failOnError(err, "Cannot open file")
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = rune(c.String("delimiter")[0])
	reader.LazyQuotes = true

	columns := parseColumns(c, reader)
	reader.FieldsPerRecord = len(columns)

	createTable := createTableStatement(db, schema, tableName, columns)
	_, err = createTable.Exec()
	failOnError(err, "Could not create table")

	txn, err := db.Begin()
	failOnError(err, "Could not start transaction")

	stmt, err := txn.Prepare(pq.CopyInSchema(schema, tableName, columns...))
	failOnError(err, "Could not prepare copy in statement")

	for {
		cols := make([]interface{}, len(columns))
		record, err := reader.Read()
		for i, col := range record {
			cols[i] = col
		}

		if err == io.EOF {
			break
		}
		failOnError(err, "Could not read csv")
		_, err = stmt.Exec(cols...)
		failOnError(err, "Could add bulk insert")
	}

	_, err = stmt.Exec()
	failOnError(err, "Could not exec the bulk copy")

	err = stmt.Close()
	failOnError(err, "Could not close")

	err = txn.Commit()
	failOnError(err, "Could not commit transaction")
}
