package main

import (
	"fmt"

	"math/rand"
	"os"
	"os/signal"
	"sync"
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
