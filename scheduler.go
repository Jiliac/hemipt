package main

import (
	"fmt"

	"math/rand"
	"sort"
	"time"
)

const fuzzRoundN = 5

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

	go sched.schedule(fitChan, len(threads))

	return sched
}

func (sched *scheduler) schedule(fitChan chan runT, threadRunningN int) {
	var seeds []*seedT

	_, sigChan := intChans.add() // Get notified when interrupted.

	fuzzContinue := true
	printTicker := time.NewTicker(printTickT)
	for fuzzContinue {
		select {
		case _ = <-sigChan:
			fuzzContinue = false
			break
		case _ = <-printTicker.C:
			printStatus(seeds)

		case newSeed := <-sched.newSeedChan:
			newSeed.exec = &executor{
				ig:             makeRatioMutator(newSeed.input, 1.0/100),
				securityPolicy: falseFitFunc{},
				fitChan:        fitChan,
				crashChan:      devNullFitChan,
			}
			seeds = append(seeds, newSeed)

		case t := <-sched.threadChan:
			threadRunningN--
			// Is this sort too slow?
			// Can be optimized but need a special structure :/
			sort.Slice(seeds, func(i, j int) bool {
				if a, b := seeds[i].running, seeds[j].running; a != b {
					return a && !b
				}
				return seeds[i].execN > seeds[j].execN
			})
			seed := seeds[len(seeds)-1]
			if seed.execN >= fuzzRoundN {
				if threadRunningN == 0 {
					fuzzContinue = false
					break
				}
				continue
			} else if seed.running {
				go sched.postponeThread(t)
				continue
			}

			if seed.execN == 0 {
				seed.exec.discoveryFit = fitnessMultiplexer{
					newBrCovFitFunc(), newPCAFitFunc()}
			}
			seed.execN, seed.running = seed.execN+1, true

			threadRunningN++
			go sched.execSeed(t, seed)
		}
	}

	sched.seedsChan <- seeds
}

func (sched *scheduler) postponeThread(t *thread) {
	r := time.Duration(rand.Intn(700)) + 300
	time.Sleep(roundTime + r*time.Millisecond)
	sched.threadChan <- t
}
func (sched scheduler) execSeed(t *thread, seed *seedT) {
	t.execChan <- seed.exec
	<-t.endChan
	seed.running = false
	sched.threadChan <- t
}

func printStatus(seeds []*seedT) {
	var cnt int
	for _, seed := range seeds {
		if seed.execN >= fuzzRoundN {
			cnt++
		}
	}
	fmt.Printf("Fuzzed seeds: %d/%d\n", cnt, len(seeds))
}
