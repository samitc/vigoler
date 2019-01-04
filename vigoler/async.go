package vigoler

import (
	"fmt"
	"sync"
)

type Async struct {
	Result         interface{}
	Error          error
	WarningsOutput string
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
	return Async{wg: wg, wa: async.wa, isFinish: false, isStopped: false, async: async}
}
func (async *Async) SetResult(result interface{}, err error, warningOutput string) {
	async.Result = result
	async.Error = err
	async.WarningsOutput = warningOutput
	async.isFinish = true
}
func (async *Async) Stop() error {
	async.isStopped = true
	if async.async != nil {
		err := async.async.Stop()
		if err != nil {
			fmt.Println(err)
		}
	}
	if async.wa != nil {
		return async.wa.Stop()
	}
	return nil
}
func (async *Async) Get() (interface{}, error, string) {
	if async.wg != nil {
		async.wg.Wait()
		async.wg = nil
	} else if async.wa != nil {
		async.wa.Wait()
		async.wa = nil
	}
	return async.Result, async.Error, async.WarningsOutput
}
func (async *Async) WillBlock() bool {
	return !async.isFinish
}
