// Copyright 2016 PingCAP, Inc.
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

package plan

import (
	"log"
	"github.com/lovelly/gleam/sql/ast"
	"github.com/lovelly/gleam/sql/expression"
)

func getUsedList(usedCols []*expression.Column, schema expression.Schema) []bool {
	used := make([]bool, schema.Len())
	for _, col := range usedCols {
		idx := schema.GetColumnIndex(col)
		if idx == -1 {
			log.Printf("Can't find column %s from schema %s.", col, schema)
		}
		used[idx] = true
	}
	return used
}

// exprHasSetVar checks if the expression has set-var function. If do, we should not prune it.
func exprHasSetVar(expr expression.Expression) bool {
	if fun, ok := expr.(*expression.ScalarFunction); ok {
		canPrune := true
		if fun.FuncName.L == ast.SetVar {
			return false
		}
		for _, arg := range fun.GetArgs() {
			canPrune = canPrune && exprHasSetVar(arg)
			if !canPrune {
				return false
			}
		}
	}
	return true
}

// PruneColumns implements LogicalPlan interface.
func (p *Projection) PruneColumns(parentUsedCols []*expression.Column) {
	child := p.GetChildByIndex(0).(LogicalPlan)
	var selfUsedCols []*expression.Column
	used := getUsedList(parentUsedCols, p.schema)
	for i := len(used) - 1; i >= 0; i-- {
		if !used[i] && exprHasSetVar(p.Exprs[i]) {
			p.schema.Columns = append(p.schema.Columns[:i], p.schema.Columns[i+1:]...)
			p.Exprs = append(p.Exprs[:i], p.Exprs[i+1:]...)
		}
	}
	for _, expr := range p.Exprs {
		selfUsedCols = append(selfUsedCols, expression.ExtractColumns(expr)...)
	}
	child.PruneColumns(selfUsedCols)
}

// PruneColumns implements LogicalPlan interface.
func (p *Selection) PruneColumns(parentUsedCols []*expression.Column) {
	child := p.GetChildByIndex(0).(LogicalPlan)
	for _, cond := range p.Conditions {
		parentUsedCols = append(parentUsedCols, expression.ExtractColumns(cond)...)
	}
	child.PruneColumns(parentUsedCols)
	p.SetSchema(child.GetSchema())
}

// PruneColumns implements LogicalPlan interface.
func (p *Aggregation) PruneColumns(parentUsedCols []*expression.Column) {
	child := p.GetChildByIndex(0).(LogicalPlan)
	used := getUsedList(parentUsedCols, p.schema)
	for i := len(used) - 1; i >= 0; i-- {
		if !used[i] {
			p.schema.Columns = append(p.schema.Columns[:i], p.schema.Columns[i+1:]...)
			p.AggFuncs = append(p.AggFuncs[:i], p.AggFuncs[i+1:]...)
		}
	}
	var selfUsedCols []*expression.Column
	for _, aggrFunc := range p.AggFuncs {
		for _, arg := range aggrFunc.GetArgs() {
			selfUsedCols = append(selfUsedCols, expression.ExtractColumns(arg)...)
		}
	}
	for _, expr := range p.GroupByItems {
		selfUsedCols = append(selfUsedCols, expression.ExtractColumns(expr)...)
	}
	child.PruneColumns(selfUsedCols)
}

// PruneColumns implements LogicalPlan interface.
func (p *Sort) PruneColumns(parentUsedCols []*expression.Column) {
	child := p.GetChildByIndex(0).(LogicalPlan)
	for _, item := range p.ByItems {
		parentUsedCols = append(parentUsedCols, expression.ExtractColumns(item.Expr)...)
	}
	child.PruneColumns(parentUsedCols)
	p.SetSchema(p.GetChildByIndex(0).GetSchema())
}

// PruneColumns implements LogicalPlan interface.
func (p *Union) PruneColumns(parentUsedCols []*expression.Column) {
	used := getUsedList(parentUsedCols, p.GetSchema())
	for i := len(used) - 1; i >= 0; i-- {
		if !used[i] {
			p.schema.Columns = append(p.schema.Columns[:i], p.schema.Columns[i+1:]...)
		}
	}
	for _, c := range p.GetChildren() {
		child := c.(LogicalPlan)
		schema := child.GetSchema()
		var newCols []*expression.Column
		for i, use := range used {
			if use {
				newCols = append(newCols, schema.Columns[i])
			}
		}
		child.PruneColumns(newCols)
	}
}

// PruneColumns implements LogicalPlan interface.
func (p *DataSource) PruneColumns(parentUsedCols []*expression.Column) {
	used := getUsedList(parentUsedCols, p.schema)
	for i := len(used) - 1; i >= 0; i-- {
		if !used[i] {
			p.schema.Columns = append(p.schema.Columns[:i], p.schema.Columns[i+1:]...)
			p.Columns = append(p.Columns[:i], p.Columns[i+1:]...)
		}
	}
}

// PruneColumns implements LogicalPlan interface.
func (p *TableDual) PruneColumns(_ []*expression.Column) {
}

// PruneColumns implements LogicalPlan interface.
func (p *Trim) PruneColumns(parentUsedCols []*expression.Column) {
	used := getUsedList(parentUsedCols, p.schema)
	for i := len(used) - 1; i >= 0; i-- {
		if !used[i] {
			p.schema.Columns = append(p.schema.Columns[:i], p.schema.Columns[i+1:]...)
		}
	}
	p.GetChildByIndex(0).(LogicalPlan).PruneColumns(parentUsedCols)
}

// PruneColumns implements LogicalPlan interface.
func (p *Exists) PruneColumns(parentUsedCols []*expression.Column) {
	p.GetChildByIndex(0).(LogicalPlan).PruneColumns(nil)
}

// PruneColumns implements LogicalPlan interface.
func (p *Insert) PruneColumns(_ []*expression.Column) {
	if len(p.GetChildren()) == 0 {
		return
	}
	child := p.GetChildByIndex(0).(LogicalPlan)
	child.PruneColumns(child.GetSchema().Columns)
}

// PruneColumns implements LogicalPlan interface.
func (p *Join) PruneColumns(parentUsedCols []*expression.Column) {
	for _, eqCond := range p.EqualConditions {
		parentUsedCols = append(parentUsedCols, expression.ExtractColumns(eqCond)...)
	}
	for _, leftCond := range p.LeftConditions {
		parentUsedCols = append(parentUsedCols, expression.ExtractColumns(leftCond)...)
	}
	for _, rightCond := range p.RightConditions {
		parentUsedCols = append(parentUsedCols, expression.ExtractColumns(rightCond)...)
	}
	for _, otherCond := range p.OtherConditions {
		parentUsedCols = append(parentUsedCols, expression.ExtractColumns(otherCond)...)
	}
	lChild := p.GetChildByIndex(0).(LogicalPlan)
	rChild := p.GetChildByIndex(1).(LogicalPlan)
	var leftCols, rightCols []*expression.Column
	for _, col := range parentUsedCols {
		if lChild.GetSchema().GetColumnIndex(col) != -1 {
			leftCols = append(leftCols, col)
		} else if rChild.GetSchema().GetColumnIndex(col) != -1 {
			rightCols = append(rightCols, col)
		}
	}
	lChild.PruneColumns(leftCols)
	rChild.PruneColumns(rightCols)
	composedSchema := expression.MergeSchema(lChild.GetSchema(), rChild.GetSchema())
	if p.JoinType == SemiJoin {
		p.schema = lChild.GetSchema().Clone()
	} else if p.JoinType == LeftOuterSemiJoin {
		joinCol := p.schema.Columns[len(p.schema.Columns)-1]
		p.schema = lChild.GetSchema().Clone()
		p.schema.Append(joinCol)
	} else {
		p.schema = composedSchema
	}
}

// PruneColumns implements LogicalPlan interface.
func (p *Apply) PruneColumns(parentUseCols []*expression.Column) {
	for _, col := range p.corCols {
		parentUseCols = append(parentUseCols, &col.Column)
	}
	p.Join.PruneColumns(parentUseCols)
}

// PruneColumns implements LogicalPlan interface.
func (p *Update) PruneColumns(parentUsedCols []*expression.Column) {
	p.baseLogicalPlan.PruneColumns(p.GetChildByIndex(0).GetSchema().Columns)
}

// PruneColumns implements LogicalPlan interface.
func (p *Delete) PruneColumns(parentUsedCols []*expression.Column) {
	p.baseLogicalPlan.PruneColumns(p.GetChildByIndex(0).GetSchema().Columns)
}
