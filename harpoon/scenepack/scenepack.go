package scenepack

import (
	"fmt"
	"os"

	"row-major/harpoon/affinetransform"
	"row-major/harpoon/camera"
	"row-major/harpoon/densesignal"
	"row-major/harpoon/geometry"
	"row-major/harpoon/material"
	"row-major/harpoon/ray"
	"row-major/harpoon/scene"
	"row-major/harpoon/scenepack/headerproto"
	"row-major/harpoon/vmath/mat33"
	"row-major/harpoon/vmath/vec3"

	"google.golang.org/protobuf/proto"
)

func LoadScene(fileName string) (*scene.Scene, error) {
	fileBytes, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("while opening scenepack: %w", err)
	}

	protoScene := &headerproto.Scene{}
	if err := proto.Unmarshal(fileBytes, protoScene); err != nil {
		return nil, fmt.Errorf("while unmarshiling scenepack header: %w", err)
	}

	realScene := &scene.Scene{}

	for _, g := range protoScene.Geometry {
		switch {
		case g.Sphere != nil:

			realScene.AddGeometry(&geometry.Sphere{
				TheMaterialCoordsMode: convertMaterialCoordsMode(g.Sphere.MaterialCoordsMode),
			})

		case g.Box != nil:
			realScene.AddGeometry(&geometry.Box{
				Spans: [3]ray.Span{
					{Lo: g.Box.XLo, Hi: g.Box.XHi},
					{Lo: g.Box.YLo, Hi: g.Box.YHi},
					{Lo: g.Box.ZLo, Hi: g.Box.ZHi},
				},
			})
		}
	}

	// TODO: Figure out a good way to express material maps.
	for _, m := range protoScene.Material {
		switch {
		case m.Emitter != nil:
			realScene.AddMaterial(&material.Emitter{
				Emissivity: material.ConstantSpectrum(densesignal.CIED65Emission(300)),
			})
		case m.GaussianRoughNonConductive != nil:
			realScene.AddMaterial(&material.GaussianRoughNonConductive{
				Variance: material.ConstantScalar(0.5),
			})
		case m.NonConductiveSmooth != nil:
			realScene.AddMaterial(&material.NonConductiveSmooth{
				InteriorIndexOfRefraction: material.ConstantSpectrum(densesignal.VisibleSpectrumRamp(1.7, 1.5)),
				ExteriorIndexOfRefraction: material.ConstantScalar(1.0),
			})
		}
	}

	realScene.InfinityMaterialIndex = int(protoScene.InfinityMaterialIndex)

	for _, e := range protoScene.Element {
		realScene.AddElement(&scene.SceneElement{
			GeometryIndex: int(e.GeometryIndex),
			MaterialIndex: int(e.MaterialIndex),
			ModelToWorld:  convertTransform(e.ModelToWorld),
		})
	}

	for _, c := range protoScene.Camera {
		switch {
		case c.PinholeCamera != nil:
			realCamera := &camera.PinholeCamera{}
			realCamera.SetEye(convertVec3(c.GetEye()))
			realCamera.SetUp(convertVec3(c.GetUp()))
			realCamera.Center = convertVec3(c.GetCenter())
			realCamera.Aperture = convertVec3(c.GetAperture())
			realScene.AddCamera(realCamera)
		}
	}

	return realScene, nil
}

func convertMaterialCoordsMode(in headerproto.MaterialCoordsMode) geometry.MaterialCoordsMode {
	switch in {
	case headerproto.MaterialCoordsMode_MATERIAL_COORDS_MODE_2D:
		return geometry.MaterialCoords2D
	case headerproto.MaterialCoordsMode_MATERIAL_COORDS_MODE_3D:
		return geometry.MaterialCoords3D
	}

	// Dead code
	return geometry.MaterialCoords3D
}

func convertTransform(in *headerproto.Transform) affinetransform.AffineTransform {
	return affinetransform.AffineTransform{
		Linear: convertMat33(in.Linear),
		Offset: convertVec3(in.Offset),
	}
}

func convertMat33(in *headerproto.Mat33) mat33.T {
	return mat33.T{
		in.E00,
		in.E01,
		in.E02,
		in.E10,
		in.E11,
		in.E12,
		in.E20,
		in.E21,
		in.E22,
	}
}

func convertVec3(in *headerproto.Vec3) vec3.T {
	return vec3.T{
		in.E0,
		in.E1,
		in.E2,
	}
}
