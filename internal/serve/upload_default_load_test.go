package serve

import (
 "fmt"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
 "gopkg.in/yaml.v3"
)

// Keep configuration tests away from the runner's real application state.
func testDefaultUploadRoot(t *testing.T) string {
 t.Helper()
 root:=t.TempDir()
 switch runtime.GOOS {
 case "linux":
  t.Setenv("STATE_DIRECTORY", root)
 case "windows":
  t.Setenv("LOCALAPPDATA", root)
 default:
  t.Skip("default upload root cannot currently be redirected in this OS test")
 }
 return root
}

func TestLoadConfigImplicitUploadModes(t *testing.T) {
 for _,tc:=range []struct{name, source string; enabled bool; targets int; errPart string}{
  {"authenticated omission","{}",true,1,""},
  {"explicit disabled","uploads:\n  enabled: false\n",false,0,""},
  {"explicit empty targets","uploads:\n  targets: []\n",false,0,""},
  {"explicit enabled empty targets","uploads:\n  enabled: true\n  targets: []\n",false,0,"uploads.enabled requires"},
  {"anonymous omission","auth:\n  enabled: false\n",false,0,""},
  {"anonymous explicit opt-in","auth:\n  enabled: false\nuploads:\n  enabled: true\n",true,1,""},
  {"anonymous explicit disabled","auth:\n  enabled: false\nuploads:\n  enabled: false\n",false,0,""},
  {"explicit disable with omitted targets","auth:\n  enabled: true\nuploads:\n  enabled: false\n",false,0,""},
 } {
  t.Run(tc.name,func(t *testing.T) {
   root:=testDefaultUploadRoot(t)
   cfgFile:=filepath.Join(t.TempDir(),"serve.yaml")
   writeConfig(t,cfgFile,tc.source)
   cfg,err:=LoadConfig(cfgFile,filepath.Join(t.TempDir(),"gooru.db"),Overrides{})
   if tc.errPart!="" {
    if err==nil || !strings.Contains(err.Error(),tc.errPart) {t.Fatalf("expected %q error, got %v",tc.errPart,err)}
    return
   }
   if err!=nil {t.Fatal(err)}
   if cfg.Uploads.Enabled!=tc.enabled || len(cfg.Uploads.Targets)!=tc.targets {t.Fatalf("uploads=%+v",cfg.Uploads)}
   if tc.targets==1 {
    want:=filepath.Join(root,"uploads")
    if runtime.GOOS=="windows" {want=filepath.Join(root,"Gooru","uploads")}
    if cfg.Uploads.Targets[0].Path!=want {t.Fatalf("target=%q want %q",cfg.Uploads.Targets[0].Path,want)}
    info,err:=os.Stat(want);if err!=nil {t.Fatal(err)}
    if !info.IsDir(){t.Fatalf("upload target is not a directory: %s",want)}
    if runtime.GOOS!="windows" && info.Mode().Perm()!=0700 {t.Fatalf("upload permissions: %v",info.Mode())}
   } else if _,err:=os.Stat(filepath.Join(root,"uploads")); !os.IsNotExist(err) {t.Fatalf("unexpected default upload directory: %v",err)}
  })
 }
}

func TestLoadConfigExplicitCustomUploadTarget(t *testing.T) {
 root:=testDefaultUploadRoot(t)
 custom:=filepath.Join(t.TempDir(),"custom")
 cfgFile:=filepath.Join(t.TempDir(),"serve.yaml")
 writeConfig(t,cfgFile,fmt.Sprintf("uploads:\n  enabled: true\n  targets:\n    - id: custom\n      name: Custom\n      path: %q\n",custom))
 cfg,err:=LoadConfig(cfgFile,filepath.Join(t.TempDir(),"gooru.db"),Overrides{})
 if err!=nil {t.Fatal(err)}
 if len(cfg.Uploads.Targets)!=1 || cfg.Uploads.Targets[0].Path!=custom {t.Fatalf("custom target overridden: %+v",cfg.Uploads.Targets)}
 if _,err:=os.Stat(filepath.Join(root,"uploads"));!os.IsNotExist(err) {t.Fatalf("implicit target unexpectedly provisioned: %v",err)}
}

func TestGeneratedConfigKeepsAuthAwareUploadsImplicit(t *testing.T) {
 data,err:=DefaultYAML(filepath.Join(t.TempDir(),"gooru.db"))
 if err!=nil {t.Fatal(err)}
 var root yaml.Node
 if err:=yaml.Unmarshal(data,&root);err!=nil {t.Fatal(err)}
 if len(root.Content)!=1 || root.Content[0].Kind!=yaml.MappingNode {t.Fatal("generated YAML root is not mapping")}
 for i:=0;i+1<len(root.Content[0].Content);i+=2 {
  if root.Content[0].Content[i].Value!="uploads" {continue}
  fields:=root.Content[0].Content[i+1]
  for j:=0;j+1<len(fields.Content);j+=2 {
   if fields.Content[j].Value=="enabled" || fields.Content[j].Value=="targets" {t.Fatalf("generated YAML freezes implicit %q",fields.Content[j].Value)}
  }
  return
 }
 t.Fatal("missing uploads section")
}
