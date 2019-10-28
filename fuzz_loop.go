package main

import (
	"fmt"
	"log"

	"encoding/csv"
	"math/rand"
	"os"
	"os/signal"
	"sync"

	"gonum.org/v1/gonum/mat"
)

func fuzzLoop(threads []*thread, seedInputs [][]byte) (executors []*executor) {
	if len(seedInputs) > len(threads) {
		log.Println("For now, does not support seed scheduling. " +
			"Need at least one thread per seed.")
		return
	}

	var wg sync.WaitGroup
	fitChan := makeGlbFitness()

	for i, seedI := range seedInputs {
		discoveryFit := fitnessMultiplexer{newBrCovFitFunc(), newPCAFitFunc()}
		e := &executor{
			ig:             makeRatioMutator(seedI, 1.0/100),
			discoveryFit:   discoveryFit,
			securityPolicy: falseFitFunc{},
			fitChan:        fitChan,
			crashChan:      devNullFitChan,
		}
		executors = append(executors, e)

		wg.Add(1)
		go func(t *thread, e *executor) {
			fuzzContinue := true
			key, sigChan := intChans.add()

			for fuzzContinue {
				select {
				case _ = <-sigChan:
					fuzzContinue = false
					break

				default:
					t.execChan <- e
					<-t.endChan
				}
			}

			fmt.Printf("Local fitness: %v\n", e.discoveryFit)

			intChans.del(key)
			wg.Done()
		}(threads[i], e)
	}

	wg.Wait()
	return executors
}

func getSeedTrace(threads []*thread, seedInputs [][]byte) (traces [][]byte) {
	traces = make([][]byte, len(seedInputs))
	fitChan := make(chan runMeta, 1)
	t := threads[0]

	for i, seedI := range seedInputs {
		e := &executor{
			ig:             seedCopier(seedI),
			discoveryFit:   trueFitFunc{},
			securityPolicy: falseFitFunc{},
			fitChan:        fitChan,
			crashChan:      devNullFitChan,
		}

		t.execChan <- e
		<-t.endChan
		runInfo := <-fitChan
		traces[i] = runInfo.trace
	}

	return traces
}

// Debug/test for now
func analyzeExecs(executors []*executor, traces [][]byte) {
	fmt.Println("")
	pcaFits := getPCAFits(executors)
	pcas := getPCAs(pcaFits)
	basis1, basis2 := pcas[0].basis, pcas[1].basis

	basisProj := new(mat.Dense)
	basisProj.Mul(basis1.T(), basis2)
	convCrit := computeConvergence(basisProj)
	fmt.Printf("convCrit: %.3v\n", convCrit)
	fmt.Printf("Basis projection:\n%.3v\n", mat.Formatted(basisProj))

	//compareHashes(pcaFits[0].hashes, pcaFits[1].hashes)
	fmt.Printf("div(s1, s1):\t%.3v\n\n", klDiv(pcas[0], pcas[0]))
	fmt.Printf("div(s1, s2):\t%.3v\n\n", klDiv(pcas[0], pcas[1]))
	fmt.Printf("div(s2, s1):\t%.3v\n\n", klDiv(pcas[1], pcas[0]))
	seedDists(pcas, traces)

	exportHistos(pcas, "./histos.csv")
}
func getPCAFits(executors []*executor) (pcaFits []*pcaFitFunc) {
	for _, e := range executors {
		df := e.discoveryFit
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
	for _, pca := range pcas {
		m := mat.NewDense(1, mapSize, nil)
		for i, v := range pca.centers[:] {
			m.Set(0, i, v-centers[i])
		}
		traceMats = append(traceMats, m)
	}

	var projs [][]float64
	for _, m := range traceMats {
		proj := new(mat.Dense)
		proj.Mul(m, glbBasis)
		projs = append(projs, proj.RawRowView(0))
	}

	for i, proj := range projs {
		fmt.Printf("proj[%d]:\t%.3v\n", i, proj)
	}

	orgDist := euclideanDist(traceMats[2].RawRowView(0), traceMats[3].RawRowView(0))
	eDist := euclideanDist(projs[2], projs[3])
	mDist := mahaDist(projs[2], projs[3], vars)
	fmt.Printf("orgDist, eDist, mDist = %.3v, %.3v, %.3v\n", orgDist, eDist, mDist)
}

func exportHistos(pcas []*dynamicPCA, path string) {
	if len(pcas) == 0 {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("Problem opening histo CSV file: %v.\n", err)
		return
	}

	w := csv.NewWriter(f)
	records := [][]string{[]string{
		"seed_n", "dim_n", "bin_n", "start", "end", "count",
	}}

	for i := range pcas {
		histos, steps := pcas[i].stats.histos, pcas[i].stats.steps
		for j, histo := range histos {
			for k, cnt := range histo {
				//start, end := float64(i)*step, float64(i+1)*step
				records = append(records, []string{
					fmt.Sprintf("%d", i),                     // seed_n
					fmt.Sprintf("%d", j),                     // dim_n
					fmt.Sprintf("%d", k),                     // bin_n
					fmt.Sprintf("%f", float64(k)*steps[j]),   // start
					fmt.Sprintf("%f", float64(k+1)*steps[j]), // end
					fmt.Sprintf("%f", cnt),                   // count
				})
			}
		}
	}

	w.WriteAll(records)
	if err := w.Error(); err != nil {
		log.Printf("Couldn't record histograms: %v.\n", err)
	}
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
