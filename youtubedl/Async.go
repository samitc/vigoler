package youtubedl

import "sync"

type Async struct {
	Result interface{}
	wg     *sync.WaitGroup
}

func CreateAsync(group *sync.WaitGroup) Async {
	return Async{wg: group}
}
func (async *Async) stop() {

}
func (async *Async) Get() interface{} {
	async.wg.Wait()
	return async.Result
}
