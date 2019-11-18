package main

import (
	"fmt"

	"math/rand"
	"os"
	"os/signal"
	"sync"
)

func execInitSeed(threads []*thread, seedInputs [][]byte) (initSeeds []*seedT) {
	initSeeds = make([]*seedT, len(seedInputs))
	fitChan := make(chan runT, 1)
	t := threads[0] // @TODO: For speed, should use all threads, not just one.

	for i, input := range seedInputs {
		e := &executor{
			ig:             seedCopier(input),
			discoveryFit:   trueFitFunc{},
			securityPolicy: falseFitFunc{},
			fitChan:        fitChan,
			crashChan:      devNullFitChan,
			oneExec:        true,
		}

		t.execChan <- e
		<-t.endChan
		runInfo := <-fitChan
		initSeeds[i] = &seedT{runT: runInfo}
	}

	return initSeeds
}

func fuzzLoop(threads []*thread, initSeeds []*seedT) (seeds []*seedT) {
	fitChan := make(chan runT, 1000)
	sched := newScheduler(threads, initSeeds, fitChan)
	stopChan := makeGlbFitness(fitChan, sched.newSeedChan, initSeeds)

	seeds = <-sched.seedsChan
	stopChan <- struct{}{}
	return seeds
}

// *****************************************************************************
// ******************************** Interrupt **********************************

var (
	intChans       = newIntMulti()
	wasInterrupted bool
)

func init() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		for s := range sigChan {
			fmt.Printf("Signal: %+v\n", s)
			wasInterrupted = true
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
