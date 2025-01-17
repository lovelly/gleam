// Copyright 2013 The ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSES/QL-LICENSE file.

// Copyright 2015 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package expression

import (
	"github.com/lovelly/gleam/sql/ast"
	"github.com/lovelly/gleam/sql/context"
	"github.com/lovelly/gleam/sql/parser/opcode"
	"github.com/lovelly/gleam/sql/util/types"
	"github.com/juju/errors"
)

// baseBuiltinFunc will be contained in every struct that implement builtinFunc interface.
type baseBuiltinFunc struct {
	args          []Expression
	argValues     []types.Datum
	ctx           context.Context
	deterministic bool
}

func newBaseBuiltinFunc(args []Expression, ctx context.Context) baseBuiltinFunc {
	return baseBuiltinFunc{
		args:          args,
		argValues:     make([]types.Datum, len(args)),
		ctx:           ctx,
		deterministic: true,
	}
}

func (b *baseBuiltinFunc) evalArgs(row []types.Datum) (_ []types.Datum, err error) {
	for i, arg := range b.args {
		b.argValues[i], err = arg.Eval(row, b.ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return b.argValues, nil
}

// isDeterministic will be true by default. Non-deterministic function will override this function.
func (b *baseBuiltinFunc) isDeterministic() bool {
	return b.deterministic
}

func (b *baseBuiltinFunc) getArgs() []Expression {
	return b.args
}

// equal only checks if both functions are non-deterministic and if these arguments are same.
// Function name will be checked outside.
func (b *baseBuiltinFunc) equal(fun builtinFunc) bool {
	if !b.isDeterministic() || !fun.isDeterministic() {
		return false
	}
	funArgs := fun.getArgs()
	if len(funArgs) != len(b.args) {
		return false
	}
	for i := range b.args {
		if !b.args[i].Equal(funArgs[i], b.ctx) {
			return false
		}
	}
	return true
}

func (b *baseBuiltinFunc) getCtx() context.Context {
	return b.ctx
}

// builtinFunc stands for a particular function signature.
type builtinFunc interface {
	// eval does evaluation by the given row.
	eval([]types.Datum) (types.Datum, error)
	// getArgs returns the arguments expressions.
	getArgs() []Expression
	// isDeterministic checks if a function is deterministic.
	// A function is deterministic if it returns same results for same inputs.
	// e.g. random is non-deterministic.
	isDeterministic() bool
	// equal check if this function equals to another function.
	equal(builtinFunc) bool
	// getCtx returns this function's context.
	getCtx() context.Context
}

// baseFunctionClass will be contained in every struct that implement functionClass interface.
type baseFunctionClass struct {
	funcName string
	minArgs  int
	maxArgs  int
}

func (b *baseFunctionClass) verifyArgs(args []Expression) error {
	l := len(args)
	if l < b.minArgs || (b.maxArgs != -1 && l > b.maxArgs) {
		return errIncorrectParameterCount.GenByArgs(b.funcName)
	}
	return nil
}

// builtinFunc stands for a class for a function which may contains multiple functions.
type functionClass interface {
	// getFunction gets a function signature by the types and the counts of given arguments.
	getFunction(args []Expression, ctx context.Context) (builtinFunc, error)
}

// BuiltinFunc is the function signature for builtin functions
type BuiltinFunc func([]types.Datum, context.Context) (types.Datum, error)

// funcs holds all registered builtin functions.
var funcs = map[string]functionClass{
	// common functions
	ast.Coalesce: &coalesceFunctionClass{baseFunctionClass{ast.Coalesce, 1, -1}},
	ast.IsNull:   &isNullFunctionClass{baseFunctionClass{ast.IsNull, 1, 1}},
	ast.Greatest: &greatestFunctionClass{baseFunctionClass{ast.Greatest, 2, -1}},
	ast.Least:    &leastFunctionClass{baseFunctionClass{ast.Least, 2, -1}},
	ast.Interval: &intervalFunctionClass{baseFunctionClass{ast.Interval, 2, -1}},

	// math functions
	ast.Abs:     &absFunctionClass{baseFunctionClass{ast.Abs, 1, 1}},
	ast.Ceil:    &ceilFunctionClass{baseFunctionClass{ast.Ceil, 1, 1}},
	ast.Ceiling: &ceilFunctionClass{baseFunctionClass{ast.Ceiling, 1, 1}},
	ast.Floor:   &floorFunctionClass{baseFunctionClass{ast.Floor, 1, 1}},
	ast.Ln:      &logFunctionClass{baseFunctionClass{ast.Log, 1, 1}},
	ast.Log:     &logFunctionClass{baseFunctionClass{ast.Log, 1, 2}},
	ast.Log2:    &log2FunctionClass{baseFunctionClass{ast.Log2, 1, 1}},
	ast.Log10:   &log10FunctionClass{baseFunctionClass{ast.Log10, 1, 1}},
	ast.Pow:     &powFunctionClass{baseFunctionClass{ast.Pow, 2, 2}},
	ast.Power:   &powFunctionClass{baseFunctionClass{ast.Pow, 2, 2}},
	ast.Rand:    &randFunctionClass{baseFunctionClass{ast.Rand, 0, 1}},
	ast.Round:   &roundFunctionClass{baseFunctionClass{ast.Round, 1, 2}},
	ast.Sign:    &signFunctionClass{baseFunctionClass{ast.Sign, 1, 1}},
	ast.Sqrt:    &sqrtFunctionClass{baseFunctionClass{ast.Sqrt, 1, 1}},
	ast.Conv:    &convFunctionClass{baseFunctionClass{ast.Conv, 3, 3}},
	ast.CRC32:   &crc32FunctionClass{baseFunctionClass{ast.CRC32, 1, 1}},

	// time functions
	ast.Curdate:          &currentDateFunctionClass{baseFunctionClass{ast.Curdate, 0, 0}},
	ast.CurrentDate:      &currentDateFunctionClass{baseFunctionClass{ast.CurrentDate, 0, 0}},
	ast.CurrentTime:      &currentTimeFunctionClass{baseFunctionClass{ast.CurrentTime, 0, 1}},
	ast.Date:             &dateFunctionClass{baseFunctionClass{ast.Date, 1, 1}},
	ast.DateDiff:         &dateDiffFunctionClass{baseFunctionClass{ast.DateDiff, 2, 2}},
	ast.DateAdd:          &dateArithFunctionClass{baseFunctionClass{ast.DateAdd, 3, 3}, ast.DateArithAdd},
	ast.AddDate:          &dateArithFunctionClass{baseFunctionClass{ast.AddDate, 3, 3}, ast.DateArithAdd},
	ast.DateSub:          &dateArithFunctionClass{baseFunctionClass{ast.DateSub, 3, 3}, ast.DateArithSub},
	ast.SubDate:          &dateArithFunctionClass{baseFunctionClass{ast.SubDate, 3, 3}, ast.DateArithSub},
	ast.DateFormat:       &dateFormatFunctionClass{baseFunctionClass{ast.DateFormat, 2, 2}},
	ast.CurrentTimestamp: &nowFunctionClass{baseFunctionClass{ast.CurrentTimestamp, 0, 1}},
	ast.Now:              &nowFunctionClass{baseFunctionClass{ast.Now, 0, 1}},
	ast.Curtime:          &currentTimeFunctionClass{baseFunctionClass{ast.Curtime, 0, 1}},
	ast.Day:              &dayFunctionClass{baseFunctionClass{ast.Day, 1, 1}},
	ast.DayName:          &dayNameFunctionClass{baseFunctionClass{ast.DayName, 1, 1}},
	ast.DayOfMonth:       &dayOfMonthFunctionClass{baseFunctionClass{ast.DayOfMonth, 1, 1}},
	ast.DayOfWeek:        &dayOfWeekFunctionClass{baseFunctionClass{ast.DayOfWeek, 1, 1}},
	ast.DayOfYear:        &dayOfYearFunctionClass{baseFunctionClass{ast.DayOfYear, 1, 1}},
	ast.FromDays:         &fromDaysFunctionClass{baseFunctionClass{ast.FromDays, 1, 1}},
	ast.Extract:          &extractFunctionClass{baseFunctionClass{ast.Extract, 2, 2}},
	ast.Hour:             &hourFunctionClass{baseFunctionClass{ast.Hour, 1, 1}},
	ast.MicroSecond:      &microSecondFunctionClass{baseFunctionClass{ast.MicroSecond, 1, 1}},
	ast.Minute:           &minuteFunctionClass{baseFunctionClass{ast.Minute, 1, 1}},
	ast.Month:            &monthFunctionClass{baseFunctionClass{ast.Month, 1, 1}},
	ast.MonthName:        &monthNameFunctionClass{baseFunctionClass{ast.MonthName, 1, 1}},
	ast.Second:           &secondFunctionClass{baseFunctionClass{ast.Second, 1, 1}},
	ast.StrToDate:        &strToDateFunctionClass{baseFunctionClass{ast.StrToDate, 2, 2}},
	ast.Sysdate:          &sysDateFunctionClass{baseFunctionClass{ast.Sysdate, 0, 1}},
	ast.Time:             &timeFunctionClass{baseFunctionClass{ast.Time, 1, 1}},
	ast.UTCDate:          &utcDateFunctionClass{baseFunctionClass{ast.UTCDate, 0, 0}},
	ast.Week:             &weekFunctionClass{baseFunctionClass{ast.Week, 1, 2}},
	ast.Weekday:          &weekDayFunctionClass{baseFunctionClass{ast.Weekday, 1, 1}},
	ast.WeekOfYear:       &weekOfYearFunctionClass{baseFunctionClass{ast.WeekOfYear, 1, 1}},
	ast.Year:             &yearFunctionClass{baseFunctionClass{ast.Year, 1, 1}},
	ast.YearWeek:         &yearWeekFunctionClass{baseFunctionClass{ast.YearWeek, 1, 2}},
	ast.FromUnixTime:     &fromUnixTimeFunctionClass{baseFunctionClass{ast.FromUnixTime, 1, 2}},
	ast.TimeDiff:         &timeDiffFunctionClass{baseFunctionClass{ast.TimeDiff, 2, 2}},
	ast.TimestampDiff:    &timestampDiffFunctionClass{baseFunctionClass{ast.TimestampDiff, 3, 3}},
	ast.UnixTimestamp:    &unixTimestampFunctionClass{baseFunctionClass{ast.UnixTimestamp, 0, 1}},

	// string functions
	ast.ASCII:          &asciiFunctionClass{baseFunctionClass{ast.ASCII, 1, 1}},
	ast.Concat:         &concatFunctionClass{baseFunctionClass{ast.Concat, 1, -1}},
	ast.ConcatWS:       &concatWSFunctionClass{baseFunctionClass{ast.ConcatWS, 2, -1}},
	ast.Convert:        &convertFunctionClass{baseFunctionClass{ast.Convert, 2, 2}},
	ast.Field:          &fieldFunctionClass{baseFunctionClass{ast.Field, 2, -1}},
	ast.Lcase:          &lowerFunctionClass{baseFunctionClass{ast.Lcase, 1, 1}},
	ast.Left:           &leftFunctionClass{baseFunctionClass{ast.Left, 2, 2}},
	ast.Length:         &lengthFunctionClass{baseFunctionClass{ast.Length, 1, 1}},
	ast.Locate:         &locateFunctionClass{baseFunctionClass{ast.Locate, 2, 3}},
	ast.Lower:          &lowerFunctionClass{baseFunctionClass{ast.Lower, 1, 1}},
	ast.LTrim:          &lTrimFunctionClass{baseFunctionClass{ast.LTrim, 1, 1}},
	ast.Repeat:         &repeatFunctionClass{baseFunctionClass{ast.Repeat, 2, 2}},
	ast.Replace:        &replaceFunctionClass{baseFunctionClass{ast.Replace, 3, 3}},
	ast.Reverse:        &reverseFunctionClass{baseFunctionClass{ast.Reverse, 1, 1}},
	ast.RTrim:          &rTrimFunctionClass{baseFunctionClass{ast.RTrim, 1, 1}},
	ast.Space:          &spaceFunctionClass{baseFunctionClass{ast.Space, 1, 1}},
	ast.Strcmp:         &strcmpFunctionClass{baseFunctionClass{ast.Strcmp, 2, 2}},
	ast.Substring:      &substringFunctionClass{baseFunctionClass{ast.Substring, 2, 3}},
	ast.Substr:         &substringFunctionClass{baseFunctionClass{ast.Substr, 2, 3}},
	ast.SubstringIndex: &substringIndexFunctionClass{baseFunctionClass{ast.SubstringIndex, 3, 3}},
	ast.Trim:           &trimFunctionClass{baseFunctionClass{ast.Trim, 1, 3}},
	ast.Upper:          &upperFunctionClass{baseFunctionClass{ast.Upper, 1, 1}},
	ast.Ucase:          &upperFunctionClass{baseFunctionClass{ast.Ucase, 1, 1}},
	ast.Hex:            &hexFunctionClass{baseFunctionClass{ast.Hex, 1, 1}},
	ast.Unhex:          &unhexFunctionClass{baseFunctionClass{ast.Unhex, 1, 1}},
	ast.Rpad:           &rpadFunctionClass{baseFunctionClass{ast.Rpad, 3, 3}},
	ast.BitLength:      &bitLengthFunctionClass{baseFunctionClass{ast.BitLength, 1, 1}},
	ast.CharFunc:       &charFunctionClass{baseFunctionClass{ast.CharFunc, 2, -1}},
	ast.CharLength:     &charLengthFunctionClass{baseFunctionClass{ast.CharLength, 1, 1}},
	ast.FindInSet:      &findInSetFunctionClass{baseFunctionClass{ast.FindInSet, 2, 2}},

	// information functions
	ast.CurrentUser: &currentUserFunctionClass{baseFunctionClass{ast.CurrentUser, 0, 0}},
	ast.Database:    &databaseFunctionClass{baseFunctionClass{ast.Database, 0, 0}},
	// This function is a synonym for DATABASE().
	// See http://dev.mysql.com/doc/refman/5.7/en/information-functions.html#function_schema
	ast.Schema:    &databaseFunctionClass{baseFunctionClass{ast.Schema, 0, 0}},
	ast.FoundRows: &foundRowsFunctionClass{baseFunctionClass{ast.FoundRows, 0, 0}},
	ast.User:      &userFunctionClass{baseFunctionClass{ast.User, 0, 0}},
	ast.Version:   &versionFunctionClass{baseFunctionClass{ast.Version, 0, 0}},

	// control functions
	ast.If:     &ifFunctionClass{baseFunctionClass{ast.If, 3, 3}},
	ast.Ifnull: &ifNullFunctionClass{baseFunctionClass{ast.Ifnull, 2, 2}},
	ast.Nullif: &nullIfFunctionClass{baseFunctionClass{ast.Nullif, 2, 2}},

	// miscellaneous functions
	ast.Sleep: &sleepFunctionClass{baseFunctionClass{ast.Sleep, 1, 1}},

	// get_lock() and release_lock() are parsed but do nothing.
	// It is used for preventing error in Ruby's activerecord migrations.
	ast.GetLock:     &lockFunctionClass{baseFunctionClass{ast.GetLock, 2, 2}},
	ast.ReleaseLock: &releaseLockFunctionClass{baseFunctionClass{ast.ReleaseLock, 1, 1}},

	// only used by new plan
	ast.AndAnd:     &andandFunctionClass{baseFunctionClass{ast.AndAnd, 2, 2}},
	ast.OrOr:       &ororFunctionClass{baseFunctionClass{ast.OrOr, 2, 2}},
	ast.GE:         &compareFunctionClass{baseFunctionClass{ast.GE, 2, 2}, opcode.GE},
	ast.LE:         &compareFunctionClass{baseFunctionClass{ast.LE, 2, 2}, opcode.LE},
	ast.EQ:         &compareFunctionClass{baseFunctionClass{ast.EQ, 2, 2}, opcode.EQ},
	ast.NE:         &compareFunctionClass{baseFunctionClass{ast.NE, 2, 2}, opcode.NE},
	ast.LT:         &compareFunctionClass{baseFunctionClass{ast.LT, 2, 2}, opcode.LT},
	ast.GT:         &compareFunctionClass{baseFunctionClass{ast.GT, 2, 2}, opcode.GT},
	ast.NullEQ:     &compareFunctionClass{baseFunctionClass{ast.NullEQ, 2, 2}, opcode.NullEQ},
	ast.Plus:       &arithmeticFunctionClass{baseFunctionClass{ast.Plus, 2, 2}, opcode.Plus},
	ast.Minus:      &arithmeticFunctionClass{baseFunctionClass{ast.Minus, 2, 2}, opcode.Minus},
	ast.Mod:        &arithmeticFunctionClass{baseFunctionClass{ast.Mod, 2, 2}, opcode.Mod},
	ast.Div:        &arithmeticFunctionClass{baseFunctionClass{ast.Div, 2, 2}, opcode.Div},
	ast.Mul:        &arithmeticFunctionClass{baseFunctionClass{ast.Mul, 2, 2}, opcode.Mul},
	ast.IntDiv:     &arithmeticFunctionClass{baseFunctionClass{ast.IntDiv, 2, 2}, opcode.IntDiv},
	ast.LeftShift:  &bitOpFunctionClass{baseFunctionClass{ast.LeftShift, 2, 2}, opcode.LeftShift},
	ast.RightShift: &bitOpFunctionClass{baseFunctionClass{ast.RightShift, 2, 2}, opcode.RightShift},
	ast.And:        &bitOpFunctionClass{baseFunctionClass{ast.And, 2, 2}, opcode.And},
	ast.Or:         &bitOpFunctionClass{baseFunctionClass{ast.Or, 2, 2}, opcode.Or},
	ast.Xor:        &bitOpFunctionClass{baseFunctionClass{ast.Xor, 2, 2}, opcode.Xor},
	ast.LogicXor:   &logicXorFunctionClass{baseFunctionClass{ast.LogicXor, 2, 2}},
	ast.UnaryNot:   &unaryOpFunctionClass{baseFunctionClass{ast.UnaryNot, 1, 1}, opcode.Not},
	ast.BitNeg:     &unaryOpFunctionClass{baseFunctionClass{ast.BitNeg, 1, 1}, opcode.BitNeg},
	ast.UnaryPlus:  &unaryOpFunctionClass{baseFunctionClass{ast.UnaryPlus, 1, 1}, opcode.Plus},
	ast.UnaryMinus: &unaryOpFunctionClass{baseFunctionClass{ast.UnaryMinus, 1, 1}, opcode.Minus},
	ast.In:         &inFunctionClass{baseFunctionClass{ast.In, 1, -1}},
	ast.IsTruth:    &isTrueOpFunctionClass{baseFunctionClass{ast.IsTruth, 1, 1}, opcode.IsTruth},
	ast.IsFalsity:  &isTrueOpFunctionClass{baseFunctionClass{ast.IsFalsity, 1, 1}, opcode.IsFalsity},
	ast.Like:       &likeFunctionClass{baseFunctionClass{ast.Like, 3, 3}},
	ast.Regexp:     &regexpFunctionClass{baseFunctionClass{ast.Regexp, 2, 2}},
	ast.Case:       &caseWhenFunctionClass{baseFunctionClass{ast.Case, 1, -1}},
	ast.RowFunc:    &rowFunctionClass{baseFunctionClass{ast.RowFunc, 2, -1}},
	ast.SetVar:     &setVarFunctionClass{baseFunctionClass{ast.SetVar, 2, 2}},
	ast.GetVar:     &getVarFunctionClass{baseFunctionClass{ast.GetVar, 1, 1}},
}

// DynamicFuncs are those functions that
// use input parameter ctx or
// return an uncertain result would not be constant folded
// the value 0 means nothing
var DynamicFuncs = map[string]int{
	"rand":           0,
	"connection_id":  0,
	"current_user":   0,
	"database":       0,
	"found_rows":     0,
	"last_insert_id": 0,
	"user":           0,
	"version":        0,
	"sleep":          0,
	ast.GetVar:       0,
	ast.SetVar:       0,
	ast.Values:       0,
}
