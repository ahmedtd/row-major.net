package ray

import (
	"math"
	"row-major/harpoon/affinetransform"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec3"
)

type Span struct {
	Lo, Hi float64
}

func NaNSpan() Span {
	return Span{math.NaN(), math.NaN()}
}

func SpanOverlaps(a, b Span) bool {
	return !(a.Lo > b.Hi || a.Hi <= b.Lo)
}

func MinContainingSpan(a, b Span) Span {
	min := a.Lo
	if b.Lo < a.Lo {
		min = b.Lo
	}

	max := a.Hi
	if b.Hi > a.Hi {
		max = b.Hi
	}

	return Span{min, max}
}

func (s Span) IsFinite() bool {
	return !math.IsInf(s.Lo, 0) && !math.IsInf(s.Hi, 0)
}

func (s Span) IsNaN() bool {
	return math.IsNaN(s.Lo) || math.IsNaN(s.Hi)
}

type Ray struct {
	Point     vec3.T
	Slope     vec3.T
	PatchArea float64
}

func (r *Ray) Eval(t float64) vec3.T {
	return vec3.T{
		r.Point[0] + t*r.Slope[0],
		r.Point[1] + t*r.Slope[1],
		r.Point[2] + t*r.Slope[2],
	}
}

func (b *Ray) Transform(a affinetransform.AffineTransform) Ray {
	return Ray{
		Point:     vec3.AddVV(mat33.MulMV(a.Linear, b.Point), a.Offset),
		Slope:     vec3.Normalize(mat33.MulMV(a.Linear, b.Slope)),
		PatchArea: b.PatchArea,
	}
}

type RaySegment struct {
	TheRay     Ray
	TheSegment Span
}

func (b *RaySegment) Transform(a affinetransform.AffineTransform) RaySegment {
	result := RaySegment{}
	result.TheRay.PatchArea = b.TheRay.PatchArea
	result.TheRay.Point = vec3.AddVV(mat33.MulMV(a.Linear, b.TheRay.Point), a.Offset)
	result.TheRay.Slope = mat33.MulMV(a.Linear, b.TheRay.Slope)
	scaleFactor := result.TheRay.Slope.Norm()
	result.TheRay.Slope = vec3.DivVS(result.TheRay.Slope, scaleFactor)
	result.TheSegment.Lo = scaleFactor * b.TheSegment.Lo
	result.TheSegment.Hi = scaleFactor * b.TheSegment.Hi
	return result
}
