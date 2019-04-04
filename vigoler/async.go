package vigoler

import (
	"sync"
)

type Async struct {
	result         interface{}
	err            error
	warningsOutput string
	wg             *sync.WaitGroup
	wa             WaitAble
	isFinish       bool
	isStopped      bool
	async          *Async
}
type WaitAble interface {
	Wait() error
	Stop() error
}

// Deprecated: Do not use. Implement with chanel instead.
type multipleWaitAble struct {
	waitAbles []*Async
	isStopped bool
}

func (mwa *multipleWaitAble) add(async *Async) {
	mwa.waitAbles = append(mwa.waitAbles, async)
}
func (mwa *multipleWaitAble) remove(async *Async) {
	waitAbleLen := len(mwa.waitAbles) - 1
	for i, v := range mwa.waitAbles {
		if v == async {
			mwa.waitAbles[i] = mwa.waitAbles[waitAbleLen]
			break
		}
	}
	mwa.waitAbles = mwa.waitAbles[:waitAbleLen]
}
func (mwa *multipleWaitAble) Wait() error {
	for _, wa := range mwa.waitAbles {
		_, err, _ := wa.Get()
		if err != nil {
			return err
		}
	}
	return nil
}
func (mwa *multipleWaitAble) Stop() error {
	for _, wa := range mwa.waitAbles {
		err := wa.Stop()
		if err != nil {
			return err
		}
	}
	mwa.isStopped = true
	return nil
}

type CancelError struct {
}

func (ce *CancelError) Error() string {
	return "The async was cancel"
}
func createAsyncWaitAble(waitAble WaitAble) Async {
	return Async{wa: waitAble, isFinish: false, isStopped: false}
}
func CreateAsyncWaitGroup(wg *sync.WaitGroup, wa WaitAble) Async {
	return Async{wg: wg, wa: wa, isFinish: false, isStopped: false}
}
func CreateAsyncFromAsyncAsWaitAble(wg *sync.WaitGroup, async *Async) Async {
	return Async{wg: wg, wa: nil, isFinish: false, isStopped: false, async: async}
}
func (async *Async) SetResult(result interface{}, err error, warningOutput string) {
	async.result = result
	async.err = err
	async.warningsOutput = warningOutput
	async.isFinish = true
}
func (async *Async) Stop() error {
	async.isStopped = true
	var err error = nil
	if async.async != nil {
		err = async.async.Stop()
	}
	if async.wa != nil {
		nErr := async.wa.Stop()
		if err == nil && nErr != nil {
			err = nErr
		}
	}
	return err
}
func (async *Async) Get() (interface{}, error, string) {
	if async.wg != nil {
		async.wg.Wait()
		async.wg = nil
	} else if async.wa != nil {
		nErr := async.wa.Wait()
		if async.err == nil {
			async.err = nErr
		}
		async.wa = nil
	}
	err := async.err
	if err == nil && async.isStopped {
		err = &CancelError{}
	}
	return async.result, async.err, async.warningsOutput
}
func (async *Async) WillBlock() bool {
	return !async.isFinish
}
