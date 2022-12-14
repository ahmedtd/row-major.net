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
	"row-major/harpoon/spectralimage/headerproto"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec2"
	"row-major/harpoon/vmath/vec3"

	"google.golang.org/protobuf/proto"
)

type DenseSignal struct {
	SrcX    float32
	LimX    float32
	Samples []float32
}

func (d *DenseSignal) StepX() float32 {
	return (d.LimX - d.SrcX) / float32(len(d.Samples))
}

func (d *DenseSignal) Interpolate(x float32) float32 {
	if x < d.SrcX || d.LimX < x {
		return 0.0
	}
	return d.Samples[int((x-d.SrcX)/d.StepX())]
}

func (d *DenseSignal) Integrate(from, to float32) float32 {
	accum := float32(0)

	curIndex := 0
	curX := d.SrcX
	nextX := d.SrcX + d.StepX()
	if d.SrcX < from {
		curX = from
		curIndex = int((from - d.SrcX) / d.StepX())
		nextX = d.SrcX + float32(curIndex+1)*d.StepX()
	}

	for curIndex < len(d.Samples) && curX < to {
		if nextX < to {
			width := nextX - curX
			accum += d.Samples[curIndex] * width
			curX = nextX
			nextX += d.StepX()
			curIndex++
		} else {
			width := to - curX
			accum += d.Samples[curIndex] * width
			curX = to
		}
	}

	return accum
}

func (d *DenseSignal) MulS(s float32) {
	for i := range d.Samples {
		d.Samples[i] *= s
	}
}

func (d *DenseSignal) DivS(s float32) {
	for i := range d.Samples {
		d.Samples[i] /= s
	}
}

func (d *DenseSignal) Normalize() {
	total := d.Integrate(d.SrcX, d.LimX)
	d.DivS(total)
}

func VisibleSpectrumSignal() *DenseSignal {
	return &DenseSignal{
		SrcX:    390.0,
		LimX:    835.0,
		Samples: make([]float32, 89),
	}
}

func VisibleSpectrumPulse(from, to, val float32) *DenseSignal {
	s := &DenseSignal{
		SrcX:    390.0,
		LimX:    835.0,
		Samples: make([]float32, 89),
	}

	x := s.SrcX
	for i := range s.Samples {
		if from <= x && x < to {
			s.Samples[i] = val
		}
		x += s.StepX()
	}

	return s
}

func VisibleSpectrumRamp(from, to float32) *DenseSignal {
	s := &DenseSignal{
		SrcX:    390.0,
		LimX:    835.0,
		Samples: make([]float32, 89),
	}

	for i := range s.Samples {
		t := float32(i) / float32(89)
		s.Samples[i] = (1-t)*from + t*to
	}

	return s
}

// Red returns a visible spectrum pulse suitable for interpreting red.
//
// Often, input data will be specified in an RGB color space, but there are an
// infinite number of specta that correspond to any given RGB triple.  This
// function gives an arbitrary but suitable choice for the red component.
func Red(val float32) *DenseSignal {
	return VisibleSpectrumPulse(620, 750, val)
}

// Green returns a visible spectrum pulse suitable for interpreting green.
//
// Often, input data will be specified in an RGB color space, but there are an
// infinite number of specta that correspond to any given RGB triple.  This
// function gives an arbitrary but suitable choice for the green component.
func Green(val float32) *DenseSignal {
	return VisibleSpectrumPulse(420, 495, val)
}

// Blue returns a visible spectrum pulse suitable for interpreting blue.
//
// Often, input data will be specified in an RGB color space, but there are an
// infinite number of specta that correspond to any given RGB triple.  This
// function gives an arbitrary but suitable choice for the blue component.
func Blue(val float32) *DenseSignal {
	return VisibleSpectrumPulse(495, 570, val)
}

func CIE2006X() *DenseSignal {
	return &DenseSignal{
		SrcX: 390,
		LimX: 835,
		Samples: []float32{
			0.003769647, 0.009382967, 0.02214302, 0.04742986, 0.08953803,
			0.1446214, 0.2035729, 0.2488523, 0.2918246, 0.3227087,
			0.3482554, 0.3418483, 0.3224637, 0.2826646, 0.2485254,
			0.2219781, 0.1806905, 0.129192, 0.08182895, 0.04600865,
			0.02083981, 0.007097731, 0.002461588, 0.003649178, 0.01556989,
			0.04315171, 0.07962917, 0.1268468, 0.1818026, 0.2405015,
			0.3098117, 0.3804244, 0.4494206, 0.5280233, 0.6133784,
			0.7016774, 0.796775, 0.8853376, 0.9638388, 1.051011,
			1.109767, 1.14362, 1.151033, 1.134757, 1.083928,
			1.007344, 0.9142877, 0.8135565, 0.6924717, 0.575541,
			0.4731224, 0.3844986, 0.2997374, 0.2277792, 0.1707914,
			0.1263808, 0.09224597, 0.0663996, 0.04710606, 0.03292138,
			0.02262306, 0.01575417, 0.01096778, 0.00760875, 0.005214608,
			0.003569452, 0.002464821, 0.001703876, 0.001186238, 0.0008269535,
			0.0005758303, 0.0004058303, 0.0002856577, 0.0002021853, 0.000143827,
			0.0001024685, 7.347551e-005, 5.25987e-005, 3.806114e-005, 2.758222e-005,
			2.004122e-005, 1.458792e-005, 1.068141e-005, 7.857521e-006, 5.768284e-006,
			4.259166e-006, 3.167765e-006, 2.358723e-006, 1.762465e-006,
		},
	}
}

func CIE2006Y() *DenseSignal {
	return &DenseSignal{
		SrcX: 390,
		LimX: 835,
		Samples: []float32{
			0.0004146161, 0.001059646, 0.002452194, 0.004971717, 0.00907986,
			0.01429377, 0.02027369, 0.02612106, 0.03319038, 0.0415794,
			0.05033657, 0.05743393, 0.06472352, 0.07238339, 0.08514816,
			0.1060145, 0.1298957, 0.1535066, 0.1788048, 0.2064828,
			0.237916, 0.285068, 0.3483536, 0.4277595, 0.5204972,
			0.6206256, 0.718089, 0.7946448, 0.8575799, 0.9071347,
			0.9544675, 0.9814106, 0.9890228, 0.9994608, 0.9967737,
			0.9902549, 0.9732611, 0.9424569, 0.8963613, 0.8587203,
			0.8115868, 0.7544785, 0.6918553, 0.6270066, 0.5583746,
			0.489595, 0.4229897, 0.3609245, 0.2980865, 0.2416902,
			0.1943124, 0.1547397, 0.119312, 0.08979594, 0.06671045,
			0.04899699, 0.03559982, 0.02554223, 0.01807939, 0.01261573,
			0.008661284, 0.006027677, 0.004195941, 0.002910864, 0.001995557,
			0.001367022, 0.0009447269, 0.000653705, 0.000455597, 0.0003179738,
			0.0002217445, 0.0001565566, 0.0001103928, 7.827442e-005, 5.578862e-005,
			3.981884e-005, 2.860175e-005, 2.051259e-005, 1.487243e-005, 0.0000108,
			7.86392e-006, 5.736935e-006, 4.211597e-006, 3.106561e-006, 2.286786e-006,
			1.693147e-006, 1.262556e-006, 9.422514e-007, 7.05386e-007,
		},
	}
}

func CIE2006Z() *DenseSignal {
	return &DenseSignal{
		SrcX: 390,
		LimX: 835,
		Samples: []float32{
			0.0184726, 0.04609784, 0.109609, 0.2369246, 0.4508369,
			0.7378822, 1.051821, 1.305008, 1.552826, 1.74828,
			1.917479, 1.918437, 1.848545, 1.664439, 1.522157,
			1.42844, 1.25061, 0.9991789, 0.7552379, 0.5617313,
			0.4099313, 0.3105939, 0.2376753, 0.1720018, 0.1176796,
			0.08283548, 0.05650407, 0.03751912, 0.02438164, 0.01566174,
			0.00984647, 0.006131421, 0.003790291, 0.002327186, 0.001432128,
			0.0008822531, 0.0005452416, 0.0003386739, 0.0002117772, 0.0001335031,
			8.494468e-005, 5.460706e-005, 3.549661e-005, 2.334738e-005, 1.554631e-005,
			1.048387e-005, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0, 0.0,
			0.0, 0.0, 0.0, 0.0,
		},
	}
}

func Sunlight() *DenseSignal {
	return &DenseSignal{
		SrcX: 390,
		LimX: 835,
		Samples: []float32{
			1.247, 1.019, 1.026, 0.855, 1.522, 1.682, 1.759, 1.674, 1.589, 1.735,
			1.532, 1.789, 1.737, 1.842, 1.684, 1.757, 1.582, 1.767, 1.698, 1.587,
			1.135, 1.646, 1.670, 1.929, 1.567, 1.713, 1.980, 1.973, 1.891, 1.973,
			2.144, 1.941, 1.979, 2.077, 1.971, 2.040, 2.104, 1.976, 1.921, 1.994,
			1.877, 2.041, 2.051, 1.956, 2.009, 2.035, 2.023, 1.969, 1.625, 1.914,
			2.007, 1.896, 2.058, 2.017, 1.866, 1.857, 1.894, 1.869, 1.961, 1.919,
			1.947, 1.867, 1.874, 1.669, 1.654, 1.831, 1.823, 1.958, 1.674, 1.897,
			1.952, 1.770, 1.858, 1.871, 1.904, 1.769, 1.825, 1.879, 1.879, 1.863,
			1.862, 1.846, 1.898, 1.821, 1.787, 1.843, 1.850, 1.854, 1.829, 1.810,
			1.769, 1.892, 1.867, 1.846, 1.783, 1.838, 1.873, 1.860, 1.830, 1.750,
			1.813, 1.808, 1.773, 1.805, 1.757, 1.746, 1.719, 1.776, 1.759, 1.743,
			1.703, 1.705, 1.713, 1.609, 1.724, 1.734, 1.713, 1.656, 1.697, 1.697,
			1.639, 1.651, 1.656, 1.654, 1.651, 1.614, 1.621, 1.627, 1.603, 1.558,
			1.606, 1.599, 1.532, 1.384, 1.549, 1.571, 1.555, 1.560, 1.535, 1.546,
			1.516, 1.521, 1.510, 1.508, 1.498, 1.492, 1.479, 1.455, 1.467, 1.461,
			1.448, 1.448, 1.436, 1.416, 1.425, 1.386, 1.388, 1.415, 1.400, 1.384,
			1.385, 1.373, 1.366, 1.354, 1.328, 1.331, 1.348, 1.350, 1.346, 1.319,
			1.326, 1.318, 1.309, 1.307, 1.278, 1.258, 1.286, 1.279, 1.283, 1.270,
			1.262, 1.259, 1.255, 1.248, 1.240, 1.237, 1.241, 1.221, 1.185, 1.203,
			1.204, 1.208, 1.188, 1.196, 1.187, 1.187, 1.176, 1.180, 1.177, 1.174,
			1.158, 1.143, 1.134, 1.152, 1.135, 1.142, 1.129, 1.115, 1.120, 1.095,
			1.114, 1.115, 1.107, 1.104, 1.063, 1.080, 1.073, 1.075, 1.080, 1.081,
			1.063, 1.051, 1.041,
		},
	}
}

func CIEA() *DenseSignal {
	// TODO: Double-check these limits.
	return &DenseSignal{
		SrcX: 300,
		LimX: 785,
		Samples: []float32{
			0.930483, 1.128210, 1.357690, 1.622190, 1.925080, 2.269800,
			2.659810, 3.098610, 3.589680, 4.136480, 4.742380, 5.410700,
			6.144620, 6.947200, 7.821350, 8.769800, 9.795100, 10.899600,
			12.085300, 13.354300, 14.708000, 16.148000, 17.675300, 19.290700,
			20.995000, 22.788300, 24.670900, 26.642500, 28.702700, 30.850800,
			33.085900, 35.406800, 37.812100, 40.300200, 42.869300, 45.517400,
			48.242300, 51.041800, 53.913200, 56.853900, 59.861100, 62.932000,
			66.063500, 69.252500, 72.495900, 75.790300, 79.132600, 82.519300,
			85.947000, 89.412400, 92.912000, 96.442300, 100.000000, 103.582000,
			107.184000, 110.803000, 114.436000, 118.080000, 121.731000, 125.386000,
			129.043000, 132.697000, 136.346000, 139.988000, 143.618000, 147.235000,
			150.836000, 154.418000, 157.979000, 161.516000, 165.028000, 168.510000,
			171.963000, 175.383000, 178.769000, 182.118000, 185.429000, 188.701000,
			191.931000, 195.118000, 198.261000, 201.359000, 204.409000, 207.411000,
			210.365000, 213.268000, 216.120000, 218.920000, 221.667000, 224.361000,
			227.000000, 229.585000, 232.115000, 234.589000, 237.008000, 239.370000,
			241.675000,
		},
	}
}

// CIEAEmission is CIE A, but normalized so that it has an integrated power of
// X.
func CIEAEmission(x float32) *DenseSignal {
	sig := CIEA()
	sig.Normalize()
	sig.MulS(x)
	return sig
}

func CIED65() *DenseSignal {
	// TODO: Double-check these limits...
	return &DenseSignal{
		SrcX: 300,
		LimX: 835,
		Samples: []float32{0.034100, 1.664300, 3.294500, 11.765200, 20.236000, 28.644700,
			37.053500, 38.501100, 39.948800, 42.430200, 44.911700, 45.775000,
			46.638300, 49.363700, 52.089100, 51.032300, 49.975500, 52.311800,
			54.648200, 68.701500, 82.754900, 87.120400, 91.486000, 92.458900,
			93.431800, 90.057000, 86.682300, 95.773600, 104.865000, 110.936000,
			117.008000, 117.410000, 117.812000, 116.336000, 114.861000, 115.392000,
			115.923000, 112.367000, 108.811000, 109.082000, 109.354000, 108.578000,
			107.802000, 106.296000, 104.790000, 106.239000, 107.689000, 106.047000,
			104.405000, 104.225000, 104.046000, 102.023000, 100.000000, 98.167100,
			96.334200, 96.061100, 95.788000, 92.236800, 88.685600, 89.345900,
			90.006200, 89.802600, 89.599100, 88.648900, 87.698700, 85.493600,
			83.288600, 83.493900, 83.699200, 81.863000, 80.026800, 80.120700,
			80.214600, 81.246200, 82.277800, 80.281000, 78.284200, 74.002700,
			69.721300, 70.665200, 71.609100, 72.979000, 74.349000, 67.976500,
			61.604000, 65.744800, 69.885600, 72.486300, 75.087000, 69.339800,
			63.592700, 55.005400, 46.418200, 56.611800, 66.805400, 65.094100,
			63.382800, 63.843400, 64.304000, 61.877900, 59.451900, 55.705400,
			51.959000, 54.699800, 57.440600, 58.876500, 60.312500,
		},
	}
}

func CIED65Emission(x float32) *DenseSignal {
	sig := CIED65()
	sig.Normalize()
	sig.MulS(x)
	return sig
}

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

type Camera interface {
	ImageToRay(curRow, imgRows, curCol, imgCols int, rng *rand.Rand) Ray
}

type PinholeCamera struct {
	Center          vec3.T
	ApertureToWorld mat33.T
	Aperture        vec3.T
}

func (c *PinholeCamera) ImageToRay(curRow, imgRows, curCol, imgCols int, rng *rand.Rand) Ray {
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

	return Ray{
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
	X, Y, Z Span
}

func AccumZeroAABox() AABox {
	return AABox{
		X: Span{math.Inf(1), math.Inf(-1)},
		Y: Span{math.Inf(1), math.Inf(-1)},
		Z: Span{math.Inf(1), math.Inf(-1)},
	}
}

func MinContainingAABox(a, b AABox) AABox {
	return AABox{
		X: MinContainingSpan(a.X, b.X),
		Y: MinContainingSpan(a.Y, b.Y),
		Z: MinContainingSpan(a.Z, b.Z),
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

func RayTestAABox(r RaySegment, b AABox) Span {
	cover := Span{math.Inf(-1), math.Inf(1)}

	coverX := Span{
		(b.X.Lo - r.TheRay.Point[0]) / r.TheRay.Slope[0],
		(b.X.Hi - r.TheRay.Point[0]) / r.TheRay.Slope[0],
	}
	if coverX.Hi < coverX.Lo {
		coverX.Lo, coverX.Hi = coverX.Hi, coverX.Lo
	}
	if !SpanOverlaps(cover, coverX) {
		return NaNSpan()
	}
	if coverX.Lo > cover.Lo {
		cover.Lo = coverX.Lo
	}
	if coverX.Hi < cover.Hi {
		cover.Hi = coverX.Hi
	}

	coverY := Span{
		(b.Y.Lo - r.TheRay.Point[1]) / r.TheRay.Slope[1],
		(b.Y.Hi - r.TheRay.Point[1]) / r.TheRay.Slope[1],
	}
	if coverY.Hi < coverY.Lo {
		coverY.Lo, coverY.Hi = coverY.Hi, coverY.Lo
	}
	if !SpanOverlaps(cover, coverY) {
		return NaNSpan()
	}
	if coverY.Lo > cover.Lo {
		cover.Lo = coverY.Lo
	}
	if coverY.Hi < cover.Hi {
		cover.Hi = coverY.Hi
	}

	coverZ := Span{
		(b.Z.Lo - r.TheRay.Point[2]) / r.TheRay.Slope[2],
		(b.Z.Hi - r.TheRay.Point[2]) / r.TheRay.Slope[2],
	}
	if coverZ.Hi < coverZ.Lo {
		coverZ.Lo, coverZ.Hi = coverZ.Hi, coverZ.Lo
	}
	if !SpanOverlaps(cover, coverZ) {
		return NaNSpan()
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
	R    Ray
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
	RayInto(query RaySegment) Contact
	RayExit(query RaySegment) Contact
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
		X: Span{Lo: -1.0, Hi: 1.0},
		Y: Span{Lo: -1.0, Hi: 1.0},
		Z: Span{Lo: -1.0, Hi: 1.0},
	}
}

func (s *Sphere) Crush(time float64) {
	// Nothing to do.
}

func (s *Sphere) RayInto(query RaySegment) Contact {
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

func (s *Sphere) RayExit(query RaySegment) Contact {
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
	Spans [3]Span
}

func (b *Box) GetAABox() AABox {
	return AABox{
		X: Span{b.Spans[0].Lo, b.Spans[0].Hi},
		Y: Span{b.Spans[1].Lo, b.Spans[1].Hi},
		Z: Span{b.Spans[2].Lo, b.Spans[2].Hi},
	}
}

func (b *Box) Crush(time float64) {}

func (b *Box) RayInto(query RaySegment) Contact {
	cover := Span{math.Inf(-1), math.Inf(1)}

	hitAxis := [3]float64{}

	for i := 0; i < 3; i++ {
		cur := Span{
			(b.Spans[i].Lo - query.TheRay.Point[i]) / query.TheRay.Slope[i],
			(b.Spans[i].Hi - query.TheRay.Point[i]) / query.TheRay.Slope[i],
		}

		normalComponent := -1.0
		if cur.Hi < cur.Lo {
			cur.Hi, cur.Lo = cur.Lo, cur.Hi
			normalComponent = 1.0
		}

		if !SpanOverlaps(cover, cur) {
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

	if !SpanOverlaps(cover, query.TheSegment) {
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

func (b *Box) RayExit(query RaySegment) Contact {
	cover := Span{math.Inf(-1), math.Inf(1)}

	// I'm beginning to regret choosing the X,Y,Z convention for vectors.  Lots
	// of things are easier if the axes can be indexed.
	point := [3]float64{query.TheRay.Point[0], query.TheRay.Point[1], query.TheRay.Point[2]}
	slope := [3]float64{query.TheRay.Slope[0], query.TheRay.Slope[1], query.TheRay.Slope[2]}

	hitAxis := [3]float64{}

	for i := 0; i < 3; i++ {
		cur := Span{
			(b.Spans[i].Lo - point[i]) / slope[i],
			(b.Spans[i].Hi - point[i]) / slope[i],
		}

		normalComponent := 1.0
		if cur.Hi < cur.Lo {
			cur.Hi, cur.Lo = cur.Lo, cur.Hi
			normalComponent = -1.0
		}

		if !SpanOverlaps(cover, cur) {
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

	if !SpanOverlaps(cover, query.TheSegment) {
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

func ConstantSpectrum(spectrum *DenseSignal) MaterialMap {
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
	IncidentRay  Ray
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
		IncidentRay: Ray{
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
			IncidentRay: Ray{
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
			IncidentRay: Ray{
				Point: contact.P,
				Slope: vec3.Reflect(contact.R.Slope, contact.N),
			},
		}
	} else {
		// Give the ray that contributed by refraction.
		return ShadeInfo{
			EmittedPower: 0.0,
			PropagationK: 1.0,
			IncidentRay: Ray{
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
		IncidentRay: Ray{
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
		IncidentRay: Ray{
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

func (s *Scene) SceneRayIntersect(worldQuery RaySegment) (Contact, int) {
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

func (s *Scene) ShadeRay(reflectedRay Ray, curWavelength float32, rng *rand.Rand) ShadeInfo {
	reflectedQuery := RaySegment{
		TheRay:     reflectedRay,
		TheSegment: Span{0.0001, math.Inf(1)},
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

func (s *Scene) SampleRay(initialQuery Ray, curWavelength float32, rng *rand.Rand, depthLim int) float32 {
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
		ConstantSpectrum(CIED65Emission(300)),
	}
	cieAEmitter := &Emitter{
		ConstantSpectrum(CIEAEmission(100)),
	}
	matte := &GaussianRoughNonConductive{
		Variance: ConstantScalar(0.5),
	}
	matte2 := &GaussianRoughNonConductive{
		Variance: ConstantScalar(0.05),
	}
	glass := &NonConductiveSmooth{
		InteriorIndexOfRefraction: ConstantSpectrum(VisibleSpectrumRamp(1.7, 1.5)),
		ExteriorIndexOfRefraction: ConstantScalar(1.0),
	}

	sphere := &Sphere{}
	centerBox := &Box{[3]Span{Span{0, 0.5}, Span{0, 0.5}, Span{0, 0.5}}}
	ground := &Box{[3]Span{Span{0, 10.1}, Span{0, 10.1}, Span{-0.5, 0}}}
	roof := &Box{[3]Span{Span{0, 10.1}, Span{0, 10.1}, Span{10, 10.1}}}
	wallN := &Box{[3]Span{Span{0, 10}, Span{10, 10.1}, Span{0, 10}}}
	wallW := &Box{[3]Span{Span{-0.1, 0}, Span{0, 10}, Span{0, 10}}}
	wallS := &Box{[3]Span{Span{0, 10}, Span{-0.1, 0}, Span{0, 10}}}

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
