package blondie

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type Options struct {
	PollInterval   time.Duration
	ExitCodeOnOk   int
	ExitCodeOnFail int
	QuietMode      bool
	OutputWriter   func(string)
}

func DefaultOptions() *Options {
	return &Options{
		PollInterval:   time.Millisecond * 250,
		ExitCodeOnFail: 1,
		ExitCodeOnOk:   0,
		QuietMode:      false,
		OutputWriter:   func(msg string) { fmt.Println(msg) },
	}
}

type netCheck struct {
	host    string
	port    int
	timeout time.Duration
}

type tcpCheck struct {
	netCheck
	dial func(string, string) (net.Conn, error)
}

func NewTcpCheck(host string, port int, timeout time.Duration) DepCheck {
	return &tcpCheck{
		netCheck: netCheck{
			host:    host,
			port:    port,
			timeout: timeout,
		},
		dial: net.Dial,
	}
}

type httpCheck struct {
	netCheck
	path         string
	successCodes []int
	get          func(string) (*http.Response, error)
}

// NewHttpCheck creates a new DepCheck for a HTTP endpoint. Path may be an empty string and successCodes may be an empty slice in which case any reponse code will be considered a successful response
func NewHttpCheck(host string, port int, timeout time.Duration, path string, successCodes []int) DepCheck {
	return &httpCheck{
		netCheck: netCheck{
			host:    host,
			port:    port,
			timeout: timeout,
		},
		successCodes: successCodes,
		path:         path,
		get:          http.Get,
	}
}

type DepCheck interface {
	Try() bool
	Timeout() time.Duration
	Address() string
}

func (check *netCheck) Timeout() time.Duration {
	return check.timeout
}

func (check *httpCheck) Try() bool {
	endpoint := fmt.Sprintf("http://%s:%v/%s", check.host, check.port, check.path)
	resp, err := check.get(endpoint)

	if err == nil {
		if len(check.successCodes) == 0 {
			return true
		}

		for _, successCode := range check.successCodes {
			if successCode == resp.StatusCode {
				return true
			}
		}
	}

	return false
}

func (check *tcpCheck) Try() bool {
	address := fmt.Sprintf("%s:%v", check.host, check.port)
	_, err := check.dial("tcp", address)
	return err == nil
}

func (check *httpCheck) Address() string {
	return fmt.Sprintf("http://%s:%v/%s", check.host, check.port, check.path)
}

func (check *tcpCheck) Address() string {
	return fmt.Sprintf("tcp://%s:%v", check.host, check.port)
}

func (options Options) Write(message string) {
	if !options.QuietMode {
		options.OutputWriter(message)
	}
}

func WaitForDeps(deps []DepCheck, opts *Options) bool {

	var waitGroup = &sync.WaitGroup{}
	success := true

	waitGroup.Add(len(deps))
	for _, target := range deps {
		opts.Write(fmt.Sprintf("Trying to connect: %s - timeout = %v seconds", target.Address(), target.Timeout()))
		go func(target DepCheck) {
			start := time.Now()
			for true {
				if target.Try() {
					waitGroup.Done()
					break
				} else if time.Now().Sub(start) > target.Timeout() {
					opts.Write(fmt.Sprintf("Timeout : %s", target.Address()))
					success = false
					waitGroup.Done()
					break
				}

				time.Sleep(opts.PollInterval)
			}
		}(target)
	}

	waitGroup.Wait()

	return success
}
