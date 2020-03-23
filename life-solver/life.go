package life

import (
	"fmt"
	"strings"
)

type CellState int

const (
	CellUnknown CellState = iota
	CellDead
	CellAlive
)

type CellBoard struct {
	NumRows int
	NumCols int
	Cells   []CellState
}

func NewCellBoard(numRows, numCols int) CellBoard {
	return CellBoard{
		NumRows: numRows,
		NumCols: numCols,
		Cells:   make([]CellState, numRows*numCols),
	}
}

func NewCellBoardFromSource(pic string) CellBoard {
	cells := make([]CellState, 0)
	nrows := 0
	ncols := 0

	for _, r := range pic {
		if r == '█' {
			if nrows == 0 {
				ncols++
			}
			cells = append(cells, CellAlive)
		} else if r == '_' {
			if nrows == 0 {
				ncols++
			}
			cells = append(cells, CellDead)
		} else if r == '\n' {
			nrows++
		} else {
		}
	}

	return CellBoard{
		NumRows: nrows,
		NumCols: ncols,
		Cells:   cells,
	}
}

func (b CellBoard) At(r, c int) CellState {
	return b.Cells[r*b.NumCols+c]
}

func (b CellBoard) Set(r, c int, cs CellState) {
	b.Cells[r*b.NumCols+c] = cs
}

func (b CellBoard) Equals(o CellBoard) bool {
	if b.NumRows != o.NumRows || b.NumCols != o.NumCols {
		return false
	}

	for r := 0; r < b.NumRows; r++ {
		for c := 0; c < b.NumCols; c++ {
			if b.At(r, c) != o.At(r, c) {
				return false
			}
		}
	}

	return true
}

func (s CellBoard) DisplayString() string {
	b := strings.Builder{}

	b.WriteRune('┌')
	for c := 0; c < s.NumCols; c++ {
		b.WriteRune('─')
	}
	b.WriteRune('┐')
	b.WriteRune('\n')

	for r := 0; r < s.NumRows; r++ {
		b.WriteRune('│')
		for c := 0; c < s.NumCols; c++ {
			switch s.At(r, c) {
			case CellUnknown:
				b.WriteRune('?')
			case CellDead:
				b.WriteRune(' ')
			case CellAlive:
				b.WriteRune('█')
			}
		}
		b.WriteRune('│')
		b.WriteRune('\n')
	}

	b.WriteRune('└')
	for c := 0; c < s.NumCols; c++ {
		b.WriteRune('─')
	}
	b.WriteRune('┘')
	b.WriteRune('\n')

	return b.String()
}

func (b CellBoard) Copy() CellBoard {
	copy := CellBoard{
		NumRows: b.NumRows,
		NumCols: b.NumCols,
		Cells:   make([]CellState, len(b.Cells)),
	}

	for i := 0; i < len(b.Cells); i++ {
		copy.Cells[i] = b.Cells[i]
	}

	return copy
}

type IntBoard struct {
	NumRows int
	NumCols int
	Cells   []int
}

func NewIntBoard(numRows, numCols int) IntBoard {
	return IntBoard{
		NumRows: numRows,
		NumCols: numCols,
		Cells:   make([]int, numRows*numCols),
	}
}

func (b *IntBoard) At(r, c int) int {
	return b.Cells[r*b.NumCols+c]
}

func (b *IntBoard) Set(r, c, cs int) {
	b.Cells[r*b.NumCols+c] = cs
}

type backtrackMode int

const (
	backtrackModeDescending backtrackMode = iota
	backtrackModeAscending
)

type traceFunction func(msg string)

type ReverseSolver struct {
	CurBoard CellBoard

	preBoard            CellBoard
	preLiveNeighborsMin IntBoard
	preLiveNeighborsMax IntBoard

	curDepth int
	btMode   backtrackMode

	traceFn traceFunction
}

type ReverseSolverOption func(s *ReverseSolver)

func WithTraceFn(traceFn traceFunction) ReverseSolverOption {
	return func(s *ReverseSolver) {
		s.traceFn = traceFn
	}
}

func NewReverseSolver(curBoard CellBoard, opts ...ReverseSolverOption) *ReverseSolver {
	s := &ReverseSolver{

		CurBoard: curBoard.Copy(),

		preBoard:            NewCellBoard(curBoard.NumRows, curBoard.NumCols),
		preLiveNeighborsMin: NewIntBoard(curBoard.NumRows, curBoard.NumCols),
		preLiveNeighborsMax: NewIntBoard(curBoard.NumRows, curBoard.NumCols),

		curDepth: 0,
		btMode:   backtrackModeDescending,
	}

	for r := 0; r < s.NumRows(); r++ {
		for c := 0; c < s.NumCols(); c++ {
			numNeighbors := 0
			s.execForEachNeighbor(r, c, func(i, j int) {
				numNeighbors++
			})
			s.preLiveNeighborsMax.Set(r, c, numNeighbors)
		}
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *ReverseSolver) NumRows() int {
	return s.CurBoard.NumRows
}

func (s *ReverseSolver) NumCols() int {
	return s.CurBoard.NumCols
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func (s *ReverseSolver) execForEachNeighbor(r, c int, f func(i, j int)) {
	for i := max(0, r-1); i <= min(r+1, s.NumRows()-1); i++ {
		for j := max(0, c-1); j <= min(c+1, s.NumCols()-1); j++ {
			if !(i == r && j == c) {
				f(i, j)
			}
		}
	}
}

func (s *ReverseSolver) checkConsistentAt(r, c int) bool {
	switch s.CurBoard.At(r, c) {
	case CellUnknown:
		panic("CellUnknown is not allowed in CurBoard")
	case CellDead:
		switch s.preBoard.At(r, c) {
		case CellUnknown:
			// Do nothing.  All live neighbor values are possibly valid if
			// we don't have an assigned preboard value.
			return true
		case CellDead:
			return !(s.preLiveNeighborsMin.At(r, c) == 3 && s.preLiveNeighborsMax.At(r, c) == 3)
		case CellAlive:
			return !(2 <= s.preLiveNeighborsMin.At(r, c) && s.preLiveNeighborsMin.At(r, c) <= 3 &&
				2 <= s.preLiveNeighborsMax.At(r, c) && s.preLiveNeighborsMax.At(r, c) <= 3)
		}
	case CellAlive:
		switch s.preBoard.At(r, c) {
		case CellUnknown:
			// Do nothing.  All live neighbor values are possibly valid if
			// we don't have an assigned preboard value.
			return true
		case CellDead:
			return s.preLiveNeighborsMin.At(r, c) <= 3 && 3 <= s.preLiveNeighborsMax.At(r, c)
		case CellAlive:
			return (s.preLiveNeighborsMin.At(r, c) <= 2 && 2 <= s.preLiveNeighborsMax.At(r, c)) ||
				(s.preLiveNeighborsMin.At(r, c) <= 3 && 3 <= s.preLiveNeighborsMax.At(r, c))
		}
	}

	panic("unreachable")
}

func (s *ReverseSolver) checkConsistentAround(r, c int) bool {
	for i := max(0, r-1); i <= min(r+1, s.NumRows()-1); i++ {
		for j := max(0, c-1); j <= min(c+1, s.NumCols()-1); j++ {
			if !s.checkConsistentAt(i, j) {
				return false
			}
		}
	}
	return true
}

func (s *ReverseSolver) preLiveNeighborsInvariant(r, c int) {
	if s.preLiveNeighborsMin.At(r, c) < 0 {
		panic(fmt.Sprintf("live neighbor min (%d, %d) < 0", r, c))
	}
	if s.preLiveNeighborsMin.At(r, c) > 8 {
		panic(fmt.Sprintf("live neighbor min (%d, %d) > 8", r, c))
	}
	if s.preLiveNeighborsMax.At(r, c) < 0 {
		panic(fmt.Sprintf("live neighbor max (%d, %d) < 0", r, c))
	}
	if s.preLiveNeighborsMax.At(r, c) > 8 {
		panic(fmt.Sprintf("live neighbor max (%d, %d) > 8", r, c))
	}
	if s.preLiveNeighborsMin.At(r, c) > s.preLiveNeighborsMax.At(r, c) {
		panic(fmt.Sprintf("live neighbor (%d, %d) max(%d) < min(%d)", r, c, s.preLiveNeighborsMax.At(r, c), s.preLiveNeighborsMin.At(r, c)))
	}
}

func (s *ReverseSolver) setPreBoard(r, c int, cs CellState) {
	if s.preBoard.At(r, c) == CellUnknown && cs == CellUnknown {
		panic("invalid preboard state transition unknown->unknown")
	} else if s.preBoard.At(r, c) == CellUnknown && cs == CellDead {
		// Decrement the live neighbor maximum for neighbors of this cell.
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.preLiveNeighborsMax.Set(i, j, s.preLiveNeighborsMax.At(i, j)-1)
			s.preLiveNeighborsInvariant(i, j)
		})
	} else if s.preBoard.At(r, c) == CellUnknown && cs == CellAlive {
		panic("invalid preboard state transition unknown->alive")
	} else if s.preBoard.At(r, c) == CellDead && cs == CellUnknown {
		panic("invalid preboard state transition dead->unknown")
	} else if s.preBoard.At(r, c) == CellDead && cs == CellDead {
		panic("invalid preboard state transition dead->dead")
	} else if s.preBoard.At(r, c) == CellDead && cs == CellAlive {
		// Increment the live neighbor maximum for neighbors of this cell
		// (transition away from dead)
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.preLiveNeighborsMax.Set(i, j, s.preLiveNeighborsMax.At(i, j)+1)
			s.preLiveNeighborsInvariant(i, j)
		})

		// Increment the live neighbor minimum for neighbors of this cell
		// (transition into alive)
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.preLiveNeighborsMin.Set(i, j, s.preLiveNeighborsMin.At(i, j)+1)
			s.preLiveNeighborsInvariant(i, j)
		})
	} else if s.preBoard.At(r, c) == CellAlive && cs == CellUnknown {
		// Decrement the live neighbor minimum for neighbors of this cell
		// (transition away from alive)
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.preLiveNeighborsMin.Set(i, j, s.preLiveNeighborsMin.At(i, j)-1)
			s.preLiveNeighborsInvariant(i, j)
		})
	} else if s.preBoard.At(r, c) == CellAlive && cs == CellDead {
		panic("invalid preboard state transition alive->dead")
	} else if s.preBoard.At(r, c) == CellAlive && cs == CellAlive {
		panic("invalid preboard state transition alive->alive")
	}

	s.preBoard.Set(r, c, cs)
}

func (s *ReverseSolver) dumpBoardStates() string {
	b := &strings.Builder{}

	b.WriteRune('┌')
	for c := 0; c < s.NumCols(); c++ {
		b.WriteRune('─')
	}
	b.WriteRune('┐')

	b.WriteRune(' ')

	b.WriteRune('┌')
	for c := 0; c < s.NumCols(); c++ {
		b.WriteRune('─')
	}
	b.WriteRune('┐')

	b.WriteRune(' ')

	b.WriteRune('┌')
	for c := 0; c < s.NumCols(); c++ {
		b.WriteString("─────")
	}
	b.WriteRune('┐')

	b.WriteRune('\n')

	for r := 0; r < s.NumRows(); r++ {
		b.WriteRune('│')
		for c := 0; c < s.NumCols(); c++ {
			switch s.CurBoard.At(r, c) {
			case CellUnknown:
				b.WriteRune(' ')
			case CellDead:
				b.WriteRune('0')
			case CellAlive:
				b.WriteRune('1')
			}
		}
		b.WriteRune('│')

		b.WriteRune(' ')

		b.WriteRune('│')
		for c := 0; c < s.NumCols(); c++ {
			switch s.preBoard.At(r, c) {
			case CellUnknown:
				b.WriteRune(' ')
			case CellDead:
				b.WriteRune('0')
			case CellAlive:
				b.WriteRune('1')
			}
		}
		b.WriteRune('│')

		b.WriteRune(' ')

		b.WriteRune('│')
		for c := 0; c < s.NumCols(); c++ {
			b.WriteString(fmt.Sprintf("[%d,%d]", s.preLiveNeighborsMin.At(r, c), s.preLiveNeighborsMax.At(r, c)))
		}
		b.WriteRune('│')

		b.WriteRune('\n')
	}

	b.WriteRune('└')
	for c := 0; c < s.NumCols(); c++ {
		b.WriteRune('─')
	}
	b.WriteRune('┘')

	b.WriteRune(' ')

	b.WriteRune('└')
	for c := 0; c < s.NumCols(); c++ {
		b.WriteRune('─')
	}
	b.WriteRune('┘')

	b.WriteRune(' ')

	b.WriteRune('└')
	for c := 0; c < s.NumCols(); c++ {
		b.WriteString("─────")
	}
	b.WriteRune('┘')

	b.WriteRune('\n')

	return b.String()
}

func (s *ReverseSolver) YieldSolution() (finished bool, board CellBoard) {
	for {
		// TODO(ahmedtd): Try other exploration orders that might have more focus.
		r := s.curDepth / s.NumCols()
		c := s.curDepth % s.NumCols()

		if s.traceFn != nil {
			if s.btMode == backtrackModeDescending {
				s.traceFn(fmt.Sprintf("descending, looking at %d (%d, %d)", s.curDepth, r, c))
			} else if s.btMode == backtrackModeAscending {
				s.traceFn(fmt.Sprintf("ascending, looking at %d (%d, %d)", s.curDepth, r, c))
			}
			s.traceFn(fmt.Sprintf("\n%s", s.dumpBoardStates()))
		}

		switch s.btMode {
		case backtrackModeDescending:
			switch s.preBoard.At(r, c) {
			case CellUnknown:
				s.setPreBoard(r, c, CellDead)
				if s.traceFn != nil {
					s.traceFn(fmt.Sprintf("set (%d, %d) = CellDead", r, c))
				}
			case CellDead:
				if s.checkConsistentAround(r, c) {
					if s.traceFn != nil {
						s.traceFn(fmt.Sprintf("choice is consistent"))
					}
					s.curDepth += 1
					if s.curDepth == s.NumRows()*s.NumCols() {
						if s.traceFn != nil {
							s.traceFn(fmt.Sprintf("yielding solution, switching to ascending"))
						}
						s.btMode = backtrackModeAscending
						s.curDepth -= 1
						return false, s.preBoard
					}
				} else {
					s.setPreBoard(r, c, CellAlive)
					if s.traceFn != nil {
						s.traceFn(fmt.Sprintf("set (%d, %d) = CellDead", r, c))
					}
				}
			case CellAlive:
				if s.checkConsistentAround(r, c) {
					if s.traceFn != nil {
						s.traceFn(fmt.Sprintf("choice is consistent"))
					}
					s.curDepth += 1
					if s.curDepth == s.NumRows()*s.NumCols() {
						if s.traceFn != nil {
							s.traceFn(fmt.Sprintf("yielding solution, switching to ascending"))
						}
						s.btMode = backtrackModeAscending
						s.curDepth -= 1
						return false, s.preBoard
					}
				} else {
					s.setPreBoard(r, c, CellUnknown)
					s.btMode = backtrackModeAscending
					if s.curDepth == 0 {
						return true, CellBoard{}
					}
					s.curDepth -= 1
				}
			}
		case backtrackModeAscending:
			switch s.preBoard.At(r, c) {
			case CellUnknown:
				panic("invariant violation: not allowed to ascend onto a cell with unassigned state")
			case CellDead:
				if s.traceFn != nil {
					s.traceFn(fmt.Sprintf("set (%d, %d) = CellAlive", r, c))
				}
				s.setPreBoard(r, c, CellAlive)
				s.btMode = backtrackModeDescending
				if s.traceFn != nil {
					s.traceFn(fmt.Sprintf("switching to descending"))
				}
			case CellAlive:
				s.setPreBoard(r, c, CellUnknown)
				if s.curDepth == 0 {
					if s.traceFn != nil {
						s.traceFn("no more solutions to yield")
					}
					return true, CellBoard{}
				}
				s.curDepth -= 1
			}
		}
	}
}

type Forward struct {
	CurBoard CellBoard
	newBoard CellBoard
}

func NewForward(board CellBoard) *Forward {
	return &Forward{
		CurBoard: board.Copy(),
		newBoard: NewCellBoard(board.NumRows, board.NumCols),
	}
}

func (f *Forward) NumRows() int {
	return f.CurBoard.NumRows
}

func (f *Forward) NumCols() int {
	return f.CurBoard.NumCols
}

func (f *Forward) newStateAt(r, c int) CellState {
	numLiveNeighbors := 0
	for i := max(0, r-1); i <= min(r+1, f.NumRows()-1); i++ {
		for j := max(0, c-1); j <= min(c+1, f.NumCols()-1); j++ {
			if !(i == r && j == c) {
				switch f.CurBoard.At(i, j) {
				case CellUnknown:
					panic("CellUnknown is not permitted in forward computation")
				case CellDead:
					// Do nothing
				case CellAlive:
					numLiveNeighbors++
				}
			}
		}
	}

	switch f.CurBoard.At(r, c) {
	case CellUnknown:
		panic("CellUnknown is not permitted in forward computation")
	case CellDead:
		if numLiveNeighbors == 3 {
			return CellAlive
		} else {
			return CellDead
		}
	case CellAlive:
		if numLiveNeighbors == 2 || numLiveNeighbors == 3 {
			return CellAlive
		} else {
			return CellDead
		}
	}

	panic("unreachable")
}

func (f *Forward) Step() CellBoard {
	for r := 0; r < f.NumRows(); r++ {
		for c := 0; c < f.NumCols(); c++ {
			f.newBoard.Set(r, c, f.newStateAt(r, c))
		}
	}

	temp := f.CurBoard
	f.CurBoard = f.newBoard
	f.newBoard = temp

	return f.CurBoard
}

func ForwardStep(board CellBoard) CellBoard {
	f := NewForward(board)
	return f.Step()
}
