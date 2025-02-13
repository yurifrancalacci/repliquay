package main

import (
	"fmt"
	"log"
	"os"
	"io"
	"sync"
	"bytes"
	"time"
	"slices"
	"flag"
	"errors"
	// "time"
	"net/http"
	"crypto/tls"
	"gopkg.in/yaml.v3"
	"gopkg.in/ini.v1"
)

type Quays struct {
	HostToken []HostToken `yaml:"quays"`
   }

type hostConnection struct {
	hostname string
	max_connections int
	queueLength int
	mx sync.Mutex
}

type HostToken struct {
	Host string `yaml:"host"`
	Token string `yaml:"token"`
	MaxConnection int `yaml:"max_connections"`
   }

type Organization struct {
	Name string `yaml:"quay_organization"`
	OrgRoleName string `yaml:"quay_organization_role_name"`
	RepoList []RepoStruct `yaml:"repositories"`
	RobotList []RobotStruct `yaml:"robots"`
	TeamsList []TeamStruct `yaml:"teams"`
   }

type RepoStruct struct {
	Name string `yaml:"name"`
	Mirror bool `yaml:"mirror"`
	PermissionList RepoPermissionStruct `yaml:"permissions"`
}

type RobotStruct struct {
	Name string `yaml:"name"`
	Description string `yaml:"desc"`
}

type TeamStruct struct {
	Name string `yaml:"name"`
	Description string `yaml:"description"`
	GroupDN string `yaml:"group_dn"`
	Role string `yaml:"role"`
}

type RepoPermissionStruct struct {
	Robots []PermStruct `yaml:"robots"`
	Teams []PermStruct `yaml:"teams"`
}

type PermStruct struct {
	Name string `yaml:"name"`
	Role string `yaml:"role"`
	PermissionKind string
	RepoName string
	Organization string
}

// vars
var(
	insecure bool
	ldapSync bool
	dryRun bool
	sleepPeriod int
	debug bool
	retries int
	verify bool
)
// func
func (hc *hostConnection) inc() {
    hc.mx.Lock()
    defer hc.mx.Unlock()
    hc.queueLength++
}

func (hc *hostConnection) dec() {
    hc.mx.Lock()
    defer hc.mx.Unlock()
    hc.queueLength--
}

func apiCall(host string, url string, method string, token string, bodyData string, action string, retry *int, hostConn *hostConnection) (httpCode int) {

	var enableTLS string
	httpCode = 0
	tr := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: verify},
    }
// move to dryRun block
// sleep if too many connections
	for hostConn.queueLength >= hostConn.max_connections  && sleepPeriod != 0 {
		if debug {
			fmt.Printf("%s: APICALL %s action too many connections %d. Sleeping %s\n",host, action, hostConn.queueLength, time.Duration(sleepPeriod) * time.Millisecond)
		}
		time.Sleep(time.Duration(sleepPeriod) * time.Millisecond)
	}
	hostConn.inc()
	if debug {
		fmt.Printf("%s: queue %d action %s\n",hostConn.hostname, hostConn.queueLength, action)
	}
	if !dryRun {	

		client := &http.Client{Transport: tr}
		req := &http.Request{}
		if !insecure {
			enableTLS = "s"
		}

		if bodyData != "" {
			jsonBody := []byte(bodyData)
			bodyReader := bytes.NewReader(jsonBody)
			req, _ = http.NewRequest(method, "http"+ enableTLS +"://" + host + url, bodyReader)
		} else {
			req, _ = http.NewRequest(method, "http"+ enableTLS +"://" + host + url, nil)
		}

		if token != "" {
			req.Header.Add("Authorization", "Bearer " + token)
			req.Header.Add("Content-Type", "application/json")
		}

		res, err := client.Do(req)

		if err != nil {
			log.Fatal(err)
		}

		res_body, err := io.ReadAll(res.Body)
		res.Body.Close()

		if res.StatusCode > 499 {
			log.Printf("%s Response failed with status code: %d and\nbody: %s\nRequest data %s url %s method %s", host, res.StatusCode, res_body, bodyData, url, method)
			if *retry > retries {
				log.Fatalf("Too many attempts: unable to execute action %s with requested data %s on host %s successfully\n", action, bodyData, host)
			} else {
				log.Printf("Sleeping %d seconds before a new attempt on %s %s %s\n", *retry, host, bodyData, action)	
				time.Sleep(time.Duration(*retry) * time.Second)
				*retry++
				apiCall(host, url, method, token, bodyData, action, retry, hostConn)
			}
		} else {
			if debug {
				log.Printf("%s Action %s completed\n", host, action)
			}
		}
		if err != nil {
			log.Fatal(err)
		}
		httpCode = res.StatusCode
	}
	hostConn.dec()
	if debug {
		fmt.Printf("%s: queue %d action %s\n",hostConn.hostname, hostConn.queueLength, action)
	}
	return
}

func checkLogin(quayHost string, token string, hostConn *hostConnection) (login_ok bool) {
	fmt.Println("check login")
	retryCounter := 0 

	apiCall(quayHost, "/api/v1/user/logs", "GET", token, "", "checking Logins", &retryCounter, hostConn)
	login_ok = true
	return
}

func createOrg(quayHost string, orgList Organization, token string, hostConn *hostConnection) (status bool) {
	retryCounter := 0 

	if debug {
		fmt.Println("Creating Org...", orgList.Name)
	}
	apiCall(
		quayHost,
		"/api/v1/organization/",
		"POST",
		token,
		`{"name":"`+ orgList.Name +`"}`,
		"create organization" + orgList.Name,
		&retryCounter,
		hostConn,
	)
	status = true
	return
}

func createRepo(quayHost string, orgName string, repoConfig []RepoStruct, token string, hostConn *hostConnection) (status bool) {
	var wg sync.WaitGroup
	retryCounter := 0 

	if debug {
		fmt.Printf("Creating %d repos\n", len(repoConfig))
	}
	for i, v := range repoConfig {
		if debug {
			fmt.Printf("Creating Repo %d: %s\n", i, v.Name)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			apiCall(
				quayHost,
				"/api/v1/repository",
				"POST",
				token,
				`{"repository":"`+ v.Name +`","visibility":"private","namespace":"`+ orgName +`","description":"repository description"}`,
				"create repository "+ v.Name,
				&retryCounter,
				hostConn,
			)
		}()
	}
	wg.Wait()
	status = true
	return
}

func createRepoPermission(quayHost string, permList []PermStruct, token string, hostConn *hostConnection) (status bool) {
	var wg sync.WaitGroup
	retryCounter := 0

	if debug {
		fmt.Printf("Creating %d permissions for host %s\n", len(permList), quayHost)
	}

	for _, v := range permList {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v.PermissionKind == "robots" {
				apiCall(
					quayHost,
					"/api/v1/repository/"+ v.Organization +"/"+v.RepoName +"/permissions/user/"+ v.Organization + "+" + v.Name ,
					"PUT",
					token,
					`{"role":"`+ v.Role +`"}`,
					"create repo permission for robot "+ v.Name + " and role "+ v.Role,
					&retryCounter,
					hostConn,
				)
			} else {
				apiCall(
					quayHost,
					"/api/v1/repository/"+ v.Organization +"/"+v.RepoName +"/permissions/team/"+ v.Name ,
					"PUT",
					token,
					`{"role":"`+ v.Role +`"}`,
					"create repo "+v.Name+" permission for teams "+ v.Name + " and role "+ v.Role,
					&retryCounter,
					hostConn,
				)
			}
		}()
	}
	wg.Wait()
	status = true
	return
}

func createPermissionList(repoConfig []RepoStruct, permissionName string, orgName string) (permList []PermStruct) {
	if debug {
		fmt.Printf("Mapping Permission for %d repos for %s\n", len(repoConfig), permissionName)
	}

	totalPermission := 0
	for _, v := range repoConfig {

		var loopList []PermStruct
		if permissionName == "robots" {
			loopList = v.PermissionList.Robots
		} else {
			loopList = v.PermissionList.Teams
		}
		if debug {
			fmt.Printf("Mapping permission for repo: %s kind %s\n", v.Name, permissionName)
		}
		for _, vv := range loopList {
			totalPermission++
			permList = append(permList, PermStruct{ Name: vv.Name, Role: vv.Role, PermissionKind: permissionName, RepoName: v.Name, Organization: orgName })	
		}
	}
	fmt.Printf("Total mapped %s permission for org: %d\n", permissionName, totalPermission)
	return
}

func createRobotTeam(quayHost string, orgName string, robotList []RobotStruct, teamList []TeamStruct, token string, hostConn *hostConnection)(status bool){
	var wg sync.WaitGroup
	retryCounter := 0 
// robot
	if debug {
		fmt.Println("creating ", len(robotList), "robots for", orgName, "host", quayHost)
	}
	for _, v := range robotList {
		if debug {
			fmt.Println("Creating robot", v)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			apiCall(quayHost, "/api/v1/organization/"+orgName+"/robots/"+ v.Name, "PUT", token, `{"description":"`+ v.Description +`"}`, "create robot "+ v.Name, &retryCounter, hostConn)
		}() 
	}
// teams
	if debug {
		fmt.Println("creating ", len(teamList), "teams")
	}
	for _, v := range teamList {
		if debug {
			fmt.Println("Creating team", v)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			apiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+ v.Name, "PUT", token, `{"role":"`+ v.Role +`"}`, "create team "+ v.Name, &retryCounter, hostConn)
			if ldapSync {
				apiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+ v.Name +"/syncing", "POST", token, `{"group_dn":"`+ v.GroupDN +`"}`, "create team sync "+ v.Name, &retryCounter, hostConn)
			}
		}() 
	}
	wg.Wait()
	status = true
	return
}

func parseIniFile(inifile string, quay string, repolist []string, sleep int, insec bool, ldap bool, dry bool ) (quaysfile string, repo []string, sleepPeriod int, insecure bool, ldapSync bool, dryRun bool){
	inidata, err := ini.Load(inifile)

	if err != nil {
		log.Print("Warning: no conf option found, loading default values from /repos/repliquay.conf")
		quaysfile, repo, sleepPeriod, insecure, ldapSync, dryRun = quay, repolist, sleep, insec, ldap, dry
		return
	}
	quaysfile = inidata.Section("quays").Key("file").String()
	repo = inidata.Section("repos").Key("files").Strings(",")
	sleepPeriod, _ = inidata.Section("params").Key("sleep").Int()
	debug, _ = inidata.Section("params").Key("debug").Bool()
	ldapSync, _ = inidata.Section("params").Key("ldapsync").Bool()
	dryRun, _ = inidata.Section("params").Key("dryrun").Bool()
	return
}

func main() {
	var quays Quays
	var org Organization
	var parsedOrg []Organization
	var permList []PermStruct
	hostConn := make(map[string]*hostConnection)

	var(
		quaysfile string
		repo []string
		confFile string
	)

	t1 := time.Now()
	flag.Func("repo", "quay repo file name", func(s string) error {
		_, err := os.Stat(s)
		if err == nil {
			repo = append(repo, s)
			return nil
		} else {
			return errors.New("file does not exists")
		}
	})
	flag.StringVar(&quaysfile, "quaysfile", "" , "quay token file name")
	flag.StringVar(&confFile, "conf", "/repos/repliquay.conf" , "repliquay config file (override all opts)")
	flag.IntVar(&sleepPeriod, "sleep", 100, "sleep length ms when reaching max connection")
	flag.IntVar(&retries, "retries", 3, "max retries on api call failure")
	flag.BoolVar(&debug, "debug", false, "print debug messages (default false)")
	flag.BoolVar(&insecure, "insecure", false, "disable TLS connection (default false)")
	flag.BoolVar(&ldapSync, "ldapsync", false, "enable ldap sync (default false)")
	flag.BoolVar(&dryRun, "dryrun", false, "enable dry run (default false)")
	flag.BoolVar(&verify, "verify", true, "enable TLS validation (default true)")

	flag.Parse()

	p, _ := os.Executable()
	_, err := os.Stat(p+"/"+confFile)
	if err != nil {
		quaysfile, repo, sleepPeriod, insecure, ldapSync, dryRun = parseIniFile(confFile, quaysfile, repo, sleepPeriod, insecure, ldapSync, dryRun)
		// fmt.Println("Parsing ini file ", iniFile, repo)
	} else {
		fmt.Println("No config file provided ")
	}

	if debug {
		for _, v := range repo {
				fmt.Printf("- Name: %s\n", v)
		}
		fmt.Printf("quayfile %s\n\ninsecure %t\n", quaysfile, insecure)
	}
	
	yamlData, err := os.ReadFile(quaysfile)
	
	if err != nil {
		log.Fatal("Error while reading quays file ", err)
	}

	yaml.Unmarshal(yamlData, &quays)
	var orgList []string
	for _, r := range repo {
		yamlData, err = os.ReadFile(r)	
		if err != nil {
			log.Fatal("Error while reading quays file ", err)
		}
		yaml.Unmarshal(yamlData, &org)
		if slices.Contains(orgList, org.Name)  {
			log.Fatalf("Duplicated organization %s", org.Name)
		} else {
			parsedOrg = append(parsedOrg, org)
			orgList = append(orgList, org.Name)	
		}
	}

	////////////////////////////
// 	for _, ooo := range parsedOrg {
// 		for i, v := range ooo.RepoList {
// 			fmt.Printf("Repository n.%d - Name: %s\n", i, v.Name)
// 	 	}
// 	}

// 	for i, v := range quays.HostToken {
// 		fmt.Printf("token n.%d\n\t- Host: %s \n\t- token: %s\n", i, v.Host, v.Token)
// 	}

// 	for i, v := range org.RepoList {
// 		fmt.Printf("Repository n.%d - Name: %s\n", i, v.Name)
// 	}

// os.Exit(0)
	fmt.Printf("Repliquay: repliquayting... be patient\n")

	var wg sync.WaitGroup
	for _, o := range parsedOrg {
		// fmt.Printf("Parsed ORG------%s\n", o.Name)
		permList = slices.Concat(permList, createPermissionList(o.RepoList, "robots", o.Name), createPermissionList(o.RepoList, "teams", o.Name))
	}

	for _, v := range quays.HostToken {
		h := hostConnection{ queueLength: 0, max_connections: v.MaxConnection, hostname: v.Host}
		hostConn[v.Host] = &h
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ! dryRun {
				if ! checkLogin(v.Host, v.Token, hostConn[v.Host]){
					log.Fatal("Error logging to quay hosts")
				}
			}
		}()
	}
	wg.Wait()

	for _, v := range quays.HostToken {
		for i, o := range parsedOrg {
			wg.Add(1)
			go func() {
				defer wg.Done()
				fmt.Printf("creating organization - Host: %s\t- %s\n", v.Host, o.Name)
				createOrg(v.Host, o, v.Token, hostConn[v.Host])
				fmt.Printf("creating robots and teams for organization %s - Host: %s\n", o.Name, v.Host)
				createRobotTeam(v.Host, o.Name, o.RobotList, o.TeamsList, v.Token, hostConn[v.Host])
				fmt.Printf("creating repositories for organization %s - Host: %s\n", o.Name, v.Host)
				createRepo(v.Host, o.Name, o.RepoList, v.Token, hostConn[v.Host])
				if i == 0 {
					createRepoPermission(v.Host, permList, v.Token, hostConn[v.Host])
				}
			}()
		}
	}
	wg.Wait()
	fmt.Printf("Repliquay: mission completed in %s\n", time.Since(t1))
}
