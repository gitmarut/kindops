package kindops

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
	"sigs.k8s.io/kind/pkg/cluster"

	"sigs.k8s.io/kind/pkg/log"
)

/* There are two config files used. One is default now. Which is mentioned as flagpole.Config
   This might need tweaking later for different options
   "https://github.com/kubernetes-sigs/kind/blob/main/pkg/apis/config/v1alpha4/default.go"

   The other one is read as flagpole itself.
*/

type flagpole struct {
	Name       string        `yaml:"name"`
	Config     string        `yaml:"config"`
	ImageName  string        `yaml:"imageName"`
	Retain     bool          `yaml:"retain"`
	Wait       time.Duration `yaml:"waitTime"`
	Kubeconfig string        `yaml:"kubeConfig"`
}

func (c *flagpole) getConf(configfile string, logger log.Logger) *flagpole {

	dat, err := os.ReadFile(configfile)
	check(err, logger)

	err = yaml.Unmarshal(dat, c)
	check(err, logger)

	if err != nil {
		logger.Errorf("Unmarshal: %v", err)
	}

	return c
}

func check(e error, logger log.Logger) {
	if e != nil {
		logger.Error(e.Error())
		panic(e)
	}
}

func CreateCluster(configfile string, logger log.Logger) error {

	var c flagpole
	c.getConf(configfile, logger)

	fmt.Printf("%+v\n", c)
	fmt.Printf("%+v\n", c.Name)

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
	check(err, logger)

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

	check(err, logger)

	return (err)

}
