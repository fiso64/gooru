{ self, nixpkgs }:

let
  supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
  forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
  version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ../VERSION);
  revision = if self ? rev then self.rev else if self ? dirtyRev then builtins.replaceStrings [ "-dirty" ] [ "" ] self.dirtyRev else "unknown";
  dirty = if self ? dirtyRev then "true" else "false";
in {
  packages = forAllSystems (system:
    let
      pkgs = nixpkgs.legacyPackages.${system};
      frontend = pkgs.buildNpmPackage {
        pname = "gooru-frontend";
        version = version;
        src = ../frontend;
        npmDepsHash = "sha256-6dL0bcxE0C39LDBG4GKQNA6xJx98NP2uSB8ifS1pxRE=";
        npmBuildScript = "build";
        installPhase = ''
          runHook preInstall
          mkdir -p $out
          cp -r build/. $out/
          runHook postInstall
        '';
      };
    in {
      default = pkgs.buildGoModule {
        pname = "gooru";
        version = version;
        src = ../.;
        vendorHash = "sha256-sZCEbsjFTNim3dOAW347LBjQRuQboA2ttXN8A3VWlFA=";
        subPackages = [ "cmd/gooru" ];
        tags = [ "govips" ];
        ldflags = [
          "-X=gooru.local/internal/buildinfo.Version=${version}"
          "-X=gooru.local/internal/buildinfo.Revision=${revision}"
          "-X=gooru.local/internal/buildinfo.Dirty=${dirty}"
          "-X=gooru.local/internal/buildinfo.Development=true"
        ];
        nativeBuildInputs = [ pkgs.pkg-config pkgs.makeWrapper ];
        nativeCheckInputs = [ pkgs.poppler-utils ];
        preCheck = ''
          command -v pdfinfo
          command -v pdftoppm
          go test -run TestProtectedPDFViewerRealPopplerGoldenPath -v ./internal/serve
        '';
        # The Nix wrapper provides JPEG conversion and PDF rendering tools.
        postFixup = ''
          wrapProgram $out/bin/gooru --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.libjpeg_turbo pkgs.poppler-utils ]}
        '';
        buildInputs = [ pkgs.vips ];

        postInstall = ''
          mkdir -p $out/share/gooru/frontend
          cp -r ${frontend}/. $out/share/gooru/frontend/
          mkdir -p $out/share/doc/gooru
          cp LICENSE THIRD_PARTY_NOTICES.md $out/share/doc/gooru/
        '';

        meta = {
          description = "Content-centric tool for tagging and organizing local files";
          homepage = "https://github.com/fiso64/gooru";
          license = pkgs.lib.licenses.agpl3Only;
          mainProgram = "gooru";
          platforms = supportedSystems;
        };
      };
    });

  nixosModules.default = import ./module.nix { inherit self; };

  checks = import ./checks.nix {
    inherit self nixpkgs supportedSystems;
  };
}
 -v ./internal/serve
        '';
        # The Nix wrapper provides JPEG conversion and PDF rendering tools.
        postFixup = ''
          wrapProgram $out/bin/gooru --prefix PATH : ${pkgs.lib.makeBinPath [ pkgs.libjpeg_turbo pkgs.poppler-utils ]}
        '';
        buildInputs = [ pkgs.vips ];

        postInstall = ''
          mkdir -p $out/share/gooru/frontend
          cp -r ${frontend}/. $out/share/gooru/frontend/
          mkdir -p $out/share/doc/gooru
          cp LICENSE THIRD_PARTY_NOTICES.md $out/share/doc/gooru/
        '';

        meta = {
          description = "Content-centric tool for tagging and organizing local files";
          homepage = "https://github.com/fiso64/gooru";
          license = pkgs.lib.licenses.agpl3Only;
          mainProgram = "gooru";
          platforms = supportedSystems;
        };
      };
    });

  nixosModules.default = import ./module.nix { inherit self; };

  checks = import ./checks.nix {
    inherit self nixpkgs supportedSystems;
  };
}
