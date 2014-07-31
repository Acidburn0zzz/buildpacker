package buildpacker_test

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func copyFileIn(container warden.Container, destination string, source string) error {
	reader, writer := io.Pipe()

	go writeTarTo(filepath.Base(destination), source, writer, 0777)

	return container.StreamIn(filepath.Dir(destination), reader)
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

var _ = Describe("testing a buildpack", func() {
	It("should do stuff", func() {
		container, err := suiteContext.WardenClient.Create(warden.ContainerSpec{})
		Ω(err).ShouldNot(HaveOccurred())

		err = copyFileIn(container, "./tailor", suiteContext.SharedContext.TailorPath)
		Ω(err).ShouldNot(HaveOccurred())

		err = copyFileIn(container, "./soldier", suiteContext.SharedContext.SoldierPath)
		Ω(err).ShouldNot(HaveOccurred())

		process, err := container.Run(warden.ProcessSpec{Path: "ls"}, warden.ProcessIO{
			Stdout: GinkgoWriter,
			Stderr: GinkgoWriter,
		})

		Ω(err).ShouldNot(HaveOccurred())
		exitCode, err := process.Wait()
		Ω(err).ShouldNot(HaveOccurred())
		Ω(exitCode).Should(Equal(0))
	})
})
