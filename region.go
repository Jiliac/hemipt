package main

import "math"

type regionT struct {
	proj []float64

	speciesMap map[uint64]struct{}
	speciesN   int
	sampleN    int

	// Additional stats
	distSum, sqDistSum, thirdDistSum, quadDistSum float64
}

func makeRegion(proj []float64) regionT {
	r := regionT{
		proj:       make([]float64, len(proj)),
		speciesMap: make(map[uint64]struct{}),
	}
	copy(r.proj, proj)
	return r
}

func findRegion(regions []regionT, pt []float64, hash uint64) {
	var closestRI int
	minDist := math.MaxFloat64
	for i, r := range regions {
		var dist float64
		for j, p := range r.proj {
			diff := p - pt[j]
			dist += diff * diff
		}
		if dist < minDist {
			minDist = dist
			closestRI = i
		}
	}

	regions[closestRI].sampleN++
	if _, ok := regions[closestRI].speciesMap[hash]; !ok {
		regions[closestRI].speciesMap[hash] = struct{}{}
		regions[closestRI].speciesN++
	}

	const recordMoreStats = true
	if recordMoreStats {
		root := math.Sqrt(minDist)
		regions[closestRI].distSum += root
		regions[closestRI].sqDistSum += minDist
		regions[closestRI].thirdDistSum += root * minDist
		regions[closestRI].quadDistSum += minDist * minDist
	}
}

func (r regionT) expectedSampleReward() float64 {
	if r.speciesN == 0 {
		return 1
	}
	specN := float64(r.speciesN)
	discoveryP := specN / float64(r.sampleN)
	discoveryR := math.Log((specN + 1) / specN)
	return discoveryP * discoveryR // If specN is high, this is approximatively 1/sampleN
}
