package vigoler

import "sync"

type Async struct {
	Result         interface{}
	Error          error
	WarningsOutput string
	wg             *sync.WaitGroup
	wa             WaitAble
	isFinish       bool
}
type WaitAble interface {
	Wait() error
	Stop() error
}

func createAsyncWaitAble(waitAble WaitAble) Async {
	return Async{wa: waitAble, isFinish: false}
}
func CreateAsyncWaitGroup(wg *sync.WaitGroup, wa WaitAble) Async {
	return Async{wg: wg, wa: wa, isFinish: false}
}
func CreateAsyncFromAsyncAsWaitAble(wg *sync.WaitGroup, async *Async) Async {
	return Async{wg: wg, wa: async.wa, isFinish: false}
}
func (async *Async) SetResult(result interface{}, err error, warningOutput string) {
	async.Result = result
	async.Error = err
	async.WarningsOutput = warningOutput
	async.isFinish = true
}
func (async *Async) Stop() error {
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
