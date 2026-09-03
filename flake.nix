{
  description = "Gooru media library";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          frontend = pkgs.buildNpmPackage {
            pname = "gooru-frontend";
            version = "0-unstable";
            src = ./frontend;
            npmDepsHash = "sha256-k3d4Md1NQZfYO/dq1xzv5U8B5NRNhsAFPAv5BNkNVXM=";
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
            version = "0-unstable";
            src = ./.;
            vendorHash = pkgs.lib.fakeHash;
            subPackages = [ "cmd/gooru" ];

            postInstall = ''
              mkdir -p $out/share/gooru/frontend
              cp -r ${frontend}/. $out/share/gooru/frontend/
            '';

            meta = {
              description = "Content-centric tool for tagging and organizing local files";
              homepage = "https://github.com/fiso64/gooru";
              license = nixpkgs.lib.licenses.mit;
              mainProgram = "gooru";
              platforms = supportedSystems;
            };
          };
        });

      nixosModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.services.gooru;
          yaml = pkgs.formats.yaml { };
          effectiveSettings = lib.recursiveUpdate {
            server = {
              listen = "127.0.0.1:5678";
              frontend_dir = "${cfg.package}/share/gooru/frontend";
            };
            database.path = "/var/lib/gooru/gooru.db";
            media.cache_dir = "/var/cache/gooru/media";
          } cfg.settings;
          configFile = yaml.generate "gooru.yaml" effectiveSettings;
        in {
          options.services.gooru = {
            enable = lib.mkEnableOption "Gooru web application";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.system}.default;
              defaultText = lib.literalExpression "self.packages.${pkgs.system}.default";
              description = "Gooru package to run.";
            };

            user = lib.mkOption {
              type = lib.types.str;
              default = "gooru";
              description = "User account under which Gooru runs.";
            };

            group = lib.mkOption {
              type = lib.types.str;
              default = "gooru";
              description = "Group under which Gooru runs.";
            };

            settings = lib.mkOption {
              inherit (yaml) type;
              default = { };
              description = "Gooru serve configuration written to YAML. Defaults provide state/cache paths and the packaged frontend.";
            };

            openFirewall = lib.mkOption {
              type = lib.types.bool;
              default = false;
              description = "Open the TCP port from server.listen in the firewall. Only literal HOST:PORT listen values are supported.";
            };
          };

          config = lib.mkIf cfg.enable {
            users.users = lib.mkIf (cfg.user == "gooru") {
              gooru = {
                isSystemUser = true;
                group = cfg.group;
                home = "/var/lib/gooru";
              };
            };
            users.groups = lib.mkIf (cfg.group == "gooru") { gooru = { }; };

            systemd.services.gooru = {
              description = "Gooru web application";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];
              serviceConfig = {
                User = cfg.user;
                Group = cfg.group;
                ExecStart = "${cfg.package}/bin/gooru serve --config ${configFile}";
                Restart = "on-failure";
                StateDirectory = "gooru";
                CacheDirectory = "gooru";
                WorkingDirectory = "/var/lib/gooru";
                NoNewPrivileges = true;
                PrivateTmp = true;
                ProtectSystem = "strict";
                ProtectHome = "read-only";
              };
            };

            networking.firewall.allowedTCPPorts = lib.mkIf cfg.openFirewall [
              (lib.toInt (lib.last (lib.splitString ":" effectiveSettings.server.listen)))
            ];
          };
        };

      checks = forAllSystems (system: {
        package = self.packages.${system}.default;
      });
    };
}
