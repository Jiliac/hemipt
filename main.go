package main

import (
	"fmt"
	"log"

	"flag"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	randSeed := time.Now().UTC().UnixNano()
	rand.Seed(randSeed)
}

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

	// ** Test **
	putArgs := strings.Split(config.cliStr, " ")
	binPath, cliArgs := putArgs[0], putArgs[1:]
	t1, ok1 := startThread(binPath, cliArgs)
	t2, ok2 := startThread(binPath, cliArgs)
	if !ok1 || !ok2 {
		log.Print("Problem starting thread.")
		return
	}
	threads := []*thread{t1, t2}
	//
	for i, in := range append(seedInputs, seedInputs...) {
		e := executor{
			ig:             seedCopier(in),
			discoveryFit:   trueFitFunc{},
			securityPolicy: falseFitFunc{},
			fitChan:        devNullFitChan,
			crashChan:      devNullFitChan,
		}

		t := threads[i%2]
		t.execChan <- &e
		<-t.endChan
	}

	for _, t := range threads {
		t.clean()
	}
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
