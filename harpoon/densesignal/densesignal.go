package densesignal

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
