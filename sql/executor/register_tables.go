package executor

import (
	"github.com/lovelly/gleam/flow"
	"github.com/lovelly/gleam/sql/model"
)

type TableColumn struct {
	ColumnName string
	ColumnType byte
}

type TableSource struct {
	Dataset   *flow.Dataset
	TableInfo *model.TableInfo
}

var (
	Tables = make(map[string]*TableSource)
)
