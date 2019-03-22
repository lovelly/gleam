// word_count.go
package main

import (
	"flag"
	"fmt"

	"github.com/lovelly/gleam/distributed"
	"github.com/lovelly/gleam/flow"
	"github.com/lovelly/gleam/gio"
	"github.com/lovelly/gleam/gio/mapper"
	"github.com/lovelly/gleam/plugins/file"
	"github.com/lovelly/gleam/util"
)

var (
	isDistributed = flag.Bool("distributed", false, "distributed mode or not")
)

func main() {

	gio.Init()

	var option flow.FlowOption
	option = distributed.Option()
	if !*isDistributed {
		option = flow.Local
	}

	flow.New("word count by unix pipes").
		Read(file.Txt("/etc/passwd", 1)).
		Map("tokenize", mapper.Tokenize).
		Pipe("lowercase", "tr 'A-Z' 'a-z'").
		Pipe("write", "tee x.out").
		Pipe("sort", "sort").
		Pipe("uniq", "uniq -c").
		OutputRow(func(row *util.Row) error {
			fmt.Printf("%s\n", gio.ToString(row.K[0]))
			return nil
		}).
		Run(option)

}
