package main

import (
	//"fmt"

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
	newSeedChan chan seedT

	// Once their executions are finished, empty thread are sent here.
	threadChan chan *thread

	executorsChan chan []*executor
}

func newScheduler(threads []*thread, seedInputs [][]byte, fitChan chan runT) (
	sched *scheduler) {

	sched = &scheduler{
		newSeedChan:   make(chan seedT),
		threadChan:    make(chan *thread),
		executorsChan: make(chan []*executor),
	}

	go func() {
		for _, seedI := range seedInputs {
			sched.newSeedChan <- seedT{runT: runT{input: seedI}}
		}
		for _, t := range threads {
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
			seeds = append(seeds, &newSeed)

		case t := <-sched.threadChan:
			sort.Slice(seeds, func(i, j int) bool {
				// Could be optimized ofc. But not necessary imo.
				if a, b := seeds[i].running, seeds[j].running; a != b {
					return a && !b
				}
				return seeds[i].execN > seeds[j].execN
			})
			seed := seeds[len(seeds)-1]
			if seed.running {
				go func(t *thread) {
					time.Sleep(roundTime + 500*time.Millisecond)
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
				//fmt.Printf("Local fitness: %v\n", e.discoveryFit)
				seed.running = false
				sched.threadChan <- t
			}(t, seed)
		}
	}

	var executors []*executor
	for _, seed := range seeds {
		if seed.execN == 0 {
			continue
		}
		executors = append(executors, seed.exec)
	}
	sched.executorsChan <- executors
}
