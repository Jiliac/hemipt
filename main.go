package main

import (
	"fmt"
	"log"

	"flag"
	"io/ioutil"
	"math/rand"
	"os"
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

	seeds := fuzzLoop(threads, seedInputs)
	// ** Epilogue **
	traces := getSeedTrace(threads, seeds)
	export(config.outDir, seeds, traces)
	saveSeeds(config.outDir, seeds)

	for _, t := range threads {
		t.clean()
	}
}

func export(outDir string, seeds []*seedT, traces [][]byte) {
	fmt.Println("")
	ok, glbProj := doGlbProjection(seeds)
	if !ok {
		return
	}

	testMLEDiv(glbProj.cleanedSeeds)

	pcas := glbProj.pcas
	exportHistos(pcas, filepath.Join(outDir, "histos.csv"))
	exportProjResults(pcas, filepath.Join(outDir, "pcas.csv"))

	exportHashes(glbProj.cleanedSeeds, filepath.Join(outDir, "hashes.csv"))
	exportDistances(glbProj, filepath.Join(outDir, "distances.csv"))
	exportCoor(glbProj, filepath.Join(outDir, "coords.csv"))
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

	createOutDir(config.outDir)

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

func createOutDir(outDir string) {
	if _, err := os.Stat(outDir); err == nil {
		os.RemoveAll(outDir)
	}

	err := os.Mkdir(outDir, 0755)
	if err != nil {
		log.Fatalf("Couldn't create output directory: %v.\n", err)
	}
}
