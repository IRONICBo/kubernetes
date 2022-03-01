/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package library

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/interpreter/functions"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// Lists provides a CEL function library extension of list utility functions.
//
// isSorted
//
// Returns true if the provided list of comparable elements is sorted, else returns false.
//
//     <list<T>>.isSorted() <bool>, T must be a comparable type
//
// Examples:
//
//     [1, 2, 3].isSorted()  // return true
//     ['a', 'b', 'b', 'c'].isSorted()  // return true
//     [2.0, 1.0].isSorted()  // return false
//     [1].isSorted()  // return true
//     [].isSorted()  // return true
//
//
// sum
//
// Returns the sum of the elements of the provided list. Supports CEL number (int, uint, double) and duration types.
//
//     <list<T>>.sum() <T>, T must be a numeric type or a duration
//
// Examples:
//
//     [1, 3].sum() // returns 4
//     [1.0, 3.0].sum() // returns 4.0
//     ['1m', '1s'].sum() // returns '1m1s'
//     emptyIntList.sum() // returns 0
//     emptyDoubleList.sum() // returns 0.0
//     [].sum() // returns 0
//
//
// min / max
//
// Returns the minimum/maximum valued element of the provided list. Supports all comparable types.
// If the list is empty, an error is returned.
//
//     <list<T>>.min() <T>, T must be a comparable type
//     <list<T>>.max() <T>, T must be a comparable type
//
// Examples:
//
//     [1, 3].min() // returns 1
//     [1, 3].max() // returns 3
//     [].min() // error
//     [1].min() // returns 1
//     ([0] + emptyList).min() // returns 0
//
//
// indexOf / lastIndexOf
//
// Returns either the first or last positional index of the provided element in the list.
// If the element is not found, -1 is returned. Supports all equatable types.
//
//     <list<T>>.indexOf(<T>) <int>, T must be an equatable type
//     <list<T>>.lastIndexOf(<T>) <int>, T must be an equatable type
//
// Examples:
//
//     [1, 2, 2, 3].indexOf(2) // returns 1
//     ['a', 'b', 'b', 'c'].lastIndexOf('b') // returns 2
//     [1.0].indexOf(1.1) // returns -1
//     [].indexOf('string') // returns -1
//
func Lists() cel.EnvOption {
	return cel.Lib(listsLib)
}

var listsLib = &lists{}

type lists struct{}

var paramA = decls.NewTypeParamType("A")

// CEL typeParams can be used to constraint to a specific trait (e.g. traits.ComparableType) if the 1st operand is the type to constrain.
// But the functions we need to constrain are <list<paramType>>, not just <paramType>.
var summableTypes = map[string]*exprpb.Type{"int": decls.Int, "uint": decls.Uint, "double": decls.Double, "duration": decls.Duration}
var comparableTypes = map[string]*exprpb.Type{"bool": decls.Bool, "int": decls.Int, "uint": decls.Uint, "double": decls.Double,
	"duration": decls.Duration, "timestamp": decls.Timestamp, "string": decls.String, "bytes": decls.Bytes}

// WARNING: All library additions or modifications must follow
// https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/2876-crd-validation-expression-language#function-library-updates
var listsLibraryDecls = []*exprpb.Decl{
	decls.NewFunction("isSorted",
		templatedOverloads(comparableTypes, func(name string, paramType *exprpb.Type) *exprpb.Decl_FunctionDecl_Overload {
			return decls.NewInstanceOverload(fmt.Sprintf("list_%s_is_sorted_bool", name),
				[]*exprpb.Type{decls.NewListType(paramType)},
				decls.Bool)
		})...,
	),
	decls.NewFunction("sum",
		templatedOverloads(summableTypes, func(name string, paramType *exprpb.Type) *exprpb.Decl_FunctionDecl_Overload {
			return decls.NewInstanceOverload(fmt.Sprintf("list_%s_sum_%s", name, name),
				[]*exprpb.Type{decls.NewListType(paramType)},
				paramType)
		})...,
	),
	decls.NewFunction("max",
		templatedOverloads(comparableTypes, func(name string, paramType *exprpb.Type) *exprpb.Decl_FunctionDecl_Overload {
			return decls.NewInstanceOverload(fmt.Sprintf("list_%s_max_%s", name, name),
				[]*exprpb.Type{decls.NewListType(paramType)},
				paramType)
		})...,
	),
	decls.NewFunction("min",
		templatedOverloads(comparableTypes, func(name string, paramType *exprpb.Type) *exprpb.Decl_FunctionDecl_Overload {
			return decls.NewInstanceOverload(fmt.Sprintf("list_%s_min_%s", name, name),
				[]*exprpb.Type{decls.NewListType(paramType)},
				paramType)
		})...,
	),
	decls.NewFunction("indexOf",
		decls.NewInstanceOverload("list_a_index_of_int",
			[]*exprpb.Type{decls.NewListType(paramA), paramA},
			decls.Int),
	),
	decls.NewFunction("lastIndexOf",
		decls.NewInstanceOverload("list_a_last_index_of_int",
			[]*exprpb.Type{decls.NewListType(paramA), paramA},
			decls.Int),
	),
}

func (*lists) CompileOptions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Declarations(listsLibraryDecls...),
	}
}

func (*lists) ProgramOptions() []cel.ProgramOption {
	return []cel.ProgramOption{
		cel.Functions(
			&functions.Overload{
				Operator: "isSorted",
				Unary:    isSorted,
			},
			// if 'sum' is called directly, it is via dynamic dispatch, and we infer the type from the 1st element of the
			// list if it has one, otherwise we return int64(0)
			&functions.Overload{
				Operator: "sum",
				Unary:    dynSum(),
			},
			// use overload names for sum so an initial accumulator value can be assigned to each
			&functions.Overload{
				Operator: "list_int_sum_int",
				Unary: sum(func() ref.Val {
					return types.Int(0)
				}),
			},
			&functions.Overload{
				Operator: "list_uint_sum_uint",
				Unary: sum(func() ref.Val {
					return types.Uint(0)
				}),
			},
			&functions.Overload{
				Operator: "list_double_sum_double",
				Unary: sum(func() ref.Val {
					return types.Double(0.0)
				}),
			},
			&functions.Overload{
				Operator: "list_duration_sum_duration",
				Unary: sum(func() ref.Val {
					return types.Duration{Duration: 0}
				}),
			},
			&functions.Overload{
				Operator: "max",
				Unary:    max(),
			},
			&functions.Overload{
				Operator: "min",
				Unary:    min(),
			},
			// use overload names for indexOf and lastIndexOf to de-conflict with function of same name in strings extension library
			&functions.Overload{
				Operator: "list_a_index_of_int",
				Binary:   indexOf,
			},
			&functions.Overload{
				Operator: "list_a_last_index_of_int",
				Binary:   lastIndexOf,
			},
		),
	}
}

func isSorted(val ref.Val) ref.Val {
	var prev traits.Comparer
	iterable, ok := val.(traits.Iterable)
	if !ok {
		return types.MaybeNoSuchOverloadErr(val)
	}
	for it := iterable.Iterator(); it.HasNext() == types.True; {
		next := it.Next()
		nextCmp, ok := next.(traits.Comparer)
		if !ok {
			return types.MaybeNoSuchOverloadErr(next)
		}
		if prev != nil {
			cmp := prev.Compare(next)
			if cmp == types.IntOne {
				return types.False
			}
		}
		prev = nextCmp
	}
	return types.True
}

func dynSum() functions.UnaryOp {
	return func(val ref.Val) ref.Val {
		iterable, ok := val.(traits.Iterable)
		if !ok {
			return types.MaybeNoSuchOverloadErr(val)
		}
		it := iterable.Iterator()
		var initval ref.Val
		if it.HasNext() == types.True {
			first := it.Next()
			switch first.Type() {
			case types.IntType:
				initval = types.Int(0)
			case types.UintType:
				initval = types.Uint(0)
			case types.DoubleType:
				initval = types.Double(0.0)
			case types.DurationType:
				initval = types.Duration{Duration: 0}
			default:
				return types.MaybeNoSuchOverloadErr(first)
			}
		} else {
			initval = types.Int(0)
		}
		initFn := func() ref.Val {
			return initval
		}
		return sum(initFn)(val)
	}
}

func sum(init func() ref.Val) functions.UnaryOp {
	return func(val ref.Val) ref.Val {
		i := init()
		acc, ok := i.(traits.Adder)
		if !ok {
			// Should never happen since all passed in init values are valid
			return types.MaybeNoSuchOverloadErr(i)
		}
		iterable, ok := val.(traits.Iterable)
		if !ok {
			return types.MaybeNoSuchOverloadErr(val)
		}
		for it := iterable.Iterator(); it.HasNext() == types.True; {
			next := it.Next()
			nextAdder, ok := next.(traits.Adder)
			if !ok {
				// Should never happen for type checked CEL programs
				return types.MaybeNoSuchOverloadErr(next)
			}
			if acc != nil {
				s := acc.Add(next)
				sum, ok := s.(traits.Adder)
				if !ok {
					// Should never happen for type checked CEL programs
					return types.MaybeNoSuchOverloadErr(s)
				}
				acc = sum
			} else {
				acc = nextAdder
			}
		}
		return acc.(ref.Val)
	}
}

func min() functions.UnaryOp {
	return cmp("min", types.IntOne)
}

func max() functions.UnaryOp {
	return cmp("max", types.IntNegOne)
}

func cmp(opName string, opPreferCmpResult ref.Val) functions.UnaryOp {
	return func(val ref.Val) ref.Val {
		var result traits.Comparer
		iterable, ok := val.(traits.Iterable)
		if !ok {
			return types.MaybeNoSuchOverloadErr(val)
		}
		for it := iterable.Iterator(); it.HasNext() == types.True; {
			next := it.Next()
			nextCmp, ok := next.(traits.Comparer)
			if !ok {
				// Should never happen for type checked CEL programs
				return types.MaybeNoSuchOverloadErr(next)
			}
			if result == nil {
				result = nextCmp
			} else {
				cmp := result.Compare(next)
				if cmp == opPreferCmpResult {
					result = nextCmp
				}
			}
		}
		if result == nil {
			return types.NewErr("%s called on empty list", opName)
		}
		return result.(ref.Val)
	}
}

func indexOf(list ref.Val, item ref.Val) ref.Val {
	lister, ok := list.(traits.Lister)
	if !ok {
		return types.MaybeNoSuchOverloadErr(list)
	}
	sz := lister.Size().(types.Int)
	for i := types.Int(0); i < sz; i++ {
		if lister.Get(types.Int(i)).Equal(item) == types.True {
			return types.Int(i)
		}
	}
	return types.Int(-1)
}

func lastIndexOf(list ref.Val, item ref.Val) ref.Val {
	lister, ok := list.(traits.Lister)
	if !ok {
		return types.MaybeNoSuchOverloadErr(list)
	}
	sz := lister.Size().(types.Int)
	for i := sz - 1; i >= 0; i-- {
		if lister.Get(types.Int(i)).Equal(item) == types.True {
			return types.Int(i)
		}
	}
	return types.Int(-1)
}

// templatedOverloads returns overloads for each of the provided types. The template function is called with each type
// name (map key) and type to construct the overloads.
func templatedOverloads(types map[string]*exprpb.Type, template func(name string, t *exprpb.Type) *exprpb.Decl_FunctionDecl_Overload) []*exprpb.Decl_FunctionDecl_Overload {
	overloads := make([]*exprpb.Decl_FunctionDecl_Overload, len(types))
	i := 0
	for name, t := range types {
		overloads[i] = template(name, t)
		i++
	}
	return overloads
}
