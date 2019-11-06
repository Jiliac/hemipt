package main

import (
	"fmt"

	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	"gonum.org/v1/gonum/mat"
)

func fuzzLoop(threads []*thread, seedInputs [][]byte) (seeds []*seedT) {
	fitChan := make(chan runT, 1000)
	sched := newScheduler(threads, seedInputs, fitChan)
	makeGlbFitness(fitChan, sched.newSeedChan)

	seeds = <-sched.seedsChan
	return seeds
}

func getSeedTrace(threads []*thread, seeds []*seedT) (traces [][]byte) {
	traces = make([][]byte, len(seeds))
	fitChan := make(chan runT, 1)
	t := threads[0]

	for i, seed := range seeds {
		e := &executor{
			ig:             seedCopier(seed.input),
			discoveryFit:   trueFitFunc{},
			securityPolicy: falseFitFunc{},
			fitChan:        fitChan,
			crashChan:      devNullFitChan,
			oneExec:        true,
		}

		t.execChan <- e
		<-t.endChan
		runInfo := <-fitChan
		traces[i] = runInfo.trace
	}

	return traces
}

// *****************************************************************************
// ******************************* Debug/Test **********************************

func analyzeExecs(outDir string, seeds []*seedT, traces [][]byte) {
	fmt.Println("")
	pcas := getPCAs(getPCAFits(seeds))
	fmt.Printf("len(seeds), len(pcas): %d, %d\n", len(seeds), len(pcas))
	//seedDists(pcas, traces)

	exportHistos(pcas, filepath.Join(outDir, "histos.csv"))
	exportProjResults(pcas, filepath.Join(outDir, "pcas.csv"))
	ok, vars, centProjs, seedProjs :=
		exportDistances(seeds, filepath.Join(outDir, "distances.csv"))
	if ok {
		exportCoor(vars, centProjs, seedProjs, filepath.Join(outDir, "coords.csv"))
	}
}
func getPCAFits(seeds []*seedT) (pcaFits []*pcaFitFunc) {
	for _, seed := range seeds {
		df := seed.exec.discoveryFit
		if ff, ok := df.(fitnessMultiplexer); ok {
			for _, ffi := range ff {
				if pcaFit, ok := ffi.(*pcaFitFunc); ok {
					pcaFits = append(pcaFits, pcaFit)
				}
			}
		} else if pcaFit, ok := df.(*pcaFitFunc); ok {
			pcaFits = append(pcaFits, pcaFit)
		}
	}
	return pcaFits
}
func getPCAs(pcaFits []*pcaFitFunc) (pcas []*dynamicPCA) {
	for _, f := range pcaFits {
		if f.dynpca == nil || !f.dynpca.phase4 {
			continue
		}
		pcas = append(pcas, f.dynpca)
	}
	return pcas
}
func compareHashes(hashes1, hashes2 map[uint64]struct{}) {
	l1, l2 := len(hashes1), len(hashes2)
	var common int
	for hash := range hashes1 {
		if _, ok := hashes2[hash]; ok {
			common++
		}
	}
	fmt.Printf("Seed hashes\tl1: %d\tl2: %d\tcommon: %d\n", l1, l2, common)
}
func seedDists(pcas []*dynamicPCA, traces [][]byte) {
	centers, vars, glbBasis := mergeBasis(pcas)
	if glbBasis == nil { // Means there was an error.
		return
	}
	fmt.Printf("vars: %.3v\n", vars)

	var traceMats []*mat.Dense
	for _, trace := range traces {
		m := mat.NewDense(1, mapSize, nil)
		for i, tr := range trace {
			v := logVals[tr]
			v -= centers[i]
			m.Set(0, i, v)
		}
		traceMats = append(traceMats, m)
	}

	var projs [][]float64
	for _, m := range traceMats {
		proj := new(mat.Dense)
		proj.Mul(m, glbBasis)
		projs = append(projs, proj.RawRowView(0))
	}

	a, b := 0, 1
	orgDist := euclideanDist(traceMats[a].RawRowView(0), traceMats[b].RawRowView(0))
	eDist := euclideanDist(projs[a], projs[b])
	mDist := mahaDist(projs[a], projs[b], vars)
	fmt.Printf("orgDist, eDist, mDist = %.3v, %.3v, %.3v\n", orgDist, eDist, mDist)
	a, b = 2, 3
	orgDist = euclideanDist(traceMats[a].RawRowView(0), traceMats[b].RawRowView(0))
	eDist = euclideanDist(projs[a], projs[b])
	mDist = mahaDist(projs[a], projs[b], vars)
	fmt.Printf("orgDist, eDist, mDist = %.3v, %.3v, %.3v\n", orgDist, eDist, mDist)
}

// *****************************************************************************
// ******************************** Interrupt **********************************

var intChans = newIntMulti()

func init() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		for s := range sigChan {
			fmt.Printf("Signal: %+v\n", s)
			intChans.signal(s)
		}
	}()
}

type interruptMultiplexer struct {
	mtx   sync.Mutex
	chans map[int]chan os.Signal
}

func newIntMulti() *interruptMultiplexer {
	return &interruptMultiplexer{
		chans: make(map[int]chan os.Signal),
	}
}

func (intChans *interruptMultiplexer) add() (
	key int, sigChan chan os.Signal) {

	sigChan = make(chan os.Signal, 1)
	key = rand.Int()
	intChans.mtx.Lock()
	intChans.chans[key] = sigChan
	intChans.mtx.Unlock()
	return key, sigChan
}
func (intChans *interruptMultiplexer) del(key int) {
	intChans.mtx.Lock()
	delete(intChans.chans, key)
	intChans.mtx.Unlock()
}

func (intChans *interruptMultiplexer) signal(s os.Signal) {
	intChans.mtx.Lock()
	for _, c := range intChans.chans {
		if len(c) > 0 { // This channel was already signaled.
			continue
		}
		c <- s
	}
	intChans.mtx.Unlock()
}
