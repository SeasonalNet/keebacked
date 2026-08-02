//go:build linux

package logging

import (
	"os"
	"testing"
)

func TestPipeIsNotTerminal(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close pipe reader: %v", err)
		}
	})
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close pipe writer: %v", err)
		}
	})

	if isTerminal(writer) {
		t.Fatal("pipe unexpectedly detected as a terminal")
	}
}
