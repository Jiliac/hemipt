package main

import (
	"sort"
)

type seedT struct {
	runT

	execN int

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
				return seeds[i].execN > seeds[j].execN
			})
			seed := seeds[len(seeds)-1]

			if seed.execN == 0 {
				seed.exec.discoveryFit = fitnessMultiplexer{
					newBrCovFitFunc(), newPCAFitFunc()}
			}
			seed.execN++

			t.execChan <- seed.exec
			go func(t *thread) { <-t.endChan; sched.threadChan <- t }(t)
		}
	}

	var executors []*executor
	for _, seed := range seeds {
		executors = append(executors, seed.exec)
	}
	sched.executorsChan <- executors
}
