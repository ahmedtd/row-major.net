(use-modules (armadillo))
(use-modules (ballistae))

(define my-camera (ballistae/camera/make))

(define infty-matr
  (ballistae/matr/make
   "phong"
   `((color_a . (0 0 0)))))

(define my-matr
  (ballistae/matr/make
   "phong"
   `((k_a   . 0.2)
     (k_d   . 1.0)
     (k_s   . 0.0)
     (color_a . (1 0 0))
     (color_d . (1 0 0))
     (lights . (,(arma/list->b64col '(0 10 10)))))))

(define my-geom
  (ballistae/geom/make
   "sphere"
   `((center . ,(arma/list->b64col '(10 1 1)))
     (radius . 4))))

(define my-scene (ballistae/scene/crush infty-matr `((,my-geom . ,my-matr))))

(ballistae/render-scene my-camera my-scene "simple-lambert-scene.jpeg" 512 512 2)
