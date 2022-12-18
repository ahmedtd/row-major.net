package material

import (
	"math"
	"math/rand"
	"row-major/harpoon/contact"
	"row-major/harpoon/densesignal"
	"row-major/harpoon/ray"
	"row-major/harpoon/vmath/vec2"
	"row-major/harpoon/vmath/vec3"
)

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
	Shade(globalContact contact.Contact, freq float32, rng *rand.Rand) ShadeInfo
}

// DirectionalEmitter is an emitter that queries an emissivity material map
// based on direction of arrival.
//
// In effect, it acts as a window into an environment map.
type DirectionalEmitter struct {
	Emissivity MaterialMap
}

func (d *DirectionalEmitter) Crush(time float64) {}

func (d *DirectionalEmitter) Shade(contact contact.Contact, freq float32, rng *rand.Rand) ShadeInfo {
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

func (e *Emitter) Shade(contact contact.Contact, freq float32, rng *rand.Rand) ShadeInfo {
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

func (l *MonteCarloLambert) Shade(contact contact.Contact, freq float32, rng *rand.Rand) ShadeInfo {
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

func (n *NonConductiveSmooth) Shade(contact contact.Contact, freq float32, rng *rand.Rand) ShadeInfo {
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

func (p *PerfectlyConductiveSmooth) Shade(contact contact.Contact, freq float32, rng *rand.Rand) ShadeInfo {
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

func (g *GaussianRoughNonConductive) Shade(contact contact.Contact, freq float32, rng *rand.Rand) ShadeInfo {
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
