package main

import (
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	"row-major/life-solver"
)

func main() {

	ph := &ProgressHandler{}
	http.Handle("/", ph)

	go func() {
		http.ListenAndServe(":8080", nil)
	}()

	for dim := 1; dim <= 30; dim++ {
		dim := dim

		runTimings := []time.Duration{}
		for i := 0; i < 10; i++ {
			r := rand.New(rand.NewSource(487489 * int64(i)))

			before := life.NewCellBoard(20, dim)
			for i := 0; i < before.NumRows*before.NumCols; i++ {
				if r.Intn(2) == 0 {
					before.Cells[i] = life.CellDead
				} else {
					before.Cells[i] = life.CellAlive
				}
			}

			after := life.ForwardStep(before)

			solver := life.NewReverseSolver(after, life.WithFoamSuppression())
			ph.Lock.Lock()
			ph.Elapsed = 0 * time.Second
			ph.CurBoard = solver.CurBoard.Copy()
			ph.PreBoard = solver.PreBoard.Copy()
			ph.MaxDepth = solver.StatsMaxDepth
			ph.StatesConsidered = solver.StatsNumStatesConsidered
			ph.NanosPerState = -1
			ph.ReadMems = solver.StatsReadMems
			ph.WriteMems = solver.StatsWriteMems
			ph.Lock.Unlock()

			start := time.Now()
			for {
				status, _ := solver.YieldSolutionBounded(100000000)
				if status == life.YieldOutOfGas {
					// Out of gas, print progress report
					elapsed := time.Now().Sub(start)
					nanosPerState := int64(elapsed) / solver.StatsNumStatesConsidered
					ph.Lock.Lock()
					ph.Elapsed = elapsed
					ph.CurBoard = solver.CurBoard.Copy()
					ph.PreBoard = solver.PreBoard.Copy()
					ph.MaxDepth = solver.StatsMaxDepth
					ph.StatesConsidered = solver.StatsNumStatesConsidered
					ph.NanosPerState = nanosPerState
					ph.ReadMems = solver.StatsReadMems
					ph.WriteMems = solver.StatsWriteMems
					ph.Lock.Unlock()
					continue
				} else {
					break
				}
			}
			elapsed := time.Now().Sub(start)

			runTimings = append(runTimings, elapsed)
		}

		sort.Slice(runTimings, func(i, j int) bool {
			return runTimings[i] < runTimings[j]
		})

		log.Printf("rows=20 cols=%d cells=%d min=%v med=%v max=%v", dim, dim*20, runTimings[0], runTimings[len(runTimings)/2], runTimings[len(runTimings)-1])
	}
}

type ProgressHandler struct {
	Lock sync.Mutex

	CurBoard life.CellBoard
	PreBoard life.CellBoard

	Elapsed          time.Duration
	MaxDepth         int
	StatesConsidered int64
	NanosPerState    int64
	ReadMems         int64
	WriteMems        int64
}

var progressTemplate = template.Must(template.New("abc").Parse(`
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Progress Report</title>
  </head>
  <body>
    <h1>Progress Report</h1>
    <table>
      <tbody>
        <tr><td>Elapsed</td><td>{{.Elapsed}}</td></tr>
        <tr><td>States Considered</td><td>{{.StatesConsidered}}</td></tr>
        <tr><td>Nanoseconds Per State</td><td>{{.NanosPerState}}</td></tr>
        <tr><td>Max Depth So Far</td><td>{{.MaxDepth}}</td></tr>
        <tr><td>Read Mems</td><td>{{.ReadMems}}</td></tr>
        <tr><td>Write Mems</td><td>{{.WriteMems}}</td></tr>
      </tbody>
    </table>
    <table>
      <thead>
        <tr><th>Input Board ({{.CurBoardRows}}x{{.CurBoardCols}})</th><th>Current Search Board</th></tr>
      </thead>
      <tbody>
        <tr>
          <td><pre>{{.CurBoardText}}<pre></td>
          <td><pre>{{.PreBoardText}}</pre></td>
        </tr>
      </tbody>
    </table>
  </body>
  <script>setTimeout(function() {location.reload();}, 30000);</script>
</html>
`))

type ProgressTemplateVars struct {
	CurBoardText string
	CurBoardRows int
	CurBoardCols int
	PreBoardText string

	Elapsed          time.Duration
	MaxDepth         int
	StatesConsidered int64
	NanosPerState    int64
	ReadMems         int64
	WriteMems        int64
}

func (h *ProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Lock.Lock()
	progressTemplate.Execute(w, ProgressTemplateVars{
		CurBoardText: h.CurBoard.DisplayString(),
		CurBoardRows: h.CurBoard.NumRows,
		CurBoardCols: h.CurBoard.NumCols,
		PreBoardText: h.PreBoard.DisplayString(),

		Elapsed:          h.Elapsed,
		MaxDepth:         h.MaxDepth,
		StatesConsidered: h.StatesConsidered,
		NanosPerState:    h.NanosPerState,
		ReadMems:         h.ReadMems,
		WriteMems:        h.WriteMems,
	})
	h.Lock.Unlock()
}
