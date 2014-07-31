package buildpacker_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry-incubator/buildpacker/utils"
	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/fraenkel/candiedyaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("the go buildpack", func() {
	var (
		buildpackPath, appPath string
		container              warden.Container
		host                   string
	)

	BeforeEach(func() {
		buildpackPath = "go-buildpack/go_buildpack-offline-v1.0.1.zip"
		appPath = "./fixtures/go_app/src/go_app"
		container, host = CreateContainer()
	})

	It("should detect the start command", func() {
		stagingInfo := Stage(container, buildpackPath, appPath)

		Ω(stagingInfo.DetectedStartCommand).ShouldNot(BeEmpty())
	})

	It("should run the app", func() {
		stagingInfo := Stage(container, buildpackPath, appPath)
		stagingInfo.DetectedStartCommand = "./bin/go-online"

		Run(container, stagingInfo)
		PingTillUp(host)

		response, err := http.Get(host)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(response.StatusCode).Should(Equal(http.StatusOK))
		io.Copy(os.Stdout, response.Body)
		response.Body.Close()
	})
})

func CreateContainer() (container warden.Container, host string) {
	fmt.Println("Creating Container")
	container, err := suiteContext.WardenClient.Create(warden.ContainerSpec{})
	Ω(err).ShouldNot(HaveOccurred(), "Failed to create container")

	hostPort, _, err := container.NetIn(0, 8080)
	Ω(err).ShouldNot(HaveOccurred(), "Failed to net-in to the container")
	return container, fmt.Sprintf("http://%s:%d/", suiteContext.ExternalAddress, hostPort)
}

func Stage(container warden.Container, buildpackPath string, appPath string) models.StagingInfo {
	fullPathToBuildpack, err := filepath.Abs("../../../../buildpacks/" + buildpackPath)
	Ω(err).ShouldNot(HaveOccurred())
	fullPathToApp, err := filepath.Abs(appPath)
	Ω(err).ShouldNot(HaveOccurred())

	buildpackName := strings.Split(filepath.Base(buildpackPath), ".")[0]

	_, err = os.Stat(fullPathToBuildpack)
	Ω(err).ShouldNot(HaveOccurred(), "Could not find buildpack")

	_, err = os.Stat(fullPathToApp)
	Ω(err).ShouldNot(HaveOccurred(), "Could not find app")

	config := models.NewCircusTailorConfig([]string{buildpackName})

	fmt.Println("Copying Tailor")
	err = utils.CopyFileIn(container, suiteContext.SharedContext.TailorPath, config.ExecutablePath)
	Ω(err).ShouldNot(HaveOccurred())

	fmt.Println("Copying App")
	err = utils.CopyDirectoryIn(container, fullPathToApp, config.AppDir())
	Ω(err).ShouldNot(HaveOccurred())

	fmt.Println("Copying Buildpack")
	err = utils.CopyZipIn(container, fullPathToBuildpack, config.BuildpackPath(buildpackName))
	Ω(err).ShouldNot(HaveOccurred())

	fmt.Println("Running Tailor")
	process, err := container.Run(warden.ProcessSpec{
		Path: config.Path(),
		Args: config.Args(),
	}, warden.ProcessIO{
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	})
	Ω(err).ShouldNot(HaveOccurred())
	exitCode, err := process.Wait()
	Ω(err).ShouldNot(HaveOccurred())
	Ω(exitCode).Should(Equal(0))

	fmt.Println("Fetching StagingInfo")
	rawStagingInfo, err := utils.GetFileContents(container, filepath.Join(config.OutputDropletDir(), "staging_info.yml"))
	Ω(err).ShouldNot(HaveOccurred())
	stagingInfo := models.StagingInfo{}
	err = candiedyaml.Unmarshal(rawStagingInfo, &stagingInfo)
	Ω(err).ShouldNot(HaveOccurred())

	return stagingInfo
}

func Run(container warden.Container, stagingInfo models.StagingInfo) {
	fmt.Println("Copying Soldier")
	err := utils.CopyFileIn(container, suiteContext.SharedContext.SoldierPath, "/tmp/circus/soldier")
	Ω(err).ShouldNot(HaveOccurred())

	fmt.Println("Moving droplet to /home/vcap")
	process, err := container.Run(warden.ProcessSpec{
		Path: "bash",
		Args: []string{"-c", "rm -rf /home/vcap/app/*; mv /tmp/droplet/app/* /home/vcap/app/; rm -rd /tmp/droplet/app ; mv /tmp/droplet/* /home/vcap/"},
	}, warden.ProcessIO{})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0))

	fmt.Println("Running app")
	process, err = container.Run(warden.ProcessSpec{
		Path: "/tmp/circus/soldier",
		Args: append([]string{"/app"}, strings.Split(stagingInfo.DetectedStartCommand, " ")...),
		Env: []string{
			"PORT=8080",
			`VCAP_APPLICATION={"instance_index": 1}`,
		},
	}, warden.ProcessIO{
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	})
	Ω(err).ShouldNot(HaveOccurred())
}

func PingTillUp(endpoint string) {
	Eventually(func() error {
		_, err := http.Get(endpoint)
		return err
	}).ShouldNot(HaveOccurred())
}
