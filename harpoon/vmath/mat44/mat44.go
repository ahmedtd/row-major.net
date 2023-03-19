package mat44

import "math"

type T [16]float64

func rowEchelonInplace(m, a *T) {
	for k := 0; k < 4; k++ {
		// Select the row below row k with the best pivot.
		maxRow := k
		for i := k; i < 4; i++ {
			if math.Abs(m[i*4+k]) > math.Abs(m[maxRow*4+k]) {
				maxRow = i
			}
		}

		// Swap selected row to current row.
		for i := 0; i < 4; i++ {
			m[k*4+i], m[maxRow*4+i] = m[maxRow*4+i], m[k*4+i]
			a[k*4+i], a[maxRow*4+i] = a[maxRow*4+i], a[k*4+i]
		}

		// Now the pivot element is at m[k, k].
		pivot := m[k*4+k]
		for r := k + 1; r < 4; r++ {
			scale := m[r*4+k] / pivot
			for c := k + 1; c < 4; c++ {
				m[r*4+c] -= m[k*4+c] * scale
			}
			for c := 0; c < 4; c++ {
				a[r*4+c] -= a[k*4+c] * scale
			}
			m[r*4+k] = 0.0
		}
	}
}

func backsubInplace(m, a *T) {
	for k := 4 - 1; k > 0; k-- {
		// m[k,k] is the pivot

		// Nullify all entries above the pivot element.
		for r := 0; r < k; r++ {
			scale := m[r*4+k] / m[k*4+k]

			m[r*4+k] = 0
			for c := k + 1; c < 4; c++ {
				m[r*4+c] -= m[k*4+c] * scale
			}

			// Mirror the action in the augmented matrix.
			for c := 0; c < 4; c++ {
				a[r*4+c] -= a[k*4+c] * scale
			}
		}
	}

	// Now we simply need to divide each row by its pivot.
	for k := 0; k < 4; k++ {
		for c := k + 1; c < 4; c++ {
			m[k*4+c] /= m[k*4+k]
		}
		for c := 0; c < 4; c++ {
			a[k*4+c] /= m[k*4+k]
		}
		m[k*4+k] = 1
	}
}

func Mat44SolveInplace(m, a *T) {
	rowEchelonInplace(m, a)
	backsubInplace(m, a)
}

func Mat44Inverse(m T) T {
	a := T{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	Mat44SolveInplace(&m, &a)
	return a
}
