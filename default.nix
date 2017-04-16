let pkgs = import <nixpkgs> {};
in
{ stdenv ? pkgs.stdenv,
  python3 ? pkgs.python3,
  pythonIRClib ? pkgs.pythonIRClib }:

  stdenv.mkDerivation {
    name = "python-nix";
    version = "0.1.0.0";
    src = ./.;
    buildInputs = [python3

                   gnumake];
  }
