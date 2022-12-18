package kdtree

import (
	"math"
	"math/rand"
	"row-major/harpoon/aabox"
)

type KDElement struct {
	// A handle back into some other storage array.
	Ref int

	// The bounds of this element.
	Bounds aabox.AABox
}

type KDNode struct {
	Bounds aabox.AABox

	Elements []KDElement

	LoChild *KDNode
	HiChild *KDNode
}

func (cur *KDNode) refineViaSurfaceAreaHeuristic(splitCost, terminationThreshold float64, rng *rand.Rand) {
	bestObjective := math.Inf(1)
	bestLoBox := aabox.AABox{}
	bestHiBox := aabox.AABox{}
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

		loBox := aabox.AccumZeroAABox()
		for _, element := range precedingElements {
			loBox = aabox.MinContainingAABox(loBox, element.Bounds)
		}

		hiBox := aabox.AccumZeroAABox()
		for _, element := range precedingElements {
			hiBox = aabox.MinContainingAABox(hiBox, element.Bounds)
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

		loBox := aabox.AccumZeroAABox()
		for _, element := range precedingElements {
			loBox = aabox.MinContainingAABox(loBox, element.Bounds)
		}

		hiBox := aabox.AccumZeroAABox()
		for _, element := range precedingElements {
			hiBox = aabox.MinContainingAABox(hiBox, element.Bounds)
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

		loBox := aabox.AccumZeroAABox()
		for _, element := range precedingElements {
			loBox = aabox.MinContainingAABox(loBox, element.Bounds)
		}

		hiBox := aabox.AccumZeroAABox()
		for _, element := range precedingElements {
			hiBox = aabox.MinContainingAABox(hiBox, element.Bounds)
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

	maxBox := aabox.AccumZeroAABox()
	for _, element := range elements {
		maxBox = aabox.MinContainingAABox(maxBox, element.Bounds)
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

type KDSelector func(b aabox.AABox) bool
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
