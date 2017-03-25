package marathon

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func waitForDeployment(id string) error {
	fmt.Println("Waiting for deploy....")
	go func() {
		defer fmt.Println("")
		for {
			for _, r := range `-\|/` {
				fmt.Printf("\r%c", r)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	url := fmt.Sprintf("%s/service/marathon/v2/deployments", viper.GetString("marathon_host"))

	for {
		time.Sleep(5 * time.Second)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		b, _ := ioutil.ReadAll(resp.Body)
		var d []Deployment
		if err := json.Unmarshal(b, &d); err != nil {
			return err
		}
		if len(d) == 0 {
			fmt.Printf("\rDONE\n")
			break
		}
	}

	return nil
}

// deploymentURL returns a Marathon API endpoint to deploy/scale
func deploymentURL() string {
	return fmt.Sprintf("%s/service/marathon/v2/groups/", viper.GetString("marathon_host"))
}

func marathonJSONPath(marathonPath, f, env string) string {
	return path.Join(marathonPath, strings.Replace(f, "{{env}}", env, -1))
}

func marathonJSON(body []byte, app, tag string) string {
	re := regexp.MustCompile(fmt.Sprintf("(%s/%s):.*(\",?)", viper.GetString("docker_repo_prefix"), app))
	return re.ReplaceAllString(string(body), fmt.Sprintf("$1:%s$2", tag))
}
