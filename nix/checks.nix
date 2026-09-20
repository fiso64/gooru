{ self, nixpkgs, supportedSystems }:

let
  forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
in
forAllSystems (system:
  let
    pkgs = nixpkgs.legacyPackages.${system};
    testPackage = pkgs.writeShellScriptBin "gooru" "exit 0";

    moduleEval = nixpkgs.lib.nixosSystem {
      inherit system;
      modules = [
        self.nixosModules.default
        {
          system.stateVersion = "26.05";
          services.gooru = {
            instances.main = {
              enable = true;
              openFirewall = true;
              settings.server.listen = "127.0.0.1:5678";
              admins.primary = {
                username = "alice";
                passwordFile = "/run/keys/gooru-main-admin-alice";
              };
            };

            instances.test = {
              enable = true;
              package = testPackage;
              initialDatabase.hashingStrategy = "full";
              settings = {
                server.listen = "127.0.0.1:5679";
                encryption = {
                  enabled = true;
                  key_file = "/run/keys/gooru-test-encryption-key";
                };
                ui = {
                  accent_color = "#2f80ed";
                  grid_type = "tile";
                };
              };
              admins.primary = {
                username = "bob";
                passwordFile = "/run/keys/gooru-test-admin-bob";
              };
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
          services.gooru.instances.main = {
            enable = true;
            settings.server.listen = "127.0.0.1:5678";
            admins.primary = {
              username = longUsername;
              passwordFile = "/run/keys/gooru-admin-too-long";
            };
          };
        }
      ];
    };

    missingListenEval = nixpkgs.lib.nixosSystem {
      inherit system;
      modules = [
        self.nixosModules.default
        {
          system.stateVersion = "26.05";
          services.gooru.instances.main.enable = true;
        }
      ];
    };

    instanceWrapper = nixpkgs.lib.findFirst (package: nixpkgs.lib.getName package == "gooru-instance") null moduleEval.config.environment.systemPackages;
    mainService = moduleEval.config.systemd.services.gooru-main;
    testService = moduleEval.config.systemd.services.gooru-test;
    invalidUsernameAssertions = invalidUsernameEval.config.assertions;
    missingListenAssertions = missingListenEval.config.assertions;

    moduleCheck =
      assert instanceWrapper != null;
      assert moduleEval.config.services.gooru.instances.main.admins.primary.username == "alice";
      assert moduleEval.config.services.gooru.instances.test.settings.ui.grid_type == "tile";
      assert builtins.hasAttr "gooru/main/serve.yaml" moduleEval.config.environment.etc;
      assert builtins.hasAttr "gooru/test/serve.yaml" moduleEval.config.environment.etc;
      assert moduleEval.config.users.users.gooru-main.home == "/var/lib/gooru-main";
      assert moduleEval.config.users.users.gooru-test.home == "/var/lib/gooru-test";
      assert mainService.serviceConfig.StateDirectory == "gooru-main";
      assert testService.serviceConfig.StateDirectory == "gooru-test";
      assert mainService.serviceConfig.CacheDirectory == "gooru-main";
      assert testService.serviceConfig.CacheDirectory == "gooru-test";
      assert mainService.serviceConfig.StateDirectoryMode == "0700";
      assert testService.serviceConfig.StateDirectoryMode == "0700";
      assert mainService.serviceConfig.WorkingDirectory == "/var/lib/gooru-main";
      assert testService.serviceConfig.WorkingDirectory == "/var/lib/gooru-test";
      assert nixpkgs.lib.hasSuffix "/bin/gooru serve --config /etc/gooru/main/serve.yaml" mainService.serviceConfig.ExecStart;
      assert testService.serviceConfig.ExecStart == "${testPackage}/bin/gooru serve --config /etc/gooru/test/serve.yaml";
      assert builtins.elem moduleEval.config.environment.etc."gooru/main/serve.yaml".source mainService.restartTriggers;
      assert builtins.elem moduleEval.config.environment.etc."gooru/test/serve.yaml".source testService.restartTriggers;
      assert builtins.elem "gooru-main-admin-primary:/run/keys/gooru-main-admin-alice" mainService.serviceConfig.LoadCredential;
      assert builtins.elem "gooru-test-admin-primary:/run/keys/gooru-test-admin-bob" testService.serviceConfig.LoadCredential;
      assert nixpkgs.lib.hasInfix "hashing-strategy partial" mainService.preStart;
      assert nixpkgs.lib.hasInfix "hashing-strategy full" testService.preStart;
      assert !(nixpkgs.lib.hasInfix "count >/dev/null" mainService.preStart);
      assert nixpkgs.lib.hasInfix "count >/dev/null" testService.preStart;
      assert nixpkgs.lib.hasInfix "/var/lib/gooru-main/declarative-admins" mainService.preStart;
      assert nixpkgs.lib.hasInfix "/var/lib/gooru-test/declarative-admins" testService.preStart;
      assert moduleEval.config.networking.firewall.allowedTCPPorts == [ 5678 ];
      assert nixpkgs.lib.any (entry: !entry.assertion && nixpkgs.lib.hasInfix "64 characters or fewer" entry.message) invalidUsernameAssertions;
      assert nixpkgs.lib.any (entry: !entry.assertion && nixpkgs.lib.hasInfix "server.listen must be set explicitly" entry.message) missingListenAssertions;
      pkgs.runCommand "gooru-nixos-module-check" { } ''
        ${pkgs.bash}/bin/bash -n ${instanceWrapper}/bin/gooru-instance
        grep -F "gooru-main" ${instanceWrapper}/bin/gooru-instance
        grep -F "/etc/gooru/main/serve.yaml" ${instanceWrapper}/bin/gooru-instance
        grep -F "${testPackage}/bin/gooru" ${instanceWrapper}/bin/gooru-instance
        grep -F "${pkgs.util-linux}/bin/runuser" ${instanceWrapper}/bin/gooru-instance
        touch $out
      '';
  in {
    package = self.packages.${system}.default;
    nixos-module = moduleCheck;
  })
