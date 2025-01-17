// Copyright 2017 PingCAP, Inc.
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

package filesort

import (
	"container/heap"
	"encoding/binary"
	"io"
	"os"
	"path"
	"sort"
	"strconv"

	"github.com/juju/errors"
	"github.com/lovelly/gleam/sql/sessionctx/variable"
	"github.com/lovelly/gleam/sql/util/codec"
	"github.com/lovelly/gleam/sql/util/types"
)

type comparableRow struct {
	key    []types.Datum
	val    []types.Datum
	handle int64
}

type item struct {
	index int // source file index
	value *comparableRow
}

// Min-heap of comparableRows
type rowHeap struct {
	sc     *variable.StatementContext
	ims    []*item
	byDesc []bool
	err    error
}

func lessThan(sc *variable.StatementContext, i []types.Datum, j []types.Datum, byDesc []bool) (bool, error) {
	for k := range byDesc {
		v1 := i[k]
		v2 := j[k]

		ret, err := v1.CompareDatum(sc, v2)
		if err != nil {
			return false, errors.Trace(err)
		}

		if byDesc[k] {
			ret = -ret
		}

		if ret < 0 {
			return true, nil
		} else if ret > 0 {
			return false, nil
		}
	}
	return false, nil
}

// Len implements heap.Interface Len interface.
func (rh *rowHeap) Len() int { return len(rh.ims) }

// Swap implements heap.Interface Swap interface.
func (rh *rowHeap) Swap(i, j int) { rh.ims[i], rh.ims[j] = rh.ims[j], rh.ims[i] }

// Less implements heap.Interface Less interface.
func (rh *rowHeap) Less(i, j int) bool {
	l := rh.ims[i].value.key
	r := rh.ims[j].value.key
	ret, err := lessThan(rh.sc, l, r, rh.byDesc)
	if rh.err == nil {
		rh.err = err
	}
	return ret
}

// Push implements heap.Interface Push interface.
func (rh *rowHeap) Push(x interface{}) {
	rh.ims = append(rh.ims, x.(*item))
}

// Push implements heap.Interface Pop interface.
func (rh *rowHeap) Pop() interface{} {
	old := rh.ims
	n := len(old)
	x := old[n-1]
	rh.ims = old[0 : n-1]
	return x
}

// FileSorter sorts the given rows according to the byDesc order.
// FileSorter can sort rows that exceed predefined memory capacity.
type FileSorter struct {
	keySize   int                        // size of key slice
	valSize   int                        // size of val slice
	bufSize   int                        // size of buf slice
	tmpDir    string                     // working directory for file sort
	sc        *variable.StatementContext // required by Datum comparison
	buf       []*comparableRow           // in-memory buffer of rows
	files     []string                   // files generated by file sort
	byDesc    []bool                     // whether or not the specific column sorted in descending order
	cursor    int                        // required when performing full in-memory sort
	closed    bool
	fetched   bool
	rowHeap   *rowHeap
	fds       []*os.File
	fileCount int
	err       error
}

// Builder builds a new FileSorter.
type Builder struct {
	sc      *variable.StatementContext
	keySize int
	valSize int
	bufSize int
	byDesc  []bool
	tmpDir  string
}

// SetSC sets StatementContext instance which is required in row comparison.
func (b *Builder) SetSC(sc *variable.StatementContext) *Builder {
	b.sc = sc
	return b
}

// SetSchema sets the schema of row, including key size and value size.
func (b *Builder) SetSchema(keySize, valSize int) *Builder {
	b.keySize = keySize
	b.valSize = valSize
	return b
}

// SetBuf sets the number of rows FileSorter can hold in memory at a time.
func (b *Builder) SetBuf(bufSize int) *Builder {
	b.bufSize = bufSize
	return b
}

// SetDesc sets the ordering rule of row comparison.
func (b *Builder) SetDesc(byDesc []bool) *Builder {
	b.byDesc = byDesc
	return b
}

// SetDir sets the working directory for FileSorter.
func (b *Builder) SetDir(tmpDir string) *Builder {
	b.tmpDir = tmpDir
	return b
}

// Build creates a FileSorter instance using given data.
func (b *Builder) Build() (*FileSorter, error) {
	// Sanity checks
	if b.sc == nil {
		return nil, errors.New("StatementContext is nil")
	}
	if b.keySize != len(b.byDesc) {
		return nil, errors.New("mismatch in key size and byDesc slice")
	}
	if b.keySize <= 0 {
		return nil, errors.New("key size is not positive")
	}
	if b.valSize <= 0 {
		return nil, errors.New("value size is not positive")
	}
	if b.bufSize <= 0 {
		return nil, errors.New("buffer size is not positive")
	}
	_, err := os.Stat(b.tmpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("tmpDir does not exist")
		}
		return nil, errors.Trace(err)
	}

	rh := &rowHeap{sc: b.sc,
		ims:    make([]*item, 0),
		byDesc: b.byDesc,
	}

	return &FileSorter{sc: b.sc,
		keySize: b.keySize,
		valSize: b.valSize,
		bufSize: b.bufSize,
		buf:     make([]*comparableRow, 0, b.bufSize),
		files:   make([]string, 0),
		byDesc:  b.byDesc,
		rowHeap: rh,
		tmpDir:  b.tmpDir,
		fds:     make([]*os.File, 0),
	}, nil
}

// Len implements sort.Interface Len interface.
func (fs *FileSorter) Len() int { return len(fs.buf) }

// Swap implements sort.Interface Swap interface.
func (fs *FileSorter) Swap(i, j int) { fs.buf[i], fs.buf[j] = fs.buf[j], fs.buf[i] }

// Less implements sort.Interface Less interface.
func (fs *FileSorter) Less(i, j int) bool {
	l := fs.buf[i].key
	r := fs.buf[j].key
	ret, err := lessThan(fs.sc, l, r, fs.byDesc)
	if fs.err == nil {
		fs.err = err
	}
	return ret
}

func (fs *FileSorter) getUniqueFileName() string {
	ret := path.Join(fs.tmpDir, strconv.Itoa(fs.fileCount))
	fs.fileCount++
	return ret
}

// Flush the buffer to file if it is full.
func (fs *FileSorter) flushToFile() error {
	var (
		err        error
		outputFile *os.File
		outputByte []byte
	)

	sort.Sort(fs)
	if fs.err != nil {
		return errors.Trace(fs.err)
	}

	fileName := fs.getUniqueFileName()

	outputFile, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	defer outputFile.Close()

	for _, row := range fs.buf {
		var body []byte
		var head = make([]byte, 8)

		body, err = codec.EncodeKey(body, row.key...)
		if err != nil {
			return errors.Trace(err)
		}
		body, err = codec.EncodeKey(body, row.val...)
		if err != nil {
			return errors.Trace(err)
		}
		body, err = codec.EncodeKey(body, types.NewIntDatum(row.handle))
		if err != nil {
			return errors.Trace(err)
		}

		binary.BigEndian.PutUint64(head, uint64(len(body)))

		outputByte = append(outputByte, head...)
		outputByte = append(outputByte, body...)
	}

	_, err = outputFile.Write(outputByte)
	if err != nil {
		return errors.Trace(err)
	}

	fs.files = append(fs.files, fileName)
	fs.buf = fs.buf[:0]
	return nil
}

// Input adds one row into FileSorter.
// Caller should not call Input after calling Output.
func (fs *FileSorter) Input(key []types.Datum, val []types.Datum, handle int64) error {
	if fs.closed {
		return errors.New("FileSorter has been closed")
	}
	if fs.fetched {
		return errors.New("call input after output")
	}

	// Sanity checks
	if len(key) != fs.keySize {
		return errors.New("mismatch in key size and key slice")
	}
	if len(val) != fs.valSize {
		return errors.New("mismatch in value size and val slice")
	}

	if len(fs.buf) >= fs.bufSize {
		err := fs.flushToFile()
		if err != nil {
			return errors.Trace(err)
		}
	}

	row := &comparableRow{
		key:    key,
		val:    val,
		handle: handle,
	}
	fs.buf = append(fs.buf, row)
	return nil
}

// Fetch the next row given the source file index.
func (fs *FileSorter) fetchNextRow(index int) (*comparableRow, error) {
	var (
		err  error
		n    int
		head = make([]byte, 8)
		dcod = make([]types.Datum, 0, fs.keySize+fs.valSize+1)
	)
	n, err = fs.fds[index].Read(head)
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if n != 8 {
		return nil, errors.New("incorrect header")
	}
	rowSize := int(binary.BigEndian.Uint64(head))

	rowBytes := make([]byte, rowSize)

	n, err = fs.fds[index].Read(rowBytes)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if n != rowSize {
		return nil, errors.New("incorrect row")
	}

	dcod, err = codec.Decode(rowBytes, fs.keySize+fs.valSize+1)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &comparableRow{
		key:    dcod[:fs.keySize],
		val:    dcod[fs.keySize : fs.keySize+fs.valSize],
		handle: dcod[fs.keySize+fs.valSize:][0].GetInt64(),
	}, nil
}

func (fs *FileSorter) openAllFiles() error {
	for _, fname := range fs.files {
		fd, err := os.Open(fname)
		if err != nil {
			return errors.Trace(err)
		}
		fs.fds = append(fs.fds, fd)
	}
	return nil
}

func (fs *FileSorter) closeAllFiles() error {
	for _, fd := range fs.fds {
		err := fd.Close()
		if err != nil {
			return errors.Trace(err)
		}
	}
	err := os.RemoveAll(fs.tmpDir)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Close terminates the input or output process and discards all remaining data.
func (fs *FileSorter) Close() error {
	if fs.closed {
		return errors.New("FileSorter has been closed")
	}
	fs.buf = fs.buf[:0]
	err := fs.closeAllFiles()
	if err != nil {
		return errors.Trace(err)
	}
	fs.closed = true
	return nil
}

// Output gets the next sorted row.
func (fs *FileSorter) Output() ([]types.Datum, []types.Datum, int64, error) {
	if fs.closed {
		return nil, nil, 0, errors.New("FileSorter has been closed")
	}
	if len(fs.files) == 0 {
		// No external files generated.
		// Perform full in-memory sort directly.
		r, err := fs.internalSort()
		if err != nil {
			return nil, nil, 0, errors.Trace(err)
		} else if r != nil {
			return r.key, r.val, r.handle, nil
		} else {
			return nil, nil, 0, nil
		}
	}
	r, err := fs.externalSort()
	if err != nil {
		return nil, nil, 0, errors.Trace(err)
	} else if r != nil {
		return r.key, r.val, r.handle, nil
	} else {
		return nil, nil, 0, nil
	}
}

// Perform full in-memory sort.
func (fs *FileSorter) internalSort() (*comparableRow, error) {
	if !fs.fetched {
		sort.Sort(fs)
		if fs.err != nil {
			return nil, errors.Trace(fs.err)
		}
		fs.fetched = true
	}
	if fs.cursor < len(fs.buf) {
		r := fs.buf[fs.cursor]
		fs.cursor++
		return r, nil
	}
	return nil, nil
}

// Perform external file sort.
func (fs *FileSorter) externalSort() (*comparableRow, error) {
	if !fs.fetched {
		if len(fs.buf) > 0 {
			err := fs.flushToFile()
			if err != nil {
				return nil, errors.Trace(err)
			}
		}

		heap.Init(fs.rowHeap)
		if fs.rowHeap.err != nil {
			return nil, errors.Trace(fs.rowHeap.err)
		}

		err := fs.openAllFiles()
		if err != nil {
			return nil, errors.Trace(err)
		}

		for id := range fs.fds {
			row, err := fs.fetchNextRow(id)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if row == nil {
				return nil, errors.New("file is empty")
			}

			im := &item{
				index: id,
				value: row,
			}

			heap.Push(fs.rowHeap, im)
			if fs.rowHeap.err != nil {
				return nil, errors.Trace(fs.rowHeap.err)
			}
		}

		fs.fetched = true
	}

	if fs.rowHeap.Len() > 0 {
		im := heap.Pop(fs.rowHeap).(*item)
		if fs.rowHeap.err != nil {
			return nil, errors.Trace(fs.rowHeap.err)
		}

		row, err := fs.fetchNextRow(im.index)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if row != nil {
			im := &item{
				index: im.index,
				value: row,
			}

			heap.Push(fs.rowHeap, im)
			if fs.rowHeap.err != nil {
				return nil, errors.Trace(fs.rowHeap.err)
			}
		}

		return im.value, nil
	}

	return nil, nil
}
