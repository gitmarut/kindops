package kindops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
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
	types "k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/log"
)

/* There are two config files needed in dep directory -
   kind_test_cluster_config.yaml & undefined. First one get converted to
   struct flagpole. Undefined config file controls more kind parameters like
   number of kubernetes nodes, their resources config etc & this is defined as
   flagpole.Config which is empty now. This might need tweaking later for
   different options
   "https://github.com/kubernetes-sigs/kind/blob/main/pkg/apis/config/v1alpha4/default.go"
*/

// flagpole is the struct to keep all the options for creating kind cluster
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
	Wordpresspath           string        `yaml:"wordpressPath"`
}

// getConf will read yaml config file and return flagPole
func (c *flagpole) getConf(configfile string, logger log.Logger) *flagpole {

	dat, err := os.ReadFile(configfile)
	check("Read Config File - ", err, logger)

	err = yaml.Unmarshal(dat, c)
	check("Unmarshal Config File - ", err, logger)

	return c
}

// check is an internal function to log an error
func check(condition string, e error, logger log.Logger) {
	if e != nil {
		logger.Error(condition + e.Error())
		panic(e)
	} else {
		logger.V(0).Infof("Success - %q ...\n", condition)
	}
}

// InstallHelmChart is a quick and dirty Helm install. Need to improve
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

// GetKubeClient will get typed and dynamic k8s client for client-go
func GetKubeClient(kubeconfigPath string, logger log.Logger) (*dynamic.DynamicClient, *kubernetes.Clientset, error) {

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	check("Build Kube Config - ", err, logger)

	dClient, err := dynamic.NewForConfig(config)
	check("Get Dynamic Client - ", err, logger)

	tClient, err := kubernetes.NewForConfig(config)
	check("Get Typed Client - ", err, logger)

	return dClient, tClient, err

}

// InstallMetalLbResources will install MetalLB CRs. Need to move metalLB
// related stuff to another package
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

	// check MetalLB pods are up before creating IPpoolList and L2Ad

	metalLbLabel := "app.kubernetes.io/name=metallb"

	err := CheckPodsUp(kubetclient, metalLbLabel, "default", logger)

	check("Checking MetalLB pods are up completely in namespace", err, logger)

	logger.V(0).Infof("Checking MetalLB CRDs ar available")

	_, err = kubedclient.
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

	// check MetalLB CRs are installed.
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
		err := errors.New("read metalLB l2ad has unexpected data")
		check("Checking the installed L2 advertisement - ", err, logger)

	}
	return (err)

}

// ApplyYAMLfile installs or deletes k8s objects from a yaml
// This can be expanded to other k8s ops as well
// Thanks to https://gist.github.com/pytimer/0ad436972a073bb37b8b6b8b474520fc
func ApplyYAMLfile(kubedclient *dynamic.DynamicClient, kubetclient *kubernetes.Clientset, yamlfile string, namespace string, optype string, logger log.Logger) error {

	yml, err := os.ReadFile(yamlfile)
	check("Read the YAML file - ", err, logger)

	//logger.V(0).Infof(string(yml))

	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(yml), 100)

	for {
		var rawObj runtime.RawExtension
		if err = decoder.Decode(&rawObj); err != nil {
			break
		}

		obj, gvk, _ := serialize.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)

		unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		fmt.Println(unstructuredMap["kind"], (unstructuredMap["metadata"].(map[string]interface{})["name"]))
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

		if optype == "create" {
			_, err = dri.Create(context.Background(), unstructuredObj, metav1.CreateOptions{})
			check("Create object - ", err, logger)
		}
		if optype == "delete" {
			err = dri.Delete(context.Background(), unstructuredMap["metadata"].(map[string]interface{})["name"].(string), metav1.DeleteOptions{})
			check("Delete object - ", err, logger)
		}
	}
	if err != io.EOF {
		return (err)
	} else {
		err := error(nil)
		return (err)

	}

}

// CheckPodsUp checks the pods with given labels are up or not.
func CheckPodsUp(kubetclient *kubernetes.Clientset, labelselector string, namespace string, logger log.Logger) error {
	listOptions := metav1.ListOptions{
		LabelSelector: labelselector,
	}

	logger.V(0).Infof("Checking %q pods are up completely in namespace - %q, will wait for 150 seconds", labelselector, namespace)

	err := error(nil)

	for checkCount := 0; checkCount < 15; checkCount++ {
		time.Sleep(time.Second * 10)
		pods, err := kubetclient.CoreV1().Pods(namespace).List(context.Background(), listOptions)

		if err != nil {
			logger.V(0).Infof(err.Error() + " ,sleep 10s again")
		} else {
			podUpCount := 0

			for _, pod := range pods.Items {
				podready := podutil.IsPodReady(&pod)
				if podready {
					podUpCount++
				}
			}

			podUpNeeded := len(pods.Items)

			if podUpCount == podUpNeeded {
				err := error(nil)
				check("Check pods are fully up - ", err, logger)
				break
			}

			if checkCount == 14 {
				err = errors.New(labelselector + " pods are not up after 150 seconds in namespace " + namespace)
				check("Check Pods are fully up - ", err, logger)
			}
		}
	}

	return (err)

}

// GetSvcIp returns the LB IP for service with given label selector
func GetSvcIp(kubetclient *kubernetes.Clientset, labelselector string, svcname string, namespace string, logger log.Logger) (string, error) {
	listOptions := metav1.ListOptions{
		LabelSelector: labelselector,
	}

	svcs, err := kubetclient.CoreV1().Services(namespace).List(context.Background(), listOptions)
	check("Check service objects can be listed - ", err, logger)

	lbIP := ""
	err = error(nil)

	for _, svc := range svcs.Items {
		if svc.ObjectMeta.Name == svcname {
			if svc.Spec.Type == "LoadBalancer" {
				lbIP = svc.Status.LoadBalancer.Ingress[0].IP
				//fmt.Println("var1 = ", reflect.TypeOf(svc.Status.LoadBalancer.Ingress[0].IP))
				break
			} else {
				lbIP = "notLB"
				err = errors.New(labelselector + " svc " + svcname + " is not LoadBalancer in namespace " + namespace)
			}
		}

	}

	if lbIP == "" {
		err = errors.New(labelselector + " svc " + svcname + " is not found in namespace " + namespace)
	}

	return lbIP, err

}

// sendHttpReq sends http requests to given URL, in this case WP
func SendHttpReq(address string, logger log.Logger) error {

	for x := 0; x < 10; x++ {
		time.Sleep(time.Second * 3)
		fmt.Println(address)
		resp, err := http.Get(address)
		fmt.Println(resp.StatusCode)

		if err == nil {
			if resp.StatusCode == 200 {
				body, _ := io.ReadAll(resp.Body)
				stringbody := string(body)
				if strings.Contains(stringbody, "Cebuano") {
					err := error(nil)
					logger.V(0).Infof("Wordpress website respeonds fine.")
					return (err)
				}
			}
		}

	}

	err := errors.New("wordpress website is not responding on loadbalance service")
	return (err)
}

// CreateCluster will create a kind cluster, install MetalLB using helm,
// install a sample wordpress app, check traffic to wordpress and delete
// wordpress.
func CreateCluster(configfile string, logger log.Logger) error {

	var c flagpole
	c.getConf(configfile, logger)

	//fmt.Printf("%+v\n", c)

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
	check("Kind Cluster Creation - ", err, logger)

	// install metalLB CRDs and deployments using Helm
	err = InstallHelmChart(c.Kubeconfig, c.Metallbchartpath, c.Metallbreleasename, c.Metallbreleasenamespace, logger)
	check("Installation of MetalLB Helm Chart - ", err, logger)

	// get static and dynamic client-go clients for further use
	dclient, tclient, err := GetKubeClient(c.Kubeconfig, logger)
	check("Get Kind Cluster's Dynamic & Typed Clients - ", err, logger)

	// install metalLb custom resources
	err = InstallMetalLbResources(dclient, tclient, c.Metallbiprange, c.Metallbreleasenamespace, logger)
	check("Install MetalLB CRs - ", err, logger)

	if err == nil {
		logger.V(0).Infof("Cluster with all dependencies completed - %q", c.Name)
	}
	return (err)

}

// DeleteCluster will delete the kind cluster mentioned in configfile
func DeleteCluster(configfile string, logger log.Logger) error {

	var c flagpole
	c.getConf(configfile, logger)

	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	err := provider.Delete(c.Name, c.Kubeconfig)

	check("Kind Cluster Deletion -  ", err, logger)

	if err == nil {
		logger.V(0).Infof("Cluster deleted - %q", c.Name)
	}

	return (err)

}

// Need to convert this as a generic function which can label / unlabel any object.

func LabelNode(kubetclient *kubernetes.Clientset, nodeList []string, labelselector []string, logger log.Logger) error {

	type patchSpec struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}

	patch1 := make([]patchSpec, 1)
	patch1[0].Op = labelselector[0]
	patch1[0].Path = labelselector[1]
	patch1[0].Value = labelselector[2]

	patchBytes, err := json.Marshal(patch1)

	check("Convert labels patch data into Json -  ", err, logger)

	for _, node := range nodeList {

		_, err := kubetclient.CoreV1().Nodes().Patch(context.Background(), node, types.JSONPatchType, patchBytes, metav1.PatchOptions{})

		msg := "Able to patch labels for node " + node + " "
		check(msg, err, logger)

	}

	return (err)

}
