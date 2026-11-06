package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Error is an API error answer: {"error":{"code","message"}}. Clients branch
// on Code; Message is for people.
type Error struct {
	Status  int
	Code    string
	Message string
	// Problems lists the bad entries of a rejected import (invalid_import).
	Problems []Problem
	// RetryAfter is how long a rate-limited (429) caller should wait.
	RetryAfter time.Duration
}

// Problem is one bad entry of an import.
type Problem struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s (HTTP %d)", e.Code, e.Status)
	}
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.Status)
}

func readError(resp *http.Response) *Error {
	e := &Error{Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var body struct {
		Error struct {
			Code     string    `json:"code"`
			Message  string    `json:"message"`
			Problems []Problem `json:"problems"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && body.Error.Code != "" {
		e.Code, e.Message, e.Problems = body.Error.Code, body.Error.Message, body.Error.Problems
		return e
	}
	// Not our error shape: a proxy or load balancer answered.
	e.Code = "http_" + strconv.Itoa(resp.StatusCode)
	e.Message = strings.TrimSpace(string(data))
	if r := []rune(e.Message); len(r) > 200 {
		e.Message = string(r[:200]) + "…"
	}
	return e
}

func retryAfter(header string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || secs < 1 {
		return time.Second
	}
	return min(time.Duration(secs)*time.Second, maxRetryWait)
}
