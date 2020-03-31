package life

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type CellState int8

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
	trailer, _ := utf8.DecodeLastRuneInString(pic)
	lines := strings.Split(pic, "\n")
	if trailer == '\n' {
		lines = lines[0 : len(lines)-1]
	}

	rows := 0
	cols := 0
	for _, line := range lines {
		first, _ := utf8.DecodeRuneInString(line)
		if first == '!' {
			continue
		}
		rows++
		cols = max(cols, utf8.RuneCountInString(line))
	}

	cells := make([]CellState, rows*cols)
	for i := 0; i < len(cells); i++ {
		cells[i] = CellDead
	}

	r := 0
	for _, line := range lines {
		first, _ := utf8.DecodeRuneInString(line)
		if first == '!' {
			continue
		}

		c := 0
		for _, ch := range line {
			if ch == '█' || ch == 'O' {
				cells[r*cols+c] = CellAlive
			} else if ch == '_' || ch == '.' {
				cells[r*cols+c] = CellDead
			}
			c++
		}

		r++
	}

	return CellBoard{
		NumRows: rows,
		NumCols: cols,
		Cells:   cells,
	}
}

func (b *CellBoard) At(r, c int) CellState {
	return b.Cells[r*b.NumCols+c]
}

func (b *CellBoard) Set(r, c int, cs CellState) {
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

func (b *CellBoard) Copy() CellBoard {
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

func RangeBoardDisplayString(mins IntBoard, maxs IntBoard) string {
	b := &strings.Builder{}

	b.WriteRune('┌')
	for c := 0; c < mins.NumCols; c++ {
		b.WriteString("─────")
	}
	b.WriteRune('┐')

	b.WriteRune('\n')

	for r := 0; r < mins.NumRows; r++ {
		b.WriteRune('│')
		for c := 0; c < mins.NumCols; c++ {
			b.WriteString(fmt.Sprintf("[%d,%d]", mins.At(r, c), maxs.At(r, c)))
		}
		b.WriteRune('│')

		b.WriteRune('\n')
	}

	b.WriteRune('└')
	for c := 0; c < mins.NumCols; c++ {
		b.WriteString("─────")
	}
	b.WriteRune('┘')

	b.WriteRune('\n')

	return b.String()
}

type backtrackMode int

const (
	backtrackModeDescending backtrackMode = iota
	backtrackModeAscending
)

type ReverseSolver struct {
	CurBoard CellBoard

	PreBoard            CellBoard
	PreLiveNeighborsMin IntBoard
	PreLiveNeighborsMax IntBoard
	PreLiveAllowed      IntBoard

	CurDepth int
	BTMode   backtrackMode

	StatsMaxDepth            int
	StatsNumStatesConsidered int64
	StatsReadMems            int64
	StatsWriteMems           int64
}

type ReverseSolverOption func(s *ReverseSolver)

func WithFoamSuppression() ReverseSolverOption {
	return func(s *ReverseSolver) {
		for r := 0; r < s.NumRows(); r++ {
			for c := 0; c < s.NumCols(); c++ {
				numLiveNeighbors := 0
				s.execForEachNeighbor(r, c, func(i, j int) {
					if s.CurBoard.At(i, j) == CellAlive {
						numLiveNeighbors++
					}
				})
				if numLiveNeighbors > 0 {
					s.PreLiveAllowed.Set(r, c, 1)
				} else {
					s.PreLiveAllowed.Set(r, c, 0)
				}
			}
		}
	}
}

func NewReverseSolver(curBoard CellBoard, opts ...ReverseSolverOption) *ReverseSolver {
	s := &ReverseSolver{
		CurBoard: curBoard.Copy(),

		PreBoard:            NewCellBoard(curBoard.NumRows, curBoard.NumCols),
		PreLiveNeighborsMin: NewIntBoard(curBoard.NumRows, curBoard.NumCols),
		PreLiveNeighborsMax: NewIntBoard(curBoard.NumRows, curBoard.NumCols),
		PreLiveAllowed:      NewIntBoard(curBoard.NumRows, curBoard.NumCols),

		CurDepth: 0,
		BTMode:   backtrackModeDescending,
	}

	for r := 0; r < s.NumRows(); r++ {
		for c := 0; c < s.NumCols(); c++ {
			numNeighbors := 0
			s.execForEachNeighbor(r, c, func(i, j int) {
				numNeighbors++
			})
			s.PreLiveNeighborsMax.Set(r, c, numNeighbors)
			s.PreLiveAllowed.Set(r, c, 1)
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
	s.StatsReadMems++
	switch s.CurBoard.At(r, c) {
	case CellUnknown:
		panic("CellUnknown is not allowed in CurBoard")
	case CellDead:
		s.StatsReadMems++
		switch s.PreBoard.At(r, c) {
		case CellUnknown:
			// Do nothing.  All live neighbor values are possibly valid if
			// we don't have an assigned preboard value.
			return true
		case CellDead:
			s.StatsReadMems += 2
			return !(s.PreLiveNeighborsMin.At(r, c) == 3 && s.PreLiveNeighborsMax.At(r, c) == 3)
		case CellAlive:
			s.StatsReadMems += 2
			return !(2 <= s.PreLiveNeighborsMin.At(r, c) && s.PreLiveNeighborsMin.At(r, c) <= 3 &&
				2 <= s.PreLiveNeighborsMax.At(r, c) && s.PreLiveNeighborsMax.At(r, c) <= 3)
		}
	case CellAlive:
		s.StatsReadMems++
		switch s.PreBoard.At(r, c) {
		case CellUnknown:
			// Do nothing.  All live neighbor values are possibly valid if
			// we don't have an assigned preboard value.
			return true
		case CellDead:
			s.StatsReadMems += 2
			return s.PreLiveNeighborsMin.At(r, c) <= 3 && 3 <= s.PreLiveNeighborsMax.At(r, c)
		case CellAlive:
			s.StatsReadMems += 2
			return (s.PreLiveNeighborsMin.At(r, c) <= 2 && 2 <= s.PreLiveNeighborsMax.At(r, c)) ||
				(s.PreLiveNeighborsMin.At(r, c) <= 3 && 3 <= s.PreLiveNeighborsMax.At(r, c))
		}
	}

	panic("unreachable")
}

func (s *ReverseSolver) checkConsistentAround(r, c int) bool {
	if s.PreBoard.At(r, c) == CellAlive && s.PreLiveAllowed.At(r, c) != 1 {
		return false
	}
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
	if s.PreLiveNeighborsMin.At(r, c) < 0 {
		panic(fmt.Sprintf("live neighbor min (%d, %d) < 0", r, c))
	}
	if s.PreLiveNeighborsMin.At(r, c) > 8 {
		panic(fmt.Sprintf("live neighbor min (%d, %d) > 8", r, c))
	}
	if s.PreLiveNeighborsMax.At(r, c) < 0 {
		panic(fmt.Sprintf("live neighbor max (%d, %d) < 0", r, c))
	}
	if s.PreLiveNeighborsMax.At(r, c) > 8 {
		panic(fmt.Sprintf("live neighbor max (%d, %d) > 8", r, c))
	}
	if s.PreLiveNeighborsMin.At(r, c) > s.PreLiveNeighborsMax.At(r, c) {
		panic(fmt.Sprintf("live neighbor (%d, %d) max(%d) < min(%d)", r, c, s.PreLiveNeighborsMax.At(r, c), s.PreLiveNeighborsMin.At(r, c)))
	}
}

func (s *ReverseSolver) setPreBoard(r, c int, cs CellState) {
	s.StatsReadMems++
	if s.PreBoard.At(r, c) == CellUnknown && cs == CellUnknown {
		panic("invalid preboard state transition unknown->unknown")
	} else if s.PreBoard.At(r, c) == CellUnknown && cs == CellDead {
		// Decrement the live neighbor maximum for neighbors of this cell.
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.StatsReadMems++
			s.StatsWriteMems++
			s.PreLiveNeighborsMax.Set(i, j, s.PreLiveNeighborsMax.At(i, j)-1)
			s.preLiveNeighborsInvariant(i, j)
		})
	} else if s.PreBoard.At(r, c) == CellUnknown && cs == CellAlive {
		panic("invalid preboard state transition unknown->alive")
	} else if s.PreBoard.At(r, c) == CellDead && cs == CellUnknown {
		panic("invalid preboard state transition dead->unknown")
	} else if s.PreBoard.At(r, c) == CellDead && cs == CellDead {
		panic("invalid preboard state transition dead->dead")
	} else if s.PreBoard.At(r, c) == CellDead && cs == CellAlive {
		// Increment the live neighbor maximum for neighbors of this cell
		// (transition away from dead)
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.StatsReadMems++
			s.StatsWriteMems++
			s.PreLiveNeighborsMax.Set(i, j, s.PreLiveNeighborsMax.At(i, j)+1)
			s.preLiveNeighborsInvariant(i, j)
		})

		// Increment the live neighbor minimum for neighbors of this cell
		// (transition into alive)
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.StatsReadMems++
			s.StatsWriteMems++
			s.PreLiveNeighborsMin.Set(i, j, s.PreLiveNeighborsMin.At(i, j)+1)
			s.preLiveNeighborsInvariant(i, j)
		})
	} else if s.PreBoard.At(r, c) == CellAlive && cs == CellUnknown {
		// Decrement the live neighbor minimum for neighbors of this cell
		// (transition away from alive)
		s.execForEachNeighbor(r, c, func(i, j int) {
			s.StatsReadMems++
			s.StatsWriteMems++
			s.PreLiveNeighborsMin.Set(i, j, s.PreLiveNeighborsMin.At(i, j)-1)
			s.preLiveNeighborsInvariant(i, j)
		})
	} else if s.PreBoard.At(r, c) == CellAlive && cs == CellDead {
		panic("invalid preboard state transition alive->dead")
	} else if s.PreBoard.At(r, c) == CellAlive && cs == CellAlive {
		panic("invalid preboard state transition alive->alive")
	}

	s.StatsWriteMems++
	s.PreBoard.Set(r, c, cs)
}

type YieldStatus int

const (
	YieldFinished YieldStatus = iota
	YieldOutOfGas
	YieldSolution
)

func (s *ReverseSolver) YieldSolution() (state YieldStatus, board CellBoard) {
	for {
		status, sol := s.YieldSolutionBounded(1000000000)
		if status == YieldFinished || status == YieldSolution {
			return status, sol
		}
	}
}

func (s *ReverseSolver) YieldSolutionBounded(gas int64) (state YieldStatus, board CellBoard) {
	for {
		gas--
		if gas < 0 {
			return YieldOutOfGas, CellBoard{}
		}

		// TODO(ahmedtd): Try other exploration orders that might have more focus.
		r := s.CurDepth / s.NumCols()
		c := s.CurDepth % s.NumCols()

		switch s.BTMode {
		case backtrackModeDescending:
			s.StatsReadMems++
			switch s.PreBoard.At(r, c) {
			case CellUnknown:
				s.StatsNumStatesConsidered++
				s.setPreBoard(r, c, CellDead)
			case CellDead:
				if s.checkConsistentAround(r, c) {
					s.CurDepth += 1
					if s.CurDepth == s.NumRows()*s.NumCols() {
						s.BTMode = backtrackModeAscending
						s.CurDepth -= 1
						return YieldSolution, s.PreBoard
					}
				} else {
					s.StatsNumStatesConsidered++
					s.setPreBoard(r, c, CellAlive)
				}
			case CellAlive:
				if s.checkConsistentAround(r, c) {
					s.CurDepth += 1
					if s.CurDepth > s.StatsMaxDepth {
						s.StatsMaxDepth = s.CurDepth
					}
					if s.CurDepth == s.NumRows()*s.NumCols() {
						s.BTMode = backtrackModeAscending
						s.CurDepth -= 1
						return YieldSolution, s.PreBoard
					}
				} else {
					s.setPreBoard(r, c, CellUnknown)
					s.BTMode = backtrackModeAscending
					if s.CurDepth == 0 {
						return YieldFinished, CellBoard{}
					}
					s.CurDepth -= 1
				}
			}
		case backtrackModeAscending:
			s.StatsReadMems++
			switch s.PreBoard.At(r, c) {
			case CellUnknown:
				panic("invariant violation: not allowed to ascend onto a cell with unassigned state")
			case CellDead:
				s.StatsNumStatesConsidered++
				s.setPreBoard(r, c, CellAlive)
				s.BTMode = backtrackModeDescending
			case CellAlive:
				s.setPreBoard(r, c, CellUnknown)
				if s.CurDepth == 0 {
					return YieldFinished, CellBoard{}
				}
				s.CurDepth -= 1
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
