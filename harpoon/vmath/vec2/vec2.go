package vec2

import "math"

type T [2]float64

func (v T) Norm() float64 {
	return math.Sqrt(v[0]*v[0] + v[1]*v[1])
}
