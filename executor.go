package main

import (
	"log"

	"os"
	"runtime"
	"sync"
	"time"
)

// *****************************************************************************
// ********************************* Thread ************************************

type thread struct {
	put *aflPutT
	cpu int

	execChan chan *executor
	endChan  chan struct{}
}

func startThread(binPath string, cliArgs []string) (t *thread, ok bool) {
	t = &thread{
		execChan: make(chan *executor),
		endChan:  make(chan struct{}),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		ok, t.cpu = lockRoutine()
		if !ok {
			wg.Done()
			log.Println("Couldn't lock routine.")
			return
		}

		t.put, ok = startAFLPUT(binPath, cliArgs, 100*time.Millisecond)
		wg.Done()
		if !ok {
			return
		}

		_, sigChan := intChans.add() // Get notified when interrupted.
		for e := range t.execChan {
			if e.oneExec {
				e.executeOne(t.put)
			} else {
				e.executeLoop(t.put, sigChan)
			}
			t.endChan <- struct{}{}
		}
	}()
	wg.Wait()

	return t, ok
}

func (t *thread) clean() { t.put.clean() }

func startMultiThreads(n int, binPath string, cliArgs []string) (
	threads []*thread, ok bool) {

	nbCPU := runtime.NumCPU()
	if n > nbCPU {
		log.Fatalf("There are only %d CPUs but you ask for %d threads.\n",
			nbCPU, n)
	}

	for i := 0; i < n; i++ {
		t, ok := startThread(binPath, cliArgs)
		if !ok {
			return threads, ok
		}
		threads = append(threads, t)
	}
	ok = true
	return threads, ok
}

// *****************************************************************************
// ******************************** Executor ***********************************
// On a thread. Execute the "big function" (meaning costly in time).

type executor struct {
	ig             inputGen
	discoveryFit   fitnessFunc
	securityPolicy fitnessFunc

	fitChan, crashChan chan<- runT
	oneExec            bool
}

func (e executor) executeOne(put *aflPutT) {
	testCase := e.ig.generate()

	runInfo, _ := put.run(testCase)

	runInfo.trace = make([]byte, len(put.trace))
	copy(runInfo.trace, put.trace)
	dF := e.discoveryFit.isFit(runInfo)
	isCrash := e.securityPolicy.isFit(runInfo)
	//
	if dF || isCrash {
		if dF {
			e.fitChan <- runInfo
		}
		if isCrash {
			e.crashChan <- runInfo
		}
	}
}
func (e executor) executeLoop(put *aflPutT, sigChan chan os.Signal) {
	timer := time.NewTimer(roundTime)
	fuzzContinue := true
	for fuzzContinue {
		select {
		case _ = <-sigChan:
			fuzzContinue = false
			break
		case _ = <-timer.C:
			fuzzContinue = false
			break

		default:
			testCase := e.ig.generate()

			runInfo, _ := put.run(testCase)

			runInfo.trace = make([]byte, len(put.trace))
			copy(runInfo.trace, put.trace)
			dF := e.discoveryFit.isFit(runInfo)
			isCrash := e.securityPolicy.isFit(runInfo)
			//
			if dF || isCrash {
				if dF {
					e.fitChan <- runInfo
				}
				if isCrash {
					e.crashChan <- runInfo
				}
			}
		}
	}
}
