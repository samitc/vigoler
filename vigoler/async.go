package vigoler

import "sync"

type Async struct {
	Result interface{}
	wg     *sync.WaitGroup
	wa     WaitAble
}
type WaitAble interface {
	Wait() error
}

func createAsyncWaitable(waitable WaitAble) Async {
	return Async{wa: waitable}
}
func createAsyncWaitGroup(wg *sync.WaitGroup) Async {
	return Async{wg: wg}
}
func (async *Async) stop() {

}
func (async *Async) Get() interface{} {
	if async.wg != nil {
		async.wg.Wait()
		async.wg = nil
	} else if async.wa != nil {
		async.wa.Wait()
		async.wa = nil
	}
	return async.Result
}
