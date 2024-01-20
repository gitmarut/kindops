package kindops

import (
	"testing"

	kindCmd "sigs.k8s.io/kind/pkg/cmd"
)

func TestCreate(t *testing.T) {

	t.Run("Create a test kind cluster", func(t *testing.T) {

		kindLogger := kindCmd.NewLogger()

		err := CreateCluster("./../config/kind_test_cluster_config.yaml", kindLogger)

		check("Create a test kind cluster - ", err, kindLogger)

		//time.Sleep(time.Second * 30)

	})

	/*

		t.Run("Delete a test kind cluster", func(t *testing.T) {

			kindLogger := kindCmd.NewLogger()

			err := DeleteCluster("/home/gitmarut/go/src/gopackages-pvt/kindops/kind_test_cluster_config.yaml", kindLogger)
			check("Delete a test kind cluster - ", err, kindLogger)


		})

	*/

}
