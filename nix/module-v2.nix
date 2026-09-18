{ self }:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.gooru;
  yaml = pkgs.formats.yaml { };
  hostSystem = pkgs.stdenv.hostPlatform.system;

  serviceName = name: "gooru-${name}";
  stateDir = name: "/var/lib/${serviceName name}";
  cacheDir = name: "/var/cache/${serviceName name}";
  configRelativePath = name: "gooru/${name}/serve.yaml";
  configPath = name: "/etc/${configRelativePath name}";

  validInstanceIdentifier = value:
    builtins.stringLength value <= 24
    && builtins.match "[a-z0-9][a-z0-9_-]*" value != null;
  validAdminIdentifier = value:
    builtins.stringLength value <= 64
    && builtins.match "[A-Za-z0-9._-]+" value != null;

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
        default = serviceName name;
        description = "User account under which this Gooru instance runs.";
      };

      group = lib.mkOption {
        type = lib.types.str;
        default = serviceName name;
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
        description = ''
          Declaratively managed Gooru administrators for this instance. Each attribute name is
          a stable declaration identity and must not be reused for a different person/account.
          Removing a declaration does not delete the Gooru user.
        '';
      };

      openFirewall = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = "Open the TCP port from this instance's server.listen in the firewall. Only literal HOST:PORT listen values are supported.";
      };
    };
  });

  enabledInstances = lib.filterAttrs (_: instance: instance.enable) cfg.instances;
  packageFor = instance: if instance.package == null then cfg.package else instance.package;
  listenFor = instance: lib.attrByPath [ "server" "listen" ] null instance.settings;

  settingsFor = name: instance:
    lib.recursiveUpdate {
      server.frontend_dir = "${packageFor instance}/share/gooru/frontend";
      database.path = "${stateDir name}/gooru.db";
      media.cache_dir = "${cacheDir name}/media";
    } instance.settings;

  configFileFor = name: instance:
    yaml.generate "gooru-${name}.yaml" (settingsFor name instance);

  mkAssertions = name: instance:
    let
      adminDeclarationIDs = lib.attrNames instance.admins;
    in [
      {
        assertion = validInstanceIdentifier name;
        message = "services.gooru.instances names must be 24 characters or fewer and contain only lowercase letters, numbers, dashes, and underscores";
      }
    ] ++ lib.optionals instance.enable [
      {
        assertion = listenFor instance != null && listenFor instance != "";
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
    ];

  preStartFor = name: instance:
    let
      package = packageFor instance;
      path = configPath name;
      declarationIDs = lib.attrNames instance.admins;
      adminStateDir = "${stateDir name}/declarative-admins";
      credentialName = declarationID: "gooru-${name}-admin-${declarationID}";
      stateName = declarationID: "gooru-admin-${declarationID}.user-id";
      initialDatabase = ''
        ${package}/bin/gooru --config ${path} init --if-missing --hashing-strategy ${instance.initialDatabase.hashingStrategy}
      '';
      protectedStoragePreparation = lib.optionalString
        (declarationIDs != [ ] && lib.attrByPath [ "encryption" "enabled" ] false (settingsFor name instance)) ''
          ${package}/bin/gooru --config ${path} count >/dev/null
        '';
      reconcileAdmins = lib.concatMapStringsSep "\n" (declarationID:
        let
          admin = instance.admins.${declarationID};
        in ''
          umask 077
          admin_state_dir="${adminStateDir}"
          mkdir -p "$admin_state_dir"
          admin_state_file="$admin_state_dir/${stateName declarationID}"
          admin_state_tmp="$admin_state_file.tmp"
          if [ -s "$admin_state_file" ]; then
            admin_user_id="$(cat "$admin_state_file")"
            GOORU_ADMIN_PASSWORD="$(cat "$CREDENTIALS_DIRECTORY/${credentialName declarationID}")" \
              ${package}/bin/gooru --config ${path} user reconcile-admin \
                --user-id "$admin_user_id" --username ${lib.escapeShellArg admin.username} > "$admin_state_tmp"
          else
            GOORU_ADMIN_PASSWORD="$(cat "$CREDENTIALS_DIRECTORY/${credentialName declarationID}")" \
              ${package}/bin/gooru --config ${path} user reconcile-admin \
                --username ${lib.escapeShellArg admin.username} > "$admin_state_tmp"
          fi
          mv "$admin_state_tmp" "$admin_state_file"
        '') declarationIDs;
    in initialDatabase + protectedStoragePreparation + reconcileAdmins;

  serviceFor = name: instance:
    let
      package = packageFor instance;
      unitName = serviceName name;
    in {
      description = "Gooru web application (${name})";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      path = [ pkgs.ffmpeg ];
      preStart = preStartFor name instance;
      serviceConfig = {
        User = instance.user;
        Group = instance.group;
        ExecStart = "${package}/bin/gooru serve --config ${configPath name}";
        Restart = "on-failure";
        StateDirectory = unitName;
        CacheDirectory = unitName;
        WorkingDirectory = stateDir name;
        NoNewPrivileges = true;
        PrivateTmp = true;
        LoadCredential = lib.mapAttrsToList (declarationID: admin:
          "gooru-${name}-admin-${declarationID}:${admin.passwordFile}"
        ) instance.admins;
      };
    };

  defaultUserInstances = lib.filterAttrs (name: instance: instance.user == serviceName name) enabledInstances;
  defaultGroupInstances = lib.filterAttrs (name: instance: instance.group == serviceName name) enabledInstances;
  firewallInstances = lib.filterAttrs (_: instance: instance.openFirewall && listenFor instance != null) enabledInstances;
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

  config = lib.mkIf (cfg.instances != { }) {
    assertions = lib.flatten (lib.mapAttrsToList mkAssertions cfg.instances);

    users.users = lib.mapAttrs' (name: instance:
      lib.nameValuePair instance.user {
        isSystemUser = true;
        group = instance.group;
        home = stateDir name;
      }
    ) defaultUserInstances;

    users.groups = lib.mapAttrs' (_: instance:
      lib.nameValuePair instance.group { }
    ) defaultGroupInstances;

    environment.systemPackages = lib.mapAttrsToList (_: packageFor) enabledInstances;

    environment.etc = lib.mapAttrs' (name: instance:
      lib.nameValuePair (configRelativePath name) {
        source = configFileFor name instance;
      }
    ) enabledInstances;

    systemd.services = lib.mapAttrs' (name: instance:
      lib.nameValuePair (serviceName name) (serviceFor name instance)
    ) enabledInstances;

    networking.firewall.allowedTCPPorts = lib.mapAttrsToList (_: instance:
      lib.toInt (lib.last (lib.splitString ":" (listenFor instance)))
    ) firewallInstances;
  };
}
