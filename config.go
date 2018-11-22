package service_util

import (
	"errors"
	log "github.com/xiaomi-tc/log15"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
)

type map_Config map[string]map[string]interface{}

var (
	strGlobalConfigPath string = "/etc/woda/GlobalConfigure.yaml"
	strYamlRootPath     string = "/etc/woda/modules/"
	strDefaultDevEnv    string = "development"
	strTestEnv          string = "test"
	strProEnv           string = "production"
	mapGlobalConfig     map_Config
)

func GetFileName(fullPath string) string {
	_, filenameWithSuffix := path.Split(fullPath)

	var fileSuffix string
	fileSuffix = path.Ext(filenameWithSuffix)

	var filenameOnly string
	filenameOnly = strings.TrimSuffix(filenameWithSuffix, fileSuffix)

	port, err := getServiceConfigPort(fullPath)
	if err != nil {
		port = 0
	}

	filenameOnly += "-" + strconv.Itoa(port)

	return filenameOnly
}

func readYaml(yamlpath string) (m map[string]interface{}) {
	data, err := ioutil.ReadFile(yamlpath)
	checkErr(err, "readYaml", false)

	err = yaml.Unmarshal([]byte(data), &m)
	checkErr(err, "Unmarshal", false)
	return m
}

func getServiceConfigPort(yamlpath string) (port int, err error) {
	m := readYaml(yamlpath)

	port = m["port"].(int)
	return port, nil
}

func setServiceConfigPort(yamlpath string, port int) (err error) {
	m := readYaml(yamlpath)

	m["port"] = port
	outer, err := yaml.Marshal(&m)

	err = ioutil.WriteFile(yamlpath, outer, 0644)

	return err
}

func GetConfig(confItem string) (result map[string]interface{}, err error) {
	if mapGlobalConfig != nil {
		if result, err := mapGlobalConfig[confItem]; err {
			return result, nil
		}
	}
	return nil, errors.New("no result")
}

func initGlobalConfig(devenv string) error {

	byteData, err := ioutil.ReadFile(strGlobalConfigPath)
	if err != nil {
		return err
	}

	mapTempConfig := make(map[string]map[string]map[string]interface{})
	err = yaml.Unmarshal([]byte(byteData), &mapTempConfig)
	if err != nil {
		return err
	}

	var bFlag bool
	if mapGlobalConfig, bFlag = mapTempConfig[devenv]; !bFlag {
		return errors.New("get data from mapTempConfig error")
	}

	return nil
}

func init() {

	if runtime.GOOS == "windows" {
		strGlobalConfigPath = "c:/etc/GlobalConfigure.yaml"
		strYamlRootPath = "c:/etc/modules/"
	}
	mapGlobalConfig = make(map_Config)

	strTempEnv := os.Getenv("WODA_ENV")
	log.Info("config-env", "env", strTempEnv)
	if strTempEnv == strDefaultDevEnv || strTempEnv == strTestEnv || strTempEnv == strProEnv {
		strDefaultDevEnv = strTempEnv
	} else {
		strTempEnv = strDefaultDevEnv
	}

	err := initGlobalConfig(strTempEnv)
	if err != nil {
		log.Error("initGlobalConfig error", "error", err)
	}
}
