{ self }:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.gooru;
  yaml = pkgs.formats.yaml { };
  hostSystem = pkgs.stdenv.hostPlatform.system;

  validInstanceIdentifier = value:
    builtins.stringLength value <= 24
    && builtins.match "[a-z0-9][a-z0-9_-]*" value != null;
  validAdminIdentifier = value:
    builtins.stringLength value <= 64
    && builtins.match "[A-Za-z0-9._-]+" value != null;

  enabledInstances = lib.filterAttrs (_: instance: instance.enable) cfg.instances;

  instanceType = lib.types.submodule ({ name, ... }: {
    options = {
      enable = lib.mkEnableOption "Gooru instance ${name}";

      package = lib.mkOption {
        type = lib.types.nullOr lib.types.package;
        default = null;
        description = "Optional package override for this instance. When null, services.gooru.package is used.";
      };

      user = lib.mkOption {
        type = lib.types.str;
        default = "gooru-${name}";
        description = "User account under which this Gooru instance runs.";
      };

      group = lib.mkOption {
        type = lib.types.str;
        default = "gooru-${name}";
        description = "Group under which this Gooru instance runs.";
      };

      settings = lib.mkOption {
        inherit (yaml) type;
        default = { };
        description = ''
          Gooru serve configuration written to YAML for this instance. State/cache
          paths and the packaged frontend are supplied automatically. Enabled
          instances must set server.listen explicitly.
        '';
      };

      initialDatabase.hashingStrategy = lib.mkOption {
        type = lib.types.enum [ "partial" "full" ];
        default = "partial";
        description = ''
          Hashing strategy used only when this instance creates its database on first boot.
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
          Declaratively managed Gooru administrators for this instance. Each attribute name is
          a stable declaration identity and must not be reused for a different person/account.
          The username defaults to that attribute name. On startup Gooru adopts or creates the
          declared user, preserves its stable user ID across username changes, and reconciles
          the password. Removing a declaration does not delete the Gooru user.
        '';
      };

      openFirewall = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = "Open the TCP port from this instance's server.listen in the firewall. Only literal HOST:PORT listen values are supported.";
      };
    };
  });

  instanceAssertions = lib.concatMap (name:
    let
      instance = cfg.instances.${name};
      adminDeclarationIDs = lib.attrNames instance.admins;
    in [
      {
        assertion = validInstanceIdentifier name;
        message = "services.gooru.instances names must be 24 characters or fewer and contain only lowercase letters, numbers, dashes, and underscores";
      }
    ] ++ lib.optionals instance.enable [
      {
        assertion = lib.hasAttrByPath [ "server" "listen" ] instance.settings
          && instance.settings.server.listen != "";
        message = "services.gooru.instances.${name}.settings.server.listen must be set explicitly for every enabled instance";
      }
      {
        assertion = lib.all validAdminIdentifier adminDeclarationIDs;
        message = "services.gooru.instances.${name}.admins declaration IDs must be 64 characters or fewer and contain only letters, numbers, dots, dashes, and underscores";
      }
      {
        assertion = lib.all (declarationID: validAdminIdentifier instance.admins.${declarationID}.username) adminDeclarationIDs;
        message = "services.gooru.instances.${name}.admins usernames must be 64 characters or fewer and contain only letters, numbers, dots, dashes, and underscores";
      }
      {
        assertion = lib.all (declarationID: instance.admins.${declarationID}.passwordFile != "") adminDeclarationIDs;
        message = "services.gooru.instances.${name}.admins passwordFile values must not be empty";
      }
    ]) (lib.attrNames cfg.instances);

  mkInstanceConfig = name: instance:
    let
      serviceName = "gooru-${name}";
      effectivePackage = if instance.package == null then cfg.package else instance.package;
      defaultUser = serviceName;
      defaultGroup = serviceName;
      stateName = serviceName;
      stateDir = "/var/lib/${stateName}";
      cacheDir = "/var/cache/${stateName}";
      configRelativePath = "gooru/${name}/serve.yaml";
      configPath = "/etc/${configRelativePath}";
      adminStateDir = "${stateDir}/declarative-admins";
      adminCredentialName = declarationID: "gooru-${name}-admin-${declarationID}";
      adminStateName = declarationID: "gooru-admin-${declarationID}.user-id";
      adminDeclarationIDs = lib.attrNames instance.admins;
      effectiveSettings = lib.recursiveUpdate {
        server.frontend_dir = "${effectivePackage}/share/gooru/frontend";
        database.path = "${stateDir}/gooru.db";
        media.cache_dir = "${cacheDir}/media";
      } instance.settings;
      configFile = yaml.generate "gooru-${name}.yaml" effectiveSettings;
      initialDatabaseCommand = ''
        ${effectivePackage}/bin/gooru --config ${configPath} init --if-missing --hashing-strategy ${instance.initialDatabase.hashingStrategy}
      '';
      # `user reconcile-admin` deliberately never performs database/storage
      # initialization. Protected-mode first boot therefore opens a normal
      # client once after `init`, allowing the standard database encryption
      # migration to complete before declarative users are reconciled.
      protectedStoragePreparationCommand = lib.optionalString
        (adminDeclarationIDs != [ ] && lib.attrByPath [ "encryption" "enabled" ] false effectiveSettings) ''
          ${effectivePackage}/bin/gooru --config ${configPath} count >/dev/null
        '';
      adminReconciliationCommands = lib.concatMapStringsSep "\n" (declarationID:
        let
          admin = instance.admins.${declarationID};
          credentialName = adminCredentialName declarationID;
          stateNameForAdmin = adminStateName declarationID;
        in ''
          umask 077
          admin_state_dir="${adminStateDir}"
          mkdir -p "$admin_state_dir"
          admin_state_file="$admin_state_dir/${stateNameForAdmin}"
          admin_state_tmp="$admin_state_file.tmp"
          if [ -s "$admin_state_file" ]; then
            admin_user_id="$(cat "$admin_state_file")"
            GOORU_ADMIN_PASSWORD="$(cat "$CREDENTIALS_DIRECTORY/${credentialName}")" \
              ${effectivePackage}/bin/gooru --config ${configPath} user reconcile-admin \
                --user-id "$admin_user_id" --username ${lib.escapeShellArg admin.username} > "$admin_state_tmp"
          else
            GOORU_ADMIN_PASSWORD="$(cat "$CREDENTIALS_DIRECTORY/${credentialName}")" \
              ${effectivePackage}/bin/gooru --config ${configPath} user reconcile-admin \
                --username ${lib.escapeShellArg admin.username} > "$admin_state_tmp"
          fi
          mv "$admin_state_tmp" "$admin_state_file"
        '') adminDeclarationIDs;
    in {
      users.users.${instance.user} = lib.mkIf (instance.user == defaultUser) {
        isSystemUser = true;
        group = instance.group;
        home = stateDir;
      };
      users.groups.${instance.group} = lib.mkIf (instance.group == defaultGroup) { };

      environment.systemPackages = [ effectivePackage ];
      environment.etc.${configRelativePath}.source = configFile;

      systemd.services.${serviceName} = {
        description = "Gooru web application (${name})";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        path = [ pkgs.ffmpeg ];
        preStart = initialDatabaseCommand + protectedStoragePreparationCommand + adminReconciliationCommands;
        serviceConfig = {
          User = instance.user;
          Group = instance.group;
          ExecStart = "${effectivePackage}/bin/gooru serve --config ${configPath}";
          Restart = "on-failure";
          StateDirectory = stateName;
          CacheDirectory = stateName;
          WorkingDirectory = stateDir;
          NoNewPrivileges = true;
          PrivateTmp = true;
          LoadCredential = lib.mapAttrsToList (declarationID: admin:
            "${adminCredentialName declarationID}:${admin.passwordFile}"
          ) instance.admins;
        };
      };

      networking.firewall.allowedTCPPorts = lib.mkIf instance.openFirewall [
        (lib.toInt (lib.last (lib.splitString ":" effectiveSettings.server.listen)))
      ];
    };
in {
  options.services.gooru = {
    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${hostSystem}.default;
      defaultText = lib.literalExpression "self.packages.${pkgs.stdenv.hostPlatform.system}.default";
      description = "Shared default Gooru package for instances.";
    };

    instances = lib.mkOption {
      type = lib.types.attrsOf instanceType;
      default = { };
      description = "Named independent Gooru service instances.";
    };
  };

  config = lib.mkMerge (
    [
      {
        assertions = instanceAssertions;
      }
    ]
    ++ lib.mapAttrsToList mkInstanceConfig enabledInstances
  );
}
