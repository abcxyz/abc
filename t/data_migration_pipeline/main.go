// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main implements a simple MySQL to Spanner data migration example.
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
	dryRun   = flag.Bool("dry-run", true, "whether the specified run is a dry run")
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

// emitResult emits data models to be written to Spanner
func emitResult(s beam.Scope, lines beam.PCollection) beam.PCollection {
	dataModels := beam.ParDo(s, func(line string, emit func(*DataModel)) {
		reader := csv.NewReader(strings.NewReader(line))
		csvLine, err := reader.Read()
		if err != nil {
			log.Fatalf("Failed to read record: %v", err)
		}
		emit(parseDataModel(csvLine))
	}, lines)
	return dataModels
}

func main() {
	flag.Parse()
	beam.Init()

	// Create the pipeline object and the root scope
	p, s := beam.NewPipelineWithRoot()

	lines, err := textio.Immediate(s, *input)
	if err != nil {
		log.Fatalf("Failed to read %v: %v", *input, err)
	}

	// Convert each line to a data model
	dataModels := emitResult(s, lines)

	// Turn on dry run monitor
	log.Println(*dryRun)
	if *dryRun {
		log.Println("dry run start")
		// TODO: print out dry run result
		return
	}
	// Write data into database
	spannerio.Write(s, *database, *table, dataModels)

	// Run the pipeline.
	if _, err := direct.Execute(context.Background(), p); err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}
}
