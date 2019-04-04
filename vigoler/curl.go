package vigoler

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	maxDownloadParts   = 10
	minPartSizeInBytes = 1 * 1024 * 1024 // 1 mb
)

type CurlWrapper struct {
	curl externalApp
}
type downloadGo struct {
	index int
	err   error
}

func CreateCurlWrapper() CurlWrapper {
	return CurlWrapper{curl: externalApp{"curl"}}
}
func addCurlHeaders(args []string, headers *map[string]string) []string {
	if headers != nil {
		for k, v := range *headers {
			args = append(args, "-H")
			args = append(args, fmt.Sprintf("%s:%s", k, v))
		}
	}
	return args
}
func (curl *CurlWrapper) getVideoSize(url string, headers *map[string]string) (int, error) {
	args := addCurlHeaders([]string{"-I", "-L"}, headers)
	args = append(args, url)
	_, oChan, err := curl.curl.runCommandChan(context.Background(), args...)
	if err != nil {
		return 0, err
	}
	sizeInBytes := -1
	for s := range oChan {
		if strings.HasPrefix(s, "Content-Length:") {
			number := strings.Split(s, " ")[1]
			sizeInBytes, err = strconv.Atoi(strings.TrimRight(number, "\r\n"))
		}
	}
	return sizeInBytes, err
}
func (curl *CurlWrapper) runCurl(url, output string, startByte, endByte int, headers *map[string]string) (*Async, error) {
	strStartByte := strconv.Itoa(startByte)
	strEndByte := ""
	if endByte != -1 {
		strEndByte = strconv.Itoa(endByte)
	}
	args := addCurlHeaders([]string{"-L", "-f", "--range", strStartByte + "-" + strEndByte, "-o", output}, headers)
	args = append(args, url)
	wa, err := curl.curl.runCommandWait(context.Background(), args...)
	async := createAsyncWaitAble(wa)
	return &async, err
}
func insertSort(a []int, num int) []int {
	i := sort.Search(len(a), func(i int) bool { return a[i] > num })
	return append(a[:i], append([]int{num}, a[i:]...)...)
}
func copyFile(input string, output *os.File) error {
	inputFile, err := os.Open(input)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, inputFile)
	inputFile.Close()
	os.Remove(input)
	return err
}
func copyParts(a []int, curPart int, output string, outputFile *os.File) ([]int, int, error) {
	i := 0
	l := len(a)
	for i < l && curPart == a[i] {
		err := copyFile(output+strconv.Itoa(a[i]), outputFile)
		if err != nil {
			return nil, curPart, err
		}
		i++
		curPart++
	}
	return a[i:], curPart, nil
}
func finishManagerDownload(res downloadGo, finished []int, savePartIndex int, output string, outputFile *os.File) ([]int, int, *os.File, error) {
	var err error
	if res.err != nil {
		finished = append(finished, res.index)
		err = res.err
	} else {
		if res.index == 0 {
			os.Rename(output+"0", output)
			outputFile, err = os.OpenFile(output, os.O_APPEND|os.O_WRONLY, 0644)
			savePartIndex = 1
		} else {
			finished = insertSort(finished, res.index)
		}
		finished, savePartIndex, err = copyParts(finished, savePartIndex, output, outputFile)
	}
	return finished, savePartIndex, outputFile, err
}
func abortCurl(workChan chan int, resChan chan downloadGo, numOfGoRot int, finished []int, output string, outputFile *os.File) error {
	go func(numToSend int) {
		for i := 0; i < numToSend; i++ {
			workChan <- -1
		}
	}(numOfGoRot)
	var err error = &CancelError{}
	for res := range resChan {
		if _, ok := res.err.(*CancelError); ok {
			numOfGoRot--
			if numOfGoRot == 0 {
				close(resChan)
				break
			}
		} else {
			nErr := os.Remove(output + strconv.Itoa(res.index))
			if _, ok := err.(*CancelError); ok && nErr != nil {
				err = nErr
			}
			if res.err != nil {
				err = res.err
			}
		}
	}
	nErr := outputFile.Close()
	if _, ok := err.(*CancelError); ok && nErr != nil {
		err = nErr
	}
	for _, f := range finished {
		nErr := os.Remove(output + strconv.Itoa(f))
		if _, ok := err.(*CancelError); ok && nErr != nil {
			err = nErr
		}
	}
	nErr = os.Remove(output)
	if _, ok := err.(*CancelError); ok && nErr != nil {
		err = nErr
	}
	return err
}
func downloadManagerHandle(numOfParts, numOfGoRot int, resChan chan downloadGo, workChan chan int, cancelChan chan error, output string) (*os.File, error) {
	curPartIndex := 0
	savePartIndex := 0
	var outputFile *os.File
	finished := make([]int, 0, numOfGoRot)
	var err error
	for curPartIndex < numOfParts {
		select {
		case _ = <-cancelChan:
			err := abortCurl(workChan, resChan, numOfGoRot, finished, output, outputFile)
			curPartIndex = numOfParts
			cancelChan <- err
			return nil, err
		case downloadRes := <-resChan:
			finished, savePartIndex, outputFile, err = finishManagerDownload(downloadRes, finished, savePartIndex, output, outputFile)
			if err != nil {
				_ = abortCurl(workChan, resChan, numOfGoRot, finished, output, outputFile)
				return outputFile, err
			}
		case workChan <- curPartIndex:
			curPartIndex++
		}
	}
	for res := range resChan {
		finished, savePartIndex, outputFile, err = finishManagerDownload(res, finished, savePartIndex, output, outputFile)
		if err != nil {
			_ = abortCurl(workChan, resChan, numOfGoRot, finished, output, outputFile)
			return outputFile, err
		}
		if savePartIndex == curPartIndex {
			return outputFile, nil
		}
	}
	return outputFile, nil
}
func (curl *CurlWrapper) downloadParts(url, output string, videoSizeInBytes int, cancelChan chan error, headers *map[string]string) error {
	numOfParts := videoSizeInBytes/minPartSizeInBytes - 1
	numOfGoRot := (int)(math.Min((float64)(maxDownloadParts), (float64)(numOfParts)))
	resChan := make(chan downloadGo)
	workChan := make(chan int)
	for i := 0; i < numOfGoRot; i++ {
		go func() {
			for index := range workChan {
				if index == -1 {
					resChan <- downloadGo{index: index, err: &CancelError{}}
					break
				} else {
					async, err := curl.runCurl(url, output+strconv.Itoa(index), index*minPartSizeInBytes, (index+1)*minPartSizeInBytes-1, headers)
					if err == nil {
						_, err, _ = async.Get()
					}
					resChan <- downloadGo{index: index, err: err}
				}
			}
		}()
	}
	outputFile, err := downloadManagerHandle(numOfParts, numOfGoRot, resChan, workChan, cancelChan, output)
	defer outputFile.Close()
	if err != nil {
		return err
	}
	async, err := curl.runCurl(url, output+"f", numOfParts*minPartSizeInBytes, -1, headers)
	if err != nil {
		return err
	}
	_, err, _ = async.Get()
	if err != nil {
		return err
	}
	err = copyFile(output+"f", outputFile)
	return err
}

type curlWaitAble struct {
	callback func() error
	wg       *sync.WaitGroup
}

func (c *curlWaitAble) Wait() error {
	c.wg.Wait()
	return nil
}

func (c *curlWaitAble) Stop() error {
	return c.callback()
}
func (curl *CurlWrapper) downloadSize(url, output string, videoSizeInBytes int, headers *map[string]string) (*Async, error) {
	const minPartsToDownloadParts = 3
	if videoSizeInBytes < minPartSizeInBytes*minPartsToDownloadParts {
		return curl.runCurl(url, output, 0, -1, headers)
	}
	cancelChan := make(chan error)
	var wg sync.WaitGroup
	var wa = curlWaitAble{wg: &wg, callback: func() error {
		cancelChan <- nil
		return <-cancelChan
	}}
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, &wa)
	go func() {
		defer wg.Done()
		err := curl.downloadParts(url, output, videoSizeInBytes, cancelChan, headers)
		async.SetResult(nil, err, "")
	}()
	return &async, nil
}
func (curl *CurlWrapper) download(url, output string, headers *map[string]string) (*Async, error) {
	videoSizeInBytes, err := curl.getVideoSize(url, headers)
	if err != nil {
		return nil, err
	}
	return curl.downloadSize(url, output, videoSizeInBytes, headers)
}
func (curl *CurlWrapper) Download(url, output string) (*Async, error) {
	return curl.download(url, output, nil)
}
func (curl *CurlWrapper) DownloadHeaders(url string, headers map[string]string, output string) (*Async, error) {
	return curl.download(url, output, &headers)
}
func (curl *CurlWrapper) GetInputSize(url string) (*Async, error) {
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, nil)
	go func() {
		defer wg.Done()
		bytes2KB := 1.0 / 1024
		size, err := curl.getVideoSize(url, nil)
		async.SetResult((int)((float64)(size)*bytes2KB), err, "")
	}()
	return &async, nil
}
func (curl *CurlWrapper) GetInputSizeHeaders(url string, headers map[string]string) (*Async, error) {
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, nil)
	go func() {
		defer wg.Done()
		bytes2KB := 1.0 / 1024
		size, err := curl.getVideoSize(url, &headers)
		async.SetResult((int)((float64)(size)*bytes2KB), err, "")
	}()
	return &async, nil
}
