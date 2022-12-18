package aabox

import (
	"math"
	"row-major/harpoon/affinetransform"
	"row-major/harpoon/ray"
	"row-major/harpoon/vmath/vec3"
)

type AABox struct {
	X, Y, Z ray.Span
}

func AccumZeroAABox() AABox {
	return AABox{
		X: ray.Span{math.Inf(1), math.Inf(-1)},
		Y: ray.Span{math.Inf(1), math.Inf(-1)},
		Z: ray.Span{math.Inf(1), math.Inf(-1)},
	}
}

func MinContainingAABox(a, b AABox) AABox {
	return AABox{
		X: ray.MinContainingSpan(a.X, b.X),
		Y: ray.MinContainingSpan(a.Y, b.Y),
		Z: ray.MinContainingSpan(a.Z, b.Z),
	}
}

func GrowAABoxToPoint(a AABox, b vec3.T) AABox {
	return AABox{}
}

func (a AABox) IsFinite() bool {
	return a.X.IsFinite() && a.Y.IsFinite() && a.Z.IsFinite()
}

func (a AABox) SurfaceArea() float64 {
	xLen := a.X.Hi - a.X.Lo
	yLen := a.Y.Hi - a.Y.Lo
	zLen := a.Z.Hi - a.Z.Lo
	return 2 * (xLen*yLen + xLen*zLen + yLen*zLen)
}

func (a AABox) Transform(t affinetransform.AffineTransform) AABox {
	points := []vec3.T{
		affinetransform.TransformPoint(t, vec3.T{a.X.Lo, a.Y.Lo, a.Z.Lo}),
		affinetransform.TransformPoint(t, vec3.T{a.X.Lo, a.Y.Lo, a.Z.Hi}),
		affinetransform.TransformPoint(t, vec3.T{a.X.Lo, a.Y.Hi, a.Z.Lo}),
		affinetransform.TransformPoint(t, vec3.T{a.X.Lo, a.Y.Hi, a.Z.Hi}),
		affinetransform.TransformPoint(t, vec3.T{a.X.Hi, a.Y.Lo, a.Z.Lo}),
		affinetransform.TransformPoint(t, vec3.T{a.X.Hi, a.Y.Lo, a.Z.Hi}),
		affinetransform.TransformPoint(t, vec3.T{a.X.Hi, a.Y.Hi, a.Z.Lo}),
		affinetransform.TransformPoint(t, vec3.T{a.X.Hi, a.Y.Hi, a.Z.Hi}),
	}

	result := AccumZeroAABox()
	for _, p := range points {
		if p[0] < result.X.Lo {
			result.X.Lo = p[0]
		}
		if p[0] > result.X.Hi {
			result.X.Hi = p[0]
		}
		if p[1] < result.Y.Lo {
			result.Y.Lo = p[1]
		}
		if p[1] > result.Y.Hi {
			result.Y.Hi = p[1]
		}
		if p[2] < result.Z.Lo {
			result.Z.Lo = p[2]
		}
		if p[2] > result.Z.Hi {
			result.Z.Hi = p[2]
		}
	}

	return result
}

func RayTestAABox(r ray.RaySegment, b AABox) ray.Span {
	cover := ray.Span{math.Inf(-1), math.Inf(1)}

	coverX := ray.Span{
		(b.X.Lo - r.TheRay.Point[0]) / r.TheRay.Slope[0],
		(b.X.Hi - r.TheRay.Point[0]) / r.TheRay.Slope[0],
	}
	if coverX.Hi < coverX.Lo {
		coverX.Lo, coverX.Hi = coverX.Hi, coverX.Lo
	}
	if !ray.SpanOverlaps(cover, coverX) {
		return ray.NaNSpan()
	}
	if coverX.Lo > cover.Lo {
		cover.Lo = coverX.Lo
	}
	if coverX.Hi < cover.Hi {
		cover.Hi = coverX.Hi
	}

	coverY := ray.Span{
		(b.Y.Lo - r.TheRay.Point[1]) / r.TheRay.Slope[1],
		(b.Y.Hi - r.TheRay.Point[1]) / r.TheRay.Slope[1],
	}
	if coverY.Hi < coverY.Lo {
		coverY.Lo, coverY.Hi = coverY.Hi, coverY.Lo
	}
	if !ray.SpanOverlaps(cover, coverY) {
		return ray.NaNSpan()
	}
	if coverY.Lo > cover.Lo {
		cover.Lo = coverY.Lo
	}
	if coverY.Hi < cover.Hi {
		cover.Hi = coverY.Hi
	}

	coverZ := ray.Span{
		(b.Z.Lo - r.TheRay.Point[2]) / r.TheRay.Slope[2],
		(b.Z.Hi - r.TheRay.Point[2]) / r.TheRay.Slope[2],
	}
	if coverZ.Hi < coverZ.Lo {
		coverZ.Lo, coverZ.Hi = coverZ.Hi, coverZ.Lo
	}
	if !ray.SpanOverlaps(cover, coverZ) {
		return ray.NaNSpan()
	}
	if coverZ.Lo > cover.Lo {
		cover.Lo = coverZ.Lo
	}
	if coverZ.Hi < cover.Hi {
		cover.Hi = coverZ.Hi
	}

	return cover
}
