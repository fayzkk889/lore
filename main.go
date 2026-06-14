package main

import (
	"github.com/fayzkk889/lore/cmd"
)

var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
