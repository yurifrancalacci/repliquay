package apicall

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type HostConnection struct {
	Hostname                            string
	Debug, SkipVerify, DryRun, Insecure bool
	SleepPeriod, Retries                int
	Max_connections                     int
	QueueLength                         int
	TotalApiCall						int
	LastCompletedApiCallTs				int64
	Mx                                  sync.Mutex
}

func (hc *HostConnection) SetGlobalVars(debug bool, skipverify bool, dryrun bool, insecure bool, sleepPeriod int, retries int) {
	hc.Debug = debug
	hc.SkipVerify = skipverify
	hc.DryRun = dryrun
	hc.Insecure = insecure
	hc.SleepPeriod = sleepPeriod
	hc.Retries = retries
}

func (hc *HostConnection) inc() {
	hc.Mx.Lock()
	defer hc.Mx.Unlock()
	hc.QueueLength++
	hc.TotalApiCall++
	hc.LastCompletedApiCallTs = time.Now().Unix()
}

func (hc *HostConnection) dec() {
	hc.Mx.Lock()
	defer hc.Mx.Unlock()
	hc.QueueLength--
}

func (hc *HostConnection) resetConnectionCounter() {
	hc.Mx.Lock()
	defer hc.Mx.Unlock()
	hc.QueueLength = 0
}

func (hc *HostConnection) ApiCall(host string, url string, method string, token string, bodyData string, action string, retry *int) (httpCode int, responseBody string) {

	var enableTLS string
	httpCode = 0

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: hc.SkipVerify},
	}
	// move to DryRun block
	// sleep if too many connections
	for hc.QueueLength >= hc.Max_connections && hc.SleepPeriod != 0 {
		if hc.Debug {
			fmt.Printf("%s: APICALL %s action too many connections %d. Sleeping %s\n", host, action, hc.QueueLength, time.Duration(hc.SleepPeriod)*time.Millisecond)
		}
		time.Sleep(time.Duration(hc.SleepPeriod) * time.Millisecond)
		if ((hc.LastCompletedApiCallTs - time.Now().Unix()) > 30 ) {
			fmt.Printf("%s: ----> APICALL last apicall at %d. Resetting counter to avoid stuck\n", host, hc.LastCompletedApiCallTs)
			hc.resetConnectionCounter()
		}
	}
	if !hc.DryRun {
		hc.inc()
		if hc.Debug {
			fmt.Printf("%s: queue %d/%d action %s\n", hc.Hostname, hc.QueueLength, hc.Max_connections, action)
		}
	
		client := &http.Client{Transport: tr}
		req := &http.Request{}
		if !hc.Insecure {
			enableTLS = "s"
		}

		if bodyData != "" {
			jsonBody := []byte(bodyData)
			bodyReader := bytes.NewReader(jsonBody)
			req, _ = http.NewRequest(method, "http"+enableTLS+"://"+host+url, bodyReader)
		} else {
			req, _ = http.NewRequest(method, "http"+enableTLS+"://"+host+url, nil)
		}

		if token != "" {
			req.Header.Add("Authorization", "Bearer "+token)
			req.Header.Add("Content-Type", "application/json")
		}

		res, err := client.Do(req)

		if err != nil {
			log.Fatal(err)
		}

		res_body, err := io.ReadAll(res.Body)
		res.Body.Close()
		hc.dec()

		if (hc.TotalApiCall % 10 == 0 ) {
			fmt.Printf("Host %s: completed %d Api Call\n", hc.Hostname, hc.TotalApiCall)
		}

		if res.StatusCode > 499 {
			log.Printf("%s Response failed with status code: %d and\nbody: %s\nRequest data %s url %s method %s", host, res.StatusCode, res_body, bodyData, url, method)
			if *retry > hc.Retries {
				log.Fatalf("Too many attempts: unable to execute action %s with requested data %s on host %s successfully\n", action, bodyData, host)
			} else {
				log.Printf("Sleeping %d seconds before a new attempt on %s %s %s\n", *retry, host, bodyData, action)
				time.Sleep(time.Duration(*retry) * time.Second)
				*retry++
				hc.ApiCall(host, url, method, token, bodyData, action, retry)
			}
		} else {
			if hc.Debug {
				log.Printf("%s Action %s completed\n", host, action)
			}
		}
		if err != nil {
			log.Fatal(err)
		}
		httpCode = res.StatusCode
		responseBody = string(res_body)
	}
	if hc.Debug {
		fmt.Printf("%s: queue %d action %s\n", hc.Hostname, hc.QueueLength, action)
	}
	return
}
