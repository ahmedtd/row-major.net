package life

import (
	"fmt"
	"math/rand"
	"testing"
)

var (
	board1x1Alive CellBoard
	board1x1Dead  CellBoard
)

func init() {
	board1x1Alive = NewCellBoard(1, 1)
	board1x1Alive.Set(0, 0, CellAlive)

	board1x1Dead = NewCellBoard(1, 1)
	board1x1Dead.Set(0, 0, CellDead)
}

func Test1x1Blank(t *testing.T) {
	solver := NewReverseSolver(board1x1Dead)

	fin, sol := solver.YieldSolution()
	if fin {
		t.Errorf("Solver didn't yield a first solution")
	}
	if !sol.Equals(board1x1Dead) {
		t.Errorf("Solver yielded bad first solution, got one live cell, want one dead cell")
	}
	if !ForwardStep(sol).Equals(board1x1Dead) {
		t.Errorf("First solution is not a valid predecessor of the input board")
	}

	fin, sol = solver.YieldSolution()
	if fin {
		t.Errorf("Solver didn't yield a second solution")
	}
	if !sol.Equals(board1x1Alive) {
		t.Errorf("Solver yielded bad second solution, got one dead cell, want one live cell")
	}
	if !ForwardStep(sol).Equals(board1x1Dead) {
		t.Errorf("Second solution is not a valid predecessor of the input board")
	}

	fin, _ = solver.YieldSolution()
	if !fin {
		t.Errorf("Solver yielded more than two solutions")
	}
}

func Test1x1Live(t *testing.T) {
	solver := NewReverseSolver(board1x1Alive)

	fin, _ := solver.YieldSolution()
	if !fin {
		t.Errorf("Solver yielded a solution, but wanted none")
	}
}

func TestAll2x2(t *testing.T) {
	for i := 0; i < 16; i++ {
		i := i
		t.Run(fmt.Sprintf("Case %d", i), func(t *testing.T) {
			input := NewCellBoard(2, 2)
			for j := 0; j < 4; j++ {
				if ((i >> j) & 0x1) == 0 {
					input.Cells[j] = CellDead
				} else {
					input.Cells[j] = CellAlive
				}
			}

			t.Logf("input:\n%s", input.DisplayString())

			solver := NewReverseSolver(input)

			for {
				fin, sol := solver.YieldSolution()
				if fin {
					break
				}
				if !ForwardStep(sol).Equals(input) {
					t.Errorf("Solution is not a valid predecessor of the input:\n%s\nsteps to:\n%s", sol.DisplayString(), ForwardStep(sol).DisplayString())
				}
			}
		})
	}
}

type lastNTracer struct {
	buf  []string
	cur  int
	size int
}

func (t *lastNTracer) trace(msg string) {
	if len(t.buf) < t.size {
		t.buf = append(t.buf, msg)
		t.cur++
	} else {
		t.cur = t.cur % len(t.buf)
		t.buf[t.cur] = msg
		t.cur++
	}
}

func (t *lastNTracer) get(i int) string {
	return t.buf[(t.cur+i)%len(t.buf)]
}

var glider1 = NewCellBoardFromSource(`_______
_______
___█___
____█__
__███__
_______
_______
`)

var glider2 = NewCellBoardFromSource(`_______
_______
_______
__█_█__
___██__
___█___
_______
`)

func TestGlider(t *testing.T) {
	// Sanity check
	if !ForwardStep(glider1).Equals(glider2) {
		t.Fatalf("Running the glider forward is broken")
	}

	t.Logf("input board:\n%s", glider2.DisplayString())

	// tracer := lastNTracer{size: 10}
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		t.Logf("Replaying trace:")
	// 		for i := 0; i < 10; i++ {
	// 			t.Log(tracer.get(i))
	// 		}
	// 		panic(r)
	// 	}
	// }()

	foundGlider1 := false

	solver := NewReverseSolver(glider2) // , WithTraceFn(tracer.trace))
	for {
		fin, sol := solver.YieldSolution()
		if fin {
			break
		}

		if sol.Equals(glider1) {
			foundGlider1 = true
		}

		stepped := ForwardStep(sol)
		if !stepped.Equals(glider2) {
			t.Errorf("Solution is not a valid predecessor of the input\nsolution\n%sstep(solution)\n%s", sol, stepped)
		}
	}

	if !foundGlider1 {
		t.Errorf("Never found the known predecessor of the glider")
	}
}

func TestRandomSmallBoards(t *testing.T) {
	for i := 0; i < 1; i++ {
		i := i

		t.Run(fmt.Sprintf("Case %d", i), func(t *testing.T) {
			r := rand.New(rand.NewSource(487489 * int64(i)))

			in := NewCellBoard(r.Intn(20), r.Intn(20))

			for i := 0; i < in.NumRows*in.NumCols; i++ {
				if r.Intn(2) == 0 {
					in.Cells[i] = CellDead
				} else {
					in.Cells[i] = CellAlive
				}
			}

			t.Logf("input board:\n%s", in.DisplayString())

			solver := NewReverseSolver(in)

			for {
				fin, sol := solver.YieldSolution()
				if fin {
					break
				}
				stepped := ForwardStep(sol)
				if !stepped.Equals(in) {
					t.Errorf("Solution is not a valid predecessor of the input\nsolution\n%sstep(solution)\n%s", sol, stepped)
				}
			}
		})
	}
}
