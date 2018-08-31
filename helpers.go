package main

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
)

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// ----------------------------------
// helper functions for HTTP handlers
// ----------------------------------
func ensure(method string, handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "This request must be "+method, 405)
			return
		}
		handler(w, r)
	}
}

func ensurePOST(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return ensure("POST", handler)
}

func ensureGET(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return ensure("GET", handler)
}

func ensurePUT(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return ensure("PUT", handler)
}

func ensureDELETE(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return ensure("DELETE", handler)
}

// --------------------------
// helper functions for stats
// --------------------------
func computeRate(input []float64) []float64 {
	output := make([]float64, 0)
	for i := len(input) - 2; i >= 0; i-- {
		value := input[i]
		diff := value - input[i+1]
		output = append([]float64{diff}, output...)
	}
	return output
}

func generateMapFromSnap(snap statsSnapshot) map[string]interface{} {
	var avgProcessingTime float64
	if snap.processingTimeCount > 0 {
		avgProcessingTime = snap.processingTimeSum / snap.processingTimeCount
	}

	result := map[string]interface{}{
		"dns_queries":           snap.totalRequests,
		"blocked_filtering":     snap.filteredLists,
		"replaced_safebrowsing": snap.filteredSafebrowsing,
		"replaced_safesearch":   snap.filteredSafesearch,
		"replaced_parental":     snap.filteredParental,
		"avg_processing_time":   avgProcessingTime,
	}
	return result
}

func generateMapFromStats(stats *periodicStats, start int, end int) map[string]interface{} {
	// clamp
	start = clamp(start, 0, statsHistoryElements)
	end = clamp(end, 0, statsHistoryElements)

	avgProcessingTime := make([]float64, 0)

	count := computeRate(stats.processingTimeCount[start:end])
	sum := computeRate(stats.processingTimeSum[start:end])
	for i := 0; i < len(count); i++ {
		var avg float64
		if count[i] != 0 {
			avg = sum[i] / count[i]
			avg *= 1000
		}
		avgProcessingTime = append(avgProcessingTime, avg)
	}

	result := map[string]interface{}{
		"dns_queries":           computeRate(stats.totalRequests[start:end]),
		"blocked_filtering":     computeRate(stats.filteredLists[start:end]),
		"replaced_safebrowsing": computeRate(stats.filteredSafebrowsing[start:end]),
		"replaced_safesearch":   computeRate(stats.filteredSafesearch[start:end]),
		"replaced_parental":     computeRate(stats.filteredParental[start:end]),
		"avg_processing_time":   avgProcessingTime,
	}
	return result
}

func produceTop(m map[string]int, top int) map[string]int {
	toMarshal := map[string]int{}
	topKeys := sortByValue(m)
	for i, k := range topKeys {
		if i == top {
			break
		}
		toMarshal[k] = m[k]
	}
	return toMarshal
}

// -------------------------------------
// helper functions for querylog parsing
// -------------------------------------
func sortByValue(m map[string]int) []string {
	type kv struct {
		k string
		v int
	}
	var ss []kv
	for k, v := range m {
		ss = append(ss, kv{k, v})
	}
	sort.Slice(ss, func(l, r int) bool {
		return ss[l].v > ss[r].v
	})

	sorted := []string{}
	for _, v := range ss {
		sorted = append(sorted, v.k)
	}
	return sorted
}

func getHost(entry map[string]interface{}) string {
	q, ok := entry["question"]
	if !ok {
		return ""
	}
	question, ok := q.(map[string]interface{})
	if !ok {
		return ""
	}
	h, ok := question["host"]
	if !ok {
		return ""
	}
	host, ok := h.(string)
	if !ok {
		return ""
	}
	return host
}

func getReason(entry map[string]interface{}) string {
	r, ok := entry["reason"]
	if !ok {
		return ""
	}
	reason, ok := r.(string)
	if !ok {
		return ""
	}
	return reason
}

func getClient(entry map[string]interface{}) string {
	c, ok := entry["client"]
	if !ok {
		return ""
	}
	client, ok := c.(string)
	if !ok {
		return ""
	}
	return client
}

// -------------------------------------------------
// helper functions for parsing parameters from body
// -------------------------------------------------
func parseParametersFromBody(r io.Reader) (map[string]string, error) {
	parameters := map[string]string{}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			// skip empty lines
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return parameters, errors.New("Got invalid request body")
		}
		parameters[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return parameters, nil
}