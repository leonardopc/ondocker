package main

import (
	"context"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/robfig/cron/v3"
)

type containerConfigs struct {
	configList map[string]*containerConfig
	cron       bool
}

type containerConfig struct {
	helperContainers  []string
	hosts             []string
	backend           string
	maxRetries        int
	currentRetries    int
	inactivityTimeout int64
	lastRequestTime   time.Time
	sleepStartTime    string
	sleepStopTime     string
}

type pageData struct {
	ContainerName  string
	Timeout        int64
	CurrentRetries int
	MaxRetries     int
}

func main() {
	configs := parseConfigFile()
	//setup cron job to look for idle containers every minute if inactivity timeout for any container > 0
	if configs.cron {
		c := cron.New()
		c.AddFunc("* * * * *", func() { configs.stopIdleContainers() })
		c.Start()
	}
	http.HandleFunc("/", handler(configs))
	log.Fatal(http.ListenAndServe(":10000", nil))
}

//read config.json, initialize and return containerConfigs
func parseConfigFile() containerConfigs {
	type jsonConfig struct {
		ContainerName     string
		HelperContainers  []string
		Hosts             []string
		Backend           string
		MaxRetries        int
		InactivityTimeout int64
		SleepStartTime    string
		SleepStopTime     string
	}
	var defaultMaxRetries int = 5
	var defaultInactivityTimeout int64 = 10
	//initialize configuration struct
	configs := containerConfigs{
		configList: make(map[string]*containerConfig),
	}
	var jsonConfigs []jsonConfig
	configFile, err := ioutil.ReadFile("/config/config.json")
	check(err)
	err = json.Unmarshal([]byte(configFile), &jsonConfigs) //parse config.json into an array
	if err != nil {
		panic(err)
	}
	//convert array into map and add lastRequesTime and currentRetries
	for i := range jsonConfigs {
		if jsonConfigs[i].ContainerName == "" {
			panic("Invalid config.json file, missing parameter 'containerName'")
		}
		if !doesContainerExist(jsonConfigs[i].ContainerName) {
			var msg = "Invalid config.json file, no container with name " + jsonConfigs[i].ContainerName + " found"
			panic(msg)
		}
		for j := range jsonConfigs[i].HelperContainers {
			if !doesContainerExist(jsonConfigs[i].HelperContainers[j]) {
				var msg = "Invalid config.json file, no helper container with name " + jsonConfigs[i].HelperContainers[j] + " found"
				panic(msg)
			}
		}
		for j := range jsonConfigs[i].HelperContainers {
			if jsonConfigs[i].Hosts[j] == "" {
				panic("Invalid config.json file, parameter 'host' cannot be empty")
			}
		}
		if jsonConfigs[i].Backend == "" {
			panic("Invalid config.json file, missing parameter 'backend'")
		}
		if jsonConfigs[i].MaxRetries < 1 {
			jsonConfigs[i].MaxRetries = defaultMaxRetries
			log.Printf("Missing or invalid parameter 'maxRetries' for configuration set #%v, using default value of %v", i, defaultMaxRetries)
		}
		if jsonConfigs[i].InactivityTimeout < 0 {
			jsonConfigs[i].InactivityTimeout = defaultInactivityTimeout
			log.Printf("Missing or invalid parameter 'inactivityTimeout' for configuration set #%v, using default value of %v", i, defaultInactivityTimeout)
		}
		containerConfig := containerConfig{
			helperContainers:  jsonConfigs[i].HelperContainers,
			hosts:             jsonConfigs[i].Hosts,
			backend:           jsonConfigs[i].Backend,
			maxRetries:        jsonConfigs[i].MaxRetries,
			currentRetries:    jsonConfigs[i].MaxRetries,
			inactivityTimeout: jsonConfigs[i].InactivityTimeout,
			lastRequestTime:   time.Now(),
			sleepStartTime:    jsonConfigs[i].SleepStartTime,
			sleepStopTime:     jsonConfigs[i].SleepStopTime,
		}
		if jsonConfigs[i].InactivityTimeout > 0 {
			configs.cron = true
		}
		configs.configList[jsonConfigs[i].ContainerName] = &containerConfig
	}

	log.Printf("config.json successfully parsed")
	return configs
}

func (configs *containerConfigs) isSleepTime(containerName string) bool {
	//fix this
	layout := "15:04"
	configSleepStartTime, err := time.Parse(layout, configs.configList[containerName].sleepStartTime)
	check(err)
	configSleepStopTime, err := time.Parse(layout, configs.configList[containerName].sleepStopTime)
	check(err)
	now := time.Now()
	SleepStartTime := time.Date(now.Year(), now.Month(), now.Day(), configSleepStartTime.Hour(), configSleepStartTime.Minute(), 0, 0, now.Location())
	SleepStopTime := time.Date(now.Year(), now.Month(), now.Day(), configSleepStopTime.Hour(), configSleepStopTime.Minute(), 0, 0, now.Location())
	return now.After(SleepStartTime) || now.Before(SleepStopTime)
}

//lookup if any container has already reached the intactivity timeout and stop those which have
func (configs *containerConfigs) stopIdleContainers() {
	for containerName, config := range configs.configList {
		if configs.isContainerRunning(containerName) {
			if configs.isSleepTime(containerName) { //if it is the sleep time, shutdown container without checking the inactivity period
				configs.stopContainer(containerName)
				log.Printf("Stopping %s because sleep time has started", containerName)
				return
			}
			if time.Now().Unix() >= config.lastRequestTime.Unix()+config.inactivityTimeout*60 {
				log.Printf("Stopping %s due to inactivity", containerName)
				configs.stopContainer(containerName)
			}
		}
	}
}

//make a get request to the backend to see if the container is healthy and running, otherwise decrease currentRetries
func (configs *containerConfigs) isBackendAlive(request string, containerName string) bool {
	resp, err := http.Get(request)

	if err != nil {
		if time.Now().Unix()-configs.configList[containerName].lastRequestTime.Unix() >= 5 { //only reduce the currenRetries if the previous request happened more than 5s ago
			configs.configList[containerName].currentRetries--
			configs.configList[containerName].lastRequestTime = time.Now()
		}
		return false
	}

	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		configs.configList[containerName].currentRetries = configs.configList[containerName].maxRetries //reset currentRetries after a successful get response
		configs.configList[containerName].lastRequestTime = time.Now()
		return true
	} else {
		configs.configList[containerName].currentRetries--
		configs.configList[containerName].lastRequestTime = time.Now()
		return false
	}
}

func handler(configs containerConfigs) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		//if container which url matches is running, redirect traffic. If not running, start it and show loading screen
		for containerName, config := range configs.configList {
			for j := range config.hosts {
				u, err := url.Parse(config.hosts[j])
				check(err)
				if r.Host == u.Host {
					log.Printf("Host matches: Request: %s config: %s", r.Host, config.hosts[j])
					backend := config.backend
					url, err := url.Parse(backend)
					check(err)
					p := httputil.NewSingleHostReverseProxy(url)
					if err != nil {
						panic(err)
					}
					if !configs.isContainerRunning(containerName) {
						configs.startContainer(containerName)
					}
					if !configs.isBackendAlive(backend, containerName) {
						pageData := pageData{
							ContainerName:  containerName,
							Timeout:        configs.configList[containerName].inactivityTimeout,
							CurrentRetries: configs.configList[containerName].currentRetries,
							MaxRetries:     configs.configList[containerName].maxRetries,
						}
						if config.currentRetries < 0 {
							//backend timed out, show error page
							t, err := template.ParseFiles("/config/static/errorPage.html")
							check(err)
							t.Execute(w, pageData)
							return
						}
						//show loading page until container is up, the pages auto-refreshes every 5 seconds
						t, err := template.ParseFiles("/config/static/loadingPage.html")
						check(err)
						t.Execute(w, pageData)
					}
					//redirect traffic to respective container if it is alive
					r.Host = backend
					w.Header().Set("X-Ben", "Rad")
					p.ServeHTTP(w, r)
					log.Printf("Redirecting request to: %s", backend)
				}
			}

		}
	}
}

//start the container using the docker API
func (configs *containerConfigs) startContainer(containerName string) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic("Could not connect to docker socket, check if it is mounted to the container.")
	}
	if configs.isSleepTime(containerName) {
		log.Printf("Container will not be started because now is sleep time")
		return
	}
	containerList := append(configs.configList[containerName].helperContainers, containerName)
	for i := range containerList {
		if err := dockerClient.ContainerStart(context.Background(), containerList[i], types.ContainerStartOptions{}); err != nil {
			log.Printf("Unable to start container %s: %s\n", containerList[i], err)
		} else {
			log.Println("started container ", containerList[i])
		}
	}

}

//stop the container using the docker API
func (configs *containerConfigs) stopContainer(containerName string) {
	var stopTimeout, _ = time.ParseDuration("5s")
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic("Could not connect to docker socket, check if it is mounted to the container.")
	}
	containerList := append(configs.configList[containerName].helperContainers, containerName)
	for i := range containerList {
		if err := dockerClient.ContainerStop(context.Background(), containerList[i], &stopTimeout); err != nil {
			log.Printf("Unable to stop container %s: %s\n", containerList[i], err)
		} else {
			log.Println("stopped container ", containerList[i])
		}
	}

}

//check against the dcker API if container is running
func (configs *containerConfigs) isContainerRunning(containerName string) bool {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic("Could not connect to docker socket, check if it is mounted to the container.")
	}
	containerList := append(configs.configList[containerName].helperContainers, containerName)
	for i := range containerList {
		filter := filters.NewArgs(filters.Arg("name", containerList[i]))
		containers, err := dockerClient.ContainerList(context.Background(), types.ContainerListOptions{All: true, Filters: filter})
		check(err)
		if containers[0].State != "running" {
			return false
		}
	}
	return true
}

func doesContainerExist(containerName string) bool {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic("Could not connect to docker socket, check if it is mounted to the container.")
	}
	filter := filters.NewArgs(filters.Arg("name", containerName))
	containers, err := dockerClient.ContainerList(context.Background(), types.ContainerListOptions{All: true, Filters: filter})
	if err != nil || len(containers) == 0 {
		return false
	}
	return true
}

func check(err error) {
	if err != nil {
		log.Println(err)
	}
}
