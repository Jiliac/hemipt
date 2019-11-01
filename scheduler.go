package main

import (
	//"fmt"

	"math/rand"
	"sort"
	"time"
)

type seedT struct {
	runT

	execN   int
	running bool

	exec *executor
}
type scheduler struct {
	// Global fitness send new seeds there.
	newSeedChan chan *seedT

	// Once their executions are finished, empty thread are sent here.
	threadChan chan *thread

	seedsChan chan []*seedT
}

func newScheduler(threads []*thread, seedInputs [][]byte, fitChan chan runT) (
	sched *scheduler) {

	sched = &scheduler{
		newSeedChan: make(chan *seedT),
		threadChan:  make(chan *thread),
		seedsChan:   make(chan []*seedT),
	}

	go func() {
		for _, seedI := range seedInputs {
			sched.newSeedChan <- &seedT{runT: runT{input: seedI}}
		}
		for _, t := range threads {
			r := time.Duration(rand.Intn(700)) + 300
			time.Sleep(r * time.Millisecond)
			sched.threadChan <- t
		}
	}()

	go sched.schedule(fitChan)

	return sched
}

func (sched *scheduler) schedule(fitChan chan runT) {
	var seeds []*seedT

	_, sigChan := intChans.add() // Get notified when interrupted.

	fuzzContinue := true
	for fuzzContinue {
		select {
		case _ = <-sigChan:
			fuzzContinue = false
			break

		case newSeed := <-sched.newSeedChan:
			newSeed.exec = &executor{
				ig:             makeRatioMutator(newSeed.input, 1.0/100),
				securityPolicy: falseFitFunc{},
				fitChan:        fitChan,
				crashChan:      devNullFitChan,
			}
			seeds = append(seeds, newSeed)

		case t := <-sched.threadChan:
			// Is this sort too slow?
			// Can be optimized but need a special structure :/
			sort.Slice(seeds, func(i, j int) bool {
				if a, b := seeds[i].running, seeds[j].running; a != b {
					return a && !b
				}
				return seeds[i].execN > seeds[j].execN
			})
			seed := seeds[len(seeds)-1]
			if seed.running {
				go func(t *thread) {
					r := time.Duration(rand.Intn(700)) + 300
					time.Sleep(roundTime + r*time.Millisecond)
					sched.threadChan <- t
				}(t)
				continue
			}

			if seed.execN == 0 {
				seed.exec.discoveryFit = fitnessMultiplexer{
					newBrCovFitFunc(), newPCAFitFunc()}
			}
			seed.execN, seed.running = seed.execN+1, true

			go func(t *thread, seed *seedT) {
				t.execChan <- seed.exec
				<-t.endChan
				seed.running = false
				sched.threadChan <- t
			}(t, seed)
		}
	}

	sched.seedsChan <- seeds
}
