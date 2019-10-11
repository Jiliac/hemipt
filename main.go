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

	seedInputs := readSeeds(config.inDir)
	if len(seedInputs) == 0 {
		log.Fatal("No seed given")
	}
	//
	putArgs := strings.Split(config.cliStr, " ")
	ok, put := startAFLPUT(putArgs[0], putArgs[1:], 100*time.Millisecond)
	if !ok {
		log.Printf("Couldn't start %s.\n", filepath.Base(putArgs[0]))
		return
	}

	// ** Test **
	for i, in := range seedInputs {
		_, _ = put.run(in)
		hash := hashTrBits(put.trace)
		fmt.Printf("seed %d hash: 0x%x\n", i, hash)
	}

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

func readSeeds(dir string) (seedInputs [][]byte) {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Printf("Couldn't read seed directory: %v.\n", err)
		return seedInputs
	} else if len(infos) == 0 {
		log.Print("No seed in seed directory.")
		return seedInputs
	}

	for _, info := range infos {
		in, err := ioutil.ReadFile(filepath.Join(dir, info.Name()))
		if err != nil {
			log.Printf("Couldn't read seed %s: %v.\n", info.Name(), err)
			continue
		}
		seedInputs = append(seedInputs, in)
	}

	return seedInputs
}
