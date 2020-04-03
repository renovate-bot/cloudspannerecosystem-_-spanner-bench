// Copyright 2020 Google Inc. All Rights Reserved.
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

package main

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
)

type benchmarks struct {
	client  *spanner.Client
	n       int
	queries []Query
}

func (b *benchmarks) start() {
	for _, q := range b.queries {
		b.run(q)
	}
}

func (b *benchmarks) run(q Query) {
	fmt.Println(q.Name)

	var results []benchmarkResult
	stmt := spanner.NewStatement(q.SQL)
	for _, opt := range q.Optimizers {
		results = append(results, b.queryN(opt, stmt))
	}
	b.print(q, results...)
}

func (b *benchmarks) queryN(v string, stmt spanner.Statement) benchmarkResult {
	var i int

	var total benchmarkResult
	for {
		if i == b.n {
			break
		}
		result, err := b.query(v, stmt)
		if err != nil {
			// TODO(jbd): Error if too many retries.
			continue
		}
		total.RowsScanned += result.RowsScanned
		total.RowsReturned += result.RowsReturned
		total.CPUTime += result.CPUTime
		total.QueryPlanTime += result.QueryPlanTime
		total.ElapsedTime += result.ElapsedTime
		i++
	}
	// TODO(jbd): Use the 50th percentile instead.
	return benchmarkResult{
		Optimizer:     v,
		RowsScanned:   total.RowsScanned / int64(i),
		RowsReturned:  total.RowsReturned / int64(i),
		QueryPlanTime: total.QueryPlanTime / time.Duration(i),
		CPUTime:       total.CPUTime / time.Duration(i),
		ElapsedTime:   total.ElapsedTime / time.Duration(i),
	}
}

func (b *benchmarks) query(v string, stmt spanner.Statement) (benchmarkResult, error) {
	mode := sppb.ExecuteSqlRequest_PROFILE
	it := b.client.Single().QueryWithOptions(context.Background(), stmt, spanner.QueryOptions{
		Mode: &mode,
		Options: &sppb.ExecuteSqlRequest_QueryOptions{
			OptimizerVersion: v,
		},
	})
	defer it.Stop()
	for {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return benchmarkResult{}, err
		}
	}

	var result benchmarkResult
	for k, v := range it.QueryStats {
		switch k {
		case "rows_scanned":
			result.RowsScanned = parseInt64(v.(string))
		case "rows_returned":
			result.RowsReturned = parseInt64(v.(string))
		case "query_plan_creation_time":
			result.QueryPlanTime = parseDuration(v.(string))
		case "cpu_time":
			result.CPUTime = parseDuration(v.(string))
		case "elapsed_time":
			result.ElapsedTime = parseDuration(v.(string))
		}
	}
	result.Optimizer = v
	return result, nil
}

func (b *benchmarks) print(q Query, r ...benchmarkResult) {
	// Print the header.
	fmt.Printf("   %10s %10s %10s %10s \n", "(scanned)", "(total)", "(cpu)", "(plan)")
	for i, result := range r {
		fmt.Println(result)
		if i > 0 {
			// TODO(jbd): Compare with the previous.
			// fmt.Println(result.Diff(r[i-1]))
		}
	}
}