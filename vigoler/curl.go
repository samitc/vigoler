package vigoler

import (
	"context"
	"strconv"
	"strings"
	"sync"
)

const (
	maxDownloadParts   = 10
	minPartSizeInBytes = 10 * 1024 * 1024 // 10 mb
)

type CurlWrapper struct {
	curl externalApp
}

func CreateCurlWrapper() CurlWrapper {
	return CurlWrapper{curl: externalApp{"curl"}}
}
func (curl *CurlWrapper) getVideoSize(url string) (int, error) {
	_, oChan, err := curl.curl.runCommandChan(context.Background(), "-L", "-I", url)
	if err != nil {
		return 0, err
	}
	var sizeInBytes int
	for s := range oChan {
		if strings.HasPrefix(s, "Content-Length:") {
			number := strings.Split(s, " ")[1]
			sizeInBytes, err = strconv.Atoi(strings.TrimRight(number, "\r\n"))
		}
	}
	return sizeInBytes, err
}
func (curl *CurlWrapper) runCurl(url, output string, startByte, endByte int) *Async {
	strStartByte := strconv.Itoa(startByte)
	strEndByte := ""
	if endByte != -1 {
		strEndByte = strconv.Itoa(endByte)
	}
	wa, _ := curl.curl.runCommandWait(context.Background(), "-L", "--range", strStartByte+"-"+strEndByte, "-o", output, url)
	async := createAsyncWaitAble(wa)
	return &async
}
func (curl *CurlWrapper) Download(url string, output string) (*Async, error) {
	videoSizeInBytes, err := curl.getVideoSize(url)
	if err != nil {
		return nil, err
	}
	numOfParts := videoSizeInBytes / minPartSizeInBytes
	if numOfParts > maxDownloadParts {
		numOfParts = maxDownloadParts
	} else if numOfParts == 0 {
		numOfParts++
	}
	sizeOfPart := (int)(videoSizeInBytes / numOfParts)
	var wa multipleWaitAble
	var wg sync.WaitGroup
	async := CreateAsyncWaitGroup(&wg, &wa)
	for i := 0; i < numOfParts-1; i++ {
		wa.add(curl.runCurl(url, output+strconv.Itoa(i), i*sizeOfPart, (i+1)*sizeOfPart))
	}
	wa.add(curl.runCurl(url, output+strconv.Itoa(numOfParts-1), (numOfParts-1)*sizeOfPart, -1))
	err = wa.Wait()
	return &async, nil
}
func (curl *CurlWrapper) GetVideoSize(url string) (*Async, error) {
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, nil)
	go func() {
		defer wg.Done()
		bytes2KB := 1.0 / 1024
		size, err := curl.getVideoSize(url)
		async.SetResult((int)((float64)(size)*bytes2KB), err, "")
	}()
	return &async, nil
}
