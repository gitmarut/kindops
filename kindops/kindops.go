package kindops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"gopkg.in/yaml.v2"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/kube"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	serialize "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/log"
	//metallbv1 "go.universe.tf/metallb/api/v1beta1"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/* There are two config files used. One is default now. Which is mentioned as flagpole.Config
   This might need tweaking later for different options
   "https://github.com/kubernetes-sigs/kind/blob/main/pkg/apis/config/v1alpha4/default.go"

   The other one is read as flagpole itself.
*/

type flagpole struct {
	Name                    string        `yaml:"name"`
	Config                  string        `yaml:"config"`
	ImageName               string        `yaml:"imageName"`
	Retain                  bool          `yaml:"retain"`
	Wait                    time.Duration `yaml:"waitTime"`
	Kubeconfig              string        `yaml:"kubeConfig"`
	Metallbchartpath        string        `yaml:"metalLbChartPath"`
	Metallbreleasename      string        `yaml:"metalLbReleaseName"`
	Metallbreleasenamespace string        `yaml:"metalLbReleaseNamespace"`
	Metallbiprange          string        `yaml:"metalLbIpRange"`
}

// quick and dirty Helm install. Need to improve
// Also need to move metalLB related stuff to another package

//func InstallMetalLbIpv4Pool()

func (c *flagpole) getConf(configfile string, logger log.Logger) *flagpole {

	dat, err := os.ReadFile(configfile)
	check("Read Config File - ", err, logger)

	err = yaml.Unmarshal(dat, c)
	check("Unmarshal Config File - ", err, logger)

	return c
}

func check(condition string, e error, logger log.Logger) {
	if e != nil {
		logger.Error(condition + e.Error())
		panic(e)
	} else {
		logger.V(0).Infof("Success - %q ...\n", condition)
	}
}

func CreateCluster(configfile string, logger log.Logger) error {

	var c flagpole
	c.getConf(configfile, logger)

	fmt.Printf("%+v\n", c)

	logger.V(0).Infof("Creating cluster %q ...\n", c.Name)

	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	err := provider.Create(c.Name, cluster.CreateWithConfigFile(c.Config),
		cluster.CreateWithNodeImage(c.ImageName),
		cluster.CreateWithRetain(c.Retain),
		cluster.CreateWithWaitForReady(c.Wait*time.Second),
		cluster.CreateWithKubeconfigPath(c.Kubeconfig),
		cluster.CreateWithDisplayUsage(true),
		cluster.CreateWithDisplaySalutation(true),
	)
	check("Kind Cluster Create - ", err, logger)

	// install metalLB from Helm
	err = InstallHelmChart(c.Kubeconfig, c.Metallbchartpath, c.Metallbreleasename, c.Metallbreleasenamespace, logger)
	check("Installation of MetalLB Helm Chart - ", err, logger)

	dclient, tclient, err := GetDynamicKubeClient(c.Kubeconfig, logger)
	check("Get Kind Cluster's Dynamic & Typed Clients - ", err, logger)

	err = InstallMetalLbResources(dclient, tclient, c.Metallbiprange, c.Metallbreleasenamespace, logger)
	check("Install MetalLB CRs - ", err, logger)

	err = ApplyYAMLfile(dclient, tclient, "wp-all.yaml", "default", logger)
	check("Install a sample Wordpress App - ", err, logger)

	if err == nil {
		logger.V(0).Infof("Cluster with all dependencies completed - %q", c.Name)
	}
	return (err)

}

func DeleteCluster(configfile string, logger log.Logger) error {

	var c flagpole
	c.getConf(configfile, logger)

	provider := cluster.NewProvider(
		//cluster.ProviderWithLogger(logger),
		cluster.ProviderWithLogger(logger),
	)

	err := provider.Delete(c.Name, c.Kubeconfig)

	check("Kind Cluster Delete - ", err, logger)

	return (err)

}

func InstallHelmChart(kubeconfigPath string, chartPath string, releaseName string, releaseNamespace string, logger log.Logger) error {

	actionConfig := new(action.Configuration)
	err := actionConfig.Init(kube.GetConfig(kubeconfigPath, "", releaseNamespace), releaseNamespace, os.Getenv("HELM_DRIVER"), logger.V(0).Infof)
	check("Init Helm - ", err, logger)

	settings := cli.New()
	iCli := action.NewInstall(actionConfig)
	fmt.Println("CHARTPATH - " + chartPath)
	chrt_path, err := iCli.LocateChart(chartPath, settings)
	check("Locate Helm Chart - ", err, logger)
	chart, err := loader.Load(chrt_path)
	check("Load Helm Chart - ", err, logger)

	iCli.Namespace = releaseNamespace
	iCli.ReleaseName = releaseName
	rel, err := iCli.Run(chart, nil)
	check("Installation of Helm - "+rel.Name, err, logger)

	return (err)

}

func GetDynamicKubeClient(kubeconfigPath string, logger log.Logger) (*dynamic.DynamicClient, *kubernetes.Clientset, error) {

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	check("Build Kube Config - ", err, logger)

	dClient, err := dynamic.NewForConfig(config)
	check("Get Dynamic Client - ", err, logger)

	tClient, err := kubernetes.NewForConfig(config)
	check("Get Typed Client - ", err, logger)

	return dClient, tClient, err

}

func InstallMetalLbResources(kubedclient *dynamic.DynamicClient, kubetclient *kubernetes.Clientset, metallbiprange string, metallbreleasenamespace string, logger log.Logger) error {

	res1 := schema.GroupVersionResource{Group: "metallb.io", Version: "v1beta1", Resource: "ipaddresspools"}
	res2 := schema.GroupVersionResource{Group: "metallb.io", Version: "v1beta1", Resource: "l2advertisements"}

	var (
		poolObjNew = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "metallb.io/v1beta1",
				"kind":       "IPAddressPool",
				"metadata": map[string]interface{}{
					"namespace":    metallbreleasenamespace,
					"generateName": "metallb-ipaddpool-",
				},
				"spec": map[string]interface{}{
					"addresses": [...]string{metallbiprange},
				},
			},
		}

		l2ObjNew1 = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "metallb.io/v1beta1",
				"kind":       "L2Advertisement",
				"metadata": map[string]interface{}{
					"namespace":    metallbreleasenamespace,
					"generateName": "metallb-l2ad-",
				},
			},
		}
	)

	// Check MetalLB pods are up before creating IPpoolList and L2Ad

	listOptions := metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=metallb",
	}

	logger.V(0).Infof("Checking MetalLB pods are up completely in namespace - %q, may take up to 120 seconds", metallbreleasenamespace)

	for checkCount := 0; checkCount < 12; checkCount++ {
		pods, _ := kubetclient.CoreV1().Pods(metallbreleasenamespace).List(context.Background(), listOptions)

		time.Sleep(time.Second * 10)

		podUpCount := 0

		for _, pod := range pods.Items {
			podready := podutil.IsPodReady(&pod)
			if podready {
				podUpCount++
			}
		}

		if podUpCount == 2 {
			err := error(nil)
			check("Check MetalLB pods are fully up - ", err, logger)
			break
		}

		if checkCount == 11 {
			err := errors.New("MetalLB pods are not up after 120 seconds")
			check("Check MetalLB pods are fully up - ", err, logger)
		}

	}

	logger.V(0).Infof("Checking MetalLB CRDs ar available")

	_, err := kubedclient.
		Resource((res1)).
		Namespace(metallbreleasenamespace).
		List(context.Background(), metav1.ListOptions{})

	check("Check MetalLB IPaddressPool CRD available - ", err, logger)

	_, err = kubedclient.
		Resource((res2)).
		Namespace(metallbreleasenamespace).
		List(context.Background(), metav1.ListOptions{})

	check("Check MetalLB L2 Advertisement CRD available - ", err, logger)

	// Create
	created1, err := kubedclient.
		Resource((res1)).
		Namespace(metallbreleasenamespace).
		Create(context.Background(), poolObjNew, metav1.CreateOptions{})
	check("Install MetalLB IPaddressPool - ", err, logger)

	created2, err := kubedclient.
		Resource((res2)).
		Namespace(metallbreleasenamespace).
		Create(context.Background(), l2ObjNew1, metav1.CreateOptions{})
	check("Install MetalLB L2 Adverrtisement - ", err, logger)

	// Read
	read1, err := kubedclient.
		Resource(res1).
		Namespace(metallbreleasenamespace).
		Get(
			context.Background(),
			created1.GetName(),
			metav1.GetOptions{},
		)
	check("Read MetalLB IPaddressPool - ", err, logger)

	data, _, _ := unstructured.NestedMap(read1.Object, "spec")
	pool := data["addresses"].([]interface{})
	if !reflect.DeepEqual(metallbiprange, pool[0]) {
		err = errors.New("read ipaaddresspool has unexpected data")
		check("Checking the installed IPAddressPool - ", err, logger)

	}

	read2, err := kubedclient.
		Resource(res2).
		Namespace(metallbreleasenamespace).
		Get(
			context.Background(),
			created2.GetName(),
			metav1.GetOptions{},
		)

	check("Read MetalLB L2 advertisement - ", err, logger)

	if read2.GetName() != created2.GetName() {
		panic(err.Error())
	}
	return (err)

}

// Thanks to https://gist.github.com/pytimer/0ad436972a073bb37b8b6b8b474520fc
func ApplyYAMLfile(kubedclient *dynamic.DynamicClient, kubetclient *kubernetes.Clientset, yamlfile string, namespace string, logger log.Logger) error {

	yml, err := os.ReadFile(yamlfile)
	check("Read the YAML file - ", err, logger)

	logger.V(0).Infof(string(yml))

	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(yml), 100)

	for {
		var rawObj runtime.RawExtension
		if err = decoder.Decode(&rawObj); err != nil {
			break
		}

		obj, gvk, _ := serialize.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)

		unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		fmt.Println(unstructuredMap["kind"], unstructuredMap["metadata"])
		check("Convert to Unstructured Map - ", err, logger)

		unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}

		gr, err := restmapper.GetAPIGroupResources(kubetclient.Discovery())
		check("Get API group from each object - ", err, logger)

		mapper := restmapper.NewDiscoveryRESTMapper(gr)
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		check("Map GVK in each object - ", err, logger)

		var dri dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			if unstructuredObj.GetNamespace() == "" {
				unstructuredObj.SetNamespace(namespace)
			}
			dri = kubedclient.Resource(mapping.Resource).Namespace(unstructuredObj.GetNamespace())
		} else {
			dri = kubedclient.Resource(mapping.Resource)
		}

		_, err = dri.Create(context.Background(), unstructuredObj, metav1.CreateOptions{})
		check("Create object - ", err, logger)
	}
	if err != io.EOF {
		return (err)
	} else {
		err := error(nil)
		return (err)

	}

}
