package chronos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/betterdoctor/duncan/consul"
	"github.com/betterdoctor/duncan/mesos"
	"github.com/spf13/viper"
)

// SlaveTasks represents Mesos slave completed tasks
type SlaveTasks struct {
	Frameworks []*Framework `json:"completed_frameworks"`
}

// Framework represents a completed framework on a Mesos slave
type Framework struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Executors []*Executor `json:"completed_executors"`
}

// Executor represents a completed executor on a Mesos slave
type Executor struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}

// TaskVars represents a one-off Chronos task
type TaskVars struct {
	App, Env, Tag, Command, TaskName string
}

// Deploy deploys Chronos tasks for a given app, env and tag
func Deploy(app, env, tag string) error {
	chronosPath := viper.GetString("chronos_json_path")
	deployment := viper.GetStringMap("apps")[app]
	if deployment == nil {
		return fmt.Errorf("invalid YAML config for %s\n", app)
	}
	for k, v := range deployment.(map[string]interface{}) {
		if k == "chronos" {
			for _, x := range v.([]interface{}) {
				cj := path.Join(chronosPath, strings.Replace(x.(string), "{{env}}", env, -1))
				body, err := ioutil.ReadFile(cj)
				if err != nil {
					return fmt.Errorf("Chronos JSON does not exist %s: %s\n", cj, err)
				}
				re := regexp.MustCompile(fmt.Sprintf("(quay.io/betterdoctor/%s):.*(\",?)", app))
				chronosJSON := re.ReplaceAllString(string(body), fmt.Sprintf("$1:%s$2", tag))

				url := fmt.Sprintf("%s/service/chronos/scheduler/iso8601", viper.GetString("chronos_host"))
				client := &http.Client{}
				req, _ := http.NewRequest("POST", url, strings.NewReader(chronosJSON))
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				if resp.StatusCode != http.StatusNoContent {
					return fmt.Errorf("failed to deploy chronos tasks for %s: %s", app, resp.Status)
				}
				fmt.Printf("deployed Chronos task %s\n", path.Base(cj))
			}
		}
	}
	return nil
}

// RunCommand spins up a Chronos task to run the given command and exits
func RunCommand(app, env, cmd string) error {
	tag, err := fetchCurrentTag(app, env)
	if err != nil {
		return err
	}

	task := &TaskVars{
		App:      app,
		Env:      env,
		Tag:      tag,
		Command:  cmd,
		TaskName: strings.Join([]string{app, env, sanitizedCommand(cmd)}, "-"),
	}
	url := fmt.Sprintf("%s/service/chronos/v1/scheduler/iso8601", viper.GetString("chronos_host"))
	if err := launchChronosOneOffCommand(url, task); err != nil {
		return err
	}

	return nil
}

func fetchCurrentTag(app, env string) (string, error) {
	resp, err := http.Get(consul.CurrentDeploymentTagURL(app, env))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("ACL does not allow you to run comand for %s-%s", app, env)
	}

	tag, err := ioutil.ReadAll(resp.Body)
	return string(tag), err
}

// sanitizedCommand strips out non-alphabet chars from commmand string
// used to generate a valid Chronos task name which cannot contain special chars
func sanitizedCommand(cmd string) string {
	var out []string
	re := regexp.MustCompile("[a-z]*")
	p := strings.Split(cmd, " ")
	for _, c := range p {
		m := re.FindAllString(strings.TrimSpace(c), -1)
		out = append(out, strings.Join(m, "-"))
	}

	return strings.Join(out, "-")
}

func renderChronosTaskJSON(task *TaskVars) (string, error) {
	t, err := template.New("chronos_task").Parse(taskTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}
	j := new(bytes.Buffer)
	if err := t.Execute(j, task); err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}
	return j.String(), nil
}

func launchChronosOneOffCommand(url string, task *TaskVars) error {
	tasks, err := scheduledTasks(task.TaskName)
	if err != nil {
		return err
	}

	cj, err := renderChronosTaskJSON(task)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", strings.NewReader(cj))
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("failed to launch command: %s", body)
	}
	fmt.Printf("executing '%s' in instance of %s:%s (%s)\n", task.Command, task.App, task.Tag, task.Env)
	if err = handleTask(task.TaskName, len(tasks)); err != nil {
		return err
	}

	return nil
}

func handleTask(name string, count int) error {
	fmt.Println("scheduling task")
	for {
		time.Sleep(500 * time.Millisecond)
		tasks, err := scheduledTasks(name)
		if err != nil {
			return err
		}
		if len(tasks) == count {
			fmt.Printf(".")
			continue
		}
		if len(tasks) == count+1 {
			task := tasks[0]

			fmt.Printf("\rtask state: %s           \n", task.State)
			switch task.State {
			case mesos.TaskRunning:
				openLogPage(task)
				continue
			case mesos.TaskStaging:
				continue
			case mesos.TaskFinished:
				openLogPage(task)
				dur, err := task.Duration()
				if err != nil {
					return err
				}
				fmt.Printf("task finished: %.02f seconds\n", dur)
				printLogs(task)
				return nil
			case mesos.TaskFailed:
				openLogPage(task)
				printLogs(task)
				dur, err := task.Duration()
				if err != nil {
					return err
				}

				return fmt.Errorf("task failed: %.02f seconds\n", dur)
			case mesos.TaskKilled:
				dur, err := task.Duration()
				if err != nil {
					return err
				}
				return fmt.Errorf("task killed: %.02f seconds\n", dur)
			default:
				return fmt.Errorf("task state unhandled: %s", task.State)
			}
		}
	}
}

func printLogs(t *mesos.Task) {
	dir, err := t.LogDirectory()
	if err != nil {
		fmt.Printf("cannot fetch logs: %s\n", err)
		os.Exit(-1)
	}

	var out string
	if t.State == mesos.TaskFinished {
		out = "stdout"
	} else {
		out = "stderr"
	}

	ip, err := t.SlaveIP()
	if err != nil {
		fmt.Println(err)
		return
	}
	url := fmt.Sprintf("http://%s:5051/files/read?path=%s/%s&offset=0", ip, dir, out)
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Printf("could not fetch logs: %s\n", resp.Status)
	}
	l := &mesos.Logs{}
	if err := json.NewDecoder(resp.Body).Decode(l); err != nil {
		fmt.Println(err)
	}
	lines := strings.Split(l.Data, "\n")
	for _, l := range lines {
		re := regexp.MustCompile("cpp:")
		if !re.MatchString(l) {
			fmt.Printf("\r%s\n", l)
		}
	}
}

func openLogPage(t *mesos.Task) {
	dir, err := t.LogDirectory()
	if err != nil {
		fmt.Printf("cannot fetch logs: %s\n", err)
		os.Exit(-1)
	}
	p := strings.Split(dir, "/var/lib/mesos/slave/slaves/")
	slaveID := strings.Split(p[1], "/")[0]
	url := fmt.Sprintf("%s/mesos/#/agents/%s/browse?path=%s", viper.GetString("marathon_host"), slaveID, dir)
	cmd := exec.Command("open", url)
	if err := cmd.Start(); err != nil {
		fmt.Println("could not open link to task logs")
	}
	cmd.Wait()
}

// scheduledTasks fetches tasks from Mesos API and returns list of tasks that
// match the given task name
func scheduledTasks(name string) ([]*mesos.Task, error) {
	var offset int
	tasks := &mesos.Tasks{}
	for {
		url := fmt.Sprintf("%s/mesos/tasks?offset=%d", viper.GetString("marathon_host"), offset)
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		t := &mesos.Tasks{}
		if err := json.NewDecoder(resp.Body).Decode(t); err != nil {
			return nil, err
		}
		for _, tt := range t.Tasks {
			tasks.Tasks = append(tasks.Tasks, tt)
		}
		if len(t.Tasks) >= 100 {
			offset += 100
			continue
		}
		break
	}

	rt, err := tasks.TasksFor(name)
	if err != nil {
		return nil, err
	}

	return rt, nil
}
