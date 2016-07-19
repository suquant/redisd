package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"k8s.io/kubernetes/pkg/api"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"

	"path/filepath"

	"github.com/golang/glog"
	"github.com/kubernetes/kubernetes/pkg/util/rand"
)

const (
	redisServer             = "/usr/bin/redis-server"
	redisCli                = "/usr/bin/redis-cli"
	sentinelConfigName      = "sentinel.conf"
	beat                    = time.Second * 5
	sleepTimeMin            = time.Second * 2
	sleepTimeMax            = time.Second * 7
	sentinelModeArg         = "--sentinel"
	masterPodLabel          = "master"
	masterPodLabelTrueValue = "true"
	redisPort               = "6379"
)

type cmdLabels [][]string

func (self *cmdLabels) String() string {
	return fmt.Sprintf("%#v", self)
}

func (self *cmdLabels) Set(value string) error {
	s := strings.Split(value, "=")
	if len(s) != 2 {
		return errors.New("must be like a \"key=value\"")
	}
	*self = append(*self, s)
	return nil
}

func main() {
	namespaceName := flag.String("namespace", api.NamespaceDefault, "namespace")
	sentinelService := flag.String("sentinel-service", "redis-sentinel", "Sentinel service name")

	sentinel := flag.Bool("sentinel", false, "Sentinel")
	masterName := flag.String("master-name", "redis-master", "Sentinel master name")
	quorum := flag.Uint64("quorum", 2, "Sentinel master quorum")
	downAfterMilliseconds := flag.Uint64("down-after-milliseconds", 60000, "Sentinel down after milliseconds")
	failoverTimeout := flag.Uint64("failover-timeout", 180000, "Sentinel failover timeout")
	parallelSyncs := flag.Uint64("parallel-syncs", 1, "Sentinel parallel syncs")

	var selectedLabels cmdLabels
	flag.Var(&selectedLabels, "labels", "--labels key1=value1 --labels key2=value2 ...")

	flag.Parse()

	manager := &RedisManager{
		Namespace:       *namespaceName,
		SentinelService: *sentinelService,
		Labels:          selectedLabels,

		Sentinel:              *sentinel,
		MasterName:            *masterName,
		Quorum:                *quorum,
		DownAfterMilliseconds: *downAfterMilliseconds,
		FailoverTimeout:       *failoverTimeout,
		ParallelSyncs:         *parallelSyncs,

		Args:  flag.Args(),
		mutex: &sync.Mutex{},
	}

	manager.Run()
}

type RedisManager struct {
	Namespace       string
	SentinelService string
	Labels          [][]string

	Sentinel              bool
	MasterName            string
	Quorum                uint64
	DownAfterMilliseconds uint64
	FailoverTimeout       uint64
	ParallelSyncs         uint64

	Args []string

	mutex  *sync.Mutex
	client *client.Client
}

func (self *RedisManager) Run() error {
	server := self.runServer()
	return server.Wait()
}

func (self *RedisManager) getClient() (*client.Client, error) {
	var err error
	self.mutex.Lock()
	if self.client == nil {
		self.client, err = client.NewInCluster()
	}
	self.mutex.Unlock()
	return self.client, err
}

func (self *RedisManager) createConfig(masterHost string) (string, error) {
	file, err := os.OpenFile(filepath.Join(os.TempDir(), sentinelConfigName), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	defer file.Close()
	if err != nil {
		return "", err
	}
	fmt.Fprintln(file, fmt.Sprintf("sentinel monitor %s %s 6379 %d", self.MasterName, masterHost, self.Quorum))
	fmt.Fprintln(file, fmt.Sprintf("sentinel down-after-milliseconds %s %d", self.MasterName, self.DownAfterMilliseconds))
	fmt.Fprintln(file, fmt.Sprintf("sentinel failover-timeout %s %d", self.MasterName, self.FailoverTimeout))
	fmt.Fprintln(file, fmt.Sprintf("sentinel parallel-syncs %s %d", self.MasterName, self.ParallelSyncs))
	return file.Name(), nil
}

func (self *RedisManager) contains(slice []string, subject string) bool {
	for _, v := range slice {
		if v == subject {
			return true
		}
	}
	return false
}

func (self *RedisManager) runServer() *exec.Cmd {
	args := self.Args
	sentinelMode := (self.Sentinel == true || self.contains(args, sentinelModeArg))
	if sentinelMode == true {
		if self.contains(args, sentinelModeArg) == false {
			args = append(args, sentinelModeArg)
		}
		masterPod := self.waitRunningMasterPod(beat)
		masterHost := masterPod.Status.PodIP
		configPath, err := self.createConfig(masterHost)
		if err != nil {
			glog.Fatalln(err.Error())
		}
		args = append([]string{configPath}, args...)
		glog.V(0).Infoln("Sentinel mode active")
	} else {
		sleepTime := rand.IntnRange(int(sleepTimeMin), int(sleepTimeMax))
		for range time.Tick(time.Duration(sleepTime)) {
			glog.V(0).Infoln("Wait master server")
			masterPod, err := self.getMasterPod()
			if err != nil {
				glog.Errorln(err.Error())
				continue
			}

			if masterPod != nil {
				if masterPod.Status.Phase != api.PodRunning {
					continue
				}
				glog.V(0).Infoln("Master server found on host: \"", masterPod.Status.PodIP, "\", pod: \"", masterPod.Name, "\"")
				args = append(args, "--slaveof", masterPod.Status.PodIP, redisPort)
				break
			} else {
				glog.V(0).Infoln("Master server not found, try mark first pod as master server")
				c, err := self.getClient()
				if err != nil {
					glog.Errorln(err.Error())
					continue
				}

				firstPod, err := self.getFirstPod()
				firstPod.Labels[masterPodLabel] = masterPodLabelTrueValue

				newMasterPod, err := c.Pods(self.Namespace).Update(firstPod)
				if err != nil {
					glog.Warningln(err.Error())
					continue
				}

				if label, ok := newMasterPod.Labels[masterPodLabel]; ok && label != masterPodLabelTrueValue {
					glog.Errorln("Can not set pod as master")
					continue
				}
				glog.V(0).Infoln("Pod: \"", newMasterPod.Name, "\", host: \"", newMasterPod.Status.PodIP, "\", marked as master server")

				break
			}
		}
	}

	cmd := exec.Command(redisServer, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		glog.Fatalln(err.Error())
	}
	return cmd
}

func (self *RedisManager) waitRunningMasterPod(sleep time.Duration) (pod *api.Pod) {
	for range time.Tick(sleep) {
		mpod, err := self.getMasterPod()
		if err != nil {
			glog.Warningln(err.Error())
			continue
		}
		if mpod == nil {
			continue
		}

		if mpod.Status.Phase == api.PodRunning {
			pod = mpod
			break
		}

		continue
	}
	return
}

func (self *RedisManager) getMasterPod() (pod *api.Pod, err error) {
	glog.V(0).Infoln("Check service \"", self.SentinelService, "\"")
	masterRecord, merr := self.getMasterPodHostAndPortFromSentinel()
	if merr != nil {
		glog.Errorln(merr.Error())
	}

	if len(masterRecord) > 0 {
		podsList, perr := self.getPods(nil)
		if perr != nil {
			err = perr
			return
		}
		for _, mpod := range podsList {
			if mpod.Status.PodIP == masterRecord[0] {
				pod = mpod
				return
			}
		}
	}

	glog.V(0).Infoln("Check cluster by extra label: ", masterPodLabel, "=\"", masterPodLabelTrueValue, "\"")
	podsList, perr := self.getPods(&labels.Set{masterPodLabel: masterPodLabelTrueValue})
	if perr != nil {
		err = perr
		return
	}

	for _, mpod := range podsList {
		pod = mpod
		return
	}

	return
}

func (self *RedisManager) getMasterPodHostAndPortFromSentinel() (record []string, err error) {
	cmd := exec.Command(redisCli, "-h", self.SentinelService, "-p", "26379", "--csv",
		"SENTINEL", "get-master-addr-by-name", self.MasterName)
	stdout, serr := cmd.StdoutPipe()
	if serr != nil {
		err = serr
		return
	}
	if err = cmd.Start(); err != nil {
		return
	}

	timer := time.AfterFunc(1*time.Second, func() {
		cmd.Process.Kill()
	})

	err = cmd.Wait()
	timer.Stop()

	if err != nil {
		r := csv.NewReader(stdout)
		record, err = r.Read()
	}
	return
}

func (self *RedisManager) getFirstPod() (pod *api.Pod, err error) {
	podsList, err := self.getPods(nil)
	if err != nil {
		return
	}

	for _, fpod := range podsList {
		pod = fpod
		break
	}

	return
}

func (self *RedisManager) getPods(extraLabels *labels.Set) (result []*api.Pod, err error) {
	c, err := self.getClient()
	if err != nil {
		return
	}

	labelSet := labels.Set{}
	for _, v := range self.Labels {
		key := v[0]
		val := v[1]
		labelSet[key] = val
	}
	if extraLabels != nil {
		for k, v := range *extraLabels {
			labelSet[k] = v
		}
	}

	podsList, err := c.Pods(self.Namespace).List(api.ListOptions{LabelSelector: labelSet.AsSelector()})
	if err != nil {
		return
	}

	for idx, _ := range podsList.Items {
		result = append(result, &podsList.Items[idx])
	}

	return
}
