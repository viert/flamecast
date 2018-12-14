package main

import (
	"cast"
	"configreader"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
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

	flameServer := cast.Start()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	defer signal.Reset()
	for range sigs {
		break
	}

	err = flameServer.Shutdown(nil)
	if err != nil {
		fmt.Printf("Error during graceful shutdown: %s\n", err)
	}

}
