package main

import (
	"fmt"

	"sync"
	"time"
)

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
			return
		}

		t.put, ok = startAFLPUT(binPath, cliArgs, 100*time.Millisecond)
		wg.Done()
		if !ok {
			return
		}

		for e := range t.execChan {
			e.execute(t.put)
			t.endChan <- struct{}{}
		}
	}()
	wg.Wait()

	return t, ok
}

func (t *thread) clean() { t.put.clean() }

// *****************************************************************************
// ******************************** Executor ***********************************
// On a thread. Execute the "big function" (meaning costly in time).

type executor struct {
	ig             inputGen
	discoveryFit   fitnessFunc
	securityPolicy fitnessFunc

	fitChan, crashChan chan<- runMeta
}

func (e executor) execute(put *aflPutT) {
	testCase := e.ig.generate()

	runInfo, _ := put.run(testCase)

	dF := e.discoveryFit.isFit(runInfo)
	isCrash := e.securityPolicy.isFit(runInfo)
	//
	if dF || isCrash {
		runInfo.trace = make([]byte, len(put.trace))
		copy(runInfo.trace, put.trace)
		if dF {
			e.fitChan <- runInfo
		}
		if isCrash {
			e.crashChan <- runInfo
		}

		hash := hashTrBits(runInfo.trace)
		fmt.Printf("hash: 0x%x\n", hash)
	}
}
