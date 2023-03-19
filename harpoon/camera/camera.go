package camera

import (
	"math/rand"
	"row-major/harpoon/ray"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec3"
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
		c.ApertureToWorld[0],
		c.ApertureToWorld[3],
		c.ApertureToWorld[6],
	}
}

func (c *PinholeCamera) Left() vec3.T {
	return vec3.T{
		c.ApertureToWorld[1],
		c.ApertureToWorld[4],
		c.ApertureToWorld[7],
	}
}

func (c *PinholeCamera) Up() vec3.T {
	return vec3.T{
		c.ApertureToWorld[2],
		c.ApertureToWorld[5],
		c.ApertureToWorld[8],
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
	c.ApertureToWorld[0] = newEye[0]
	c.ApertureToWorld[3] = newEye[1]
	c.ApertureToWorld[6] = newEye[2]
}

func (c *PinholeCamera) setLeftDirect(newLeft vec3.T) {
	c.ApertureToWorld[1] = newLeft[0]
	c.ApertureToWorld[4] = newLeft[1]
	c.ApertureToWorld[7] = newLeft[2]
}

func (c *PinholeCamera) setUpDirect(newUp vec3.T) {
	c.ApertureToWorld[2] = newUp[0]
	c.ApertureToWorld[5] = newUp[1]
	c.ApertureToWorld[8] = newUp[2]
}
