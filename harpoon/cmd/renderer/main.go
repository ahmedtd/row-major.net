package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"

	"row-major/harpoon/affinetransform"
	"row-major/harpoon/camera"
	"row-major/harpoon/densesignal"
	"row-major/harpoon/geometry"
	"row-major/harpoon/material"
	"row-major/harpoon/ray"
	"row-major/harpoon/scene"
	"row-major/harpoon/spectralimage"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec3"
)

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
	options := &scene.RenderOptions{
		MaxDepth:         *renderMaxDepth,
		TargetSubsamples: *renderTargetSubsamples,
	}

	var sampleDB *spectralimage.SpectralImage
	if *resume {
		var err error
		sampleDB, err = spectralimage.ReadSpectralImageFromFile(*outputFile)
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

		sampleDB = &spectralimage.SpectralImage{
			WavelengthMin: float32(*wavelengthMin),
			WavelengthMax: float32(*wavelengthMax),
		}
		sampleDB.Resize(*outputRows, *outputCols, *wavelengthBins)
	}

	theScene := &scene.Scene{}

	cieD65Emitter := &material.Emitter{
		material.ConstantSpectrum(densesignal.CIED65Emission(300)),
	}
	cieAEmitter := &material.Emitter{
		material.ConstantSpectrum(densesignal.CIEAEmission(100)),
	}
	matte := &material.GaussianRoughNonConductive{
		Variance: material.ConstantScalar(0.5),
	}
	matte2 := &material.GaussianRoughNonConductive{
		Variance: material.ConstantScalar(0.05),
	}
	glass := &material.NonConductiveSmooth{
		InteriorIndexOfRefraction: material.ConstantSpectrum(densesignal.VisibleSpectrumRamp(1.7, 1.5)),
		ExteriorIndexOfRefraction: material.ConstantScalar(1.0),
	}

	sphere := &geometry.Sphere{}
	centerBox := &geometry.Box{[3]ray.Span{ray.Span{0, 0.5}, ray.Span{0, 0.5}, ray.Span{0, 0.5}}}
	ground := &geometry.Box{[3]ray.Span{ray.Span{0, 10.1}, ray.Span{0, 10.1}, ray.Span{-0.5, 0}}}
	roof := &geometry.Box{[3]ray.Span{ray.Span{0, 10.1}, ray.Span{0, 10.1}, ray.Span{10, 10.1}}}
	wallN := &geometry.Box{[3]ray.Span{ray.Span{0, 10}, ray.Span{10, 10.1}, ray.Span{0, 10}}}
	wallW := &geometry.Box{[3]ray.Span{ray.Span{-0.1, 0}, ray.Span{0, 10}, ray.Span{0, 10}}}
	wallS := &geometry.Box{[3]ray.Span{ray.Span{0, 10}, ray.Span{-0.1, 0}, ray.Span{0, 10}}}

	theScene.InfinityMaterial = cieD65Emitter
	theScene.Elements = []*scene.SceneElement{
		{
			TheGeometry:  sphere,
			TheMaterial:  cieAEmitter,
			ModelToWorld: affinetransform.Compose(affinetransform.Translate(vec3.T{5, 4, 0}), affinetransform.Scale(1)),
		},
		{
			TheGeometry:  sphere,
			TheMaterial:  matte,
			ModelToWorld: affinetransform.Compose(affinetransform.Translate(vec3.T{5, 6, 0}), affinetransform.Scale(1)),
		},
		{
			TheGeometry:  ground,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		{
			TheGeometry:  roof,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		{
			TheGeometry:  wallN,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		{
			TheGeometry:  wallW,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		{
			TheGeometry:  wallS,
			TheMaterial:  matte2,
			ModelToWorld: affinetransform.Identity(),
		},
		{
			TheGeometry:  centerBox,
			TheMaterial:  glass,
			ModelToWorld: affinetransform.Translate(vec3.T{3, 3, 0}),
		},
	}

	theScene.Crush(0.0)

	camera := &camera.PinholeCamera{
		Center:          vec3.T{1, 1, 2},
		ApertureToWorld: mat33.T{Elts: [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}},
		Aperture:        vec3.T{0.02, 0.018, 0.012},
	}
	camera.SetEye(vec3.SubVV(vec3.T{5, 5, 1}, camera.Center))

	progress := func(cur, tot int) {
		fmt.Fprintf(os.Stderr, "\r%d/%d %d%%", cur, tot, 100*cur/tot)
	}

	scene.RenderScene(theScene, options, sampleDB, camera, progress)
	fmt.Fprintf(os.Stderr, "\n")

	out, err := os.Create(*outputFile)
	if err != nil {
		return fmt.Errorf("while opening output file: %w", err)
	}
	defer out.Close()

	if err := spectralimage.WriteSpectralImage(sampleDB, out); err != nil {
		return fmt.Errorf("while writing spectral image: %w", err)
	}

	return nil
}
