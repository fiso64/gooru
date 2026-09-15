{
  description = "Gooru media library";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      version = builtins.replaceStrings [ "\n" ] [ "" ] (builtins.readFile ./VERSION);
      revision = if self ? rev then self.rev else if self ? dirtyRev then builtins.replaceStrings [ "-dirty" ] [ "" ] self.dirtyRev else "unknown";
      dirty = if self ? dirtyRev then "true" else "false";
    in {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          frontend = pkgs.buildNpmPackage {
            pname = "gooru-frontend";
            version = version;
            src = ./frontend;
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
            src = ./.;
            vendorHash = "sha256-sZCEbsjFTNim3dOAW347LBjQRuQboA2ttXN8A3VWlFA=";
            subPackages = [ "cmd/gooru" ];
            tags = [ "govips" ];
            ldflags = [
              "-X=gooru.local/internal/buildinfo.Version=${version}"
              "-X=gooru.local/internal/buildinfo.Revision=${revision}"
              "-X=gooru.local/internal/buildinfo.Dirty=${dirty}"
              "-X=gooru.local/internal/buildinfo.Development=true"
            ];
            nativeBuildInputs = [ pkgs.pkg-config ];
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

      nixosModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.services.gooru;
          yaml = pkgs.formats.yaml { };
          hostSystem = pkgs.stdenv.hostPlatform.system;
          effectiveSettings = lib.recursiveUpdate {
            server = {
              listen = "127.0.0.1:5678";
              frontend_dir = "${cfg.package}/share/gooru/frontend";
            };
            database.path = "/var/lib/gooru/gooru.db";
            media.cache_dir = "/var/cache/gooru/media";
          } cfg.settings;
          configFile = yaml.generate "gooru.yaml" effectiveSettings;
          adminCredentialName = username: "gooru-admin-${username}";
          initialAdminNames = lib.attrNames cfg.initialAdmins;
          validInitialAdminUsername = username:
            builtins.stringLength username <= 64
            && builtins.match "[A-Za-z0-9._-]+" username != null;
        in {
          options.services.gooru = {
            enable = lib.mkEnableOption "Gooru web application";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${hostSystem}.default;
              defaultText = lib.literalExpression "self.packages.${pkgs.stdenv.hostPlatform.system}.default";
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

            initialAdmins = lib.mkOption {
              type = lib.types.attrsOf (lib.types.submodule ({ ... }: {
                options.passwordFile = lib.mkOption {
                  type = lib.types.str;
                  description = ''
                    Runtime path to a file containing the initial password for this admin.
                    The file is loaded through systemd credentials and is never copied into
                    the Nix store. Existing users are left unchanged, including their password.
                  '';
                };
              }));
              default = { };
              example = lib.literalExpression ''
                {
                  admin.passwordFile = "/run/secrets/gooru-admin";
                }
              '';
              description = ''
                Admin accounts to create when they are missing. Attribute names are Gooru
                usernames. Passwords are consumed only when creating a missing account;
                this option does not reconcile or rotate passwords for existing users.
              '';
            };

            openFirewall = lib.mkOption {
              type = lib.types.bool;
              default = false;
              description = "Open the TCP port from server.listen in the firewall. Only literal HOST:PORT listen values are supported.";
            };
          };

          config = lib.mkIf cfg.enable {
            assertions = [
              {
                assertion = lib.all validInitialAdminUsername initialAdminNames;
                message = "services.gooru.initialAdmins usernames must be 64 characters or fewer and contain only letters, numbers, dots, dashes, and underscores";
              }
              {
                assertion = lib.all (username: cfg.initialAdmins.${username}.passwordFile != "") initialAdminNames;
                message = "services.gooru.initialAdmins passwordFile values must not be empty";
              }
            ];

            users.users = lib.mkIf (cfg.user == "gooru") {
              gooru = {
                isSystemUser = true;
                group = cfg.group;
                home = "/var/lib/gooru";
              };
            };
            users.groups = lib.mkIf (cfg.group == "gooru") { gooru = { }; };

            environment.systemPackages = [ cfg.package ];
            environment.etc."gooru/serve.yaml".source = configFile;

            systemd.services.gooru = {
              description = "Gooru web application";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];
              path = [ pkgs.ffmpeg ];
              preStart = lib.concatMapStringsSep "\n" (username:
                let credentialName = adminCredentialName username;
                in ''
                  GOORU_ADMIN_PASSWORD="$(cat "$CREDENTIALS_DIRECTORY/${credentialName}")" \
                    ${cfg.package}/bin/gooru --config /etc/gooru/serve.yaml user create-admin --if-missing --username ${lib.escapeShellArg username}
                '') initialAdminNames;
              serviceConfig = {
                User = cfg.user;
                Group = cfg.group;
                ExecStart = "${cfg.package}/bin/gooru serve --config /etc/gooru/serve.yaml";
                Restart = "on-failure";
                StateDirectory = "gooru";
                CacheDirectory = "gooru";
                WorkingDirectory = "/var/lib/gooru";
                NoNewPrivileges = true;
                PrivateTmp = true;
                LoadCredential = lib.mapAttrsToList (username: admin:
                  "${adminCredentialName username}:${admin.passwordFile}"
                ) cfg.initialAdmins;
              };
            };

            networking.firewall.allowedTCPPorts = lib.mkIf cfg.openFirewall [
              (lib.toInt (lib.last (lib.splitString ":" effectiveSettings.server.listen)))
            ];
          };
        };

      checks = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          moduleEval = nixpkgs.lib.nixosSystem {
            inherit system;
            modules = [
              self.nixosModules.default
              {
                system.stateVersion = "26.05";
                services.gooru = {
                  enable = true;
                  initialAdmins.alice.passwordFile = "/run/secrets/gooru-admin-alice";
                };
              }
            ];
          };
          longUsername = builtins.concatStringsSep "" (nixpkgs.lib.replicate 65 "a");
          invalidUsernameEval = nixpkgs.lib.nixosSystem {
            inherit system;
            modules = [
              self.nixosModules.default
              {
                system.stateVersion = "26.05";
                services.gooru = {
                  enable = true;
                  initialAdmins.${longUsername}.passwordFile = "/run/secrets/gooru-admin-too-long";
                };
              }
            ];
          };
          moduleService = moduleEval.config.systemd.services.gooru;
          invalidUsernameAssertions = invalidUsernameEval.config.assertions;
          moduleCheck =
            assert builtins.elem "gooru-admin-alice:/run/secrets/gooru-admin-alice" moduleService.serviceConfig.LoadCredential;
            assert nixpkgs.lib.hasInfix "--if-missing" moduleService.preStart;
            assert nixpkgs.lib.hasInfix "$CREDENTIALS_DIRECTORY/gooru-admin-alice" moduleService.preStart;
            assert nixpkgs.lib.any (entry: !entry.assertion && nixpkgs.lib.hasInfix "64 characters or fewer" entry.message) invalidUsernameAssertions;
            pkgs.runCommand "gooru-nixos-module-check" { } ''
              touch $out
            '';
        in {
          package = self.packages.${system}.default;
          nixos-module = moduleCheck;
        });
    };
}
