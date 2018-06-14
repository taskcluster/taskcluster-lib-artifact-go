package artifact

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

var allTheBytes = []byte{1, 3, 7, 15, 31, 63, 127, 255}

func setup(t *testing.T) (*os.File, []byte, func()) {

	var b bytes.Buffer
	var err error

	if err = os.MkdirAll("testdata", os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}

	file, err := ioutil.TempFile("testdata", "body-test")
	if err != nil {
		t.Fatal(err)
	}

	out := io.MultiWriter(file, &b)

	// We declare this outside of the loop to minimize allocations
	randbuf := make([]byte, 8)

	for i := 0; i < 256; i++ {
		// Using repeated bytes is nice for debugging purposes
		if _, ok := os.LookupEnv("USE_REPEATED_BYTES"); ok {
			_, err := out.Write(allTheBytes)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			_, err = rand.Read(randbuf)
			if err != nil {
				t.Fatal(err)
			}
			_, err = out.Write(randbuf)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}

	return file, b.Bytes(), func() {
		err := file.Close()
		if err != nil {
			t.Error(err)
		}
		err = os.Remove(file.Name())
		if err != nil {
			t.Error(err)
		}
	}
}

func TestBodyReading(t *testing.T) {

	SetLogOutput(newUnitTestLogWriter(t))

	t.Run("should return error if size is zero", func(t *testing.T) {
		file, _, teardown := setup(t)
		defer teardown()
		_, err := newBody(file, 128, 0)
		if err == nil {
			t.Fatal("Expected an error")
		}
	})

	t.Run("should read a complete 2048 byte file", func(t *testing.T) {
		file, b, teardown := setup(t)
		defer teardown()
		body, err := newBody(file, 0, 2048)
		if err != nil {
			t.Fatal(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}

		if len(bodyData) != 2048 {
			t.Fatal("Body data was not 2048 bytes")
		}

		if !bytes.Equal(bodyData, b) {
			t.Fatalf("Body data did not match")
		}
	})

	t.Run("should read first 1024 bytes of a 2048 byte file", func(t *testing.T) {
		file, b, teardown := setup(t)
		defer teardown()
		body, err := newBody(file, 0, 1024)
		if err != nil {
			t.Fatal(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}

		if len(bodyData) != 1024 {
			t.Fatal("Body data was not 1024 bytes")
		}

		if !bytes.Equal(bodyData, b[:1024]) {
			t.Fatalf("Body data did not match")
		}
	})

	t.Run("should read second 1024 bytes of a 2048 byte file", func(t *testing.T) {
		file, b, teardown := setup(t)
		defer teardown()
		body, err := newBody(file, 1024, 1024)
		if err != nil {
			t.Fatal(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}

		if len(bodyData) != 1024 {
			t.Fatal("Body data was not 1024 bytes")
		}

		if !bytes.Equal(bodyData, b[1024:]) {
			t.Fatalf("Body data did not match")
		}
	})

	t.Run("should read middle 1024 bytes of a 2048 byte file", func(t *testing.T) {
		file, b, teardown := setup(t)
		defer teardown()
		body, err := newBody(file, 512, 1024)
		if err != nil {
			t.Fatal(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}

		if len(bodyData) != 1024 {
			t.Fatal("Body data was not 1024 bytes")
		}

		if !bytes.Equal(bodyData, b[512:1024+512]) {
			t.Fatalf("Body data did not match")
		}
	})

	t.Run("should read exactly one unique byte", func(t *testing.T) {
		// We make this buf 2 so that the io.Reader could theoretically read in
		// more than a single byte if things go wrong.
		file, b, teardown := setup(t)
		defer teardown()
		body, err := newBody(file, 3, 1)
		if err != nil {
			t.Fatal(err)
		}

		bodyData, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}

		if len(bodyData) != 1 {
			t.Fatalf("Expected to read a single byte, got %d", len(bodyData))
		}

		if bodyData[0] != b[3] {
			t.Fatalf("Expected single byte to be %d, got %d", b[3], bodyData[0])
		}
	})
}
