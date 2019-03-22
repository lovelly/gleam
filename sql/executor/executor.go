package executor

import (
	"github.com/lovelly/gleam/flow"
	"github.com/lovelly/gleam/sql/expression"
)

type Executor interface {
	Exec() *flow.Dataset
	Schema() expression.Schema
}
