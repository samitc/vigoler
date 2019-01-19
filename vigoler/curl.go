package vigoler

import (
	"context"
	"strconv"
	"strings"
	"sync"
)

type CurlWrapper struct {
	curl externalApp
}

func CreateCurlWrapper() CurlWrapper {
	return CurlWrapper{curl: externalApp{"curl"}}
}
func (curl *CurlWrapper) Download(url string, output string) (*Async, error) {
	return nil, nil
}
func (curl *CurlWrapper) GetVideoSize(url string) (*Async, error) {
	wa, oChan, err := curl.curl.runCommandChan(context.Background(), "-L", "-I", url)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, wa)
	go func() {
		defer wg.Done()
		var sizeInBytes int
		var err error
		bytes2KB := 1.0 / 1024
		for s := range oChan {
			if strings.HasPrefix(s, "Content-Length:") {
				number := strings.Split(s, " ")[1]
				sizeInBytes, err = strconv.Atoi(strings.TrimRight(number, "\r\n"))
			}
		}
		async.SetResult((int)((float64)(sizeInBytes)*bytes2KB), err, "")
	}()
	return &async, nil
}
