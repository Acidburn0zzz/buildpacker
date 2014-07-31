package utils

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/pivotal-golang/archiver/compressor"
	"github.com/pivotal-golang/archiver/extractor"

	"github.com/cloudfoundry-incubator/garden/warden"
)

func CopyFileIn(container warden.Container, source string, destination string) error {
	reader, writer := io.Pipe()

	go writeTarTo(filepath.Base(destination), source, writer, 0777)

	return container.StreamIn(filepath.Dir(destination), reader)
}

func CopyZipIn(container warden.Container, source string, destination string) error {
	extractionDir, err := extract(source)
	if err != nil {
		return fmt.Errorf("EXTRACTION FAILED: %s", err.Error())
	}

	defer os.RemoveAll(extractionDir)

	err = copyDirectory(container, extractionDir, destination)
	if err != nil {
		return fmt.Errorf("COPY FAILED: %s", err.Error())
	}

	return err
}

func CopyDirectoryIn(container warden.Container, source string, destination string) error {
	return copyDirectory(container, source, destination)
}

func GetFileContents(container warden.Container, path string) ([]byte, error) {
	streamOut, err := container.StreamOut(path)
	if err != nil {
		return nil, err
	}

	defer streamOut.Close()

	reader := tar.NewReader(streamOut)
	_, err = reader.Next()
	if err != nil {
		return nil, err
	}

	buffer := &bytes.Buffer{}
	io.Copy(buffer, reader)

	return buffer.Bytes(), nil
}

func writeTarTo(name string, sourcePath string, destination *io.PipeWriter, mode int64) {
	source, err := os.Open(sourcePath)
	if err != nil {
		destination.CloseWithError(err)
		return
	}
	defer source.Close()

	tarWriter := tar.NewWriter(destination)

	fileInfo, err := source.Stat()
	if err != nil {
		destination.CloseWithError(err)
		return
	}

	err = tarWriter.WriteHeader(&tar.Header{
		Name:       name,
		Size:       fileInfo.Size(),
		Mode:       mode,
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
	})
	if err != nil {
		destination.CloseWithError(err)
		return
	}

	_, err = io.Copy(tarWriter, source)
	if err != nil {
		destination.CloseWithError(err)
		return
	}

	if err := tarWriter.Flush(); err != nil {
		destination.CloseWithError(err)
		return
	}

	destination.Close()
}

func extract(downloadedPath string) (string, error) {
	extractionDir, err := ioutil.TempDir("", "extracted")
	if err != nil {
		return "", err
	}

	detectingExtractor := extractor.NewDetectable()

	err = detectingExtractor.Extract(downloadedPath, extractionDir)
	if err != nil {
		return "", err
	}

	return extractionDir, nil
}

func copyDirectory(container warden.Container, source string, destination string) error {
	reader, writer := io.Pipe()

	go func() {
		err := compressor.WriteTar(source+string(filepath.Separator), writer)
		if err == nil {
			writer.Close()
		} else {
			writer.CloseWithError(err)
		}
	}()

	return container.StreamIn(destination, reader)
}
