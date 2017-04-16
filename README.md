# row-major.net

A small static site built on `make` and `jinja`.  Features:

  * CSS and HTML minification.
  * Generates gzip files that `nginx` will use as precompressed files.
  * Incremental rebuilding with `make`.

To build:

    make

To clean:

    make clean

To deploy:

    make deploy

# Dependencies

  * GNU Make
  * python3
  * csscompressor (from PyPI)
  * htmlmin (from PyPI)
  * jinja2 (from PyPI)
