package mat44

import "math"

type T struct {
	Elts [16]float64
}

func rowEchelonInplace(m, a *T) {
	for k := 0; k < 4; k++ {
		// Select the row below row k with the best pivot.
		maxRow := k
		for i := k; i < 4; i++ {
			if math.Abs(m.Elts[i*4+k]) > math.Abs(m.Elts[maxRow*4+k]) {
				maxRow = i
			}
		}

		// Swap selected row to current row.
		for i := 0; i < 4; i++ {
			m.Elts[k*4+i], m.Elts[maxRow*4+i] = m.Elts[maxRow*4+i], m.Elts[k*4+i]
			a.Elts[k*4+i], a.Elts[maxRow*4+i] = a.Elts[maxRow*4+i], a.Elts[k*4+i]
		}

		// Now the pivot element is at m[k, k].
		pivot := m.Elts[k*4+k]
		for r := k + 1; r < 4; r++ {
			scale := m.Elts[r*4+k] / pivot
			for c := k + 1; c < 4; c++ {
				m.Elts[r*4+c] -= m.Elts[k*4+c] * scale
			}
			for c := 0; c < 4; c++ {
				a.Elts[r*4+c] -= a.Elts[k*4+c] * scale
			}
			m.Elts[r*4+k] = 0.0
		}
	}
}

func backsubInplace(m, a *T) {
	for k := 4 - 1; k > 0; k-- {
		// m[k,k] is the pivot

		// Nullify all entries above the pivot element.
		for r := 0; r < k; r++ {
			scale := m.Elts[r*4+k] / m.Elts[k*4+k]

			m.Elts[r*4+k] = 0
			for c := k + 1; c < 4; c++ {
				m.Elts[r*4+c] -= m.Elts[k*4+c] * scale
			}

			// Mirror the action in the augmented matrix.
			for c := 0; c < 4; c++ {
				a.Elts[r*4+c] -= a.Elts[k*4+c] * scale
			}
		}
	}

	// Now we simply need to divide each row by its pivot.
	for k := 0; k < 4; k++ {
		for c := k + 1; c < 4; c++ {
			m.Elts[k*4+c] /= m.Elts[k*4+k]
		}
		for c := 0; c < 4; c++ {
			a.Elts[k*4+c] /= m.Elts[k*4+k]
		}
		m.Elts[k*4+k] = 1
	}
}

func Mat44SolveInplace(m, a *T) {
	rowEchelonInplace(m, a)
	backsubInplace(m, a)
}

func Mat44Inverse(m T) T {
	a := T{Elts: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	Mat44SolveInplace(&m, &a)
	return a
}
