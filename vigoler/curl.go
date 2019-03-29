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
func (curl *CurlWrapper) runCurl(url, output string, startByte, endByte int, headers *map[string]string) *Async {
	strStartByte := strconv.Itoa(startByte)
	strEndByte := ""
	if endByte != -1 {
		strEndByte = strconv.Itoa(endByte)
	}
	args := addCurlHeaders([]string{"-L", "--range", strStartByte + "-" + strEndByte, "-o", output}, headers)
	args = append(args, url)
	wa, _ := curl.curl.runCommandWait(context.Background(), args...)
	async := createAsyncWaitAble(wa)
	return &async
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
func downloadManagerHandle(numOfParts, numOfGoRot int, resChan chan downloadGo, workChan chan int, output string) (*os.File, error) {
	curPartIndex := 0
	savePartIndex := 0
	var outputFile *os.File
	finished := make([]int, 0, numOfGoRot)
	var err error
	for curPartIndex < numOfParts {
		select {
		case downloadRes := <-resChan:
			finished, savePartIndex, outputFile, err = finishManagerDownload(downloadRes, finished, savePartIndex, output, outputFile)
			if err != nil {
				return outputFile, err
			}
		case workChan <- curPartIndex:
			curPartIndex++
		}
	}
	for res := range resChan {
		finished, savePartIndex, outputFile, err = finishManagerDownload(res, finished, savePartIndex, output, outputFile)
		if err != nil {
			return outputFile, err
		}
		if savePartIndex == curPartIndex {
			return outputFile, nil
		}
	}
	return outputFile, nil
}
func (curl *CurlWrapper) downloadParts(url, output string, videoSizeInBytes int, wa multipleWaitAble, headers *map[string]string) error {
	numOfParts := videoSizeInBytes/minPartSizeInBytes - 1
	if numOfParts < 2 {
		async := curl.runCurl(url, output, 0, -1, headers)
		_, err, _ := async.Get()
		return err
	}
	numOfGoRot := (int)(math.Min((float64)(maxDownloadParts), (float64)(numOfParts)))
	resChan := make(chan downloadGo)
	workChan := make(chan int)
	for i := 0; i < numOfGoRot; i++ {
		go func() {
			for index := range workChan {
				async := curl.runCurl(url, output+strconv.Itoa(index), index*minPartSizeInBytes, (index+1)*minPartSizeInBytes-1, headers)
				wa.add(async)
				_, err, _ := async.Get()
				wa.remove(async)
				resChan <- downloadGo{index: index, err: err}
			}
		}()
	}
	outputFile, err := downloadManagerHandle(numOfParts, numOfGoRot, resChan, workChan, output)
	defer outputFile.Close()
	if err != nil {
		return err
	}
	async := curl.runCurl(url, output+"f", numOfParts*minPartSizeInBytes, -1, headers)
	wa.add(async)
	_, err, _ = async.Get()
	wa.remove(async)
	if err != nil {
		return err
	}
	err = copyFile(output+"f", outputFile)
	return err
}
func (curl *CurlWrapper) download(url, output string, headers *map[string]string) (*Async, error) {
	videoSizeInBytes, err := curl.getVideoSize(url, headers)
	if err != nil {
		return nil, err
	}
	var wa multipleWaitAble
	var wg sync.WaitGroup
	wg.Add(1)
	async := CreateAsyncWaitGroup(&wg, &wa)
	go func() {
		defer wg.Done()
		var err error
		if videoSizeInBytes < minPartSizeInBytes {
			_, err, _ = curl.runCurl(url, output, 0, -1, headers).Get()
		} else {
			err = curl.downloadParts(url, output, videoSizeInBytes, wa, headers)
		}
		async.SetResult(nil, err, "")
	}()
	return &async, nil
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
