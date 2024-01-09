package functional_tests

import (
	"context"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	gstruct "github.com/onsi/gomega/gstruct"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	machinev1 "github.com/openshift/client-go/machine/clientset/versioned"
	. "github.com/project-codeflare/codeflare-common/support"
	"github.com/project-codeflare/instascale/controllers"
	"github.com/project-codeflare/instascale/pkg/config"
	mcadv1beta1 "github.com/project-codeflare/multi-cluster-app-dispatcher/pkg/apis/controller/v1beta1"
	mc "github.com/project-codeflare/multi-cluster-app-dispatcher/pkg/client/clientset/versioned"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg       *rest.Config
	k8sClient client.Client // You'll be using this client in your tests.
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
)

func startEnvTest(t *testing.T) *envtest.Environment {
	test := With(t)
	//specify testEnv configuration
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "github.com", "project-codeflare", "multi-cluster-app-dispatcher@v1.38.1", "config", "crd", "bases"),
			filepath.Join(build.Default.GOPATH, "pkg", "mod", "github.com", "openshift", "api@v0.0.0-20220411210816-c3bb724c282a", "machine", "v1beta1"),
		},
	}
	cfg, err = testEnv.Start()
	test.Expect(err).NotTo(HaveOccurred())

	return testEnv
}

func establishClient(t *testing.T) {
	test := With(t)
	err = mcadv1beta1.AddToScheme(scheme.Scheme)
	err = machinev1beta1.AddToScheme(scheme.Scheme)
	test.Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	test.Expect(err).NotTo(HaveOccurred())
	test.Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	test.Expect(err).ToNot(HaveOccurred())

	instaScaleController := &controllers.AppWrapperReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		// Config: cfg.InstaScale.InstaScaleConfiguration,
		Config: config.InstaScaleConfiguration{
			MachineSetsStrategy: "reuse",
			MaxScaleoutAllowed:  5,
		},
	}
	err = instaScaleController.SetupWithManager(context.Background(), k8sManager)
	test.Expect(err).ToNot(HaveOccurred())

	go func() {

		err = k8sManager.Start(ctrl.SetupSignalHandler())
		test.Expect(err).ToNot(HaveOccurred())
	}()

}

func teardownTestEnv(testEnv *envtest.Environment) {
	logger := ctrl.LoggerFrom(ctx)
	if err := testEnv.Stop(); err != nil {
		logger.Error(err, "Error stopping test Environment\n")
	}
}

func instascaleAppwrapper(namespace string) *mcadv1beta1.AppWrapper {
	aw := &mcadv1beta1.AppWrapper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "abhi-test",
			Namespace: namespace,
			Labels: map[string]string{
				// "orderedinstance": "g4dn.xlarge",
				"orderedinstance": "test.instance1",
			},
			// Labels: map[string]string{
			// 	"orderedinstance": "test.instance1_test.instance2",
			// },
		},
		Spec: mcadv1beta1.AppWrapperSpec{
			AggrResources: mcadv1beta1.AppWrapperResourceList{
				GenericItems: []mcadv1beta1.AppWrapperGenericResource{
					{
						DesiredAvailable: 1,
						CustomPodResources: []mcadv1beta1.CustomPodResourceTemplate{
							{
								Replicas: 1,
								Requests: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse("250m"),
									apiv1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Limits: apiv1.ResourceList{
									apiv1.ResourceCPU:    resource.MustParse("1"),
									apiv1.ResourceMemory: resource.MustParse("1G"),
								},
							},
						},
					},
				},
			},
		},
	}
	return aw
}

type MyReconciler struct {
	*controllers.AppWrapperReconciler
}

func (r *MyReconciler) printMsReplicas() int {
	allMachineSets := machinev1beta1.MachineSetList{}
	logger := ctrl.LoggerFrom(ctx)
	if err := r.List(ctx, &allMachineSets); err != nil {
		logger.Error(err, "Error listing MachineSets")
	}
	for _, aMachineSet := range allMachineSets.Items {
		msName, msReplica := aMachineSet.Name, aMachineSet.Spec.Replicas
		fmt.Println(msName, " : ", msReplica)
	}
	return 0
}

func TestReconciler(t *testing.T) {
	testEnv = startEnvTest(t)
	defer teardownTestEnv(testEnv)

	establishClient(t)
	test := With(t)

	machineClient, err := machinev1.NewForConfig(cfg)
	test.Expect(err).ToNot(HaveOccurred())

	b, err := os.ReadFile("qqq.txt")
	test.Expect(err).ToNot(HaveOccurred())

	decode := scheme.Codecs.UniversalDeserializer().Decode
	ms, _, err := decode(b, nil, nil)
	test.Expect(err).ToNot(HaveOccurred())
	msa := ms.(*machinev1beta1.MachineSet)

	// namespace := test.NewTestNamespace()
	aw := instascaleAppwrapper("default")

	ms, err = machineClient.MachineV1beta1().MachineSets("default").Create(test.Ctx(), msa, metav1.CreateOptions{})
	test.Expect(err).ToNot(HaveOccurred())

	// skip test if not using machine sets
	// clusterType := GetClusterType(test)
	// if clusterType != OcpCluster {
	// 	test.T().Skipf("Skipping test as not running on an OCP cluster, resolved cluster type: %s", clusterType)
	// }

	// create client for interacting with cluster API using cfg provided
	mcadClient, err := mc.NewForConfig(cfg)
	test.Expect(err).ToNot(HaveOccurred())

	// look for machine belonging to the machine set, there should be none
	// test.Expect(GetMachines(test, aw.Name)).Should(BeEmpty())
	// fmt.Println("Appwrapper with given name not found....")

	// create appwrapper resource using mcadClient
	_, err = mcadClient.WorkloadV1beta1().AppWrappers("default").Create(test.Ctx(), aw, metav1.CreateOptions{})
	test.Expect(err).ToNot(HaveOccurred())

	fmt.Println("appwrapper created!")
	time.Sleep(10 * time.Second)

	machineset, err := machineClient.MachineV1beta1().MachineSets("default").Get(test.Ctx(), "test-instascale", metav1.GetOptions{})
	test.Expect(err).ToNot(HaveOccurred())
	fmt.Println(machineset.Spec.Replicas)
	test.Expect(machineset.Spec.Replicas).To(gstruct.PointTo(Equal(int32(1))))

	// assert that AppWrapper goes to "Running" state
	// test.Eventually(AppWrapper(test, namespace, aw.Name), TestTimeoutGpuProvisioning).
	// 	Should(WithTransform(AppWrapperState, Equal(mcadv1beta1.AppWrapperStateActive)))
	// fmt.Println("Appwrapper goes to running test...")

	// // look for machine belonging to the machine set - expect to find it
	// test.Eventually(Machines(test, aw.Name), TestTimeoutLong).Should(HaveLen(1))

	// fmt.Println("machine expected!")

	// // look for machine belonging to the machine set - there should be none
	// test.Eventually(Machines(test, app.Name), TestTimeoutLong).Should(BeEmpty())

}
