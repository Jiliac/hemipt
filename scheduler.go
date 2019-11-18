package main

import (
	"fmt"

	"math/rand"
	"sort"
	"time"
)

type scheduler struct {
	// Global fitness send new seeds there.
	newSeedChan chan *seedT

	// Once their executions are finished, empty thread are sent here.
	threadChan chan *thread

	seedsChan chan []*seedT
}

func newScheduler(threads []*thread, initSeeds []*seedT, fitChan chan runT) (
	sched *scheduler) {

	sched = &scheduler{
		newSeedChan: make(chan *seedT),
		threadChan:  make(chan *thread),
		seedsChan:   make(chan []*seedT),
	}

	go func() {
		for _, seed := range initSeeds {
			sched.newSeedChan <- seed
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
	var sleepingThreads []*thread

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
			if newSeed.exec == nil {
				newSeed.exec = &executor{
					ig:             makeRatioMutator(newSeed.input, mutationRatio),
					securityPolicy: falseFitFunc{},
					fitChan:        fitChan,
					crashChan:      devNullFitChan,
				}
			} else {
				newSeed.exec.fitChan = fitChan
			}
			seeds = append(seeds, newSeed)
			//
			if len(sleepingThreads) > 0 {
				threadRunningN++
				l := len(sleepingThreads)
				t := sleepingThreads[l-1]
				sleepingThreads = sleepingThreads[:l-1]
				go func() { sched.threadChan <- t }()
			}

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
				sleepingThreads = append(sleepingThreads, t)
				continue
			} else if seed.running {
				threadRunningN++
				go sched.postponeThread(t)
				continue
			}

			if seed.exec.discoveryFit == nil {
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
	var cnt, totExecN int
	for _, seed := range seeds {
		totExecN += seed.execN
		if seed.execN >= fuzzRoundN {
			cnt++
		}
	}

	goal := fuzzRoundN * len(seeds)
	progress := 100 * float64(totExecN) / float64(goal)
	fmt.Printf("Fuzzed seeds: %d/%d\tprogress: %d/%d (%.1f%%)\n",
		cnt, len(seeds), totExecN, goal, progress)
}
