package main

// On a thread. Execute the "big function" (meaning costly in time).

type executor struct {
	ig             inputGen
	put            aflPutT
	discoveryFit   fitnessFunc
	securityPolicy fitnessFunc

	fitChan, crashChan chan<- runMeta
}

func (e executor) execute() {
	testCase := e.ig.generate()

	runInfo, _ := e.put.run(testCase)

	dF := e.discoveryFit.isFit(runInfo)
	isCrash := e.securityPolicy.isFit(runInfo)
	//
	if dF || isCrash {
		runInfo.trace = make([]byte, len(e.put.trace))
		copy(runInfo.trace, e.put.trace)
		if dF {
			e.fitChan <- runInfo
		}
		if isCrash {
			e.crashChan <- runInfo
		}
	}
}
