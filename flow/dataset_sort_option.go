package flow

import (
	"fmt"
	"strings"

	"github.com/lovelly/gleam/instruction"
)

type SortOption struct {
	orderByList []instruction.OrderBy
}

func Field(indexes ...int) *SortOption {
	ret := &SortOption{}
	for _, index := range indexes {
		ret.orderByList = append(ret.orderByList, instruction.OrderBy{
			Index: index,
			Order: instruction.Ascending,
		})
	}
	return ret
}

func OrderBy(index int, ascending bool) *SortOption {
	ret := &SortOption{
		orderByList: []instruction.OrderBy{
			{
				Index: index,
				Order: instruction.Descending,
			},
		},
	}
	if ascending {
		ret.orderByList[0].Order = instruction.Ascending
	}
	return ret
}

// OrderBy chains a list of sorting order by
func (o *SortOption) By(index int, ascending bool) *SortOption {
	order := instruction.Descending
	if ascending {
		order = instruction.Ascending
	}
	o.orderByList = append(o.orderByList, instruction.OrderBy{
		Index: index,
		Order: order,
	})
	return o
}

// return a list of indexes
func (o *SortOption) Indexes() []int {
	var ret []int
	for _, x := range o.orderByList {
		ret = append(ret, x.Index)
	}
	return ret
}

func (o *SortOption) String() string {
	var buf strings.Builder
	for _, orderBy := range o.orderByList {
		buf.WriteString(fmt.Sprintf("%d ", orderBy.Index))
		if orderBy.Order == instruction.Ascending {
			buf.WriteString("asc")
		} else {
			buf.WriteString("desc")
		}
	}
	return buf.String()
}
