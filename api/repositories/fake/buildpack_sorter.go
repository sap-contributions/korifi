// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"sync"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type BuildpackSorter struct {
	SortStub        func([]repositories.BuildpackRecord, string) []repositories.BuildpackRecord
	sortMutex       sync.RWMutex
	sortArgsForCall []struct {
		arg1 []repositories.BuildpackRecord
		arg2 string
	}
	sortReturns struct {
		result1 []repositories.BuildpackRecord
	}
	sortReturnsOnCall map[int]struct {
		result1 []repositories.BuildpackRecord
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *BuildpackSorter) Sort(arg1 []repositories.BuildpackRecord, arg2 string) []repositories.BuildpackRecord {
	var arg1Copy []repositories.BuildpackRecord
	if arg1 != nil {
		arg1Copy = make([]repositories.BuildpackRecord, len(arg1))
		copy(arg1Copy, arg1)
	}
	fake.sortMutex.Lock()
	ret, specificReturn := fake.sortReturnsOnCall[len(fake.sortArgsForCall)]
	fake.sortArgsForCall = append(fake.sortArgsForCall, struct {
		arg1 []repositories.BuildpackRecord
		arg2 string
	}{arg1Copy, arg2})
	stub := fake.SortStub
	fakeReturns := fake.sortReturns
	fake.recordInvocation("Sort", []interface{}{arg1Copy, arg2})
	fake.sortMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *BuildpackSorter) SortCallCount() int {
	fake.sortMutex.RLock()
	defer fake.sortMutex.RUnlock()
	return len(fake.sortArgsForCall)
}

func (fake *BuildpackSorter) SortCalls(stub func([]repositories.BuildpackRecord, string) []repositories.BuildpackRecord) {
	fake.sortMutex.Lock()
	defer fake.sortMutex.Unlock()
	fake.SortStub = stub
}

func (fake *BuildpackSorter) SortArgsForCall(i int) ([]repositories.BuildpackRecord, string) {
	fake.sortMutex.RLock()
	defer fake.sortMutex.RUnlock()
	argsForCall := fake.sortArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2
}

func (fake *BuildpackSorter) SortReturns(result1 []repositories.BuildpackRecord) {
	fake.sortMutex.Lock()
	defer fake.sortMutex.Unlock()
	fake.SortStub = nil
	fake.sortReturns = struct {
		result1 []repositories.BuildpackRecord
	}{result1}
}

func (fake *BuildpackSorter) SortReturnsOnCall(i int, result1 []repositories.BuildpackRecord) {
	fake.sortMutex.Lock()
	defer fake.sortMutex.Unlock()
	fake.SortStub = nil
	if fake.sortReturnsOnCall == nil {
		fake.sortReturnsOnCall = make(map[int]struct {
			result1 []repositories.BuildpackRecord
		})
	}
	fake.sortReturnsOnCall[i] = struct {
		result1 []repositories.BuildpackRecord
	}{result1}
}

func (fake *BuildpackSorter) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.sortMutex.RLock()
	defer fake.sortMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *BuildpackSorter) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ repositories.BuildpackSorter = new(BuildpackSorter)
