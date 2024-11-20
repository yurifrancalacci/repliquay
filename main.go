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
	"gopkg.in/yaml.v3"
	"gopkg.in/ini.v1"
)

type Quays struct {
	HostToken []HostToken `yaml:"quays"`
   }

type HostToken struct {
	Host string `yaml:"host"`
	Token string `yaml:"token"`
	MaxConnection int `yaml:"max_connections"`
	QueueLength int
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
)
// func
func apiCall(host string, url string, method string, token string, bodyData string, action string, retry *int) (httpCode int) {

	var enableTLS string
	httpCode = 0

	if !dryRun {	

		client := &http.Client{}
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

		if res.StatusCode > 299 {
			log.Printf("%s Response failed with status code: %d and\nbody: %s\nRequest data %s url %s method %s", host, res.StatusCode, res_body, bodyData, url, method)
			if *retry >= retries {
				log.Printf("Too many attempts: unable to execute action %s with requested data %s on host %s successfully\n", action, bodyData, host)	
			} else {
				log.Printf("Sleeping %d seconds before a new attempt on %s %s %s\n", *retry, host, bodyData, action)	
				time.Sleep(time.Duration(*retry) * time.Second)
				*retry++
				apiCall(host, url, method, token, bodyData, action, retry)
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
	return
}

func checkLogin(quays Quays) (login_ok bool) {
	var wg sync.WaitGroup
	fmt.Println("check login")
	retryCounter := 0 

	for _, v := range quays.HostToken {
		v.QueueLength++

		for v.QueueLength > v.MaxConnection  && sleepPeriod != 0 {
			if debug {
				fmt.Printf("%s: Check login sleep %d, queue length %d\n",v.Host, time.Duration(sleepPeriod), v.QueueLength)
			}
			time.Sleep(time.Duration(sleepPeriod) * time.Millisecond)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(v.Host, "/api/v1/user/logs", "GET", v.Token, "", "checking Logins", &retryCounter)
		}(&v.QueueLength)
	}
	wg.Wait()
	login_ok = true
	return
}

func createOrg(quayHost string, orgList Organization, token string) (status bool) {
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
	)
	status = true
	return
}

func createRepo(quayHost string, orgName string, repoConfig []RepoStruct, token string, queueLength *int, max_conn int) (status bool) {
	var wg sync.WaitGroup
	retryCounter := 0 

	if debug {
		fmt.Printf("Creating %d repos\n", len(repoConfig))
	}
	for i, v := range repoConfig {
		if debug {
			fmt.Printf("Creating Repo %d: %s\n", i, v.Name)
		}
		*queueLength++
		for *queueLength > max_conn  && sleepPeriod != 0 {
			if debug {
				fmt.Printf("%s: create repo sleep %d, queue length %d\n",quayHost, time.Duration(sleepPeriod), *queueLength)
			}
			time.Sleep(time.Duration(sleepPeriod) * time.Millisecond)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(
				quayHost,
				"/api/v1/repository",
				"POST",
				token,
				`{"repository":"`+ v.Name +`","visibility":"private","namespace":"`+ orgName +`","description":"repository description"}`,
				"create repository "+ v.Name,
				&retryCounter,
			)
		}(queueLength)
	}
	wg.Wait()
	status = true
	return
}

func createRepoPermission(quayHost string, permList []PermStruct, token string, queueLength *int, max_conn int) (status bool) {
	var wg sync.WaitGroup
	retryCounter := 0

	if debug {
		fmt.Printf("Creating %d permissions for host %s\n", len(permList), quayHost)
	}

	for _, v := range permList {
		*queueLength++

		for *queueLength > max_conn  && sleepPeriod != 0 {
			if debug {
				fmt.Printf("%s: Repo permission sleep %d, queue length %d\n",quayHost, time.Duration(sleepPeriod), *queueLength)
			}
			time.Sleep(time.Duration(sleepPeriod) * time.Millisecond)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			if v.PermissionKind == "robots" {
				apiCall(
					quayHost,
					"/api/v1/repository/"+ v.Organization +"/"+v.RepoName +"/permissions/user/"+ v.Organization + "+" + v.Name ,
					"PUT",
					token,
					`{"role":"`+ v.Role +`"}`,
					"create repo permission for robot "+ v.Name + " and role "+ v.Role,
					&retryCounter,
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
				)
			}
		}(queueLength)
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

func createRobotTeam(quayHost string, orgName string, robotList []RobotStruct, teamList []TeamStruct, token string, queueLength *int, max_conn int)(status bool){
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
		*queueLength++
		for *queueLength > max_conn  && sleepPeriod != 0 {
			if debug {
				fmt.Printf("%s: Robot permission sleep %d, queue length %d\n",quayHost, time.Duration(sleepPeriod), *queueLength)
			}
			time.Sleep(time.Duration(sleepPeriod) * time.Millisecond)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(quayHost, "/api/v1/organization/"+orgName+"/robots/"+ v.Name, "PUT", token, `{"description":"`+ v.Description +`"}`, "create robot "+ v.Name, &retryCounter)
		}(queueLength) 
	}
// teams
	if debug {
		fmt.Println("creating ", len(teamList), "teams")
	}
	for _, v := range teamList {
		if debug {
			fmt.Println("Creating team", v)
		}
		*queueLength++
		for *queueLength > max_conn && sleepPeriod != 0 {
			if debug {
				fmt.Printf("%s: Robot permission sleep %d, queue length %d\n",quayHost, time.Duration(sleepPeriod), *queueLength)
			}
			time.Sleep(time.Duration(sleepPeriod) * time.Millisecond)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+ v.Name, "PUT", token, `{"role":"`+ v.Role +`"}`, "create team "+ v.Name, &retryCounter)
			if ldapSync {
				apiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+ v.Name +"/syncing", "POST", token, `{"group_dn":"`+ v.GroupDN +`"}`, "create team sync "+ v.Name, &retryCounter)
			}
		}(queueLength) 
	}
	wg.Wait()
	status = true
	return
}
func parseIniFile(inifile string, quay string, repolist []string, sleep int, insec bool, ldap bool, dry bool ) (quaysfile string, repo []string, sleepPeriod int, insecure bool, ldapSync bool, dryRun bool){
	inidata, err := ini.Load(inifile)

	if err != nil {
		log.Print("Warning: no ini file found, loading default values")
		quaysfile, repo, sleepPeriod, insecure, ldapSync, dryRun = quay , repolist, sleep, insec, ldap, dry
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
	flag.StringVar(&confFile, "conf", "/etc/repliquay.conf" , "repliquay config file (override all opts)")
	flag.IntVar(&sleepPeriod, "sleep", 100, "sleep length ms when reaching max connection")
	flag.IntVar(&retries, "retries", 3, "max retries on api call failure")
	flag.BoolVar(&debug, "debug", false, "print debug messages (default false)")
	flag.BoolVar(&insecure, "insecure", false, "disable TLS connection (default false)")
	flag.BoolVar(&ldapSync, "ldapsync", false, "enable ldap sync (default false)")
	flag.BoolVar(&dryRun, "dryrun", false, "enable dry run (default false)")

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
		log.Fatal("Error while reading token file", err)
	}

	yaml.Unmarshal(yamlData, &quays)
	var orgList []string
	for _, r := range repo {
		yamlData, err = os.ReadFile(r)	
		if err != nil {
			log.Fatal("Error while reading token file", err)
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
	////////////////////////////
	if ! dryRun {
		if ! checkLogin(quays){
			log.Fatal("Error logging to quay hosts")
		}
	}
	// os.Exit(0)
	fmt.Printf("Repliquay: repliquayting... be patient\n")

	var wg sync.WaitGroup
	for _, o := range parsedOrg {
		// fmt.Printf("Parsed ORG------%s\n", o.Name)
		permList = slices.Concat(permList, createPermissionList(o.RepoList, "robots", o.Name), createPermissionList(o.RepoList, "teams", o.Name))
	}

	for _, v := range quays.HostToken {
		v.QueueLength = 0
		for i, o := range parsedOrg {
			wg.Add(1)
			go func() {
				defer wg.Done()
				fmt.Printf("creating organization - Host: %s\t- %s\n", v.Host, o.Name)
				createOrg(v.Host, o, v.Token)
				fmt.Printf("creating robots and teams for organization %s - Host: %s\n", o.Name, v.Host)
				createRobotTeam(v.Host, o.Name, o.RobotList, o.TeamsList, v.Token, &v.QueueLength, v.MaxConnection)
				fmt.Printf("creating repositories for organization %s - Host: %s\n", o.Name, v.Host)
				createRepo(v.Host, o.Name, o.RepoList, v.Token, &v.QueueLength, v.MaxConnection)
				if i == 0 {
					createRepoPermission(v.Host, permList, v.Token, &v.QueueLength, v.MaxConnection)
					// permList = slices.Concat(permList, createPermissionList(o.RepoList, "robots", o.Name), createPermissionList(o.RepoList, "teams", o.Name))
				}
			}()
		}
	}
	wg.Wait()
	fmt.Printf("Repliquay: mission completed in %s\n", time.Since(t1))
}
