package main

import (
	"context"
	"encoding/csv"
	"flag"
	"io"
	"log"
	"os"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/spannerio"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/runners/direct"
)

var (
	input = flag.String("input-csv-path", "", "The path of the input MySQL CSV dumped .")
	path  = flag.String("spanner-database-path", "", "The path of the output Spanner database.")
	table = flag.String("spanner-table", "", "The name of the output Spanner table.")
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

// readCSVFile reads the CSV file and returns a PCollection of strings representing each line.
func readCSVFile(s beam.Scope, filePath string) beam.PCollection {
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read each line from the CSV file.
	dataModels := beam.Impulse(s)
	return beam.ParDo(s, func(ctx context.Context, emit func(*DataModel)) {
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("Failed to read record: %v", err)
			}
			emit(parseDataModel(record))
		}
	}, dataModels)
}

func main() {
	flag.Parse()
	beam.Init()

	// Create the pipeline object and the root scope
	p, s := beam.NewPipelineWithRoot()

	dataModels := readCSVFile(s, *input)

	spannerio.Write(s, database, table, dataModels)

	// Run the pipeline.
	if _, err := direct.Execute(context.Background(), p); err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}
}
