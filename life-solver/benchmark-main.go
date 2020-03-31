package main

import (
	"log"
	"math/rand"
	"sort"
	"time"

	"row-major/life-solver/life"
)

func main() {
	for dim := 1; dim <= 30; dim++ {
		dim := dim

		runTimings := []time.Duration{}
		for i := 0; i < 2; i++ {
			r := rand.New(rand.NewSource(487489 * int64(i)))

			before := life.NewCellBoard(r.Intn(dim)+1, r.Intn(dim)+1)
			for i := 0; i < before.NumRows*before.NumCols; i++ {
				if r.Intn(2) == 0 {
					before.Cells[i] = life.CellDead
				} else {
					before.Cells[i] = life.CellAlive
				}
			}

			after := life.ForwardStep(before)

			start := time.Now()
			solver := life.NewReverseSolver(after)
			_, _ = solver.YieldSolution()
			elapsed := time.Now().Sub(start)

			runTimings = append(runTimings, elapsed)
		}

		sort.Slice(runTimings, func(i, j int) bool {
			return runTimings[i] < runTimings[j]
		})

		log.Printf("%dx%d min=%v med=%v max=%v", dim, dim, runTimings[0], runTimings[4], runTimings[10])
	}
}
