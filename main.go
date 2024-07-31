package main

import (
	"context"
	"time"

	kindops "github.com/gitmarut/kindops/pkg/kindops"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kindCmd "sigs.k8s.io/kind/pkg/cmd"
	"sigs.k8s.io/kind/pkg/log"
)

func main() {

	kindLogger := kindCmd.NewLogger()

	err := kindops.CreateCluster("./config/kind_test_cluster_config.yaml", kindLogger)
	check("Create a test kind cluster - ", err, kindLogger)

	// kind_test_cluster_config.yaml under config directory mentions kubeConfig
	// where previous step will write the kubeconfig file.
	dclient, tclient, err := kindops.GetKubeClient("./config/cluster108.yaml", kindLogger)
	check("Get Kind Cluster's Dynamic & Typed Clients - ", err, kindLogger)

	// install wordpress sample apps - deploy yaml at ./config/wp-all.yaml
	err = kindops.ApplyYAMLfile(dclient, tclient, "./config/wp-all.yaml", "default", "create", kindLogger)
	check("Install a sample Wordpress App - ", err, kindLogger)

	// check wordpress pods are up and running
	wplabel := "app=wordpress"
	err = kindops.CheckPodsUp(tclient, wplabel, "default", kindLogger)
	check("Checking Wordpress pods are up completely - ", err, kindLogger)

	// get wordpress service IP
	svcip, err := kindops.GetSvcIp(tclient, wplabel, "wordpress", "default", kindLogger)
	check("Getting Wordpress svcIP - ", err, kindLogger)

	// remove metalLb exclude label for the nodes - https://github.com/gitmarut/kindops/issues/1
	nodes, _ := tclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})

	var nodesList []string
	for _, node := range nodes.Items {
		nodesList = append(nodesList, node.Name)
	}

	var labels []string
	//labels = append(labels, "replace", "/metadata/labels/node.kubernetes.io~1exclude-from-external-load-balancers", "false")
	labels = append(labels, "remove", "/metadata/labels/node.kubernetes.io~1exclude-from-external-load-balancers", "false")

	kindops.LabelNode(tclient, nodesList, labels, kindLogger)

	// check wordpress service responds on LB address
	urladdress := "http://" + svcip + "/wp-admin/install.php"
	err = kindops.SendHttpReq(urladdress, kindLogger)
	check("Check traffic can be sent/received on a LB address in Kind cluster -", err, kindLogger)

	// delete sample wordpress app - - deploy yaml at ./config/wp-all.yaml
	time.Sleep(time.Second * 10)
	err = kindops.ApplyYAMLfile(dclient, tclient, "./config/wp-all.yaml", "default", "delete", kindLogger)
	check("Delete the sample Wordpress App - ", err, kindLogger)

}

func check(condition string, e error, logger log.Logger) {
	if e != nil {
		logger.Error(condition + e.Error())
		panic(e)
	} else {
		logger.V(0).Infof("Success - %q ...\n", condition)
	}
}
