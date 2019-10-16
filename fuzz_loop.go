package main

import (
	"fmt"
	"log"

	"math/rand"
	"os"
	"os/signal"
	"sync"
)

func fuzzLoop(threads []*thread, seedInputs [][]byte) {
	if len(seedInputs) > len(threads) {
		log.Println("For now, does not support seed scheduling. " +
			"Need at least one thread per seed.")
		return
	}

	fitChan := makeGlbFitness()

	var wg sync.WaitGroup
	for i, seedI := range seedInputs {
		discoveryFit := fitnessMultiplexer{newBrCovFitFunc(), newPCAFitFunc()}
		e := &executor{
			ig:             makeRatioMutator(seedI, 1.0/100),
			discoveryFit:   discoveryFit,
			securityPolicy: falseFitFunc{},
			fitChan:        fitChan,
			crashChan:      devNullFitChan,
		}

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

			intChans.del(key)
			wg.Done()

			fmt.Printf("Local fitness: %v\n", e.discoveryFit)
		}(threads[i], e)
	}

	wg.Wait()
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
