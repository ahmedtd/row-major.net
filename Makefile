# Enable sanity
.SUFFIXES:
.ONESHELL:
.DELETE_ON_ERROR:

# Life is too short for POSIX shell
SHELL=/bin/bash

# `all` is the first target, so it's built by default
.PHONY: all
all:

all_clean_files :=

all_templated := \
  index.html \
  articles/rl-force-tube/index.html \
  about/index.html \
  ballistae/index.html \
  frustum/index.html \
  masters-thesis-presentation/index.html \
  resume/index.html \
  webgl-raytracer/index.html
all_clean_files += $(all_templated)

all: $(all_templated)
$(all_templated): %.html: %.html.jinja jinjaize.py base.html.jinja
	PYTHONIOENCODING=utf8 python3 jinjaize.py --template-path='./' <$< >$@ || exit 1

# Static incompressible files.  They will not be gzipped.
all_static_incompressible := \
  articles/rl-force-tube/rl-force-tube.js \
  ballistae/angry-bunny.jpeg \
  ballistae/crystal-bunny.jpeg \
  masters-thesis-presentation/katex/fonts/KaTeX_Size2-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-BoldItalic.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_AMS-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Bold.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Bold.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Size4-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Script-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Typewriter-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-BoldItalic.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Size2-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Size4-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Bold.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-BoldItalic.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Bold.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-BoldItalic.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Size3-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Size2-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Bold.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Bold.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Script-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Size1-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Italic.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Size4-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Size1-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Size4-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Italic.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Typewriter-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Italic.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Size3-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_AMS-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Regular.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Typewriter-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Bold.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Bold.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Bold.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Bold.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Bold.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Bold.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Size3-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Fraktur-Bold.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Italic.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Italic.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Italic.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Typewriter-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Size2-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Size3-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Bold.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Size1-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Main-Italic.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_AMS-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Italic.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Italic.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_Size1-Regular.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_Script-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Italic.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Italic.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Script-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Bold.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Regular.eot \
  masters-thesis-presentation/katex/fonts/KaTeX_Math-Italic.woff2 \
  masters-thesis-presentation/katex/fonts/KaTeX_AMS-Regular.woff \
  masters-thesis-presentation/katex/fonts/KaTeX_Caligraphic-Bold.ttf \
  masters-thesis-presentation/katex/fonts/KaTeX_SansSerif-Regular.woff2 \
  masters-thesis-presentation/test-setup-close.jpeg \
  masters-thesis-presentation/test-setup-far.jpeg \
  masters-thesis-presentation/an-spy.jpeg \
  masters-thesis-presentation/ska.jpeg \
  masters-thesis-presentation/trial-1-results.png \
  masters-thesis-presentation/trial-2-results.png \
  \

# Static compressible files.  They will get the gzip pre-treatment
# below.
all_static_compressibles := \
  articles/rl-force-tube/d3/d3.js \
  articles/rl-force-tube/rl-force-tube.js \
  masters-thesis-presentation/reveal.js \
  masters-thesis-presentation/reveal.css \
  masters-thesis-presentation/white.css \
  masters-thesis-presentation/katex/katex.css \
  masters-thesis-presentation/katex/katex.js \
  masters-thesis-presentation/katex/contrib/auto-render.min.js \
  masters-thesis-presentation/phase-space-intro.svg \
  masters-thesis-presentation/phase-space-classical-steering.svg \
  masters-thesis-presentation/phase-space-deactivation-steering.svg \
  masters-thesis-presentation/find-maximal-interval.svg \
  masters-thesis-presentation/deactivation-pattern-width5pct.svg \
  masters-thesis-presentation/deactivation-pattern-width10pct.svg \
  masters-thesis-presentation/deactivation-pattern-width25pct.svg \
  masters-thesis-presentation/deactivation-pattern-width41pct.svg \
  masters-thesis-presentation/deactivation-pattern-width50pct.svg \
  masters-thesis-presentation/deactivation-pattern-width75pct.svg \
  masters-thesis-presentation/deactivation-pattern-width100pct.svg \
  masters-thesis-presentation/main-beam-comparison.svg \
  masters-thesis-presentation/deactivation-main-beam-vs-w.svg \
  masters-thesis-presentation/classical-pattern.svg \
  masters-thesis-presentation/captured-elements.svg \
  masters-thesis-presentation/phase-space-pigeonhole.svg \
  masters-thesis-presentation/example-array.svg \
  masters-thesis-presentation/experiment-architecture.svg \
  masters-thesis-presentation/diamond-robots.svg \
  masters-thesis-presentation/classical-specific-pattern.svg \
  masters-thesis-presentation/deactivation-specific-pattern.svg \
  masters-thesis-presentation/unsteered-specific-pattern.svg \
  webgl-raytracer/raytracer.frag \
  webgl-raytracer/raytracer.js \
  webgl-raytracer/raytracer.vert \
  \

all_static = $(all_static_incompressible) $(all_static_compressible)

all_compressibles := $(all_templated) $(all_static_compressibles)
all_compressed := $(all_compressibles:=.gz)

all_clean_files += $(all_compressed)
all: $(all_compressed)
$(all_compressed): %.gz: %
	gzip <$< >$@ || exit 1

# These files have to have the tilde in them.  The exist to not break
# Ergun Akleman's TAMU CSCE Image Synthesis class galleries.  Don't
# bother minifying them, since hardly anyone fetches them.
all_troublesome := \
  old-homedir/classes/csce647/basic_style.css \
  old-homedir/classes/csce647/index.html \
  old-homedir/classes/csce647/pr01/00.jpg \
  old-homedir/classes/csce647/pr01/ballistae-pr01.tar.gz         \
  old-homedir/classes/csce647/pr01/index.html                        \
  old-homedir/classes/csce647/pr01/picture_0xss.jpeg             \
  old-homedir/classes/csce647/pr01/picture_1xss.jpeg             \
  old-homedir/classes/csce647/pr01/picture_2xss.jpeg               \
  old-homedir/classes/csce647/pr01/picture_3xss.jpeg             \
  old-homedir/classes/csce647/pr01/thumbnail.jpg                 \
  old-homedir/classes/csce647/pr02/00.jpg                          \
  old-homedir/classes/csce647/pr02/index.html                        \
  old-homedir/classes/csce647/pr02/vp1_fov0.jpeg                 \
  old-homedir/classes/csce647/pr02/vp1_fov1.jpeg                 \
  old-homedir/classes/csce647/pr02/vp1_fov2.jpeg                 \
  old-homedir/classes/csce647/pr02/vp2_fov0.jpeg                 \
  old-homedir/classes/csce647/pr02/vp2_fov1.jpeg                 \
  old-homedir/classes/csce647/pr02/vp2_fov2.jpeg                 \
  old-homedir/classes/csce647/pr03/00.jpg                            \
  old-homedir/classes/csce647/pr03/index.html                        \
  old-homedir/classes/csce647/pr03/simple-gooch-scene.jpeg           \
  old-homedir/classes/csce647/pr03/simple-gooch-scene.scm            \
  old-homedir/classes/csce647/pr03/simple-lambert-scene.jpeg       \
  old-homedir/classes/csce647/pr03/simple-lambert-scene.scm      \
  old-homedir/classes/csce647/pr03/simple-phong-scene.jpeg           \
  old-homedir/classes/csce647/pr03/simple-phong-scene.scm            \
  old-homedir/classes/csce647/pr04/00.jpg                            \
  old-homedir/classes/csce647/pr04/dirlight.jpeg                   \
  old-homedir/classes/csce647/pr04/index.html                        \
  old-homedir/classes/csce647/pr04/two-pointlights.jpeg          \
  old-homedir/classes/csce647/pr04/two-spotlights.jpeg               \
  old-homedir/classes/csce647/pr05/00.jpg                            \
  old-homedir/classes/csce647/pr05/index.html                        \
  old-homedir/classes/csce647/pr05/soft-shadow-directional.jpeg    \
  old-homedir/classes/csce647/pr05/thumbnail.jpg                 \
  old-homedir/classes/csce647/pr05/volumetric-light.jpeg         \
  old-homedir/classes/csce647/pr06/00.jpg                            \
  old-homedir/classes/csce647/pr06/2d-textures-1.jpeg                \
  old-homedir/classes/csce647/pr06/index.html                        \
  old-homedir/classes/csce647/pr07/00.jpg                          \
  old-homedir/classes/csce647/pr07/index.html                        \
  old-homedir/classes/csce647/pr07/solid-textures.jpeg               \
  old-homedir/classes/csce647/pr08/00.jpg                            \
  old-homedir/classes/csce647/pr08/index.html                        \
  old-homedir/classes/csce647/pr08/meshes.jpeg                       \
  old-homedir/classes/csce647/pr09/00.jpg                          \
  old-homedir/classes/csce647/pr09/index.html                        \
  old-homedir/classes/csce647/pr09/reflectance-demo-1.jpeg           \
  old-homedir/classes/csce647/pr10/00.jpg                          \
  old-homedir/classes/csce647/pr10/bunny-through-glass.jpeg        \
  old-homedir/classes/csce647/pr10/index.html                        \
  old-homedir/classes/csce647/pr10/scene-through-dodecahedron.jpeg   \
  old-homedir/classes/csce647/pr10/transparency-demo-1.jpeg        \
  old-homedir/classes/csce647/pr10/transparency-demo-2.jpeg        \
  old-homedir/classes/csce647/pr11/00.jpg                            \
  old-homedir/classes/csce647/pr11/01.jpg                            \
  old-homedir/classes/csce647/pr11/index.html                        \
  old-homedir/classes/csce647/pr11/thumbnail.jpg                 \
  old-homedir/classes/csce647/pr12/00.jpg                            \
  old-homedir/classes/csce647/pr12/index.html                        \
  old-homedir/classes/csce647/pr12/ior-animation-frame.jpeg        \
  old-homedir/classes/csce647/pr12/rolling-marble-frame.jpeg     \
  old-homedir/classes/csce647/pr13/00.jpg                            \
  old-homedir/classes/csce647/pr13/ambient-occlusion-demo.jpeg       \
  old-homedir/classes/csce647/pr13/bunny-lit-by-bunny.jpeg           \
  old-homedir/classes/csce647/pr13/index.html                        \
  old-homedir/classes/csce647/pr13/reflectance-demo-1.jpeg           \
  old-homedir/classes/csce647/pr13/soft-shadow-directional.jpeg  \
  old-homedir/classes/csce647/pr14/00.jpg                          \
  old-homedir/classes/csce647/pr14/01.jpg                            \
  old-homedir/classes/csce647/pr14/index.html                        \
  old-homedir/classes/csce647/pr14/thumbnail.jpg                 \
  old-homedir/classes/csce647/pr15/00.jpg                            \
  old-homedir/classes/csce647/pr15/01.jpg                            \
  old-homedir/classes/csce647/pr15/index.html                        \
  old-homedir/classes/csce647/pr15/thumbnail.jpg                 \

all_deploy := $(all_templated) $(all_static) $(all_compressed)

.PHONY: build-dist
build-dist: $(all_deploy)
	rm -rf ./dist || exit 1
	mkdir ./dist || exit 1
	rsync -vR $(all_deploy) dist/ || exit 1
	rsync -vr old-homedir/ dist/~ahmedtd || exit 1

gcp-project-id := bomsync-214520
image-tag := gcr.io/$(gcp-project-id)/row-major-website

.PHONY: container-push-gke
container-push-gke:
	bash tools/container-push.bash ./ $(image-tag) manifests/

.PHONY: apply-gke
apply-gke:
	kustomize build manifests/ | kubectl apply -f - || exit 1

.PHONY: deploy-gke
deploy-gke: container-push-gke apply-gke

.PHONY: clean-gke
clean-gke:
	kustomize build manifests/ | kubectl delete -f - || exit 1

.PHONY: clean
clean:
	rm -rf $(all_clean_files) || exit 1
	rm -rf dist || exit 1
