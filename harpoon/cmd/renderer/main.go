package main

import (
	"compress/zlib"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"

	"row-major/harpoon/affinetransform"
	"row-major/harpoon/densesignal"
	"row-major/harpoon/ray"
	"row-major/harpoon/spectralimage/headerproto"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec2"
	"row-major/harpoon/vmath/vec3"

	"google.golang.org/protobuf/proto"
)

type Camera interface {
	ImageToRay(curRow, imgRows, curCol, imgCols int, rng *rand.Rand) ray.Ray
}

type PinholeCamera struct {
	Center          vec3.T
	ApertureToWorld mat33.T
	Aperture        vec3.T
}

func (c *PinholeCamera) ImageToRay(curRow, imgRows, curCol, imgCols int, rng *rand.Rand) ray.Ray {
	imageCoords := vec3.T{
		1.0,
		1.0 - 2.0*(float64(curCol)-rng.Float64())/float64(imgCols),
		1.0 - 2.0*(float64(curRow)-rng.Float64())/float64(imgRows),
	}

	apertureCoords := vec3.T{
		imageCoords[0] * c.Aperture[0],
		imageCoords[1] * c.Aperture[1],
		imageCoords[2] * c.Aperture[2],
	}

	return ray.Ray{
		Point: c.Center,
		Slope: vec3.Normalize(mat33.MulMV(c.ApertureToWorld, apertureCoords)),
	}
}

func (c *PinholeCamera) Eye() vec3.T {
	return vec3.T{
		c.ApertureToWorld.Elts[0],
		c.ApertureToWorld.Elts[3],
		c.ApertureToWorld.Elts[6],
	}
}

func (c *PinholeCamera) Left() vec3.T {
	return vec3.T{
		c.ApertureToWorld.Elts[1],
		c.ApertureToWorld.Elts[4],
		c.ApertureToWorld.Elts[7],
	}
}

func (c *PinholeCamera) Up() vec3.T {
	return vec3.T{
		c.ApertureToWorld.Elts[2],
		c.ApertureToWorld.Elts[5],
		c.ApertureToWorld.Elts[8],
	}
}

func (c *PinholeCamera) SetEye(newEye vec3.T) {
	c.setEyeDirect(vec3.Normalize(newEye))
	c.setUpDirect(vec3.Normalize(vec3.Reject(c.Eye(), c.Up())))
	c.setLeftDirect(vec3.CProd(c.Up(), c.Eye()))
}

func (c *PinholeCamera) SetUp(newUp vec3.T) {
	c.setUpDirect(vec3.Normalize(vec3.Reject(c.Eye(), newUp)))
	c.setLeftDirect(vec3.CProd(c.Up(), c.Eye()))
}

func (c *PinholeCamera) setEyeDirect(newEye vec3.T) {
	c.ApertureToWorld.Elts[0] = newEye[0]
	c.ApertureToWorld.Elts[3] = newEye[1]
	c.ApertureToWorld.Elts[6] = newEye[2]
}

func (c *PinholeCamera) setLeftDirect(newLeft vec3.T) {
	c.ApertureToWorld.Elts[1] = newLeft[0]
	c.ApertureToWorld.Elts[4] = newLeft[1]
	c.ApertureToWorld.Elts[7] = newLeft[2]
}

func (c *PinholeCamera) setUpDirect(newUp vec3.T) {
	c.ApertureToWorld.Elts[2] = newUp[0]
	c.ApertureToWorld.Elts[5] = newUp[1]
	c.ApertureToWorld.Elts[8] = newUp[2]
}

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

type Geometry interface {
	GetAABox() AABox
	Crush(time float64)
	RayInto(query ray.RaySegment) Contact
	RayExit(query ray.RaySegment) Contact
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

func (s *Sphere) GetAABox() AABox {
	return AABox{
		X: ray.Span{Lo: -1.0, Hi: 1.0},
		Y: ray.Span{Lo: -1.0, Hi: 1.0},
		Z: ray.Span{Lo: -1.0, Hi: 1.0},
	}
}

func (s *Sphere) Crush(time float64) {
	// Nothing to do.
}

func (s *Sphere) RayInto(query ray.RaySegment) Contact {
	b := vec3.IProd(query.TheRay.Slope, query.TheRay.Point)
	c := vec3.IProd(query.TheRay.Point, query.TheRay.Point) - 1.0

	tMin := -b - math.Sqrt(b*b-c)

	if tMin < query.TheSegment.Lo || query.TheSegment.Hi <= tMin {
		return ContactNaN()
	}

	p := query.TheRay.Eval(tMin)

	result := Contact{
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

func (s *Sphere) RayExit(query ray.RaySegment) Contact {
	b := vec3.IProd(query.TheRay.Slope, query.TheRay.Point)
	c := vec3.IProd(query.TheRay.Point, query.TheRay.Point) - 1.0

	tMax := -b - math.Sqrt(b*b-c)

	if tMax < query.TheSegment.Lo || query.TheSegment.Hi <= tMax {
		return ContactNaN()
	}

	p := query.TheRay.Eval(tMax)
	return Contact{
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

func (b *Box) GetAABox() AABox {
	return AABox{
		X: ray.Span{b.Spans[0].Lo, b.Spans[0].Hi},
		Y: ray.Span{b.Spans[1].Lo, b.Spans[1].Hi},
		Z: ray.Span{b.Spans[2].Lo, b.Spans[2].Hi},
	}
}

func (b *Box) Crush(time float64) {}

func (b *Box) RayInto(query ray.RaySegment) Contact {
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
			return ContactNaN()
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
		return ContactNaN()
	}

	val := Contact{
		T:    cover.Lo,
		R:    query.TheRay,
		P:    query.TheRay.Eval(cover.Lo),
		N:    vec3.T{hitAxis[0], hitAxis[1], hitAxis[2]},
		Mtl2: vec2.T{0, 0},
		Mtl3: query.TheRay.Eval(cover.Lo),
	}
	return val
}

func (b *Box) RayExit(query ray.RaySegment) Contact {
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
			return ContactNaN()
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
		return ContactNaN()
	}

	return Contact{
		T:    cover.Hi,
		R:    query.TheRay,
		P:    query.TheRay.Eval(cover.Hi),
		N:    vec3.T{hitAxis[0], hitAxis[1], hitAxis[2]},
		Mtl2: vec2.T{0, 0},
		Mtl3: query.TheRay.Eval(cover.Hi),
	}
}

type MaterialCoords struct {
	Mtl2 vec2.T
	Mtl3 vec3.T
	Freq float32
}

type MaterialMap func(MaterialCoords) float64

func ConstantScalar(scalar float64) MaterialMap {
	return func(coords MaterialCoords) float64 {
		return scalar
	}
}

func ConstantSpectrum(spectrum *densesignal.DenseSignal) MaterialMap {
	return func(coords MaterialCoords) float64 {
		val := float64(spectrum.Interpolate(coords.Freq))
		return val
	}
}

func LerpBetween(t, a, b MaterialMap) MaterialMap {
	return func(coords MaterialCoords) float64 {
		tVal := t(coords)
		return (1.0-tVal)*a(coords) + tVal*b(coords)
	}
}

func SwitchBetween(tSwitch float64, t, a, b MaterialMap) MaterialMap {
	return func(coords MaterialCoords) float64 {
		tVal := t(coords)
		if tVal < tSwitch {
			return a(coords)
		} else {
			return b(coords)
		}
	}
}

func Clamp(min, max float64, a MaterialMap) MaterialMap {
	return func(coords MaterialCoords) float64 {
		aVal := a(coords)
		if aVal < min {
			return min
		}
		if aVal >= max {
			return max
		}
		return aVal
	}
}

func CheckerboardSurface(period float64) MaterialMap {
	return func(coords MaterialCoords) float64 {
		parity := 0

		qx := coords.Mtl2[0] / period
		fx := math.Floor(qx)
		rx := qx - fx
		if rx > 0.5 {
			parity ^= 1
		}

		qy := coords.Mtl2[1] / period
		fy := math.Floor(qy)
		ry := qy - fy
		if ry > 0.5 {
			parity ^= 1
		}

		if parity == 1 {
			return 1.0
		} else {
			return 0.0
		}
	}
}

func CheckerboardVolume(period float64) MaterialMap {
	return func(coords MaterialCoords) float64 {
		parity := 0

		qx := coords.Mtl3[0] / period
		fx := math.Floor(qx)
		rx := qx - fx
		if rx > 0.5 {
			parity ^= 1
		}

		qy := coords.Mtl3[1] / period
		fy := math.Floor(qy)
		ry := qy - fy
		if ry > 0.5 {
			parity ^= 1
		}

		qz := coords.Mtl3[2] / period
		fz := math.Floor(qz)
		rz := qz - fz
		if rz > 0.5 {
			parity ^= 1
		}

		if parity == 1 {
			return 1.0
		} else {
			return 0.0
		}
	}
}

func BullseyeSurface(period float64) MaterialMap {
	return func(coords MaterialCoords) float64 {
		d := coords.Mtl2.Norm() / period
		if _, frac := math.Modf(d); frac < 0.5 {
			return 0.0
		} else {
			return 1.0
		}
	}
}

func BullseyeVolume(period float64) MaterialMap {
	return func(coords MaterialCoords) float64 {
		d := coords.Mtl3.Norm() / period
		if _, frac := math.Modf(d); frac < 0.5 {
			return 0.0
		} else {
			return 1.0
		}
	}
}

// A multiplicative hash (in Knuth's style), that makes use of the fact that we
// only use 24 input bits.
//
// The multiplicative constant is floor(2^24 / (golden ratio)), tweaked a bit to
// avoid attractors above 0xc in the last digit.
//
// Copied from the C++ version, I don't remember any of this shit.
func hashmul(x uint32) uint32 {
	x = ((x >> 16) ^ x) * 0x45d9f3b
	x = ((x >> 16) ^ x) * 0x45d9f3b
	x = ((x >> 16) ^ x)
	return x
}

func perlinDotGrad(c0, c1, c2 uint32, d0, d1, d2 float64) float64 {
	// I have totally forgotten how this works.  The comment is copied from my
	// C++ version.
	hash := hashmul(((c0 & 0xff) << 16) | ((c1 & 0xff) << 8) | (c2&0xff)<<0)

	switch hash & 0x0f {
	case 0x0:
		return d0 + d1
	case 0x1:
		return d0 - d1
	case 0x2:
		return -d0 + d1
	case 0x3:
		return -d0 - d1

	case 0x4:
		return d1 + d2
	case 0x5:
		return d1 - d2
	case 0x6:
		return -d1 + d2
	case 0x7:
		return -d1 - d2

	case 0x8:
		return d2 + d0
	case 0x9:
		return d2 - d0
	case 0xa:
		return -d2 + d0
	case 0xb:
		return -d2 - d0

	case 0xc:
		return d0 + d1
	case 0xd:
		return -d0 + d1
	case 0xe:
		return -d1 + d2
	case 0xf:
		return -d1 - d2
	}

	// Dead code
	return 0
}

func fade(x float64) float64 {
	return x * x * x * (x*(x*6.0-15.0) + 10.0)
}

func lerp(t, a, b float64) float64 {
	return (1-t)*a + t*b
}

func PerlinSurface(period float64) MaterialMap {
	return func(coords MaterialCoords) float64 {
		x := coords.Mtl2[0] * 256.0 / period
		y := coords.Mtl2[1] * 256.0 / period
		z := 0.0

		cellX := uint32(int32(math.Floor(x)) & 0xff)
		cellY := uint32(int32(math.Floor(y)) & 0xff)
		cellZ := uint32(int32(math.Floor(z)) & 0xff)

		xRel := x - math.Floor(x)
		yRel := y - math.Floor(x)
		zRel := z - math.Floor(z)

		return lerp(fade(yRel),
			lerp(fade(xRel),
				perlinDotGrad(cellX+0, cellY+0, cellZ+0, xRel-0, yRel-0, zRel-0),
				perlinDotGrad(cellX+1, cellY+0, cellZ+0, xRel-1, yRel-0, zRel-0),
			),
			lerp(fade(xRel),
				perlinDotGrad(cellX+0, cellY+1, cellZ+0, xRel-0, yRel-1, zRel-0),
				perlinDotGrad(cellX+1, cellY+1, cellZ+0, xRel-1, yRel-1, zRel-0),
			),
		)
	}
}

func PerlinVolume(period float64) MaterialMap {
	return func(coords MaterialCoords) float64 {
		x := coords.Mtl3[1] * 256.0 / period
		y := coords.Mtl3[1] * 256.0 / period
		z := coords.Mtl3[1] * 256.0 / period

		cellX := uint32(int32(math.Floor(x)) & 0xff)
		cellY := uint32(int32(math.Floor(y)) & 0xff)
		cellZ := uint32(int32(math.Floor(z)) & 0xff)

		xRel := x - math.Floor(x)
		yRel := y - math.Floor(x)
		zRel := z - math.Floor(z)

		return lerp(fade(zRel),
			lerp(fade(yRel),
				lerp(fade(xRel),
					perlinDotGrad(cellX+0, cellY+0, cellZ+0, xRel-0, yRel-0, zRel-0),
					perlinDotGrad(cellX+1, cellY+0, cellZ+0, xRel-1, yRel-0, zRel-0),
				),
				lerp(fade(xRel),
					perlinDotGrad(cellX+0, cellY+1, cellX+0, xRel-0, yRel-1, zRel-0),
					perlinDotGrad(cellX+1, cellX+0, cellX+0, xRel-1, yRel-0, zRel-0),
				),
			),
			lerp(fade(yRel),
				lerp(fade(xRel),
					perlinDotGrad(cellX+0, cellY+0, cellZ+1, xRel-0, yRel-0, zRel-1),
					perlinDotGrad(cellX+1, cellY+0, cellZ+1, xRel-1, yRel-0, zRel-1),
				),
				lerp(fade(xRel),
					perlinDotGrad(cellX+0, cellY+1, cellZ+1, xRel-0, yRel-1, zRel-1),
					perlinDotGrad(cellX+1, cellY+1, cellZ+1, xRel-1, yRel-1, zRel-1),
				),
			),
		)
	}
}

type ShadeInfo struct {
	PropagationK float32
	EmittedPower float32
	IncidentRay  ray.Ray
}

type Material interface {
	Crush(time float64)
	Shade(globalContact Contact, freq float32, rng *rand.Rand) ShadeInfo
}

// DirectionalEmitter is an emitter that queries an emissivity material map
// based on direction of arrival.
//
// In effect, it acts as a window into an environment map.
type DirectionalEmitter struct {
	Emissivity MaterialMap
}

func (d *DirectionalEmitter) Crush(time float64) {}

func (d *DirectionalEmitter) Shade(contact Contact, freq float32, rng *rand.Rand) ShadeInfo {
	materialCoords := MaterialCoords{
		Mtl3: contact.R.Slope,
		Mtl2: vec2.T{
			math.Atan2(contact.R.Slope[0], contact.R.Slope[1]),
			math.Acos(contact.R.Slope[2]),
		},
		Freq: freq,
	}

	return ShadeInfo{
		EmittedPower: float32(d.Emissivity(materialCoords)),
	}
}

type Emitter struct {
	Emissivity MaterialMap
}

func (e *Emitter) Crush(time float64) {}

func (e *Emitter) Shade(contact Contact, freq float32, rng *rand.Rand) ShadeInfo {
	materialCoords := MaterialCoords{
		Mtl3: contact.Mtl3,
		Mtl2: contact.Mtl2,
		Freq: freq,
	}

	return ShadeInfo{
		EmittedPower: float32(e.Emissivity(materialCoords)),
	}
}

type MonteCarloLambert struct {
	Reflectance MaterialMap
}

func (l *MonteCarloLambert) Crush(time float64) {}

func (l *MonteCarloLambert) Shade(contact Contact, freq float32, rng *rand.Rand) ShadeInfo {
	dir := vec3.HemisphereUnitVec3Distribution(contact.N, rng)
	reflectance := l.Reflectance(MaterialCoords{contact.Mtl2, contact.Mtl3, freq})
	propagation := float32(vec3.IProd(contact.N, dir) * reflectance)

	return ShadeInfo{
		IncidentRay: ray.Ray{
			Point: contact.P,
			Slope: dir,
		},
		PropagationK: propagation,
		EmittedPower: 0.0,
	}
}

type NonConductiveSmooth struct {
	InteriorIndexOfRefraction MaterialMap
	ExteriorIndexOfRefraction MaterialMap
}

func (n *NonConductiveSmooth) Crush(time float64) {}

func (n *NonConductiveSmooth) Shade(contact Contact, freq float32, rng *rand.Rand) ShadeInfo {
	coord := MaterialCoords{
		Mtl2: contact.Mtl2,
		Mtl3: contact.Mtl3,
		Freq: freq,
	}
	nA := n.ExteriorIndexOfRefraction(coord)
	nB := n.InteriorIndexOfRefraction(coord)

	aCos := vec3.IProd(contact.R.Slope, contact.N)
	if aCos > 0.0 {
		nA, nB = nB, nA
	}

	nR := nA / nB

	// Now the problem is regularized. We have a ray that was emitted into
	// region A from a planar boundary.  There are two incident rays that put
	// power into this ray:
	//
	// * One shone on the boundary from region A, and was partially reflected
	//   into the ray we have.
	//
	//
	// * One shone on the boundary from region B, and was partially transmitted
	// into the ray we have.

	snell := 1.0 - (nR*nR)*(1.0-(aCos*aCos))
	if snell < 0.0 {
		// All power was contributed by the reflected ray (total internal
		// reflection).
		return ShadeInfo{
			EmittedPower: 0.0,
			PropagationK: 1.0,
			IncidentRay: ray.Ray{
				Point: contact.P,
				Slope: vec3.Reflect(contact.R.Slope, contact.N),
			},
		}
	}

	bCos := math.Sqrt(snell)
	if aCos < 0.0 {
		bCos = -bCos
	}

	ab := nR * bCos / aCos
	abI := 1.0 / ab

	// We are solving the reverse problem, so the coefficient of transmission
	// must actually be calculated from the perspective of the ray shining on the
	// boundary from region B.
	reflectionCoefficient := ((1 - ab) / (1 + ab)) * ((1 - ab) / (1 + ab))
	transmissionCoefficient := abI * (2.0 / (1 + abI)) * (2.0 / (1 + abI))

	sample := rng.Float64() * (reflectionCoefficient + transmissionCoefficient)
	if sample < reflectionCoefficient {
		// Give the ray that contributed by reflection.
		return ShadeInfo{
			EmittedPower: 0.0,
			PropagationK: 1.0,
			IncidentRay: ray.Ray{
				Point: contact.P,
				Slope: vec3.Reflect(contact.R.Slope, contact.N),
			},
		}
	} else {
		// Give the ray that contributed by refraction.
		return ShadeInfo{
			EmittedPower: 0.0,
			PropagationK: 1.0,
			IncidentRay: ray.Ray{
				Point: contact.P,
				Slope: vec3.AddVV(vec3.MulVS(contact.N, bCos-nR*aCos), vec3.MulVS(contact.R.Slope, nR)),
			},
		}
	}
}

type PerfectlyConductiveSmooth struct {
	Reflectance MaterialMap
}

func (p *PerfectlyConductiveSmooth) Crush(time float64) {}

func (p *PerfectlyConductiveSmooth) Shade(contact Contact, freq float32, rng *rand.Rand) ShadeInfo {
	propagation := p.Reflectance(MaterialCoords{
		Mtl2: contact.Mtl2,
		Mtl3: contact.Mtl3,
		Freq: freq,
	})

	return ShadeInfo{
		EmittedPower: 0.0,
		PropagationK: float32(propagation),
		IncidentRay: ray.Ray{
			Point: contact.P,
			Slope: vec3.Reflect(contact.R.Slope, contact.N),
		},
	}
}

type GaussianRoughNonConductive struct {
	Variance MaterialMap
}

func (g *GaussianRoughNonConductive) Crush(time float64) {}

func (g *GaussianRoughNonConductive) Shade(contact Contact, freq float32, rng *rand.Rand) ShadeInfo {
	variance := g.Variance(MaterialCoords{contact.Mtl2, contact.Mtl3, freq})
	facetNormal := vec3.GaussianUnitVec3Distribution(contact.N, variance, rng)

	// Performance hack
	if vec3.IProd(facetNormal, contact.R.Slope) < 0.0 {
		facetNormal = vec3.Reflect(facetNormal, contact.N)
	}

	return ShadeInfo{
		EmittedPower: 0.0,
		PropagationK: 0.8,
		IncidentRay: ray.Ray{
			Point: contact.P,
			Slope: vec3.Reflect(contact.R.Slope, facetNormal),
		},
	}
}

type KDElement struct {
	// A handle back into some other storage array.
	Ref int

	// The bounds of this element.
	Bounds AABox
}

type KDNode struct {
	Bounds AABox

	Elements []KDElement

	LoChild *KDNode
	HiChild *KDNode
}

func (cur *KDNode) refineViaSurfaceAreaHeuristic(splitCost, terminationThreshold float64, rng *rand.Rand) {
	bestObjective := math.Inf(1)
	bestLoBox := AABox{}
	bestHiBox := AABox{}
	bestPrecedingElements := []KDElement{}
	bestSucceedingElements := []KDElement{}

	// Check 5 random splits on the X axis.
	for i := 0; i < 5; i++ {
		trialCut := cur.Elements[rng.Intn(len(cur.Elements))].Bounds.X.Hi

		precedingElements := []KDElement{}
		succeedingElements := []KDElement{}
		for _, element := range cur.Elements {
			if element.Bounds.X.Hi < trialCut {
				precedingElements = append(precedingElements, element)
			} else {
				succeedingElements = append(succeedingElements, element)
			}
		}

		loBox := AccumZeroAABox()
		for _, element := range precedingElements {
			loBox = MinContainingAABox(loBox, element.Bounds)
		}

		hiBox := AccumZeroAABox()
		for _, element := range precedingElements {
			hiBox = MinContainingAABox(hiBox, element.Bounds)
		}

		objective := 0.0
		if len(precedingElements) != 0 {
			objective += loBox.SurfaceArea()
		}
		if len(succeedingElements) != 0 {
			objective += hiBox.SurfaceArea()
		}

		if objective < bestObjective {
			bestObjective = objective
			bestPrecedingElements = precedingElements
			bestSucceedingElements = succeedingElements
			bestLoBox = loBox
			bestHiBox = hiBox
		}
	}

	// Check 5 random splits on the Y axis
	for i := 0; i < 5; i++ {
		trialCut := cur.Elements[rng.Intn(len(cur.Elements))].Bounds.Y.Hi

		precedingElements := []KDElement{}
		succeedingElements := []KDElement{}
		for _, element := range cur.Elements {
			if element.Bounds.Y.Hi < trialCut {
				precedingElements = append(precedingElements, element)
			} else {
				succeedingElements = append(succeedingElements, element)
			}
		}

		loBox := AccumZeroAABox()
		for _, element := range precedingElements {
			loBox = MinContainingAABox(loBox, element.Bounds)
		}

		hiBox := AccumZeroAABox()
		for _, element := range precedingElements {
			hiBox = MinContainingAABox(hiBox, element.Bounds)
		}

		objective := 0.0
		if len(precedingElements) != 0 {
			objective += loBox.SurfaceArea()
		}
		if len(succeedingElements) != 0 {
			objective += hiBox.SurfaceArea()
		}

		if objective < bestObjective {
			bestObjective = objective
			bestPrecedingElements = precedingElements
			bestSucceedingElements = succeedingElements
			bestLoBox = loBox
			bestHiBox = hiBox
		}
	}

	// Check 5 random splits on the Z axis.
	for i := 0; i < 5; i++ {
		trialCut := cur.Elements[rng.Intn(len(cur.Elements))].Bounds.Z.Hi

		precedingElements := []KDElement{}
		succeedingElements := []KDElement{}
		for _, element := range cur.Elements {
			if element.Bounds.Z.Hi < trialCut {
				precedingElements = append(precedingElements, element)
			} else {
				succeedingElements = append(succeedingElements, element)
			}
		}

		loBox := AccumZeroAABox()
		for _, element := range precedingElements {
			loBox = MinContainingAABox(loBox, element.Bounds)
		}

		hiBox := AccumZeroAABox()
		for _, element := range precedingElements {
			hiBox = MinContainingAABox(hiBox, element.Bounds)
		}

		objective := 0.0
		if len(precedingElements) != 0 {
			objective += loBox.SurfaceArea()
		}
		if len(succeedingElements) != 0 {
			objective += hiBox.SurfaceArea()
		}

		if objective < bestObjective {
			bestObjective = objective
			bestPrecedingElements = precedingElements
			bestSucceedingElements = succeedingElements
			bestLoBox = loBox
			bestHiBox = hiBox
		}
	}

	// Now we have a pretty good split, but we need to check that it's a
	// good-enough improvement over just not splitting.
	parentObjective := float64(len(cur.Elements)) * cur.Bounds.SurfaceArea()
	if bestObjective+splitCost <= terminationThreshold+parentObjective {
		return
	}

	if len(bestPrecedingElements) != 0 {
		cur.LoChild = &KDNode{
			Bounds:   bestLoBox,
			Elements: bestPrecedingElements,
		}
	}

	if len(bestSucceedingElements) != 0 {
		cur.HiChild = &KDNode{
			Bounds:   bestHiBox,
			Elements: bestSucceedingElements,
		}
	}

	// All of cur's elements have been divided among its children.
	cur.Elements = []KDElement{}
}

type KDTree struct {
	Root *KDNode
}

func NewKDTree(elements []KDElement) *KDTree {
	tree := &KDTree{}

	maxBox := AccumZeroAABox()
	for _, element := range elements {
		maxBox = MinContainingAABox(maxBox, element.Bounds)
	}

	tree.Root = &KDNode{
		Bounds:   maxBox,
		Elements: elements,
	}

	return tree
}

func (t *KDTree) RefineViaSurfaceAreaHeuristic(splitCost, threshold float64) {
	rng := rand.New(rand.NewSource(12345))

	workStack := []*KDNode{t.Root}
	for {
		if len(workStack) == 0 {
			return
		}

		cur := workStack[len(workStack)-1]
		workStack = workStack[:len(workStack)-1]

		if len(cur.Elements) < 2 {
			continue
		}

		cur.refineViaSurfaceAreaHeuristic(splitCost, threshold, rng)

		if cur.LoChild != nil {
			workStack = append(workStack, cur.LoChild)
		}
		if cur.HiChild != nil {
			workStack = append(workStack, cur.HiChild)
		}
	}
}

type KDSelector func(b AABox) bool
type KDVisitor func(i int)

func (t *KDTree) Query(selector KDSelector, visitor KDVisitor) {
	workStack := []*KDNode{t.Root}
	for len(workStack) != 0 {
		cur := workStack[len(workStack)-1]
		workStack = workStack[:len(workStack)-1]

		if !selector(cur.Bounds) {
			continue
		}

		for i := range cur.Elements {
			// TODO(ahmedtd): Test against cur.Elements[i].Bounds?
			visitor(cur.Elements[i].Ref)
		}

		if cur.LoChild != nil {
			workStack = append(workStack, cur.LoChild)
		}
		if cur.HiChild != nil {
			workStack = append(workStack, cur.HiChild)
		}
	}
}

type SceneElement struct {
	TheGeometry Geometry
	TheMaterial Material

	// The transform that takes model space to world space.
	ModelToWorld affinetransform.AffineTransform
}

type CrushedSceneElement struct {
	TheGeometry Geometry
	TheMaterial Material

	// The transform that takes a ray from world space to model space.
	WorldToModel affinetransform.AffineTransform

	// The transform that takes rays and contacts from model space to world
	// space.
	ModelToWorld affinetransform.AffineTransform

	// The linear map that takes normal vectors from model space to world space.
	ModelToWorldNormals mat33.T

	// The element's bounding box in world coordinates.
	WorldBounds AABox
}

type Scene struct {
	Elements         []*SceneElement
	CrushedElements  []*CrushedSceneElement
	InfinityMaterial Material
	QueryAccelerator *KDTree
}

func (s *Scene) Crush(time float64) {
	// Geometry, materials, and material maps are crushed in a dependency-based
	// fashion, whith each crushing its own dependencies.  To prevent redundant
	// crushes, objects should cache whether they have been crushed at the given
	// key value (time).

	s.InfinityMaterial.Crush(time)

	kdElements := []KDElement{}
	for i, element := range s.Elements {
		element.TheGeometry.Crush(time)
		element.TheMaterial.Crush(time)

		worldBounds := element.TheGeometry.GetAABox().Transform(element.ModelToWorld)

		crushedElement := &CrushedSceneElement{
			TheGeometry:         element.TheGeometry,
			TheMaterial:         element.TheMaterial,
			WorldToModel:        element.ModelToWorld.Invert(),
			ModelToWorld:        element.ModelToWorld,
			ModelToWorldNormals: element.ModelToWorld.NormalTransformMat(),
			WorldBounds:         worldBounds,
		}

		s.CrushedElements = append(s.CrushedElements, crushedElement)

		kdElements = append(kdElements, KDElement{i, worldBounds})
	}

	s.QueryAccelerator = NewKDTree(kdElements)
	s.QueryAccelerator.RefineViaSurfaceAreaHeuristic(1.0, 0.9)
}

func (s *Scene) SceneRayIntersect(worldQuery ray.RaySegment) (Contact, int) {
	minContact := Contact{}
	minElementIndex := -1

	selector := func(b AABox) bool {
		return !RayTestAABox(worldQuery, b).IsNaN()
	}

	visitor := func(i int) {
		elt := s.CrushedElements[i]
		mdlQuery := worldQuery.Transform(elt.WorldToModel)
		entryContact := elt.TheGeometry.RayInto(mdlQuery)
		if !math.IsNaN(entryContact.T) && mdlQuery.TheSegment.Lo <= entryContact.T && entryContact.T <= mdlQuery.TheSegment.Hi {
			worldEntryContact := entryContact.Transform(elt.ModelToWorld, elt.ModelToWorldNormals)
			worldQuery.TheSegment.Hi = worldEntryContact.T
			minContact = worldEntryContact
			minElementIndex = i
		}
		exitContact := elt.TheGeometry.RayExit(mdlQuery)
		if !math.IsNaN(exitContact.T) && mdlQuery.TheSegment.Lo <= exitContact.T && exitContact.T <= mdlQuery.TheSegment.Hi {
			worldExitContact := exitContact.Transform(elt.ModelToWorld, elt.ModelToWorldNormals)
			worldQuery.TheSegment.Hi = worldExitContact.T
			minContact = worldExitContact
			minElementIndex = i
		}
	}

	s.QueryAccelerator.Query(selector, visitor)

	return minContact, minElementIndex
}

func (s *Scene) ShadeRay(reflectedRay ray.Ray, curWavelength float32, rng *rand.Rand) ShadeInfo {
	reflectedQuery := ray.RaySegment{
		TheRay:     reflectedRay,
		TheSegment: ray.Span{0.0001, math.Inf(1)},
	}

	glbContact, hitIndex := s.SceneRayIntersect(reflectedQuery)
	if hitIndex == -1 {
		p := reflectedRay.Eval(math.Inf(1))
		c := Contact{
			T:    math.Inf(1),
			R:    reflectedRay,
			P:    p,
			N:    vec3.MulVS(reflectedRay.Slope, -1.0),
			Mtl2: vec2.T{math.Atan2(reflectedRay.Slope[0], reflectedRay.Slope[1]), math.Acos(reflectedRay.Slope[2])},
			Mtl3: p,
		}
		return s.InfinityMaterial.Shade(c, curWavelength, rng)
	}

	return s.CrushedElements[hitIndex].TheMaterial.Shade(glbContact, curWavelength, rng)
}

func (s *Scene) SampleRay(initialQuery ray.Ray, curWavelength float32, rng *rand.Rand, depthLim int) float32 {
	var accumPower float32
	var curK float32 = 1.0
	curRay := initialQuery

	for i := 0; i < depthLim; i++ {
		shading := s.ShadeRay(curRay, curWavelength, rng)
		accumPower += curK * shading.EmittedPower

		curK = shading.PropagationK
		if shading.PropagationK == 0.0 {
			continue
		}
		curRay = shading.IncidentRay
	}

	return accumPower
}

type SpectralImage struct {
	RowSize, ColSize, WavelengthSize int
	WavelengthMin, WavelengthMax     float32
	PowerDensitySums                 []float32
	PowerDensityCounts               []float32
}

type SpectralImageSample struct {
	WavelengthLo, WavelengthHi         float32
	PowerDensitySum, PowerDensityCount float32
}

func (s *SpectralImage) Resize(rowSize, colSize, wavelengthSize int) {
	s.RowSize = rowSize
	s.ColSize = colSize
	s.WavelengthSize = wavelengthSize

	s.PowerDensitySums = make([]float32, rowSize*colSize*wavelengthSize)
	s.PowerDensityCounts = make([]float32, rowSize*colSize*wavelengthSize)
}

func (s *SpectralImage) WavelengthBin(i int) (float32, float32) {
	binWidth := (s.WavelengthMax - s.WavelengthMin) / float32(s.WavelengthSize)
	lo := s.WavelengthMin + float32(i)*binWidth
	if i == s.WavelengthSize-1 {
		return lo, s.WavelengthMax
	}
	return lo, lo + binWidth
}

func (s *SpectralImage) RecordSample(r, c, w int, powerDensity float32) {
	idx := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w
	s.PowerDensitySums[idx] += powerDensity
	s.PowerDensityCounts[idx] += 1
}

func (s *SpectralImage) ReadSample(r, c, w int) SpectralImageSample {
	idx := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w
	binLo, binHi := s.WavelengthBin(w)
	return SpectralImageSample{
		WavelengthLo:      binLo,
		WavelengthHi:      binHi,
		PowerDensitySum:   s.PowerDensitySums[idx],
		PowerDensityCount: s.PowerDensityCounts[idx],
	}
}

func (s *SpectralImage) Cut(rowSrc, rowLim, colSrc, colLim int) *SpectralImage {
	dst := &SpectralImage{
		WavelengthMin: s.WavelengthMin,
		WavelengthMax: s.WavelengthMax,
	}
	dst.Resize(rowLim-rowSrc, colLim-colSrc, s.WavelengthSize)

	dstIndex := 0
	for r := rowSrc; r < rowLim; r++ {
		for c := colSrc; c < colLim; c++ {
			for w := 0; w < s.WavelengthSize; w++ {
				srcIndex := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w

				dst.PowerDensitySums[dstIndex] = s.PowerDensitySums[srcIndex]
				dst.PowerDensityCounts[dstIndex] = s.PowerDensityCounts[srcIndex]

				dstIndex++
			}
		}
	}

	return dst
}

func (s *SpectralImage) Paste(src *SpectralImage, rowSrc, colSrc int) {
	rowLim := rowSrc + src.RowSize
	colLim := colSrc + src.ColSize

	for r := rowSrc; r < rowLim; r++ {
		for c := colSrc; c < colLim; c++ {
			for w := 0; w < s.WavelengthSize; w++ {
				dstIndex := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w
				srcIndex := (r-rowSrc)*src.ColSize*s.WavelengthSize + (c-colSrc)*src.WavelengthSize + w

				s.PowerDensitySums[dstIndex] = src.PowerDensitySums[srcIndex]
				s.PowerDensityCounts[dstIndex] = src.PowerDensityCounts[srcIndex]
			}
		}
	}
}

func ReadSpectralImage(in io.Reader) (*SpectralImage, error) {
	// Read header length.
	var headerLength uint64
	if err := binary.Read(in, binary.LittleEndian, &headerLength); err != nil {
		return nil, fmt.Errorf("while reading header length: %w", err)
	}

	headerBytes := make([]byte, int(headerLength))
	if _, err := in.Read(headerBytes); err != nil {
		return nil, fmt.Errorf("while reading header bytes: %w", err)
	}

	hdr := &headerproto.SpectralImageHeader{}
	if err := proto.Unmarshal(headerBytes, hdr); err != nil {
		return nil, fmt.Errorf("while unmarshaling header: %w", err)
	}

	if hdr.GetDataLayoutVersion() != 1 {
		return nil, fmt.Errorf("bad data layout version: %v", hdr.GetDataLayoutVersion())
	}

	im := &SpectralImage{}
	im.WavelengthMin = hdr.GetWavelengthMin()
	im.WavelengthMax = hdr.GetWavelengthMax()

	im.Resize(int(hdr.GetRowSize()), int(hdr.GetColSize()), int(hdr.GetWavelengthSize()))

	zipReader, err := zlib.NewReader(in)
	if err != nil {
		return nil, fmt.Errorf("while opening zip reader: %w", err)
	}
	defer zipReader.Close()

	if err := binary.Read(zipReader, binary.LittleEndian, &im.PowerDensitySums); err != nil {
		return nil, fmt.Errorf("while reading power density sums: %w", err)
	}

	if err := binary.Read(zipReader, binary.LittleEndian, &im.PowerDensityCounts); err != nil {
		return nil, fmt.Errorf("while reading power density counts: %w", err)
	}

	return im, nil
}

func ReadSpectralImageFromFile(name string) (*SpectralImage, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("while opening file: %w", err)
	}
	defer f.Close()

	return ReadSpectralImage(f)
}

func WriteSpectralImage(im *SpectralImage, w io.Writer) error {
	hdr := &headerproto.SpectralImageHeader{
		RowSize:           uint32(im.RowSize),
		ColSize:           uint32(im.ColSize),
		WavelengthSize:    uint32(im.WavelengthSize),
		WavelengthMin:     im.WavelengthMin,
		WavelengthMax:     im.WavelengthMax,
		DataLayoutVersion: uint32(1),
	}

	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return fmt.Errorf("while marshaling header: %w", err)
	}

	headerLengthBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(headerLengthBytes, uint64(len(hdrBytes)))
	if _, err := w.Write(headerLengthBytes); err != nil {
		return fmt.Errorf("while writing header length: %w", err)
	}

	if _, err := w.Write(hdrBytes); err != nil {
		return fmt.Errorf("while writing header: %w", err)
	}

	zipWriter := zlib.NewWriter(w)

	if err := binary.Write(zipWriter, binary.LittleEndian, im.PowerDensitySums); err != nil {
		return fmt.Errorf("while writing power density sums: %w", err)
	}

	if err := binary.Write(zipWriter, binary.LittleEndian, im.PowerDensityCounts); err != nil {
		return fmt.Errorf("while writing power density counts: %w", err)
	}

	if err := zipWriter.Flush(); err != nil {
		return fmt.Errorf("while flushing zip writer: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("while closing zip writer: %w", err)
	}

	return nil
}

type ChunkWorker struct {
	sampleDB         *SpectralImage
	rng              *rand.Rand
	progressFunction func(int)

	maxDepth      int
	targetSamples int

	// These are the dimensions of the overall image, not just
	imgRows int
	imgCols int

	rowSrc int
	rowLim int

	colSrc int
	colLim int

	camera Camera
	scene  *Scene
}

func (w *ChunkWorker) Render() {
	samplesCollected := 0
	for cr := w.rowSrc; cr < w.rowLim; cr++ {
		for cc := w.colSrc; cc < w.colLim; cc++ {
			for cw := 0; cw < w.sampleDB.WavelengthSize; cw++ {
				r := cr - w.rowSrc
				c := cc - w.colSrc

				samp := w.sampleDB.ReadSample(r, c, cw)
				if int(samp.PowerDensityCount) > w.targetSamples {
					continue
				}
				samplesToAdd := w.targetSamples - int(samp.PowerDensityCount)

				for cs := 0; cs < samplesToAdd; cs++ {
					curWavelength, _ := w.sampleDB.WavelengthBin(cw)
					curQuery := w.camera.ImageToRay(cr, w.imgRows, cc, w.imgCols, w.rng)

					// We get a power density sample in W / m^2
					sampledPower := w.scene.SampleRay(curQuery, curWavelength, w.rng, w.maxDepth)
					w.sampleDB.RecordSample(r, c, cw, sampledPower)
					samplesCollected++
				}
			}
		}

		w.progressFunction(samplesCollected)
		samplesCollected = 0
	}
}

type RenderOptions struct {
	MaxDepth         int
	TargetSubsamples int
}

type ProgressFunction func(int, int)

func RenderScene(scene *Scene, options *RenderOptions, sampleDB *SpectralImage,
	camera Camera, progressFunction ProgressFunction) {
	curProgress := 0

	// progressMutex locks both curProgress and sampleDB.
	progressMutex := sync.Mutex{}

	// Count the total number of samples recorded in sample_db.  When we
	// resume a render, we don't want to just repeat our same RNG choices
	// again!
	existingSamples := 0
	for i := 0; i < len(sampleDB.PowerDensityCounts); i++ {
		existingSamples += int(sampleDB.PowerDensityCounts[i])
	}

	// Count the number of samples we want to have at the end of the render, for
	// reporting progress.
	wantSamples := options.TargetSubsamples * sampleDB.RowSize * sampleDB.ColSize * sampleDB.WavelengthSize
	totalSamples := 0
	if wantSamples > existingSamples {
		totalSamples = wantSamples - existingSamples
	}

	processorCount := runtime.NumCPU()

	// We chunk work by rows.
	workUnit := sampleDB.RowSize / processorCount

	var wg sync.WaitGroup
	for i := 0; i < processorCount; i++ {
		rowLim := (i + 1) * workUnit
		if rowLim > sampleDB.RowSize {
			rowLim = sampleDB.RowSize
		}

		worker := &ChunkWorker{
			// TODO(ahmedtd): Think about how to make this more repeatable.
			rng: rand.New(rand.NewSource(int64(existingSamples))),
			progressFunction: func(subProgress int) {
				progressMutex.Lock()
				defer progressMutex.Unlock()
				curProgress += subProgress
				progressFunction(curProgress, totalSamples)
			},
			maxDepth:      options.MaxDepth,
			targetSamples: options.TargetSubsamples,
			imgRows:       sampleDB.RowSize,
			imgCols:       sampleDB.ColSize,
			rowSrc:        i * workUnit,
			rowLim:        rowLim,
			colSrc:        0,
			colLim:        sampleDB.ColSize,
			camera:        camera,
			scene:         scene,
		}
		worker.sampleDB = sampleDB.Cut(worker.rowSrc, worker.rowLim, 0, sampleDB.ColSize)

		wg.Add(1)
		go func() {
			defer wg.Done()
			worker.Render()

			progressMutex.Lock()
			defer progressMutex.Unlock()

			sampleDB.Paste(worker.sampleDB, worker.rowSrc, worker.colSrc)
		}()
	}

	wg.Wait()
}

var (
	outputFile     = flag.String("output-file", "output.spectral", "Output spectral sample db")
	outputRows     = flag.Int("output-rows", 512, "Output image rows")
	outputCols     = flag.Int("output-cols", 768, "Output image columns")
	wavelengthBins = flag.Int("wavelength-bins", 25, "Output wavelength bins")
	wavelengthMin  = flag.Float64("wavelength-min", 390.0, "Output wavelength min")
	wavelengthMax  = flag.Float64("wavelength-max", 935.0, "Output wavelength max")

	renderTargetSubsamples = flag.Int("render-target-subsamples", 4, "Number of subsamples to collect from each pixel and frequency bin")
	renderMaxDepth         = flag.Int("render-max-depth", 8, "Maximum number of bounces to consider")

	resume = flag.Bool("resume", false, "Should we re-open the output file to add more samples?")

	cpuprofile = flag.String("cpu-profile", "", "write cpu profile to `file`")
	memprofile = flag.String("mem-profile", "", "write memory profile to `file`")
)

func main() {
	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	if err := do(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		//runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
}

func do() error {
	options := &RenderOptions{
		MaxDepth:         *renderMaxDepth,
		TargetSubsamples: *renderTargetSubsamples,
	}

	var sampleDB *SpectralImage
	if *resume {
		var err error
		sampleDB, err = ReadSpectralImageFromFile(*outputFile)
		if err != nil {
			return fmt.Errorf("resumption requested, but encountered error loading existing file: %w", err)
		}

		if sampleDB.RowSize != *outputRows {
			return fmt.Errorf("resumption requested, but the existing spectral image doesn't have the right number of rows (got %d, want %d)", sampleDB.RowSize, *outputRows)
		}

		if sampleDB.ColSize != *outputCols {
			return fmt.Errorf("resumption requested, but the existing spectral image doesn't have the right number of columns (got %d, want %d)", sampleDB.ColSize, *outputCols)
		}

		if sampleDB.WavelengthSize != *wavelengthBins {
			return fmt.Errorf("resumption requested, but the existing spectral image doesn't have the right number of wavelength bins (got %d, want %d)", sampleDB.WavelengthSize, *wavelengthBins)
		}

		if sampleDB.WavelengthMin != float32(*wavelengthMin) {
			return fmt.Errorf("resumption requested, but the existing spectral image doesn't have the right wavelength min (got %v, want %v)", sampleDB.WavelengthMin, float32(*wavelengthMin))
		}

		if sampleDB.WavelengthMax != float32(*wavelengthMax) {
			return fmt.Errorf("resumption requested, but the existing spectral image doesn't have the right wavelength max (got %v, want %v)", sampleDB.WavelengthMax, float32(*wavelengthMax))
		}
	} else {
		// Check that the output file doesn't exist, to avoid blowing away hours
		// of render time.
		_, err := os.Stat(*outputFile)
		if err == nil {
			return fmt.Errorf("resumption not requested, but output file exists")
		}

		sampleDB = &SpectralImage{
			WavelengthMin: float32(*wavelengthMin),
			WavelengthMax: float32(*wavelengthMax),
		}
		sampleDB.Resize(*outputRows, *outputCols, *wavelengthBins)
	}

	scene := &Scene{}

	cieD65Emitter := &Emitter{
		ConstantSpectrum(densesignal.CIED65Emission(300)),
	}
	cieAEmitter := &Emitter{
		ConstantSpectrum(densesignal.CIEAEmission(100)),
	}
	matte := &GaussianRoughNonConductive{
		Variance: ConstantScalar(0.5),
	}
	matte2 := &GaussianRoughNonConductive{
		Variance: ConstantScalar(0.05),
	}
	glass := &NonConductiveSmooth{
		InteriorIndexOfRefraction: ConstantSpectrum(densesignal.VisibleSpectrumRamp(1.7, 1.5)),
		ExteriorIndexOfRefraction: ConstantScalar(1.0),
	}

	sphere := &Sphere{}
	centerBox := &Box{[3]ray.Span{ray.Span{0, 0.5}, ray.Span{0, 0.5}, ray.Span{0, 0.5}}}
	ground := &Box{[3]ray.Span{ray.Span{0, 10.1}, ray.Span{0, 10.1}, ray.Span{-0.5, 0}}}
	roof := &Box{[3]ray.Span{ray.Span{0, 10.1}, ray.Span{0, 10.1}, ray.Span{10, 10.1}}}
	wallN := &Box{[3]ray.Span{ray.Span{0, 10}, ray.Span{10, 10.1}, ray.Span{0, 10}}}
	wallW := &Box{[3]ray.Span{ray.Span{-0.1, 0}, ray.Span{0, 10}, ray.Span{0, 10}}}
	wallS := &Box{[3]ray.Span{ray.Span{0, 10}, ray.Span{-0.1, 0}, ray.Span{0, 10}}}

	scene.InfinityMaterial = cieD65Emitter
	scene.Elements = []*SceneElement{
		&SceneElement{
			TheGeometry:  sphere,
			TheMaterial:  cieAEmitter,
			ModelToWorld: affinetransform.Compose(affinetransform.Translate(vec3.T{5, 4, 0}), affinetransform.Scale(1)),
		},
		&SceneElement{
			TheGeometry:  sphere,
			TheMaterial:  matte,
			ModelToWorld: affinetransform.Compose(affinetransform.Translate(vec3.T{5, 6, 0}), affinetransform.Scale(1)),
		},
		&SceneElement{
			TheGeometry:  ground,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		&SceneElement{
			TheGeometry:  roof,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		&SceneElement{
			TheGeometry:  wallN,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		&SceneElement{
			TheGeometry:  wallW,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		&SceneElement{
			TheGeometry:  wallS,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		&SceneElement{
			TheGeometry:  centerBox,
			TheMaterial:  glass,
			ModelToWorld: affinetransform.Translate(vec3.T{3, 3, 0}),
		},
	}

	scene.Crush(0.0)

	camera := &PinholeCamera{
		Center:          vec3.T{1, 1, 2},
		ApertureToWorld: mat33.T{Elts: [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}},
		Aperture:        vec3.T{0.02, 0.018, 0.012},
	}
	camera.SetEye(vec3.SubVV(vec3.T{5, 5, 1}, camera.Center))

	progress := func(cur, tot int) {
		fmt.Fprintf(os.Stderr, "\r%d/%d %d%%", cur, tot, 100*cur/tot)
	}

	RenderScene(scene, options, sampleDB, camera, progress)
	fmt.Fprintf(os.Stderr, "\n")

	out, err := os.Create(*outputFile)
	if err != nil {
		return fmt.Errorf("while opening output file: %w", err)
	}
	defer out.Close()

	if err := WriteSpectralImage(sampleDB, out); err != nil {
		return fmt.Errorf("while writing spectral image: %w", err)
	}

	return nil
}
