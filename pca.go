package main

import (
	"fmt"
	"log"

	"math"
	"sort"
	"sync"
	"time"

	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"

	"github.com/olekukonko/tablewriter"
	"strings"
)

// Phase 1 (short): fitness function collect some traces and then initialize the
// dynamicPCA. (Implemented in newDynPCA).
// Phase 2 (short): collect data to recenter.
// Phase 3 (short): collect data to rotate the basis.
// Phase 4 (indefinite): full DynPCA algorithm. rotate and add new axis.

type dynamicPCA struct {
	// Y = (X-center)' * basis
	centers [mapSize]float64
	basis   *mat.Dense

	sampleN int
	sqNorm  float64
	sums    [mapSize]float64
	covMat  *mat.Dense // Covariance Matrix; cumulative

	// Phase-based initialization
	startT, recenterT      time.Time
	phase2, phase3, phase4 bool
}

func newDynPCA(queue [][]byte) (ok bool, dynpca *dynamicPCA) {
	dynpca = new(dynamicPCA)

	// ** 1. Compute centers **
	for _, trace := range queue {
		for j, tr := range trace {
			dynpca.sums[j] += logVals[tr]
		}
	}

	// ** 2. Format Data**
	dynpca.sampleN = len(queue)
	samplesMat := mat.NewDense(dynpca.sampleN, mapSize, nil)
	for j := 0; j < mapSize; j++ {
		dynpca.centers[j] = dynpca.sums[j] / float64(dynpca.sampleN)
		for i := 0; i < dynpca.sampleN; i++ {
			y := logVals[queue[i][j]] - dynpca.centers[j]
			samplesMat.Set(i, j, y)
		}
	}

	// ** 3. PCA **
	var pc stat.PC
	ok = pc.PrincipalComponents(samplesMat, nil)
	if !ok {
		return ok, dynpca
	}
	ok = true

	// ** 4. Prepare Structure **
	vecs := new(mat.Dense)
	pc.VectorsTo(vecs)
	dynpca.basis = mat.DenseCopyOf(vecs.Slice(0, mapSize, 0, pcaInitDim))
	//
	dynpca.covMat = mat.NewDense(pcaInitDim, pcaInitDim, nil)
	vars := pc.VarsTo(nil)
	for i := 0; i < pcaInitDim; i++ {
		dynpca.covMat.Set(i, i, float64(dynpca.sampleN)*vars[i])
	}
	//
	dynpca.phase2 = true
	dynpca.startT = time.Now()

	return ok, dynpca
}

func (dynpca *dynamicPCA) newSample(trace []byte) {
	if dynpca.phase2 && time.Now().Sub(dynpca.startT) > phase2Dur {
		dynpca.recenter()
		dynpca.recenterT = time.Now()
		dynpca.phase2, dynpca.phase3 = false, true
		//
	} else if dynpca.phase3 && time.Now().Sub(dynpca.recenterT) > phase3Dur {
		// @TODO: start adding new axis (Gram-Schmidt)?
		ok := dynpca.rotate()
		if ok {
			dynpca.phase3, dynpca.phase4 = false, true
		} else {
			dynpca.recenterT = time.Now()
		}
	}

	// ** 1. Center data **
	dynpca.sampleN++
	sampMat := mat.NewDense(1, mapSize, nil)
	for i, tr := range trace {
		v := logVals[tr]
		dynpca.sums[i] += v
		v -= dynpca.centers[i]
		dynpca.sqNorm += v * v
		sampMat.Set(0, i, v)
	}

	// ** 2. Project **
	projMat := new(mat.Dense)
	projMat.Mul(sampMat, dynpca.basis)

	// ** 3. Update covariance matrix **
	covs := new(mat.Dense)
	covs.Mul(projMat.T(), projMat)
	dynpca.covMat.Add(dynpca.covMat, covs)
}

func (dynpca *dynamicPCA) recenter() {
	var diff float64
	n := float64(dynpca.sampleN)
	newSampN := dynpca.sampleN / 10
	for i := 0; i < mapSize; i++ {
		c := dynpca.sums[i] / n
		d := dynpca.centers[i] - c
		diff += d * d
		//
		dynpca.centers[i] = c
		dynpca.sums[i] = c * float64(newSampN)
	}
	diff = math.Sqrt(diff)

	m := new(mat.Dense)
	ratio := float64(newSampN) / n
	m.Scale(ratio, dynpca.covMat)
	dynpca.covMat = m
	dynpca.sampleN = newSampN
}

func (dynpca *dynamicPCA) rotate() (ok bool) {
	// ** 1. Prepare data **
	_, basisSize := dynpca.basis.Dims()
	covs := make([]float64, basisSize*basisSize)
	for i := 0; i < basisSize; i++ {
		for j := 0; j < basisSize; j++ {
			cov := dynpca.covMat.At(i, j) / float64(dynpca.sampleN)
			covs[i*basisSize+j] = cov
		}
	}
	covMat := mat.NewSymDense(basisSize, covs)

	// ** 2. Rotate **
	ok, eVals, eVecs := factorize(covMat, basisSize)
	if !ok {
		log.Print("Could not factorize covariance matrix.")
		return ok
	}
	ok = true

	// Test print
	convCrit := computeConvergence(eVecs)
	m := new(mat.Dense)
	m.Scale(1/float64(dynpca.sampleN), dynpca.covMat)
	m.Mul(eVecs.T(), m)
	m.Mul(m, eVecs)

	// ** 3. Apply decomposition **
	if convCrit > convCritFloor {
		dynpca.covMat = mat.NewDense(basisSize, basisSize, nil)
		for i := 0; i < basisSize; i++ {
			dynpca.covMat.Set(i, i, eVals[i]*float64(dynpca.sampleN))
		}
		dynpca.basis.Mul(dynpca.basis, eVecs)
	}

	return ok
}
func factorize(symMat *mat.SymDense, basisSize int) (
	ok bool, eVals []float64, eVecs *mat.Dense) {
	// Gonum library is a little weird about its eigen decomposiiton
	// implementation. So make a helper function around it.

	var eigsym mat.EigenSym
	ok = eigsym.Factorize(symMat, true)
	if !ok {
		return
	}
	//
	vars := eigsym.Values(nil)
	eVecs = new(mat.Dense)
	eigsym.VectorsTo(eVecs)

	var (
		perm    = make([]int, basisSize)
		permMat = new(mat.Dense)
	)
	eVals = make([]float64, basisSize)
	// a. Find permutation to order eigen vectors/values.
	for i := range perm {
		perm[i] = i
	}
	sort.Slice(perm, func(i, j int) bool {
		indexI, indexJ := perm[i], perm[j]
		return vars[indexI] > vars[indexJ]
	})
	// b. Apply permutation
	for i, index := range perm {
		eVals[i] = vars[index]
	}
	permMat.Permutation(basisSize, perm)
	eVecs.Mul(eVecs, permMat)

	// @TODO: Forth moments should be reset :/
	// But annoying because then I need a number of samples just for this.

	return ok, eVals, eVecs
}
func computeConvergence(ev *mat.Dense) (convCrit float64) {
	r, c := ev.Dims()
	//
	for j := 0; j < c; j++ {
		var maxJ, v float64
		for i := 0; i < r; i++ {
			v = ev.At(i, j)
			v *= v
			if v > maxJ {
				maxJ = v
			}
			convCrit += v
		}
		convCrit -= maxJ
	}
	//
	convCrit /= float64(c)
	return convCrit
}

func (dynpca *dynamicPCA) String() (str string) {
	str = fmt.Sprintf("#sample: %.3v\n", float64(dynpca.sampleN))
	//
	normalizer := 1 / float64(dynpca.sampleN)
	sqNorm := dynpca.sqNorm * normalizer
	//
	var m mat.Dense
	var totSpaceVar float64
	_, basisSize := dynpca.basis.Dims()
	m.Scale(normalizer, dynpca.covMat)
	for i := 0; i < basisSize; i++ {
		totSpaceVar += m.At(i, i)
	}
	str += fmt.Sprintf("Square Norm: %.3v (%.1f%%)\n",
		sqNorm, 100*totSpaceVar/sqNorm)
	//
	str += fmt.Sprintf("Covariance Matrix:\n%.3v", mat.Formatted(&m))

	if false {
		var logDet float64
		for i := 0; i < basisSize; i++ {
			logDet += math.Log(m.At(i, i))
		}
		logDet2, sign := mat.LogDet(&m)
		str += fmt.Sprintf(
			"\nlogDet, logDet2, sign: %.3v, %.3v, %.3v", logDet, logDet2, sign)
	}

	dynpca.recenter()
	return str
}

// *****************************************************************************
// *****************************************************************************
// *************************** Basis Statistics ********************************

type basisStats struct {
	sqNorm   float64    // Square norm (2nd moment); cumulative
	thirdMos *mat.Dense // Third moments; cumulative
	forthMos *mat.Dense // Forth moments; cumulative

	// Histograms (on for each dimensions)
	useHisto bool
	steps    []float64 // The bucket size of each dimension
	histos   []map[int]float64
}

func newStats() *basisStats {
	return &basisStats{
		forthMos: mat.NewDense(1, pcaInitDim, make([]float64, pcaInitDim)),
		thirdMos: mat.NewDense(1, pcaInitDim, make([]float64, pcaInitDim)),
	}
}

func (stats *basisStats) initHisto(vars []float64) { // This is mostly just allocation.
	stats.useHisto = true
	stats.steps = make([]float64, len(vars))
	stats.histos = make([]map[int]float64, len(vars))
	for i, v := range vars {
		stats.steps[i] = math.Sqrt(v) / bucketSensitiveness
		stats.histos[i] = make(map[int]float64)
		for j := -5; j <= 5; j++ {
			stats.histos[i][j] = 0
		}
	}
}

func tripling(i, j int, v float64) float64    { return v * v * v }
func quadrupling(i, j int, v float64) float64 { return v * v * v * v }
func (stats *basisStats) addProj(projMat *mat.Dense) {
	if stats.useHisto {
		var ok bool
		proj := projMat.RawRowView(0)
		for i, val := range proj {
			// If x \in bucket n, then x \in [n*step, (n+1)*step]
			bucket := int(val / stats.steps[i])
			if val < 0 {
				bucket--
			}
			//
			if _, ok = stats.histos[i][bucket]; !ok {
				stats.histos[i][bucket] = 0
			}
			stats.histos[i][bucket] += 1
		}
	}

	var tripleM, quadM mat.Dense
	tripleM.Apply(tripling, projMat)
	quadM.Apply(quadrupling, projMat)
	stats.thirdMos.Add(stats.thirdMos, &tripleM)
	stats.forthMos.Add(stats.forthMos, &quadM)
}

func (stats *basisStats) getMoments(covMat *mat.Dense, normalizer float64) (
	tm, fm *mat.Dense) {
	tm, fm = new(mat.Dense), new(mat.Dense)
	tm.Scale(normalizer, stats.thirdMos)
	fm.Scale(normalizer, stats.forthMos)
	tm.Apply(func(i, j int, v float64) float64 {
		variance := covMat.At(j, j)
		return v / math.Pow(variance, 1.5)
	}, tm)
	fm.Apply(func(i, j int, v float64) float64 {
		variance := covMat.At(j, j)
		return v / (variance * variance)
	}, fm)
	return tm, fm
}

// *****************************************************************************
// ************************** Histogram Analysis *******************************
// Some modelization test.

// *** Some distribtutions CDF definition ***
var sqrt2 = math.Sqrt(2)

type distribution interface{ cdf(x float64) float64 }
type uniDist struct{ start, end float64 }
type normalDist struct{ mu, sig float64 }

func (ud uniDist) cdf(x float64) float64 {
	if x < ud.start {
		return 0
	} else if x > ud.end {
		return 1
	}
	return (x - ud.start) / (ud.end - ud.start)
}
func (nd normalDist) cdf(x float64) (c float64) {
	c = (x - nd.mu) / (nd.sig * sqrt2)
	c = 0.5 * (1 + math.Erf(c))
	return c
}

// *** Statistical Test on Histogram ***
func statHistoTest(histo map[int]float64, step float64, dist distribution) (
	test float64) {
	var min, max int
	var tot float64
	for i, v := range histo {
		tot += v
		if i < min {
			min = i
		} else if i > max {
			max = i
		}
	}

	cum := 1.0
	for i := min; i < max; i++ {
		if _, ok := histo[i]; !ok {
			continue
		}
		start, end := float64(i)*step, float64(i+1)*step
		distVal := dist.cdf((start + end) / 2)
		cnt := histo[i]
		test += cramerVonMises(cum, cum+cnt, tot, distVal)
		cum += cnt
	}
	return math.Sqrt(test)
}
func cramerVonMises(start, end, tot, f float64) (test float64) {
	// sum_{i=a}^b ((2.i-1)/(2.n) - f) =
	//   (b-a+1) [(a.a + a.b - 2.a + b.b - b) / (3.n.n) - c.(a+b)/n + c.c]
	squareSum := start*start + start*end - 2*start + end*end - end
	squareSum = squareSum / (3 * tot * tot)
	sum := f * (start + end) / tot
	test = squareSum - sum + f*f
	//
	interval := end - start + 1
	test *= interval / tot
	return test
}

// *** Printing ***
func printCompoStats(stats *basisStats, tm, fm *mat.Dense) string {
	var b strings.Builder
	table := tablewriter.NewWriter(&b)
	table.SetHeader([]string{"PC", "Skewness", "Kurtosis", "Normal_RSS", "Uni RSS"})

	for i, histo := range stats.histos {
		sig := stats.steps[i] * bucketSensitiveness
		normal := normalDist{mu: 0, sig: sig}
		normRSS := statHistoTest(histo, stats.steps[i], normal)
		uniRSS := statHistoTest(histo, stats.steps[i], uniDist{-sig, sig})

		table.Append([]string{
			fmt.Sprintf("%02d", i),
			fmt.Sprintf("%.3v", tm.At(0, i)),
			fmt.Sprintf("%.3v", fm.At(0, i)),
			fmt.Sprintf("%.3v", normRSS),
			fmt.Sprintf("%.3v", uniRSS),
		})
	}

	table.Render()
	return b.String()
}

// *****************************************************************************
// ***************************** Seed Distance *********************************

func klDiv(p, q *dynamicPCA) (div float64) {
	// 1. Project P covariance matric in Q basis.
	// This projection is not correct. But otherwise, divergence goes to
	// infinity. It's too easy for the divergence to go to infinity :/
	// EMD/Wasserstein would be better?
	qCovMat, inverseQ, basis := inverseMat(q)
	changeMat := new(mat.Dense)
	changeMat.Mul(p.basis.T(), basis)
	//
	tmpProj, projPCovMat, pCovMat := new(mat.Dense), new(mat.Dense), new(mat.Dense)
	pCovMat.Scale(1/float64(p.sampleN), p.covMat)
	tmpProj.Mul(changeMat.T(), pCovMat)
	projPCovMat.Mul(tmpProj, changeMat)

	// 2. Compute the divergence
	dim, _ := pCovMat.Dims()
	//detP, _ := mat.LogDet(pCovMat) // Not sure about this.
	detP, _ := mat.LogDet(projPCovMat)
	detQ, _ := mat.LogDet(qCovMat)
	//
	prod := new(mat.Dense)
	prod.Mul(inverseQ, projPCovMat)
	tr := prod.Trace()
	//
	diffProj, prod2, prod3 := new(mat.Dense), new(mat.Dense), new(mat.Dense)
	diff := matDiff(p.centers[:], q.centers[:])
	diffProj.Mul(diff, basis)
	prod2.Mul(diffProj, inverseQ)
	prod3.Mul(prod2, diffProj.T())
	dist := prod3.At(0, 0)
	//
	div = detQ - detP + tr - float64(dim) + dist

	if div < 0 || div > 1e10 || math.IsInf(div, 0) || math.IsNaN(div) {
		//fmt.Printf("Q-1:\n%.2v\n", mat.Formatted(inverseQ))
		fmt.Printf("(step1) div: %.3v\tdetP, detQ: %.3v, %.3v\n"+
			"(step2) div: %.3v\tTrace-D: %.3v\n"+
			"(step3) div: %.3v\tcenters dist: %.3v\n\n",
			detQ-detP, detP, detQ,
			detQ-detP+tr-float64(dim), tr-float64(dim),
			div, dist)
		//fmt.Printf("Centers Eucledian distance: %.3v\n",
		//	euclideanDist(p.centers[:], q.centers[:]))
	}

	div /= 2
	return div
}
func matDiff(mup, muq []float64) *mat.Dense {
	diff := mat.NewDense(1, mapSize, nil)
	for i, tp := range mup {
		tq := muq[i]
		diff.Set(0, i, tq-tp)
	}
	return diff
}
func inverseMat(pca *dynamicPCA) (covMat, inv, basis *mat.Dense) {
	covMat, inv = new(mat.Dense), new(mat.Dense)
	covMat.Scale(1/float64(pca.sampleN), pca.covMat)
	dim, _ := covMat.Dims()

	maxDim := -1
	for i := 0; i < dim; i++ {
		if covMat.At(i, i) < 1e-5 {
			maxDim = i
			break
		}
	}
	//
	basis = pca.basis
	if maxDim != -1 {
		covMat = covMat.Slice(0, maxDim, 0, maxDim).(*mat.Dense)
		basis = basis.Slice(0, mapSize, 0, maxDim).(*mat.Dense)
	}
	inv.Inverse(covMat)

	return covMat, inv, basis
}

func euclideanDist(mup, muq []float64) (dist float64) {
	for i, tp := range mup {
		diff := tp - muq[i]
		dist += diff * diff
	}
	return math.Sqrt(dist)
}

// *****************************************************************************
// ****************************** Merge Basis **********************************

type mergedBasis struct {
	centers []float64
	basis   *mat.Dense
	vars    []float64
	dimN    int
}

func doMergeBasis(pcas []*dynamicPCA) mergedBasis {
	var (
		totDim    int
		weights   []float64
		basisList []mat.Matrix

		start, end int
		pc         stat.PC
	)

	// ** 1. Compute centers **
	glbCenters := make([]float64, mapSize)
	pcaN := float64(len(pcas))
	for i := range glbCenters {
		for _, pca := range pcas {
			glbCenters[i] += pca.centers[i]
		}
		glbCenters[i] /= pcaN
	}

	// ** 2. Prepare PCA **
	for _, pca := range pcas {
		n := float64(pca.sampleN)
		_, c := pca.basis.Dims()
		for i := 0; i < c; i++ {
			weights = append(weights, pca.covMat.At(i, i)/n)
		}
	}
	weightThreshold := choseWeightThreshold(weights)
	//
	weights = nil
	for _, pca := range pcas {
		n := float64(pca.sampleN)
		_, c := pca.basis.Dims()
		var basisN int
		for i := 0; i < c; i++ {
			w := pca.covMat.At(i, i) / n
			if w < weightThreshold {
				break
			}
			weights = append(weights, w)
			totDim, basisN = totDim+1, basisN+1
		}
		if basisN == 0 {
			continue
		}
		// @TODO: Should shift all the vector of the basis by the new center.
		basisList = append(basisList, pca.basis.Slice(0, mapSize, 0, basisN).T())
	}
	if len(weights) < pcaInitDim {
		log.Println("Too many bad basis to join.")
	}
	//
	m := mat.NewDense(totDim, mapSize, nil)
	for _, basis := range basisList {
		r, _ := basis.Dims()
		end += r
		w := m.Slice(start, end, 0, mapSize).(*mat.Dense)
		w.Copy(basis)
		//
		start = end
	}

	// ** 3. Do PCA **
	r, c := m.Dims()
	fmt.Printf("Joining all: %d, %d\n", r, c)
	ok := pc.PrincipalComponents(m, weights)
	if !ok {
		log.Print("Couldn't do PCA on basis.")
		return mergedBasis{}
	}
	//
	vecs := new(mat.Dense)
	pc.VectorsTo(vecs)
	//
	glbBasis := mat.DenseCopyOf(vecs.Slice(0, mapSize, 0, pcaInitDim))
	//
	vars := make([]float64, pcaInitDim)
	for _, pca := range pcas {
		changeBasisM, newCovMat := new(mat.Dense), new(mat.Dense)
		changeBasisM.Mul(pca.basis.T(), glbBasis)
		newCovMat.Mul(changeBasisM.T(), pca.covMat)
		newCovMat.Mul(newCovMat, changeBasisM)
		for i := range vars {
			vars[i] += newCovMat.At(i, i) / float64(pca.sampleN)
		}
	}
	for i := range vars {
		vars[i] /= float64(len(pcas))
	}

	// Optional: compute how much of the space we lost.
	if true {
		pcVars := pc.VarsTo(nil)
		var inVar, totVar float64
		for i, v := range pcVars {
			totVar += v
			if i < pcaInitDim {
				inVar += v
			}
		}
		fmt.Printf("Loss at merging: %.2f%%\n", 100-100*inVar/totVar)
	}

	return mergedBasis{glbCenters, glbBasis, vars, pcaInitDim}
}

func choseWeightThreshold(weights []float64) float64 {
	const maxWN = 1000
	if len(weights) <= maxWN {
		return 1e-10
	}
	sort.Slice(weights, func(i, j int) bool { return weights[i] > weights[j] })
	thresh := weights[maxWN]
	//
	// Some verbose
	var totVar, lossVar float64
	for i, w := range weights {
		totVar += w
		if i >= maxWN {
			lossVar += w
		}
	}
	fmt.Printf("Basis variance threshold: %.3v\tloss: %.2f%%\n", thresh, 100*lossVar/totVar)
	//
	return thresh
}

// *************
// *** Other ***
func mahaDist(proj1, proj2, vars []float64) (dist float64) {
	for i, p1 := range proj1 {
		diff := p1 - proj2[i]
		dist += diff * diff / vars[i]
	}
	return math.Sqrt(dist)
}

// *****************************************************************************
// ************************* Recurrent Merge Basis *****************************

func prepareMerging(pcas []*dynamicPCA) (basisSlice []mergedBasis) {
	// ** 1. Compute centers **
	glbCenters := make([]float64, mapSize)
	pcaN := float64(len(pcas))
	for i := range glbCenters {
		for _, pca := range pcas {
			glbCenters[i] += pca.centers[i]
		}
		glbCenters[i] /= pcaN
	}

	// ** 2. **
	var wg sync.WaitGroup
	basisSlice = make([]mergedBasis, len(pcas))
	for i, pca := range pcas {
		wg.Add(1)
		go func(i int, pca *dynamicPCA) {

			covMat := new(mat.Dense)
			covMat.Scale(1/float64(pca.sampleN), pca.covMat)

			var pc stat.PC
			pc.PrincipalComponents(covMat, nil)
			vecs, basis := new(mat.Dense), new(mat.Dense)
			pc.VectorsTo(vecs)
			basis.Mul(pca.basis, vecs)

			vars := pc.VarsTo(nil)
			newDim := -1
			for j, v := range vars {
				if v > 1e-10 {
					newDim = j
				}
			}
			if newDim == -1 {
				wg.Done()
				return
			}
			newDim++

			_, dimN := pca.basis.Dims()
			if newDim != dimN {
				basis = basis.Slice(0, mapSize, 0, newDim).(*mat.Dense)
			}
			//for j, c := range glbCenters {
			//	diff := c - pca.centers[j]
			//	r := basis.RawRowView(j)
			//	for k := range r {
			//		r[k] += diff
			//	}
			//}

			basisSlice[i] = mergedBasis{centers: glbCenters, basis: basis,
				vars: vars[:newDim], dimN: newDim}

			wg.Done()
		}(i, pca)
	}
	wg.Wait()

	return basisSlice
}

func doMergeBasisBis(basisSlice []mergedBasis, targetDim int) (bool, mergedBasis) {
	if len(basisSlice) == 0 {
		fmt.Println("Cannot merge empty slice of basis")
		return false, mergedBasis{}
	}

	var totDimN int
	for _, basis := range basisSlice {
		totDimN += basis.dimN
	}

	if totDimN > maxPCADimN {
		if len(basisSlice) == 1 {
			// @TODO: split a basis in multiple parts.
			// Theoritically possible but shouldn't happen ATM.
			panic("Basis division not implemented")
		}
		//
		tasks := prepareTasks(basisSlice)
		ok, mbs := runTasks(tasks, targetDim)
		if !ok {
			return false, mergedBasis{}
		}
		ok, mb := doMergeBasisBis(mbs, targetDim)
		return ok, mb
	}

	// ************************************
	// ************ Reduction *************
	var (
		start, end int
		weights    []float64
		pc         stat.PC
	)
	m := mat.NewDense(totDimN, mapSize, nil)

	for _, basis := range basisSlice {
		end += basis.dimN
		tmp := m.Slice(start, end, 0, mapSize).(*mat.Dense)
		tmp.Copy(basis.basis.T())
		weights = append(weights, basis.vars...)
		start = end
	}

	okPC := pc.PrincipalComponents(m, weights)
	if !okPC {
		log.Println("Couldn't reduce basis")
		return false, mergedBasis{}
	}
	vecs := new(mat.Dense)
	pc.VectorsTo(vecs)
	_, c := vecs.Dims()
	glbBasis := vecs
	if c > targetDim {
		glbBasis = mat.DenseCopyOf(glbBasis.Slice(0, mapSize, 0, targetDim))
	} else {
		targetDim = c
	}
	//
	vars := make([]float64, targetDim)
	newVarEval(basisSlice, glbBasis, vars)
	//loss := newVarEval(basisSlice, glbBasis, vars)
	//fmt.Printf("Intermediary projection loss: %.1f%%\n", 100*loss)

	// @TODO: Cut very low dimensions?

	return true, mergedBasis{basisSlice[0].centers, glbBasis, vars, targetDim}
}

func prepareTasks(basisSlice []mergedBasis) (tasks [][]mergedBasis) {
	var task []mergedBasis
	var taskDimN int
	for _, basis := range basisSlice {
		if taskDimN+basis.dimN > maxPCADimN {
			tasks = append(tasks, task)
			task, taskDimN = nil, 0
		}
		taskDimN += basis.dimN
		task = append(task, basis)
	}
	return tasks
}
func runTasks(tasks [][]mergedBasis, targetDim int) (ok bool, mbs []mergedBasis) {
	var wg sync.WaitGroup
	oks := make([]bool, len(tasks))
	mbs = make([]mergedBasis, len(tasks))
	for i, task := range tasks {
		wg.Add(1)
		go func(i int, task []mergedBasis) {
			//
			oks[i], mbs[i] = doMergeBasisBis(task, targetDim)
			//
			wg.Done()
		}(i, task)
	}
	wg.Wait()
	//
	ok = true
	for _, oki := range oks {
		if !oki {
			ok = false
		}
	}
	return ok, mbs
}

func newVarEval(basisSlice []mergedBasis, glbBasis *mat.Dense, vars []float64) (
	lossVar float64) {
	for _, basis := range basisSlice {
		var totOldVar, totNewVar float64
		oldCovM := mat.NewDense(basis.dimN, basis.dimN, nil)
		for i, v := range basis.vars {
			oldCovM.Set(i, i, v)
			totOldVar += v
		}
		//
		changeBasisM, newCovMat, tmp := new(mat.Dense), new(mat.Dense), new(mat.Dense)
		changeBasisM.Mul(basis.basis.T(), glbBasis)
		tmp.Mul(changeBasisM.T(), oldCovM)
		newCovMat.Mul(tmp, changeBasisM)
		//
		for i := range vars {
			v := newCovMat.At(i, i)
			vars[i] += v
			totNewVar += v
		}
		lossVar += totOldVar - totNewVar
	}

	var totVar float64
	for i := range vars {
		vars[i] /= float64(len(basisSlice))
		totVar += vars[i]
	}
	lossVar /= float64(len(basisSlice))
	lossVar /= lossVar + totVar
	return lossVar
}
