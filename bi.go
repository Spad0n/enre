package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"io"
	"strconv"
)

func readBlobField(f *bufio.Reader, name []byte) ([]byte, error) {
	line, err := f.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	field := append([]byte(":b "), name...)
	field = append(field, ' ')

	if !bytes.HasPrefix(line, field) {
		return nil, fmt.Errorf("expected line to start with %s", field)
	}

	if !bytes.HasSuffix(line, []byte("\n")) {
		return nil, fmt.Errorf("line does not end with newline")
	}

	sizeStr := string(line[len(field) : len(line)-1])
	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return nil, err
	}

	blob := make([]byte, size)
	_, err = io.ReadFull(f, blob)
	if err != nil {
		return nil, err
	}

	newline, err := f.ReadByte()
	if err != nil {
		return nil, err
	}

	if newline != '\n' {
		return nil, fmt.Errorf("expected final newline, got: %v", newline)
	}

	return blob, nil
}

func readIntField(f *bufio.Reader, name []byte) (int, error) {
	line, err := f.ReadBytes('\n')
	if err != nil {
		return 0, err
	}

	field := append([]byte(":i "), name...)
	field = append(field, ' ')

	if !bytes.HasPrefix(line, field) {
		return 0, fmt.Errorf("expected line to start with %s", field)
	}

	if !bytes.HasSuffix(line, []byte("\n")) {
		return 0, fmt.Errorf("line does not end with newline")
	}

	intStr := string(line[len(field) : len(line)-1])
	value, err := strconv.Atoi(intStr)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func writeIntField(file *os.File, name string, value int) {
	fmt.Fprintf(file, ":i %s %d\n", name, value)
}

func writeBlobField(file *os.File, name string, blob []byte) {
	fmt.Fprintf(file, ":b %s %d\n", name, len(blob))
	fmt.Fprintln(file, string(blob))
}
