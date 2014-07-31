package buildpacker_test

import (
	"encoding/json"
	"fmt"
	"os"

	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	WardenRunner "github.com/cloudfoundry-incubator/warden-linux/integration/runner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/inigo/inigo_server"
)

var DEFAULT_EVENTUALLY_TIMEOUT = 15 * time.Second
var DEFAULT_CONSISTENTLY_DURATION = 5 * time.Second

var wardenRunner *WardenRunner.Runner

type sharedContextType struct {
	TailorPath  string
	SoldierPath string
	WardenPath  string
}

func DecodeSharedContext(data []byte) sharedContextType {
	var context sharedContextType
	err := json.Unmarshal(data, &context)
	Ω(err).ShouldNot(HaveOccurred())

	return context
}

func (d sharedContextType) Encode() []byte {
	data, err := json.Marshal(d)
	Ω(err).ShouldNot(HaveOccurred())
	return data
}

type Runner interface {
	KillWithFire()
}

type suiteContextType struct {
	SharedContext sharedContextType

	ExternalAddress string

	WardenProcess ifrit.Process
	WardenClient  warden.Client
}

var suiteContext suiteContextType

func beforeSuite(encodedSharedContext []byte) {
	sharedContext := DecodeSharedContext(encodedSharedContext)

	context := suiteContextType{
		SharedContext:   sharedContext,
		ExternalAddress: os.Getenv("EXTERNAL_ADDRESS"),
	}

	Ω(context.ExternalAddress).ShouldNot(BeEmpty())

	wardenBinPath := os.Getenv("WARDEN_BINPATH")
	wardenRootfs := os.Getenv("WARDEN_ROOTFS")

	if wardenBinPath == "" || wardenRootfs == "" {
		println("Please define either WARDEN_NETWORK and WARDEN_ADDR (for a running Warden), or")
		println("WARDEN_BINPATH and WARDEN_ROOTFS (for the tests to start it)")
		println("")

		Fail("warden is not set up")
	}

	wardenAddress := fmt.Sprintf("/tmp/warden_%d.sock", config.GinkgoConfig.ParallelNode)
	wardenRunner = WardenRunner.New(
		"unix",
		wardenAddress,
		context.SharedContext.WardenPath,
		wardenBinPath,
		wardenRootfs,
	)

	// make context available to all tests
	suiteContext = context
}

func TestVizzini(t *testing.T) {
	registerDefaultTimeouts()

	RegisterFailHandler(Fail)

	nodeOne := &nodeOneType{}

	SynchronizedBeforeSuite(func() []byte {
		nodeOne.CompileTestedExecutables()

		return nodeOne.context.Encode()
	}, beforeSuite)

	BeforeEach(func() {
		suiteContext.WardenClient = wardenRunner.NewClient()
		suiteContext.WardenProcess = ifrit.Envoke(wardenRunner)
		Eventually(wardenRunner.TryDial, 10).ShouldNot(HaveOccurred())

		inigo_server.Start(suiteContext.WardenClient)

		currentTestDescription := CurrentGinkgoTestDescription()
		fmt.Fprintf(GinkgoWriter, "\n%s\n%s\n\n", strings.Repeat("~", 50), currentTestDescription.FullTestText)
	})

	AfterEach(func() {
		inigo_server.Stop(suiteContext.WardenClient)

		suiteContext.WardenProcess.Signal(syscall.SIGKILL)
		Eventually(suiteContext.WardenProcess.Wait(), 10*time.Second).Should(Receive())
	})

	RunSpecs(t, "Inigo Integration Suite")
}

func registerDefaultTimeouts() {
	var err error
	if os.Getenv("DEFAULT_EVENTUALLY_TIMEOUT") != "" {
		DEFAULT_EVENTUALLY_TIMEOUT, err = time.ParseDuration(os.Getenv("DEFAULT_EVENTUALLY_TIMEOUT"))
		if err != nil {
			panic(err)
		}
	}

	if os.Getenv("DEFAULT_CONSISTENTLY_DURATION") != "" {
		DEFAULT_CONSISTENTLY_DURATION, err = time.ParseDuration(os.Getenv("DEFAULT_CONSISTENTLY_DURATION"))
		if err != nil {
			panic(err)
		}
	}

	SetDefaultEventuallyTimeout(DEFAULT_EVENTUALLY_TIMEOUT)
	SetDefaultConsistentlyDuration(DEFAULT_CONSISTENTLY_DURATION)
}

type nodeOneType struct {
	context sharedContextType
}

func (node *nodeOneType) CompileTestedExecutables() {
	var err error

	node.context.WardenPath, err = gexec.BuildIn(os.Getenv("WARDEN_LINUX_GOPATH"), "github.com/cloudfoundry-incubator/warden-linux", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	node.context.TailorPath, err = gexec.BuildIn(os.Getenv("LINUX_CIRCUS_GOPATH"), "github.com/cloudfoundry-incubator/linux-circus/tailor", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	node.context.SoldierPath, err = gexec.BuildIn(os.Getenv("LINUX_CIRCUS_GOPATH"), "github.com/cloudfoundry-incubator/linux-circus/soldier", "-race")
	Ω(err).ShouldNot(HaveOccurred())
}
