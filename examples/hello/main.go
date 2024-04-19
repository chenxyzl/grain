package main

import (
	"github.com/chenxyzl/grain/actor"
	petcd "github.com/chenxyzl/grain/actor/provider/etcd"
)

func main() {
	config := actor.NewConfig("hello", []string{"127.0.0.1:2379"})
	system := actor.NewSystem[*petcd.ClusterProviderEtcd](config)
	_ = system
}
