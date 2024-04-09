package main

import (
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/actor/def"
	petcd "github.com/chenxyzl/grain/actor/provider/etcd"
)

func main() {
	config := def.NewConfig("hello", []string{"127.0.0.1:2379"})
	system := actor.NewSystem[*petcd.ClusterProviderEtcd](config)
	_ = system
}
