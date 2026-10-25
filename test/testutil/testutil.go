package testutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func EnvOf(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
}

type TarEntry struct {
	Name     string
	Body     string
	Typeflag byte
	Linkname string
}

func TarGz(t *testing.T, entries ...TarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag := e.Typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.Name, Typeflag: typeflag, Mode: 0o755, Linkname: e.Linkname}
		if typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.Body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
