package main

import (
	"cast"
	"configreader"
	"flag"
	"fmt"
)

func usage() {
	fmt.Println("Usage: flamecast -c <configFile>")
}

func main() {
	var configFilename string
	flag.StringVar(&configFilename, "c", "", "config filename")
	flag.Parse()

	if configFilename == "" {
		usage()
		return
	}

	config, err := configreader.Load(configFilename)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = cast.Configure(config)
	if err != nil {
		fmt.Println(err)
		return
	}
	cast.Start()
}
