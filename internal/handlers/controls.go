package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	maxResponseDelay = 10 * time.Second
	maxResponseSize  = 1 << 20
)

type responseControls struct {
	status int
	delay  time.Duration
	size   int
}

func parseResponseControls(rawQuery string) (responseControls, error) {
	controls := responseControls{status: http.StatusOK, size: -1}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return controls, fmt.Errorf("invalid query encoding")
	}
	for _, name := range []string{"status", "delay", "size"} {
		values, present := query[name]
		if !present {
			continue
		}
		if len(values) != 1 {
			return controls, fmt.Errorf("%s must have exactly one value", name)
		}
		switch name {
		case "status":
			controls.status, err = strconv.Atoi(values[0])
			if err != nil || controls.status < 200 || controls.status > 599 {
				return controls, fmt.Errorf("status must be between 200 and 599")
			}
		case "delay":
			controls.delay, err = time.ParseDuration(values[0])
			if err != nil || controls.delay < 0 || controls.delay > maxResponseDelay {
				return controls, fmt.Errorf("delay must be between 0s and 10s")
			}
		case "size":
			controls.size, err = strconv.Atoi(values[0])
			if err != nil || controls.size < 0 || controls.size > maxResponseSize {
				return controls, fmt.Errorf("size must be between 0 and 1048576 bytes")
			}
		}
	}
	return controls, nil
}

type zeroReader struct{}

func (zeroReader) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}
