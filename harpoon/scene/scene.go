package scene

import (
	"math"
	"math/rand"
	"runtime"
	"sync"

	"row-major/harpoon/aabox"
	"row-major/harpoon/affinetransform"
	"row-major/harpoon/camera"
	"row-major/harpoon/contact"
	"row-major/harpoon/geometry"
	"row-major/harpoon/kdtree"
	"row-major/harpoon/material"
	"row-major/harpoon/ray"
	"row-major/harpoon/spectralimage"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec2"
	"row-major/harpoon/vmath/vec3"
)

type SceneElement struct {
	GeometryIndex int
	MaterialIndex int

	// The transform that takes model space to world space.
	ModelToWorld affinetransform.AffineTransform
}

type CrushedSceneElement struct {
	TheGeometry geometry.Geometry
	TheMaterial material.Material

	// The transform that takes a ray from world space to model space.
	WorldToModel affinetransform.AffineTransform

	// The transform that takes rays and contacts from model space to world
	// space.
	ModelToWorld affinetransform.AffineTransform

	// The linear map that takes normal vectors from model space to world space.
	ModelToWorldNormals mat33.T

	// The element's bounding box in world coordinates.
	WorldBounds aabox.AABox
}

type Scene struct {
	Geometries            []geometry.Geometry
	Materials             []material.Material
	InfinityMaterialIndex int

	Elements        []*SceneElement
	CrushedElements []*CrushedSceneElement

	Cameras []camera.Camera

	QueryAccelerator *kdtree.KDTree
}

// AddGeometry is a convenience function to register a geometry and get its
// index.
func (s *Scene) AddGeometry(g geometry.Geometry) int {
	s.Geometries = append(s.Geometries, g)
	return len(s.Geometries) - 1
}

// AddMaterial is a convenience function to register a material and get its
// index.
func (s *Scene) AddMaterial(m material.Material) int {
	s.Materials = append(s.Materials, m)
	return len(s.Materials) - 1
}

func (s *Scene) AddElement(e *SceneElement) int {
	s.Elements = append(s.Elements, e)
	return len(s.Elements) - 1
}

func (s *Scene) AddCamera(c camera.Camera) int {
	s.Cameras = append(s.Cameras, c)
	return len(s.Cameras) - 1
}

func (s *Scene) Crush(time float64) {
	// Geometry, materials, and material maps are crushed in a dependency-based
	// fashion, whith each crushing its own dependencies.  To prevent redundant
	// crushes, objects should cache whether they have been crushed at the given
	// key value (time).

	for _, g := range s.Geometries {
		g.Crush(time)
	}

	for _, m := range s.Materials {
		m.Crush(time)
	}

	kdElements := []kdtree.KDElement{}
	for i, element := range s.Elements {
		g := s.Geometries[element.GeometryIndex]
		m := s.Materials[element.MaterialIndex]

		worldBounds := g.GetAABox().Transform(element.ModelToWorld)

		crushedElement := &CrushedSceneElement{
			TheGeometry:         g,
			TheMaterial:         m,
			WorldToModel:        element.ModelToWorld.Invert(),
			ModelToWorld:        element.ModelToWorld,
			ModelToWorldNormals: element.ModelToWorld.NormalTransformMat(),
			WorldBounds:         worldBounds,
		}

		s.CrushedElements = append(s.CrushedElements, crushedElement)

		kdElements = append(kdElements, kdtree.KDElement{i, worldBounds})
	}

	s.QueryAccelerator = kdtree.NewKDTree(kdElements)
	s.QueryAccelerator.RefineViaSurfaceAreaHeuristic(1.0, 0.9)
}

func (s *Scene) SceneRayIntersect(worldQuery ray.RaySegment) (contact.Contact, int) {
	minContact := contact.Contact{}
	minElementIndex := -1

	selector := func(b aabox.AABox) bool {
		return !aabox.RayTestAABox(worldQuery, b).IsNaN()
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

func (s *Scene) ShadeRay(reflectedRay ray.Ray, curWavelength float32, rng *rand.Rand) material.ShadeInfo {
	reflectedQuery := ray.RaySegment{
		TheRay:     reflectedRay,
		TheSegment: ray.Span{0.0001, math.Inf(1)},
	}

	glbContact, hitIndex := s.SceneRayIntersect(reflectedQuery)
	if hitIndex == -1 {
		p := reflectedRay.Eval(math.Inf(1))
		c := contact.Contact{
			T:    math.Inf(1),
			R:    reflectedRay,
			P:    p,
			N:    vec3.MulVS(reflectedRay.Slope, -1.0),
			Mtl2: vec2.T{math.Atan2(reflectedRay.Slope[0], reflectedRay.Slope[1]), math.Acos(reflectedRay.Slope[2])},
			Mtl3: p,
		}
		return s.Materials[s.InfinityMaterialIndex].Shade(c, curWavelength, rng)
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

type ChunkWorker struct {
	sampleDB         *spectralimage.SpectralImage
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
					curQuery := w.scene.Cameras[0].ImageToRay(cr, w.imgRows, cc, w.imgCols, w.rng)

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

func RenderScene(scene *Scene, options *RenderOptions, sampleDB *spectralimage.SpectralImage, progressFunction ProgressFunction) {
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
