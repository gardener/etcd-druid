package main

import (
	"os"

	"github.com/gardener/etcd-druid/druidctl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
