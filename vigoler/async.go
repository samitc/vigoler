package vigoler

import "sync"

type Async struct {
	Result         interface{}
	Error          error
	WarningsOutput string
	wg             *sync.WaitGroup
	wa             WaitAble
}
type WaitAble interface {
	Wait() error
}

func createAsyncWaitable(waitable WaitAble) Async {
	return Async{wa: waitable}
}
func CreateAsyncWaitGroup(wg *sync.WaitGroup) Async {
	return Async{wg: wg}
}
func (async *Async) SetResult(result interface{}, err error, warningOutput string) {
	async.Result = result
	async.Error = err
	async.WarningsOutput = warningOutput
}
func (async *Async) Stop() {
	panic("Not implement") //TODO
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
