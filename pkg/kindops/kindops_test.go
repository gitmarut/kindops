package kindops

import (
	"testing"
	"time"

	kindCmd "sigs.k8s.io/kind/pkg/cmd"
)

func TestCreate(t *testing.T) {

	kindLogger := kindCmd.NewLogger()

	t.Run("Create a test kind cluster", func(t *testing.T) {

		err := CreateCluster("./../../config/kind_test_cluster_config.yaml", kindLogger)

		check("Create a test kind cluster - ", err, kindLogger)

		//time.Sleep(time.Second * 30)

	})

	t.Run("Test a wordpress app can be installed and test traffic to it thru a LB", func(t *testing.T) {

		var c flagpole
		c.getConf("./../../config/kind_test_cluster_config.yaml", kindLogger)

		dclient, tclient, err := GetKubeClient(c.Kubeconfig, kindLogger)
		check("Get Kind Cluster's Dynamic & Typed Clients - ", err, kindLogger)

		// install wordpress sample apps
		err = ApplyYAMLfile(dclient, tclient, c.Wordpresspath, "default", "create", kindLogger)
		check("Install a sample Wordpress App - ", err, kindLogger)

		// check wordpress pods are up and running
		wplabel := "app=wordpress"
		err = CheckPodsUp(tclient, wplabel, "default", kindLogger)
		check("Checking Wordpress pods are up completely - ", err, kindLogger)

		// get wordpress service IP
		svcip, err := GetSvcIp(tclient, wplabel, "wordpress", "default", kindLogger)
		check("Getting Wordpress svcIP - ", err, kindLogger)

		// check wordpress service responds on LB address
		urladdress := "http://" + svcip + "/wp-admin/install.php"
		err = sendHttpReq(urladdress, kindLogger)
		check("Check traffic can be sent/received on a LB address in Kind cluster -", err, kindLogger)

		// delete sample wordpress app
		time.Sleep(time.Second * 10)
		err = ApplyYAMLfile(dclient, tclient, c.Wordpresspath, "default", "delete", kindLogger)
		check("Delete the sample Wordpress App - ", err, kindLogger)

	})

	/*

		t.Run("Delete a test kind cluster", func(t *testing.T) {

			kindLogger := kindCmd.NewLogger()

			err := DeleteCluster("./../../kind_test_cluster_config.yaml", kindLogger)
			check("Delete a test kind cluster - ", err, kindLogger)


		})

	*/

}
