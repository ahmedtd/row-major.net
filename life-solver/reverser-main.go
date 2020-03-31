package main

import (
	"flag"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"row-major/life-solver/life"
)

var (
	successorFile = flag.String("successor-board", "", "The board file for which we will search for predecessors.")
)

func main() {
	flag.Parse()

	data, err := ioutil.ReadFile(*successorFile)
	if err != nil {
		log.Fatalf("Error reading successor board: %v", err)
	}

	successorBoard := life.NewCellBoardFromSource(string(data))

	ph := &ProgressHandler{}
	http.Handle("/", ph)

	go func() {
		http.ListenAndServe(":8080", nil)
	}()

	solver := life.NewReverseSolver(successorBoard, life.WithFoamSuppression())
	start := time.Now()
	for {
		status, sol := solver.YieldSolutionBounded(100000000)
		if status == life.YieldFinished {
			log.Fatalf("Finished search without finding solution")
		} else if status == life.YieldOutOfGas {
			// Out of gas, print progress report
			elapsed := time.Now().Sub(start)
			nanosPerState := int64(elapsed) / solver.StatsNumStatesConsidered
			log.Printf(
				"Searching: elapsed_time=%v max_depth=%d states_considered=%d ns/state=%d readmems=%d writemems=%d",
				elapsed,
				solver.StatsMaxDepth,
				solver.StatsNumStatesConsidered,
				nanosPerState,
				solver.StatsReadMems,
				solver.StatsWriteMems,
			)
			ph.Elapsed = elapsed
			ph.CurBoardText = solver.CurBoard.DisplayString()
			ph.PreBoardText = solver.PreBoard.DisplayString()
			ph.MaxDepth = solver.StatsMaxDepth
			ph.StatesConsidered = solver.StatsNumStatesConsidered
			ph.NanosPerState = nanosPerState
			ph.ReadMems = solver.StatsReadMems
			ph.WriteMems = solver.StatsWriteMems
			continue
		} else {
			// Solution
			log.Printf("Found solution:\n%s", sol.DisplayString())
			break
		}
	}
}

type ProgressHandler struct {
	CurBoardText string
	PreBoardText string

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
        <tr><th>Input Board</th><th>Current Search Board</th></tr>
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

func (h *ProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	progressTemplate.Execute(w, h)
}
