package main

import "encoding/gob"

type Input struct {
	Data       []byte
	EOF        bool
	WindowSize *WindowSize
}

type WindowSize struct {
	Columns int
	Rows    int
}

type inputWriter struct {
	enc *gob.Encoder
}

func (w *inputWriter) Write(d []byte) (int, error) {
	err := w.enc.Encode(Input{Data: d})
	if err != nil {
		return 0, err
	}

	return len(d), nil
}

func (w *inputWriter) Close() error {
	return w.enc.Encode(Input{EOF: true})
}

func (w *inputWriter) SetWindowSize(cols, rows int) error {
	return w.enc.Encode(Input{
		WindowSize: &WindowSize{
			Columns: cols,
			Rows:    rows,
		},
	})
}
