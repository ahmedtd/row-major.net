{ pkgs ? import <nixpkgs> {} }:

with pkgs;

stdenv.mkDerivation {
  name = "row-major-net-deps";
  propagatedBuildInputs = [python36Packages.csscompressor
                           python36Packages.htmlmin
                           python36Packages.jinja2
                           rsync
                           openssh
                           gnumake];
}
