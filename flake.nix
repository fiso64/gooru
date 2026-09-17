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
          adminCredentialName = declarationID: "gooru-admin-${declarationID}";
          adminStateName = declarationID: "gooru-admin-${declarationID}.user-id";
          adminDeclarationIDs = lib.attrNames cfg.admins;
          validAdminIdentifier = value:
            builtins.stringLength value <= 64
            && builtins.match "[A-Za-z0-9._-]+" value != null;
          initialDatabaseCommand = ''
            ${cfg.package}/bin/gooru --config /etc/gooru/serve.yaml init --if-missing --hashing-strategy ${cfg.initialDatabase.hashingStrategy}
          '';
          # `user reconcile-admin` deliberately never performs database/storage
          # initialization. Protected-mode first boot therefore opens a normal
          # client once after `init`, allowing the standard database encryption
          # migration to complete before declarative users are reconciled.
          protectedStoragePreparationCommand = lib.optionalString
            (adminDeclarationIDs != [ ] && lib.attrByPath [ "encryption" "enabled" ] false effectiveSettings) ''
              ${cfg.package}/bin/gooru --config /etc/gooru/serve.yaml count >/dev/null
            '';
          adminReconciliationCommands = lib.concatMapStringsSep "\n" (declarationID:
            let
              admin = cfg.admins.${declarationID};
              credentialName = adminCredentialName declarationID;
              stateName = adminStateName declarationID;
            in ''
              umask 077
              admin_state_dir="/var/lib/gooru/declarative-admins"
              mkdir -p "$admin_state_dir"
              admin_state_file="$admin_state_dir/${stateName}"
              admin_state_tmp="$admin_state_file.tmp"
              if [ -s "$admin_state_file" ]; then
                admin_user_id="$(cat "$admin_state_file")"
                GOORU_ADMIN_PASSWORD="$(cat "$CREDENTIALS_DIRECTORY/${credentialName}")" \
                  ${cfg.package}/bin/gooru --config /etc/gooru/serve.yaml user reconcile-admin \
                    --user-id "$admin_user_id" --username ${lib.escapeShellArg admin.username} > "$admin_state_tmp"
              else
                GOORU_ADMIN_PASSWORD="$(cat "$CREDENTIALS_DIRECTORY/${credentialName}")" \
                  ${cfg.package}/bin/gooru --config /etc/gooru/serve.yaml user reconcile-admin \
                    --username ${lib.escapeShellArg admin.username} > "$admin_state_tmp"
              fi
              mv "$admin_state_tmp" "$admin_state_file"
            '') adminDeclarationIDs;
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

            initialDatabase.hashingStrategy = lib.mkOption {
              type = lib.types.enum [ "partial" "full" ];
              default = "partial";
              description = ''
                Hashing strategy used only when the service creates its database on first boot.
                Once a database file exists, its stored strategy is authoritative and is never
                reconciled to this option.
              '';
            };

            admins = lib.mkOption {
              type = lib.types.attrsOf (lib.types.submodule ({ name, ... }: {
                options = {
                  username = lib.mkOption {
                    type = lib.types.str;
                    default = name;
                    description = ''
                      Desired Gooru username for this stable declaration identity. Changing the
                      username renames the existing Gooru user while preserving its stable user ID
                      and per-user data.
                    '';
                  };
                  passwordFile = lib.mkOption {
                    type = lib.types.str;
                    description = ''
                      Runtime path to a file containing this admin's desired password. The file is
                      loaded through systemd credentials and is never copied into the Nix store.
                      This works directly with runtime-secret providers such as agenix or sops-nix.
                    '';
                  };
                };
              }));
              default = { };
              example = lib.literalExpression ''
                {
                  primary = {
                    username = "admin";
                    passwordFile = "/run/agenix/gooru-admin";
                  };
                }
              '';
              description = ''
                Declaratively managed Gooru administrators. Each attribute name is a stable
                declaration identity and must not be reused for a different person/account.
                The username defaults to that attribute name. On startup Gooru adopts or creates
                the declared user, preserves its stable user ID across username changes, and
                reconciles the password. Removing a declaration does not delete the Gooru user.
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
                assertion = lib.all validAdminIdentifier adminDeclarationIDs;
                message = "services.gooru.admins declaration IDs must be 64 characters or fewer and contain only letters, numbers, dots, dashes, and underscores";
              }
              {
                assertion = lib.all (declarationID: validAdminIdentifier cfg.admins.${declarationID}.username) adminDeclarationIDs;
                message = "services.gooru.admins usernames must be 64 characters or fewer and contain only letters, numbers, dots, dashes, and underscores";
              }
              {
                assertion = lib.all (declarationID: cfg.admins.${declarationID}.passwordFile != "") adminDeclarationIDs;
                message = "services.gooru.admins passwordFile values must not be empty";
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
              preStart = initialDatabaseCommand + protectedStoragePreparationCommand + adminReconciliationCommands;
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
                LoadCredential = lib.mapAttrsToList (declarationID: admin:
                  "${adminCredentialName declarationID}:${admin.passwordFile}"
                ) cfg.admins;
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
                  admins.primary = {
                    username = "alice";
                    passwordFile = "/run/secrets/gooru-admin-alice";
                  };
                };
              }
            ];
          };
          fullHashModuleEval = nixpkgs.lib.nixosSystem {
            inherit system;
            modules = [
              self.nixosModules.default
              {
                system.stateVersion = "26.05";
                services.gooru = {
                  enable = true;
                  initialDatabase.hashingStrategy = "full";
                };
              }
            ];
          };
          protectedModuleEval = nixpkgs.lib.nixosSystem {
            inherit system;
            modules = [
              self.nixosModules.default
              {
                system.stateVersion = "26.05";
                services.gooru = {
                  enable = true;
                  settings.encryption = {
                    enabled = true;
                    key_file = "/run/secrets/gooru-encryption-key";
                  };
                  admins.primary = {
                    username = "alice";
                    passwordFile = "/run/secrets/gooru-admin-alice";
                  };
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
                  admins.primary = {
                    username = longUsername;
                    passwordFile = "/run/secrets/gooru-admin-too-long";
                  };
                };
              }
            ];
          };
          moduleService = moduleEval.config.systemd.services.gooru;
          fullHashService = fullHashModuleEval.config.systemd.services.gooru;
          protectedService = protectedModuleEval.config.systemd.services.gooru;
          modulePreStartLines = nixpkgs.lib.filter (line: line != "") (nixpkgs.lib.splitString "\n" moduleService.preStart);
          invalidUsernameAssertions = invalidUsernameEval.config.assertions;
          moduleCheck =
            assert moduleEval.config.services.gooru.admins.primary.username == "alice";
            assert builtins.elem "gooru-admin-primary:/run/secrets/gooru-admin-alice" moduleService.serviceConfig.LoadCredential;
            assert nixpkgs.lib.hasInfix "init --if-missing --hashing-strategy partial" moduleService.preStart;
            assert nixpkgs.lib.hasInfix "init --if-missing --hashing-strategy partial" (builtins.head modulePreStartLines);
            assert nixpkgs.lib.hasInfix "init --if-missing --hashing-strategy full" fullHashService.preStart;
            assert nixpkgs.lib.hasInfix "count >/dev/null" protectedService.preStart;
            assert !(nixpkgs.lib.hasInfix "count >/dev/null" moduleService.preStart);
            assert nixpkgs.lib.hasInfix "user reconcile-admin" moduleService.preStart;
            assert nixpkgs.lib.hasInfix "alice" moduleService.preStart;
            assert nixpkgs.lib.hasInfix "/var/lib/gooru/declarative-admins" moduleService.preStart;
            assert nixpkgs.lib.hasInfix "gooru-admin-primary.user-id" moduleService.preStart;
            assert nixpkgs.lib.hasInfix "$CREDENTIALS_DIRECTORY/gooru-admin-primary" moduleService.preStart;
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
