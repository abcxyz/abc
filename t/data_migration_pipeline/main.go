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
	"os/signal"
	"strings"
	"syscall"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/core/metrics"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/spannerio"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/textio"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/runners/direct"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/x/beamx"
)

var (
	flagInput    = flag.String("input-csv-path", "", "The path of the input MySQL CSV dumped .")
	flagDatabase = flag.String("spanner-database", "", "The path of the output Spanner database.")
	flagTable    = flag.String("spanner-table", "", "The name of the output Spanner table.")
	flagDryRun   = flag.Bool("dry-run", false, "whether the specified run is a dry run")
)

var count = beam.NewCounter("dry-run", "count")

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
func emitResult(ctx context.Context, s beam.Scope, lines beam.PCollection) beam.PCollection {
	dataModels := beam.ParDo(s, func(line string, emit func(*DataModel)) {
		reader := csv.NewReader(strings.NewReader(line))
		csvLine, err := reader.Read()
		if err != nil {
			log.Fatalf("Failed to read record: %v", err)
		}
		emit(parseDataModel(csvLine))
		count.Inc(ctx, 1)
	}, lines)

	return dataModels
}

func main() {
	// Handle context cancellation.
	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Set up metric
	ctx = metrics.SetPTransformID(ctx, "pipeline")

	flag.Parse()
	beam.Init()

	// Create the pipeline object and the root scope
	p, s := beam.NewPipelineWithRoot()

	lines, err := textio.Immediate(s, *flagInput)
	if err != nil {
		log.Fatalf("Failed to read %v: %v", *flagInput, err)
	}

	// Convert each line to a data model
	dataModels := emitResult(ctx, s, lines)

	// Verify data on dry run mode
	if *flagDryRun {
		log.Println("dry run start....")
		if _, err := beamx.RunWithMetrics(ctx, p); err != nil {
			log.Fatalf("Pipeline failed: %v", err)
		}
		metrics.DumpToOutFromContext(ctx)
		return
	}

	// Write data into database
	spannerio.Write(s, *flagDatabase, *flagTable, dataModels)

	// Run the pipeline.
	if _, err := direct.Execute(ctx, p); err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}
}
