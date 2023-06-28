package main

import (
	"context"
	"encoding/csv"
	"flag"
	"log"
	"strings"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/spannerio"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/textio"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/runners/direct"
)

var (
	input    = flag.String("input-csv-path", "", "The path of the input MySQL CSV dumped .")
	database = flag.String("spanner-database", "", "The path of the output Spanner database.")
	table    = flag.String("spanner-table", "", "The name of the output Spanner table.")
)

type DataModel struct {
	/*
		Your data model goes here.
	*/
	ID string
}

// parseDataModel parses a CSV line and returns the DataModel.
func parseDataModel(record []string) *DataModel {
	/*
		Your data parsing logic goes here.
	*/
	return &DataModel{
		ID: record[0],
	}
}

func main() {
	flag.Parse()
	beam.Init()

	// Create the pipeline object and the root scope
	p, s := beam.NewPipelineWithRoot()

	lines, err := textio.Immediate(s, *input)
	if err != nil {
		log.Fatal("Failed to read %v: %v", *input, err)
		return
	}

	// Convert each line to a data model
	dataModels := beam.ParDo(s, func(line string, emit func(*DataModel)) {
		reader := csv.NewReader(strings.NewReader(line))
		csvLine, err := reader.Read()
		if err != nil {
			log.Fatalf("Failed to read record: %v", err)
		}
		emit(parseDataModel(csvLine))
	}, lines)

	spannerio.Write(s, *database, *table, dataModels)

	// Run the pipeline.
	if _, err := direct.Execute(context.Background(), p); err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}
}
