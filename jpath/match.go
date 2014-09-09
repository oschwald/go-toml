package jpath

import (
	"fmt"
	. "github.com/pelletier/go-toml"
)

// base match
type matchBase struct {
	next PathFn
}

func (f *matchBase) SetNext(next PathFn) {
	f.next = next
}

// terminating functor - gathers results
type terminatingFn struct {
	// empty
}

func newTerminatingFn() *terminatingFn {
	return &terminatingFn{}
}

func (f *terminatingFn) SetNext(next PathFn) {
	// do nothing
}

func (f *terminatingFn) Call(node interface{}, ctx *queryContext) {
	ctx.appendResult(node)
}

// shim to ease functor writing
func treeValue(tree *TomlTree, key string) interface{} {
	return tree.GetPath([]string{key})
}

// match single key
type matchKeyFn struct {
	matchBase
	Name string
}

func newMatchKeyFn(name string) *matchKeyFn {
	return &matchKeyFn{Name: name}
}

func (f *matchKeyFn) Call(node interface{}, ctx *queryContext) {
	if tree, ok := node.(*TomlTree); ok {
		item := treeValue(tree, f.Name)
		if item != nil {
			f.next.Call(item, ctx)
		}
	}
}

// match single index
type matchIndexFn struct {
	matchBase
	Idx int
}

func newMatchIndexFn(idx int) *matchIndexFn {
	return &matchIndexFn{Idx: idx}
}

func (f *matchIndexFn) Call(node interface{}, ctx *queryContext) {
	if arr, ok := node.([]interface{}); ok {
		if f.Idx < len(arr) && f.Idx >= 0 {
			f.next.Call(arr[f.Idx], ctx)
		}
	}
}

// filter by slicing
type matchSliceFn struct {
	matchBase
	Start, End, Step int
}

func newMatchSliceFn(start, end, step int) *matchSliceFn {
	return &matchSliceFn{Start: start, End: end, Step: step}
}

func (f *matchSliceFn) Call(node interface{}, ctx *queryContext) {
	if arr, ok := node.([]interface{}); ok {
		// adjust indexes for negative values, reverse ordering
		realStart, realEnd := f.Start, f.End
		if realStart < 0 {
			realStart = len(arr) + realStart
		}
		if realEnd < 0 {
			realEnd = len(arr) + realEnd
		}
		if realEnd < realStart {
			realEnd, realStart = realStart, realEnd // swap
		}
		// loop and gather
		for idx := realStart; idx < realEnd; idx += f.Step {
			f.next.Call(arr[idx], ctx)
		}
	}
}

// match anything
type matchAnyFn struct {
	matchBase
}

func newMatchAnyFn() *matchAnyFn {
	return &matchAnyFn{}
}

func (f *matchAnyFn) Call(node interface{}, ctx *queryContext) {
	if tree, ok := node.(*TomlTree); ok {
		for _, key := range tree.Keys() {
			item := treeValue(tree, key)
			f.next.Call(item, ctx)
		}
	}
}

// filter through union
type matchUnionFn struct {
	Union []PathFn
}

func (f *matchUnionFn) SetNext(next PathFn) {
	for _, fn := range f.Union {
		fn.SetNext(next)
	}
}

func (f *matchUnionFn) Call(node interface{}, ctx *queryContext) {
	for _, fn := range f.Union {
		fn.Call(node, ctx)
	}
}

// match every single last node in the tree
type matchRecursiveFn struct {
	matchBase
}

func newMatchRecursiveFn() *matchRecursiveFn {
	return &matchRecursiveFn{}
}

func (f *matchRecursiveFn) Call(node interface{}, ctx *queryContext) {
	if tree, ok := node.(*TomlTree); ok {
		var visit func(tree *TomlTree)
		visit = func(tree *TomlTree) {
			for _, key := range tree.Keys() {
				item := treeValue(tree, key)
				f.next.Call(item, ctx)
				switch node := item.(type) {
				case *TomlTree:
					visit(node)
				case []*TomlTree:
					for _, subtree := range node {
						visit(subtree)
					}
				}
			}
		}
		visit(tree)
	}
}

// match based on an externally provided functional filter
type matchFilterFn struct {
	matchBase
	Pos  Position
	Name string
}

func newMatchFilterFn(name string, pos Position) *matchFilterFn {
	return &matchFilterFn{Name: name, Pos: pos}
}

func (f *matchFilterFn) Call(node interface{}, ctx *queryContext) {
	fn, ok := (*ctx.filters)[f.Name]
	if !ok {
		panic(fmt.Sprintf("%s: query context does not have filter '%s'",
			f.Pos, f.Name))
	}
	switch castNode := node.(type) {
	case *TomlTree:
		for _, k := range castNode.Keys() {
			v := castNode.GetPath([]string{k})
			if fn(v) {
				f.next.Call(v, ctx)
			}
		}
	case []interface{}:
		for _, v := range castNode {
			if fn(v) {
				f.next.Call(v, ctx)
			}
		}
	}
}

// match based using result of an externally provided functional filter
type matchScriptFn struct {
	matchBase
	Pos  Position
	Name string
}

func newMatchScriptFn(name string, pos Position) *matchScriptFn {
	return &matchScriptFn{Name: name, Pos: pos}
}

func (f *matchScriptFn) Call(node interface{}, ctx *queryContext) {
	fn, ok := (*ctx.scripts)[f.Name]
	if !ok {
		panic(fmt.Sprintf("%s: query context does not have script '%s'",
			f.Pos, f.Name))
	}
	switch result := fn(node).(type) {
	case string:
		nextMatch := newMatchKeyFn(result)
		nextMatch.SetNext(f.next)
		nextMatch.Call(node, ctx)
	case int:
		nextMatch := newMatchIndexFn(result)
		nextMatch.SetNext(f.next)
		nextMatch.Call(node, ctx)
		//TODO: support other return types?
	}
}
