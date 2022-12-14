package affinetransform

import (
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/mat44"
	"row-major/harpoon/vmath/vec3"
)

type AffineTransform struct {
	Linear mat33.T
	Offset vec3.T
}

func Identity() AffineTransform {
	return AffineTransform{
		Linear: mat33.T{
			[9]float64{1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0},
		},
		Offset: vec3.T{0.0, 0.0, 0.0},
	}
}

func Scale(s float64) AffineTransform {
	return AffineTransform{
		Linear: mat33.T{
			[9]float64{s, 0.0, 0.0, 0.0, s, 0.0, 0.0, 0.0, s},
		},
		Offset: vec3.T{0.0, 0.0, 0.0},
	}
}

func Translate(x vec3.T) AffineTransform {
	result := Identity()
	result.Offset = x
	return result
}

func Compose(a, b AffineTransform) AffineTransform {
	return AffineTransform{
		Linear: mat33.MulMM(a.Linear, b.Linear),
		Offset: vec3.AddVV(a.Offset, mat33.MulMV(a.Linear, b.Offset)),
	}
}

func (t AffineTransform) Invert() AffineTransform {
	mat := mat44.T{[16]float64{
		t.Linear.Elts[0], t.Linear.Elts[1], t.Linear.Elts[2], t.Offset[0],
		t.Linear.Elts[3], t.Linear.Elts[4], t.Linear.Elts[5], t.Offset[1],
		t.Linear.Elts[6], t.Linear.Elts[7], t.Linear.Elts[8], t.Offset[2],
		0, 0, 0, 1,
	}}

	inv := mat44.Mat44Inverse(mat)

	return AffineTransform{
		Linear: mat33.T{[9]float64{
			inv.Elts[0], inv.Elts[1], inv.Elts[2],
			inv.Elts[4], inv.Elts[5], inv.Elts[6],
			inv.Elts[8], inv.Elts[9], inv.Elts[10],
		}},
		Offset: vec3.T{inv.Elts[3], inv.Elts[7], inv.Elts[11]},
	}
}

func (t AffineTransform) NormalTransformMat() mat33.T {
	return mat33.Transpose(mat33.Inverse(t.Linear))
}

func TransformPoint(a AffineTransform, b vec3.T) vec3.T {
	return vec3.AddVV(mat33.MulMV(a.Linear, b), a.Offset)
}
