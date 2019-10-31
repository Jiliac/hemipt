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

	putArgs := strings.Split(config.cliStr, " ")
	binPath, cliArgs := putArgs[0], putArgs[1:]
	threads, ok := startMultiThreads(config.threadN, binPath, cliArgs)
	if !ok {
		log.Print("Problem starting thread.")
		return
	}

	//seedExecTest(threads, seedInputs) // Old test

	executors := fuzzLoop(threads, seedInputs)
	// ** Test **
	traces := getSeedTrace(threads, seedInputs)
	analyzeExecs(executors, traces)

	for _, t := range threads {
		t.clean()
	}
}

// *****************************************************************************
// ************************* Command-Line Interface ****************************

type configOptions struct {
	// PUT interface
	cliStr string

	// Fuzzer configuration
	inDir, outDir string
	threadN       int
}

func parseCLI() (config configOptions) {
	flag.StringVar(&config.cliStr, "cli", "", "PUT command-line interface")
	flag.StringVar(&config.inDir, "i", "", "Seed directory")
	flag.StringVar(&config.outDir, "o", "", "Output directory")
	flag.IntVar(&config.threadN, "n", 2, "Number of threads Hemipt uses")

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
