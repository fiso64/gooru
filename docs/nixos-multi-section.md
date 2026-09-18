## NixOS

The flake exports `gooru.nixosModules.default`. Keep a shared default package at `services.gooru.package` and define deployments under `services.gooru.instances.<name>`. Each enabled instance must set `settings.server.listen` explicitly.

### Two-instance example

```nix
services.gooru.instances = {
  main = {
    enable = true;
    settings = {
      server.listen = "127.0.0.1:5678";
      uploads = {
        enabled = true;
        targets = [{
          id = "default";
          name = "Default";
          path = "/srv/gooru/incoming";
        }];
      };
    };
    admins.primary = {
      username = "admin";
      passwordFile = "/run/gooru-main-admin";
    };
  };

  test = {
    enable = true;
    settings = {
      server.listen = "127.0.0.1:5679";
      encryption = {
        enabled = true;
        key_file = "/run/gooru-test-key";
      };
      ui.accent_color = "#2f80ed";
    };
    initialDatabase.hashingStrategy = "full";
  };
};
```

Set `services.gooru.package` to change the shared default package. Set `services.gooru.instances.<name>.package` for an instance-specific override; its generated `server.frontend_dir` follows the effective package.

### First boot and administrators

`services.gooru.instances.<name>.initialDatabase.hashingStrategy` accepts `"partial"` or `"full"` and applies only when that instance creates a missing database. Existing database state remains authoritative.

`services.gooru.instances.<name>.admins` is ongoing declarative state. The attribute name is a stable declaration identity; `username` is mutable desired state. Password files are runtime paths loaded through namespaced systemd credentials and should not contain plaintext via Nix-store paths. Removing an admin declaration does not delete the Gooru user.

### Per-instance resources

For an instance named `main`, the defaults are:

- unit, user, and group: `gooru-main`;
- config: `/etc/gooru/main/serve.yaml`;
- state/database: `/var/lib/gooru-main`;
- media cache: `/var/cache/gooru-main`;
- declarative-admin state below `/var/lib/gooru-main/declarative-admins`.

Other names receive the same collision-free namespacing. If you override an instance's `user` or `group`, create it separately; the module automatically creates only the default `gooru-<name>` identity.

Create upload directories with permissions appropriate for each instance. Set `services.gooru.instances.<name>.openFirewall = true` only when that instance's literal `HOST:PORT` listen address should be opened in the host firewall.

