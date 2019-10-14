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

	var wg sync.WaitGroup
	for i, seedI := range seedInputs {

		e := &executor{
			ig:             seedCopier(seedI),
			discoveryFit:   trueFitFunc{},
			securityPolicy: falseFitFunc{},
			fitChan:        devNullFitChan,
			crashChan:      devNullFitChan,
		}

		wg.Add(1)
		go func(t *thread, e *executor) {
			t.execChan <- e
			<-t.endChan
			wg.Done()
		}(threads[i], e)
	}

	wg.Wait()
}
