package serve
import("fmt";"gopkg.in/yaml.v3")
func resolveImplicitUploadDefaults(c *Config,src []byte)(bool,error){
 var root map[string]yaml.Node
 if err:=yaml.Unmarshal(src,&root);err!=nil{return false,err}
 var enabled,targets bool
 up:=root["uploads"]
 for i:=0;i+1<len(up.Content);i+=2{
  switch up.Content[i].Value{case "enabled":enabled=true;case "targets":targets=true}
 }
 if !enabled{c.Uploads.Enabled=c.Auth.Enabled&&(!targets||len(c.Uploads.Targets)>0)}
 auto:=c.Uploads.Enabled&&!targets&&len(c.Uploads.Targets)==0
 if auto{
  path,err:=defaultUploadPath()
  if err!=nil{return false,fmt.Errorf("resolve default upload target: %w",err)}
  c.Uploads.Targets=[]UploadTarget{{ID:"default",Name:"Default",Path:path}}
 }
 return auto,nil
}
