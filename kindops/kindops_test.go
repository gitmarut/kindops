package kindops

import (
	"fmt"
	"time"

	"testing"

	kindCmd "sigs.k8s.io/kind/pkg/cmd"
)

func TestCreate(t *testing.T) {

	/*
		checkErr := func(t testing.TB, err error) {

			t.Helper()

			if err != nil {
				t.Error(err.Error())
			}
		}
	*/

	t.Run("Create a basic cluster", func(t *testing.T) {

		kindLogger := kindCmd.NewLogger()

		err := CreateCluster("./kind_test_cluster_config.yaml", kindLogger)

		//checkErr(t, err)
		fmt.Println(err)

		time.Sleep(time.Second * 30)

	})

	/*

		t.Run("Delete a basic cluster", func(t *testing.T) {

			kindLogger := kindCmd.NewLogger()

			err := DeleteCluster("/home/gitmarut/go/src/gopackages-pvt/kindops/kind_test_cluster_config.yaml", kindLogger)

			//checkErr(t, err)
			fmt.Println(err)

		})

	*/

}
