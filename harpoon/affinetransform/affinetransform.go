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
		Linear: mat33.T{1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0},
		Offset: vec3.T{0.0, 0.0, 0.0},
	}
}

func Scale(s float64) AffineTransform {
	return AffineTransform{
		Linear: mat33.T{s, 0.0, 0.0, 0.0, s, 0.0, 0.0, 0.0, s},
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
	mat := mat44.T{
		t.Linear[0], t.Linear[1], t.Linear[2], t.Offset[0],
		t.Linear[3], t.Linear[4], t.Linear[5], t.Offset[1],
		t.Linear[6], t.Linear[7], t.Linear[8], t.Offset[2],
		0, 0, 0, 1,
	}

	inv := mat44.Mat44Inverse(mat)

	return AffineTransform{
		Linear: mat33.T{
			inv[0], inv[1], inv[2],
			inv[4], inv[5], inv[6],
			inv[8], inv[9], inv[10],
		},
		Offset: vec3.T{inv[3], inv[7], inv[11]},
	}
}

func (t AffineTransform) NormalTransformMat() mat33.T {
	return mat33.Transpose(mat33.Inverse(t.Linear))
}

func TransformPoint(a AffineTransform, b vec3.T) vec3.T {
	return vec3.AddVV(mat33.MulMV(a.Linear, b), a.Offset)
}
