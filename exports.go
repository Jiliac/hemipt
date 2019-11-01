package main

import (
	"fmt"
	"log"

	"encoding/csv"
	"os"
)

// Helpers
func makeCSVFile(path string) (ok bool, w *csv.Writer) {
	f, err := os.Create(path)
	if err != nil {
		log.Printf("Problem opening histo CSV file: %v.\n", err)
		return
	}
	//
	ok, w = true, csv.NewWriter(f)
	return ok, w
}
func writeCSV(w *csv.Writer, records [][]string) {
	w.WriteAll(records)
	if err := w.Error(); err != nil {
		log.Printf("Couldn't record CSV: %v.\n", err)
	}
}

func exportHistos(pcas []*dynamicPCA, path string) {
	if len(pcas) == 0 {
		return
	}
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	records := [][]string{[]string{
		"seed_n", "dim_n", "bin_n", "start", "end", "count",
	}}
	for i := range pcas {
		histos, steps := pcas[i].stats.histos, pcas[i].stats.steps
		for j, histo := range histos {
			for k, cnt := range histo {
				//start, end := float64(i)*step, float64(i+1)*step
				records = append(records, []string{
					fmt.Sprintf("%d", i),                     // seed_n
					fmt.Sprintf("%d", j),                     // dim_n
					fmt.Sprintf("%d", k),                     // bin_n
					fmt.Sprintf("%f", float64(k)*steps[j]),   // start
					fmt.Sprintf("%f", float64(k+1)*steps[j]), // end
					fmt.Sprintf("%f", cnt),                   // count
				})
			}
		}
	}

	writeCSV(w, records)
}
