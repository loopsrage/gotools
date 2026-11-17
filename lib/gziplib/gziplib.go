package gziplib

import (
	"bytes"
	"compress/gzip"
	"io"
)

func IsGzip(buffer []byte) bool {
	if buffer == nil || len(buffer) < 2 {
		return false
	}
	return buffer[0] == 0x1F && buffer[1] == 0x8B
}

func EnsureGunzip(buffer *bytes.Buffer) (*bytes.Buffer, error) {
	pld := buffer.Bytes()
	for IsGzip(pld) {
		rdr, err := gzip.NewReader(bytes.NewReader(pld))
		if err != nil {
			return nil, err
		}

		pld, err = io.ReadAll(rdr)
		if err != nil {
			return nil, err
		}
	}
	return bytes.NewBuffer(pld), nil
}

func EnsureGunzipRW(buffer io.ReadWriter) (*bytes.Buffer, error) {
	pld, err := io.ReadAll(buffer)
	if err != nil {
		return nil, err
	}

	for IsGzip(pld) {
		rdr, err := gzip.NewReader(bytes.NewReader(pld))
		if err != nil {
			return nil, err
		}

		pld, err = io.ReadAll(rdr)
		if err != nil {
			return nil, err
		}
	}
	return bytes.NewBuffer(pld), nil
}

func Gzip(input []byte) ([]byte, error) {
	buff := bytes.NewBuffer(nil)
	wr := gzip.NewWriter(buff)
	_, err := wr.Write(input)
	if err != nil {
		return nil, err
	}
	err = wr.Close()
	if err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}
