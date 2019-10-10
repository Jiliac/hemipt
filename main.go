package main

import (
	"fmt"
	"log"

	"flag"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"
)

var (
	workDir = "/tmp" // @TODO: make it a user option
)

func main() {
	fmt.Println("Hemipt start.")
	config := parseCLI()

	putArgs := strings.Split(config.cliStr, " ")
	ok, put := startAFLPUT(putArgs[0], putArgs[1:], 100*time.Millisecond)
	if !ok {
		log.Printf("Couldn't start %s.\n", filepath.Base(putArgs[0]))
		return
	}

	// ** Test **

	put.clean()
}

type configOptions struct {
	cliStr        string
	inDir, outDir string
}

func parseCLI() (config configOptions) {
	flag.StringVar(&config.cliStr, "cli", "", "PUT command-line interface")
	flag.StringVar(&config.inDir, "i", "", "Seed directory")
	flag.StringVar(&config.outDir, "o", "", "Output directory")

	flag.Parse()

	if len(config.cliStr) == 0 {
		flag.Usage()
		log.Fatal("Please provide CLI argument.")
	} else if len(config.inDir) == 0 {
		flag.Usage()
		log.Fatal("Please provide a seed directory.")
	} else if len(config.outDir) == 0 {
		flag.Usage()
		log.Fatal("Please provide an output directory.")
	}

	return config
}
