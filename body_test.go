package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

var allTheBytes []byte = []byte{1, 3, 7, 15, 31, 63, 127, 255}

func TestBodyReading(t *testing.T) {

	filename := "test-files/body-reading"

	_file, _err := os.Create(filename)
	if _err != nil {
		t.Error(_err)
	}
	for i := 0; i < 256; i++ {
		_file.Write(allTheBytes)
	}
	_file.Close()

  filename2 := "test-files/select-single-byte"
  _file, _err = os.Create(filename2)
  if _err != nil {
    t.Error(_err)
  }

  _file.Write([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8})
  _file.Close()

	t.Run("should return error if file doesn't exist", func(t *testing.T) {
		_, err := newBody("file", 128, 128)
		if !os.IsNotExist(err) {
			t.Error(err)
		}
	})

	t.Run("should return error if size is zero", func(t *testing.T) {
		_, err := newBody(filename, 128, 0)
		if os.IsNotExist(err) {
			t.Error(err)
		} else if err == nil {
			t.Error("Expected an error")
		}

	})

	t.Run("should read a complete 2048 byte file", func(t *testing.T) {
		body, err := newBody(filename, 0, 2048)
		if err != nil {
			t.Error(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Error(err)
		}
		body.Close()

		if len(bodyData) != 2048 {
			t.Error("Body data was not expected 2048 bytes")
		}

		for i := 0; i < 2047; i += 8 {
			if !bytes.Equal(allTheBytes, bodyData[i:i+8]) {
				t.Errorf("Body data did not match between bytes %d and %d", i, i+8)
			}
		}
	})

	t.Run("should read first 1024 bytes of a 2048 byte file", func(t *testing.T) {
		body, err := newBody(filename, 0, 1024)
		if err != nil {
			t.Error(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Error(err)
		}
		body.Close()

		if len(bodyData) != 1024 {
			t.Error("Body data was not expected 2048 bytes")
		}

		for i := 0; i < 1024; i += 8 {
			if !bytes.Equal(allTheBytes, bodyData[i:i+8]) {
				t.Errorf("Body data did not match between bytes %d and %d", i, i+8)
			}
		}
	})

	t.Run("should read second 1024 bytes of a 2048 byte file", func(t *testing.T) {
		body, err := newBody(filename, 1024, 1024)
		if err != nil {
			t.Error(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Error(err)
		}
		body.Close()

		if len(bodyData) != 1024 {
			t.Error("Body data was not expected 2048 bytes")
		}

		for i := 0; i < 1024; i += 8 {
			if !bytes.Equal(allTheBytes, bodyData[i:i+8]) {
				t.Errorf("Body data did not match between bytes %d and %d", i, i+8)
			}
		}
	})

	t.Run("should read middle 1024 bytes of a 2048 byte file", func(t *testing.T) {
		body, err := newBody(filename, 512, 1024)
		if err != nil {
			t.Error(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Error(err)
		}
		body.Close()

		if len(bodyData) != 1024 {
			t.Error("Body data was not expected 2048 bytes")
		}

		for i := 0; i < 1024; i += 8 {
			if !bytes.Equal(allTheBytes, bodyData[i:i+8]) {
				t.Errorf("Body data did not match between bytes %d and %d", i, i+8)
			}
		}
	})

	t.Run("should read exactly one unique byte", func(t *testing.T) {
		// We make this buf 2 so that the io.Reader could theoretically read in
		// more than a single byte if things go wrong.
		buf := make([]byte, 2)

		body, err := newBody(filename2, 3, 1)
		defer body.Close()
		if err != nil {
			t.Error(err)
		}
		nBytes, err := body.Read(buf)
		if err != nil {
			t.Error(err)
		}

		if nBytes != 1 {
			t.Errorf("Expected to read a single byte, got %d", nBytes)
		}

		if buf[0] != 3 {
			t.Errorf("Expected single byte to be 3, got %d", buf[0])
		}
	})
}
