package geometry

import (
	"math"
	"row-major/harpoon/aabox"
	"row-major/harpoon/contact"
	"row-major/harpoon/ray"
	"row-major/harpoon/vmath/vec2"
	"row-major/harpoon/vmath/vec3"
)

type Geometry interface {
	GetAABox() aabox.AABox
	Crush(time float64)
	RayInto(query ray.RaySegment) contact.Contact
	RayExit(query ray.RaySegment) contact.Contact
}

type MaterialCoordsMode int

const (
	MaterialCoords3D = iota
	MaterialCoords2D
)

// Sphere is a Geometry that represents a unit sphere.
type Sphere struct {
	TheMaterialCoordsMode MaterialCoordsMode
}

func (s *Sphere) GetAABox() aabox.AABox {
	return aabox.AABox{
		X: ray.Span{Lo: -1.0, Hi: 1.0},
		Y: ray.Span{Lo: -1.0, Hi: 1.0},
		Z: ray.Span{Lo: -1.0, Hi: 1.0},
	}
}

func (s *Sphere) Crush(time float64) {
	// Nothing to do.
}

func (s *Sphere) RayInto(query ray.RaySegment) contact.Contact {
	b := vec3.IProd(query.TheRay.Slope, query.TheRay.Point)
	c := vec3.IProd(query.TheRay.Point, query.TheRay.Point) - 1.0

	tMin := -b - math.Sqrt(b*b-c)

	if tMin < query.TheSegment.Lo || query.TheSegment.Hi <= tMin {
		return contact.ContactNaN()
	}

	p := query.TheRay.Eval(tMin)

	result := contact.Contact{
		T:    tMin,
		P:    p,
		N:    vec3.Normalize(p),
		R:    query.TheRay,
		Mtl3: p,
	}

	if s.TheMaterialCoordsMode == MaterialCoords2D {
		result.Mtl2 = vec2.T{math.Atan2(p[0], p[1]), math.Acos(p[2])}
	}

	return result
}

func (s *Sphere) RayExit(query ray.RaySegment) contact.Contact {
	b := vec3.IProd(query.TheRay.Slope, query.TheRay.Point)
	c := vec3.IProd(query.TheRay.Point, query.TheRay.Point) - 1.0

	tMax := -b - math.Sqrt(b*b-c)

	if tMax < query.TheSegment.Lo || query.TheSegment.Hi <= tMax {
		return contact.ContactNaN()
	}

	p := query.TheRay.Eval(tMax)
	return contact.Contact{
		T:    tMax,
		P:    p,
		N:    vec3.Normalize(p),
		Mtl2: vec2.T{math.Atan2(p[0], p[1]), math.Acos(p[2])},
		Mtl3: p,
		R:    query.TheRay,
	}
}

type Box struct {
	Spans [3]ray.Span
}

func (b *Box) GetAABox() aabox.AABox {
	return aabox.AABox{
		X: ray.Span{b.Spans[0].Lo, b.Spans[0].Hi},
		Y: ray.Span{b.Spans[1].Lo, b.Spans[1].Hi},
		Z: ray.Span{b.Spans[2].Lo, b.Spans[2].Hi},
	}
}

func (b *Box) Crush(time float64) {}

func (b *Box) RayInto(query ray.RaySegment) contact.Contact {
	cover := ray.Span{math.Inf(-1), math.Inf(1)}

	hitAxis := [3]float64{}

	for i := 0; i < 3; i++ {
		cur := ray.Span{
			(b.Spans[i].Lo - query.TheRay.Point[i]) / query.TheRay.Slope[i],
			(b.Spans[i].Hi - query.TheRay.Point[i]) / query.TheRay.Slope[i],
		}

		normalComponent := -1.0
		if cur.Hi < cur.Lo {
			cur.Hi, cur.Lo = cur.Lo, cur.Hi
			normalComponent = 1.0
		}

		if !ray.SpanOverlaps(cover, cur) {
			return contact.ContactNaN()
		}

		if cover.Lo < cur.Lo {
			cover.Lo = cur.Lo
			hitAxis = [3]float64{}
			hitAxis[i] = normalComponent
		}

		if cur.Hi < cover.Hi {
			cover.Hi = cur.Hi
		}
	}

	if !ray.SpanOverlaps(cover, query.TheSegment) {
		return contact.ContactNaN()
	}

	val := contact.Contact{
		T:    cover.Lo,
		R:    query.TheRay,
		P:    query.TheRay.Eval(cover.Lo),
		N:    vec3.T{hitAxis[0], hitAxis[1], hitAxis[2]},
		Mtl2: vec2.T{0, 0},
		Mtl3: query.TheRay.Eval(cover.Lo),
	}
	return val
}

func (b *Box) RayExit(query ray.RaySegment) contact.Contact {
	cover := ray.Span{math.Inf(-1), math.Inf(1)}

	// I'm beginning to regret choosing the X,Y,Z convention for vectors.  Lots
	// of things are easier if the axes can be indexed.
	point := [3]float64{query.TheRay.Point[0], query.TheRay.Point[1], query.TheRay.Point[2]}
	slope := [3]float64{query.TheRay.Slope[0], query.TheRay.Slope[1], query.TheRay.Slope[2]}

	hitAxis := [3]float64{}

	for i := 0; i < 3; i++ {
		cur := ray.Span{
			(b.Spans[i].Lo - point[i]) / slope[i],
			(b.Spans[i].Hi - point[i]) / slope[i],
		}

		normalComponent := 1.0
		if cur.Hi < cur.Lo {
			cur.Hi, cur.Lo = cur.Lo, cur.Hi
			normalComponent = -1.0
		}

		if !ray.SpanOverlaps(cover, cur) {
			return contact.ContactNaN()
		}

		if cover.Lo < cur.Lo {
			cover.Lo = cur.Lo
		}

		if cur.Hi < cover.Hi {
			cover.Hi = cur.Hi
			hitAxis = [3]float64{}
			hitAxis[i] = normalComponent
		}
	}

	if !ray.SpanOverlaps(cover, query.TheSegment) {
		return contact.ContactNaN()
	}

	return contact.Contact{
		T:    cover.Hi,
		R:    query.TheRay,
		P:    query.TheRay.Eval(cover.Hi),
		N:    vec3.T{hitAxis[0], hitAxis[1], hitAxis[2]},
		Mtl2: vec2.T{0, 0},
		Mtl3: query.TheRay.Eval(cover.Hi),
	}
}
