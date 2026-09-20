package serve

import (
 "path/filepath"
 "testing"
)

func TestImplicitUploadDefaultDecision(t *testing.T) {
 t.Setenv("STATE_DIRECTORY", t.TempDir())
 cases := []struct {
  name, source string
  auth, enabled bool
  targets int
 }{
  {"authenticated omission", "", true, true, 1},
  {"explicit disable", "uploads:\n  enabled: false\n", true, false, 0},
  {"explicit empty", "uploads:\n  targets: []\n", true, false, 0},
  {"anonymous omission", "auth:\n  enabled: false\n", false, false, 0},
  {"anonymous opt-in", "uploads:\n  enabled: true\n", false, true, 1},
 }
 for _, tc := range cases {
  t.Run(tc.name,func(t *testing.T){
   cfg:=DefaultConfig(filepath.Join(t.TempDir(),"gooru.db"))
   cfg.Auth.Enabled=tc.auth
   automatic,err:=resolveImplicitUploadDefaults(&cfg,[]byte(tc.source))
   if err!=nil {t.Fatal(err)}
   if cfg.Uploads.Enabled!=tc.enabled || len(cfg.Uploads.Targets)!=tc.targets || automatic!=(tc.enabled&&tc.targets==1) {
    t.Fatalf("uploads=%+v automatic=%t",cfg.Uploads,automatic)
   }
   if err:=cfg.Validate();err!=nil {t.Fatal(err)}
  })
 }
}
