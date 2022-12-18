package contact

import (
	"math"
	"row-major/harpoon/affinetransform"
	"row-major/harpoon/ray"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec2"
	"row-major/harpoon/vmath/vec3"
)

type Contact struct {
	T    float64
	R    ray.Ray
	P    vec3.T
	N    vec3.T
	Mtl2 vec2.T
	Mtl3 vec3.T
}

func ContactNaN() Contact {
	return Contact{
		T: math.NaN(),
	}
}

// Transform applies an affine transform to a contact.
//
// nm is the transpose inverse of the linear part of the transform.  Taken as an
// argument rather than calculating it every time.
func (c Contact) Transform(t affinetransform.AffineTransform, nm mat33.T) Contact {
	result := c

	// We don't use the standard ray-transforming support, since we need to know
	// how the transform changes the scale of the underlying space.
	result.R.Point = vec3.AddVV(mat33.MulMV(t.Linear, result.R.Point), t.Offset)
	result.R.Slope = mat33.MulMV(t.Linear, result.R.Slope)

	scaleFactor := result.R.Slope.Norm()
	result.R.Slope = vec3.DivVS(result.R.Slope, scaleFactor)
	result.T = result.T * scaleFactor

	result.P = vec3.AddVV(mat33.MulMV(t.Linear, result.P), t.Offset)
	result.N = vec3.Normalize(mat33.MulMV(nm, result.N))

	return result
}
