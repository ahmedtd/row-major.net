package mat33

import (
	"math"

	"row-major/harpoon/vmath/vec3"
)

type T struct {
	Elts [9]float64
}

func MulMM(a, b T) T {
	result := T{}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				result.Elts[i*3+j] += a.Elts[i*3+k] * b.Elts[k*3+j]
			}
		}
	}
	return result
}

func MulMV(a T, b vec3.T) vec3.T {
	return vec3.T{
		a.Elts[0]*b[0] + a.Elts[1]*b[1] + a.Elts[2]*b[2],
		a.Elts[3]*b[0] + a.Elts[4]*b[1] + a.Elts[5]*b[2],
		a.Elts[6]*b[0] + a.Elts[7]*b[1] + a.Elts[8]*b[2],
	}
}

func Transpose(m T) T {
	transpose := T{}
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			transpose.Elts[c*3+r] = m.Elts[r*3+c]
		}
	}
	return transpose
}

func rowEchelonInplace(m, a *T) {
	for k := 0; k < 3; k++ {
		// Select the row below row k with the best pivot.
		maxRow := k
		for i := k; i < 3; i++ {
			if math.Abs(m.Elts[i*3+k]) > math.Abs(m.Elts[maxRow*3+k]) {
				maxRow = i
			}
		}

		// Swap selected row to current row.
		for i := 0; i < 3; i++ {
			m.Elts[k*3+i], m.Elts[maxRow*3+i] = m.Elts[maxRow*3+i], m.Elts[k*3+i]
			a.Elts[k*3+i], a.Elts[maxRow*3+i] = a.Elts[maxRow*3+i], a.Elts[k*3+i]
		}

		// Now the pivot element is at m[k, k].
		pivot := m.Elts[k*3+k]
		for r := k + 1; r < 3; r++ {
			scale := m.Elts[r*3+k] / pivot
			for c := k + 1; c < 3; c++ {
				m.Elts[r*3+c] -= m.Elts[k*3+c] * scale
			}
			for c := 0; c < 3; c++ {
				a.Elts[r*3+c] -= a.Elts[k*3+c] * scale
			}
			m.Elts[r*3+k] = 0.0
		}
	}

}

func backsubInplace(m, a *T) {
	for k := 3 - 1; k > 0; k-- {
		// m[k,k] is the pivot

		// Nullify all entries above the pivot element.
		for r := 0; r < k; r++ {
			scale := m.Elts[r*3+k] / m.Elts[k*3+k]

			m.Elts[r*3+k] = 0
			for c := k + 1; c < 3; c++ {
				m.Elts[r*3+c] -= m.Elts[k*3+c] * scale
			}

			// Mirror the action in the augmented matrix.
			for c := 0; c < 3; c++ {
				a.Elts[r*3+c] -= a.Elts[k*3+c] * scale
			}
		}
	}

	// Now we simply need to divide each row by its pivot.
	for k := 0; k < 3; k++ {
		for c := k + 1; c < 3; c++ {
			m.Elts[k*3+c] /= m.Elts[k*3+k]
		}
		for c := 0; c < 3; c++ {
			a.Elts[k*3+c] /= m.Elts[k*3+k]
		}
		m.Elts[k*3+k] = 1
	}
}

func SolveInplace(m, a *T) {
	rowEchelonInplace(m, a)
	backsubInplace(m, a)
}

func Inverse(m T) T {
	a := T{[9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}}
	SolveInplace(&m, &a)
	return a
}
