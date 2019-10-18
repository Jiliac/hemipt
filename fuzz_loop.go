package main

import (
	"fmt"
	"log"

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

// Debug/test for now
func analyzeExecs(executors []*executor) {
	fmt.Println("")
	pcas := getPCAs(executors)
	basis1, basis2 := pcas[0].basis, pcas[1].basis

	basisProj := new(mat.Dense)
	basisProj.Mul(basis1.T(), basis2)
	convCrit := computeConvergence(basisProj)
	fmt.Printf("convCrit: %.3v\n", convCrit)
	fmt.Printf("Basis projection:\n%.3v\n", mat.Formatted(basisProj))
}
func getPCAs(executors []*executor) (pcas []*dynamicPCA) {
	for _, e := range executors {
		df := e.discoveryFit
		if ff, ok := df.(fitnessMultiplexer); ok {
			for _, ffi := range ff {
				if pcaFit, ok := ffi.(*pcaFitFunc); ok {
					pcas = append(pcas, pcaFit.dynpca)
				}
			}
		} else if pcaFit, ok := df.(*pcaFitFunc); ok {
			pcas = append(pcas, pcaFit.dynpca)
		}
	}
	return pcas
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
